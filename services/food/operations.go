package food

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"henukit.dev/food/internal/contract"
)

type commandInput struct {
	Kind            string          `json:"kind"`
	ResourceID      string          `json:"resource_id"`
	ExpectedVersion int             `json:"expected_version"`
	Payload         decisionPayload `json:"payload"`
}
type decisionPayload struct {
	Note           string  `json:"note"`
	VenueName      *string `json:"venue_name,omitempty"`
	ItemName       *string `json:"item_name,omitempty"`
	Description    *string `json:"description,omitempty"`
	Campus         *string `json:"campus,omitempty"`
	Tier           *string `json:"tier,omitempty"`
	ReviewText     *string `json:"review_text,omitempty"`
	PriceReference *string `json:"price_reference,omitempty"`
	HoursReference *string `json:"hours_reference,omitempty"`
	Hidden         *bool   `json:"hidden,omitempty"`
}
type operationRecord struct {
	Operation, State, RequestHash string
	Response                      []byte
	ErrorCode                     string
}

var errIdempotencyConflict = errors.New("idempotency key conflict")

func (h *service) command(w http.ResponseWriter, r *http.Request) {
	var input commandInput
	body, ok := decode(w, r, &input)
	if !ok {
		return
	}
	permission, target, finalStatus, valid := commandMetadata(input.Kind)
	trimmedNote := strings.TrimSpace(input.Payload.Note)
	if !valid || input.ExpectedVersion < 1 || utf8.RuneCountInString(trimmedNote) < 2 || utf8.RuneCountInString(input.Payload.Note) > 1000 {
		writeError(w, r, http.StatusBadRequest, "INVALID_COMMAND", "Food command is invalid")
		return
	}
	if finalStatus == "" && !validateEditPayload(input.Kind, input.Payload) {
		writeError(w, r, http.StatusBadRequest, "INVALID_COMMAND", "Food command is invalid")
		return
	}
	if _, err := uuid.Parse(input.ResourceID); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_COMMAND", "resource_id is invalid")
		return
	}
	value, ok := h.require(w, r, permission)
	if !ok {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	digest := sha256.Sum256(append([]byte(r.Method+"\n"+contract.CommandRoute+"\n"), body...))
	record, status, err := h.execute(r, value, key, input, target, finalStatus, hex.EncodeToString(digest[:]))
	if errors.Is(err, errIdempotencyConflict) {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for another request")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusConflict, "FOOD_CONFLICT", "Food resource version or state changed")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food operation is unavailable")
		return
	}
	if status == http.StatusConflict {
		writeError(w, r, status, record.ErrorCode, "Food resource version or state changed")
		return
	}
	h.writeStoredOperation(w, r, record)
}

