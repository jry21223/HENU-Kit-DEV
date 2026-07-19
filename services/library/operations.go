package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"henukit.dev/library/internal/contract"
)

type commandInput struct {
	Kind            string          `json:"kind"`
	ResourceID      string          `json:"resource_id"`
	ExpectedVersion string          `json:"expected_version"`
	Payload         json.RawMessage `json:"payload"`
}

type operationRecord struct {
	ID                            uuid.UUID
	RequestHash, Operation, State string
	Response                      []byte
	ErrorCode                     string
}

func (h *service) command(w http.ResponseWriter, r *http.Request) {
	var input commandInput
	body, ok := decode(w, r, &input)
	if !ok {
		return
	}
	permission := commandPermission(input.Kind)
	value, ok := h.require(w, r, permission)
	if !ok {
		return
	}
	if _, _, err := legacyCommandRoute(input); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_COMMAND", err.Error())
		return
	}
	if _, err := filteredPayload(input.Kind, input.Payload); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_COMMAND", err.Error())
		return
	}
	if input.Kind != "course_create" && input.Kind != "material_create" {
		if _, err := uuid.Parse(input.ResourceID); err != nil || input.ExpectedVersion == "" {
			writeError(w, r, http.StatusBadRequest, "INVALID_COMMAND", "resource_id and expected_version are required")
			return
		}
	}
	h.runOperation(w, r, value, input, body)
}

func (h *service) versionMatches(r *http.Request, input commandInput) bool {
	expected, err := time.Parse(time.RFC3339, input.ExpectedVersion)
	if err != nil {
		return false
	}
	workspace := h.legacy.workspace(r.Context())
	switch {
	case strings.HasPrefix(input.Kind, "course_"):
		for _, item := range workspace.Courses {
			if item.ID == input.ResourceID {
				return item.UpdatedAt.Equal(expected)
			}
		}
	case strings.HasPrefix(input.Kind, "submission_"):
		for _, item := range workspace.Submissions {
			if item.ID == input.ResourceID {
				return item.UpdatedAt.Equal(expected)
			}
		}
	case strings.HasPrefix(input.Kind, "correction_"):
		for _, item := range workspace.Corrections {
			if item.ID == input.ResourceID {
				return item.UpdatedAt.Equal(expected)
			}
		}
	default:
		for _, item := range workspace.Materials {
			if item.ID == input.ResourceID {
				return item.UpdatedAt.Equal(expected)
			}
		}
	}
	return false
}

func (h *service) runOperation(w http.ResponseWriter, r *http.Request, value actor, input commandInput, body []byte) {
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	digest := sha256.Sum256(append([]byte(r.Method+"\n"+contract.CommandRoute+"\n"), body...))
	hash := hex.EncodeToString(digest[:])
	record, created, err := h.loadOrCreateOperation(r, value, key, input, hash)
	if errors.Is(err, errIdempotencyConflict) {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for another request")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Library operation ledger is unavailable")
		return
	}
	if !created {
		record = h.awaitOperation(r, record)
		h.writeStoredOperation(w, r, record)
		return
	}
	if input.Kind != "course_create" && input.Kind != "material_create" && !h.versionMatches(r, input) {
		_ = h.completeOperation(r, record.ID, value, input, "failed", nil, "LIBRARY_CONFLICT")
		writeError(w, r, http.StatusConflict, "LIBRARY_CONFLICT", "legacy resource version changed")
		return
	}
	resourceID, err := h.legacy.command(r.Context(), input)
	if err != nil {
		var upstream upstreamError
		if !errors.As(err, &upstream) {
			_ = h.recordUnknown(r, record.ID, value, input)
			writeError(w, r, http.StatusServiceUnavailable, "OPERATION_RESULT_UNKNOWN", "legacy write result is unknown")
			return
		}
		_ = h.completeOperation(r, record.ID, value, input, "failed", nil, "LEGACY_WRITE_REJECTED")
		writeError(w, r, http.StatusConflict, "LEGACY_WRITE_REJECTED", "Study Legacy API rejected the operation")
		return
	}
	input.ResourceID = resourceID
	response, _ := json.Marshal(map[string]any{"operation": input.Kind, "resource_id": input.ResourceID, "state": "succeeded"})
	if err := h.completeOperation(r, record.ID, value, input, "succeeded", response, ""); err != nil {
		_ = h.recordUnknown(r, record.ID, value, input)
		writeError(w, r, http.StatusServiceUnavailable, "OPERATION_RESULT_UNKNOWN", "legacy write completed but ledger update is unknown")
		return
	}
	var data any
	_ = json.Unmarshal(response, &data)
	writeData(w, r, http.StatusOK, data)
}

var errIdempotencyConflict = errors.New("idempotency key conflict")