func (h *service) execute(r *http.Request, value actor, key string, input commandInput, target, finalStatus, hash string) (operationRecord, int, error) {
	tx, err := h.database.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		return operationRecord{}, 0, err
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	lock := strings.Join([]string{h.clientID, value.userID, http.MethodPost, contract.CommandRoute, key}, "\n")
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lock); err != nil {
		return operationRecord{}, 0, err
	}
	var record operationRecord
	err = tx.QueryRow(r.Context(), `SELECT operation,state,request_hash,COALESCE(response,'null'::jsonb),COALESCE(error_code,'') FROM food_operations WHERE client_id=$1 AND actor_user_id=$2 AND method='POST' AND normalized_route=$3 AND idempotency_key=$4`, h.clientID, value.userID, contract.CommandRoute, key).Scan(&record.Operation, &record.State, &record.RequestHash, &record.Response, &record.ErrorCode)
	if err == nil {
		if record.RequestHash != hash || record.Operation != input.Kind {
			return operationRecord{}, 0, errIdempotencyConflict
		}
		if err = tx.Commit(r.Context()); err != nil {
			return operationRecord{}, 0, err
		}
		if record.State == "failed" {
			return record, http.StatusConflict, nil
		}
		return record, http.StatusOK, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return operationRecord{}, 0, err
	}
	table, allowedStatus := targetTable(target)
	var version int
	var currentStatus string
	var query string
	var scanArgs []any
	if allowedStatus == "" {
		// Targets without a status column (e.g. published posts) gate on the
		// optimistic version alone.
		query = `SELECT version FROM ` + table + ` WHERE id=$1 FOR UPDATE`
		scanArgs = []any{&version}
	} else {
		query = `SELECT version,status FROM ` + table + ` WHERE id=$1 FOR UPDATE`
		scanArgs = []any{&version, &currentStatus}
	}
	err = tx.QueryRow(r.Context(), query, input.ResourceID).Scan(scanArgs...)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return operationRecord{}, 0, err
	}
	id := uuid.New()
	if errors.Is(err, pgx.ErrNoRows) || version != input.ExpectedVersion || (allowedStatus != "" && currentStatus != allowedStatus) {
		_, err = tx.Exec(r.Context(), `INSERT INTO food_operations(id,client_id,actor_user_id,method,normalized_route,idempotency_key,operation,request_hash,request_id,target_type,target_id,state,error_code) VALUES($1,$2,$3,'POST',$4,$5,$6,$7,$8,$9,$10,'failed','FOOD_CONFLICT')`, id, h.clientID, value.userID, contract.CommandRoute, key, input.Kind, hash, requestID(r), target, input.ResourceID)
		if err != nil {
			return operationRecord{}, 0, err
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO food_audit_events(id,operation_id,actor_user_id,request_id,action,target_type,target_id,note,outcome) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'failed')`, uuid.New(), id, value.userID, requestID(r), input.Kind, target, input.ResourceID, input.Payload.Note)
		if err != nil {
			return operationRecord{}, 0, err
		}
		if err = tx.Commit(r.Context()); err != nil {
			return operationRecord{}, 0, err
		}
		return operationRecord{Operation: input.Kind, State: "failed", ErrorCode: "FOOD_CONFLICT"}, http.StatusConflict, nil
	}
	version++
	var update string
	var args []any
	if finalStatus == "" {
		// Content-edit command: keep the row's status and update only the
		// fields the caller provided, still versioned and audited.
		update, args = contentEditUpdate(table, input.Payload)
		args = append([]any{input.ResourceID, version}, args...)
	} else {
		update = `UPDATE ` + table + ` SET status=$2,version=$3,updated_at=now() WHERE id=$1`
		args = []any{input.ResourceID, finalStatus, version}
	}
	if _, err = tx.Exec(r.Context(), update, args...); err != nil {
		return operationRecord{}, 0, err
	}
	response, _ := json.Marshal(map[string]any{"operation": input.Kind, "resource_id": input.ResourceID, "state": "succeeded", "version": version})
	_, err = tx.Exec(r.Context(), `INSERT INTO food_operations(id,client_id,actor_user_id,method,normalized_route,idempotency_key,operation,request_hash,request_id,target_type,target_id,state,response) VALUES($1,$2,$3,'POST',$4,$5,$6,$7,$8,$9,$10,'succeeded',$11)`, id, h.clientID, value.userID, contract.CommandRoute, key, input.Kind, hash, requestID(r), target, input.ResourceID, response)
	if err != nil {
		return operationRecord{}, 0, err
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO food_audit_events(id,operation_id,actor_user_id,request_id,action,target_type,target_id,note,outcome) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'succeeded')`, uuid.New(), id, value.userID, requestID(r), input.Kind, target, input.ResourceID, input.Payload.Note)
	if err != nil {
		return operationRecord{}, 0, err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return operationRecord{}, 0, err
	}
	return operationRecord{Operation: input.Kind, State: "succeeded", Response: response}, http.StatusOK, nil
}

// validateEditPayload checks the optional content-edit fields of a command
// whose kind carries no final status. Edit commands must change at least one
// field; each provided field must satisfy its column constraint and belong to
// the kind's target table (a submission edit cannot carry post-only fields).
// Post fields use the same wire values as post creation (English tier keys).
func validateEditPayload(kind string, p decisionPayload) bool {
	if p.VenueName == nil && p.ItemName == nil && p.Description == nil && p.Campus == nil && p.Tier == nil && p.ReviewText == nil && p.PriceReference == nil && p.HoursReference == nil && p.Hidden == nil {
		return false
	}
	switch kind {
	case "submission_edit":
		if p.Tier != nil || p.ReviewText != nil || p.PriceReference != nil || p.HoursReference != nil || p.Hidden != nil {
			return false
		}
	case "post_edit":
		if p.ItemName != nil || p.Description != nil {
			return false
		}
	}
	if p.VenueName != nil && (utf8.RuneCountInString(strings.TrimSpace(*p.VenueName)) < 1 || utf8.RuneCountInString(*p.VenueName) > 160) {
		return false
	}
	if p.ItemName != nil && (utf8.RuneCountInString(strings.TrimSpace(*p.ItemName)) < 1 || utf8.RuneCountInString(*p.ItemName) > 160) {
		return false
	}
	if p.Description != nil && utf8.RuneCountInString(*p.Description) > 2000 {
		return false
	}
	if p.Campus != nil && !validCampus(*p.Campus) {
		return false
	}
	if p.Tier != nil {
		if _, ok := postTierLabels[*p.Tier]; !ok {
			return false
		}
	}
	if p.ReviewText != nil && (utf8.RuneCountInString(strings.TrimSpace(*p.ReviewText)) < 2 || utf8.RuneCountInString(*p.ReviewText) > 2000) {
		return false
	}
	if p.PriceReference != nil && utf8.RuneCountInString(*p.PriceReference) > 200 {
		return false
	}
	return p.HoursReference == nil || utf8.RuneCountInString(*p.HoursReference) <= 200
}

// contentEditUpdate builds an UPDATE for the content-edit branch: column names
// come from the target table's whitelist, values are bound parameters, and
// version/updated_at are always bumped. Only provided fields are written.
func contentEditUpdate(table string, p decisionPayload) (string, []any) {
	var sets []string
	var values []any
	bind := func(column string, value any) {
		values = append(values, value)
		sets = append(sets, column+"=$"+strconv.Itoa(len(values)+2))
	}
	switch table {
	case "food_submissions":
		if p.VenueName != nil {
			bind("venue_name", *p.VenueName)
		}
		if p.ItemName != nil {
			bind("item_name", *p.ItemName)
		}
		if p.Description != nil {
			bind("description", *p.Description)
		}
		if p.Campus != nil {
			bind("campus", *p.Campus)
		}
	case "food_posts":
		if p.VenueName != nil {
			bind("venue_name", *p.VenueName)
		}
		if p.Campus != nil {
			bind("campus", *p.Campus)
		}
		if p.Tier != nil {
			bind("tier", postTierLabels[*p.Tier])
		}
		if p.ReviewText != nil {
			bind("review_text", *p.ReviewText)
		}
		if p.PriceReference != nil {
			bind("price_reference", *p.PriceReference)
		}
		if p.HoursReference != nil {
			bind("hours_reference", *p.HoursReference)
		}
		if p.Hidden != nil {
			bind("hidden", *p.Hidden)
		}
	}
	sets = append(sets, "version=$2", "updated_at=now()")
	return `UPDATE ` + table + ` SET ` + strings.Join(sets, ",") + ` WHERE id=$1`, values
}

func commandMetadata(kind string) (permission, target, finalStatus string, ok bool) {
	switch kind {
	case "submission_edit":
		return "food.review", "submission", "", true
	case "post_edit":
		return "food.review", "post", "", true
	case "submission_approve":
		return "food.review", "submission", "approved", true
	case "submission_reject":
		return "food.review", "submission", "rejected", true
	case "anomaly_resolve":
		return "food.anomaly", "anomaly", "resolved", true
	case "anomaly_dismiss":
		return "food.anomaly", "anomaly", "dismissed", true
	case "tier_adjustment_confirm":
		return "food.tier_adjust", "tier_adjustment", "confirmed", true
	case "tier_adjustment_reject":
		return "food.tier_adjust", "tier_adjustment", "rejected", true
	default:
		return "", "", "", false
	}
}
func targetTable(target string) (string, string) {
	switch target {
	case "submission":
		return "food_submissions", "pending"
	case "post":
		return "food_posts", ""
	case "anomaly":
		return "food_anomaly_tickets", "open"
	default:
		return "food_tier_adjustments", "pending"
	}
}

func (h *service) operationStatus(w http.ResponseWriter, r *http.Request) {
	key, operation := r.Header.Get("Idempotency-Key"), chi.URLParam(r, "operation")
	permission, _, _, valid := commandMetadata(operation)
	if !valid {
		writeError(w, r, http.StatusBadRequest, "INVALID_COMMAND", "operation is invalid")
		return
	}
	value, ok := h.require(w, r, permission)
	if !ok {
		return
	}
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	var record operationRecord
	err := h.database.QueryRow(r.Context(), `SELECT operation,state,COALESCE(response,'null'::jsonb),COALESCE(error_code,'') FROM food_operations WHERE client_id=$1 AND actor_user_id=$2 AND method='POST' AND normalized_route=$3 AND idempotency_key=$4`, h.clientID, value.userID, contract.CommandRoute, key).Scan(&record.Operation, &record.State, &record.Response, &record.ErrorCode)
	if errors.Is(err, pgx.ErrNoRows) {
		writeData(w, r, http.StatusOK, map[string]any{"operation": operation, "state": "unknown"})
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food operation ledger is unavailable")
		return
	}
	if record.Operation != operation {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "operation does not match Idempotency-Key history")
		return
	}
	h.writeStoredOperation(w, r, record)
}
func (h *service) writeStoredOperation(w http.ResponseWriter, r *http.Request, record operationRecord) {
	if record.State == "failed" {
		writeError(w, r, http.StatusConflict, record.ErrorCode, "stored Food operation failed")
		return
	}
	var data any
	if json.Unmarshal(record.Response, &data) != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "stored Food operation is invalid")
		return
	}
	writeData(w, r, http.StatusOK, data)
}