func (h *service) loadOrCreateOperation(r *http.Request, value actor, key string, input commandInput, hash string) (operationRecord, bool, error) {
	tx, err := h.database.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		return operationRecord{}, false, err
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	lock := strings.Join([]string{h.clientID, value.userID, http.MethodPost, contract.CommandRoute, key}, "\n")
	if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lock); err != nil {
		return operationRecord{}, false, err
	}
	var existing operationRecord
	err = tx.QueryRow(r.Context(), `SELECT id,request_hash,operation,state,COALESCE(response,'null'::jsonb),COALESCE(error_code,'') FROM library_adapter_operations WHERE client_id=$1 AND actor_user_id=$2 AND method='POST' AND normalized_route=$3 AND idempotency_key=$4 AND expires_at>now()`, h.clientID, value.userID, contract.CommandRoute, key).Scan(&existing.ID, &existing.RequestHash, &existing.Operation, &existing.State, &existing.Response, &existing.ErrorCode)
	if err == nil {
		if existing.RequestHash != hash || existing.Operation != input.Kind {
			return operationRecord{}, false, errIdempotencyConflict
		}
		if err := tx.Commit(r.Context()); err != nil {
			return operationRecord{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return operationRecord{}, false, err
	}
	id := uuid.New()
	targetType := commandTargetType(input.Kind)
	_, err = tx.Exec(r.Context(), `INSERT INTO library_adapter_operations(id,client_id,method,normalized_route,idempotency_key,operation,request_hash,state,actor_user_id,request_id,target_type,target_id) VALUES($1,$2,'POST',$3,$4,$5,$6,'pending',$7,$8,$9,NULLIF($10,''))`, id, h.clientID, contract.CommandRoute, key, input.Kind, hash, value.userID, requestID(r), targetType, input.ResourceID)
	if err != nil {
		return operationRecord{}, false, err
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO library_adapter_audit_events(id,operation_id,actor_user_id,request_id,action,target_type,target_id,outcome) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),'attempted')`, uuid.New(), id, value.userID, requestID(r), input.Kind, targetType, input.ResourceID)
	if err != nil {
		return operationRecord{}, false, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return operationRecord{}, false, err
	}
	return operationRecord{ID: id, RequestHash: hash, Operation: input.Kind, State: "pending", Response: []byte("null")}, true, nil
}

func (h *service) awaitOperation(r *http.Request, record operationRecord) operationRecord {
	for attempt := 0; attempt < 100 && record.State == "pending"; attempt++ {
		time.Sleep(10 * time.Millisecond)
		_ = h.database.QueryRow(r.Context(), `SELECT state,COALESCE(response,'null'::jsonb),COALESCE(error_code,'') FROM library_adapter_operations WHERE id=$1`, record.ID).Scan(&record.State, &record.Response, &record.ErrorCode)
	}
	return record
}

func (h *service) completeOperation(r *http.Request, id uuid.UUID, value actor, input commandInput, state string, response []byte, errorCode string) error {
	tx, err := h.database.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	_, err = tx.Exec(r.Context(), `UPDATE library_adapter_operations SET state=$2,response=$3,error_code=NULLIF($4,''),target_id=COALESCE(NULLIF($5,''),target_id),completed_at=now() WHERE id=$1 AND state='pending'`, id, state, response, errorCode, input.ResourceID)
	if err != nil {
		return err
	}
	targetType := commandTargetType(input.Kind)
	_, err = tx.Exec(r.Context(), `INSERT INTO library_adapter_audit_events(id,operation_id,actor_user_id,request_id,action,target_type,target_id,outcome) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8)`, uuid.New(), id, value.userID, requestID(r), input.Kind, targetType, input.ResourceID, state)
	if err != nil {
		return err
	}
	return tx.Commit(r.Context())
}

func (h *service) recordUnknown(r *http.Request, id uuid.UUID, value actor, input commandInput) error {
	_, err := h.database.Exec(r.Context(), `INSERT INTO library_adapter_audit_events(id,operation_id,actor_user_id,request_id,action,target_type,target_id,outcome) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),'unknown')`, uuid.New(), id, value.userID, requestID(r), input.Kind, commandTargetType(input.Kind), input.ResourceID)
	return err
}

func commandTargetType(kind string) string {
	switch {
	case strings.HasPrefix(kind, "course_"):
		return "course"
	case strings.HasPrefix(kind, "correction_"):
		return "correction"
	default:
		return "material"
	}
}

func commandPermission(kind string) string {
	if strings.HasPrefix(kind, "submission_") || strings.HasPrefix(kind, "correction_") {
		return "library.review"
	}
	return "library.manage"
}

func (h *service) writeStoredOperation(w http.ResponseWriter, r *http.Request, record operationRecord) {
	if record.State == "pending" {
		writeData(w, r, http.StatusOK, map[string]string{"operation": record.Operation, "state": "unknown"})
		return
	}
	if record.State == "failed" {
		writeError(w, r, http.StatusConflict, record.ErrorCode, "stored Library operation failed")
		return
	}
	var data any
	if json.Unmarshal(record.Response, &data) != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "stored operation is invalid")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func (h *service) operationStatus(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	operation := chi.URLParam(r, "operation")
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	if _, _, err := legacyCommandRoute(commandInput{Kind: operation, ResourceID: "resource"}); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_COMMAND", "operation is invalid")
		return
	}
	value, ok := h.require(w, r, commandPermission(operation))
	if !ok {
		return
	}
	var record operationRecord
	err := h.database.QueryRow(r.Context(), `SELECT id,request_hash,operation,state,COALESCE(response,'null'::jsonb),COALESCE(error_code,'') FROM library_adapter_operations WHERE client_id=$1 AND actor_user_id=$2 AND method='POST' AND normalized_route=$3 AND idempotency_key=$4 AND expires_at>now()`, h.clientID, value.userID, contract.CommandRoute, key).Scan(&record.ID, &record.RequestHash, &record.Operation, &record.State, &record.Response, &record.ErrorCode)
	if errors.Is(err, pgx.ErrNoRows) {
		writeData(w, r, http.StatusOK, map[string]string{"operation": operation, "state": "unknown"})
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Library operation ledger is unavailable")
		return
	}
	if record.Operation != operation {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "operation does not match Idempotency-Key history")
		return
	}
	h.writeStoredOperation(w, r, record)
}
