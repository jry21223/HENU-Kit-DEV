package notice

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"henukit.dev/notice/internal/contract"
)

var errConflict = errors.New("notice state conflict")

func (h *service) writeOperation(w http.ResponseWriter, r *http.Request, operation string, body []byte, apply func(pgx.Tx) (map[string]any, error)) {
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	route := chi.RouteContext(r.Context()).RoutePattern()
	digest := sha256.Sum256(append([]byte(r.Method+"\n"+route+"\n"), body...))
	requestHash := hex.EncodeToString(digest[:])
	tx, err := h.database.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice database is unavailable")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	lockScope := strings.Join([]string{h.clientID, r.Method, route, key}, "\n")
	if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockScope); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice idempotency lock is unavailable")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM notice_operations WHERE client_id=$1 AND method=$2 AND normalized_route=$3 AND idempotency_key=$4 AND expires_at <= now()`, h.clientID, r.Method, route, key); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice idempotency history is unavailable")
		return
	}
	var storedHash, storedOperation string
	var stored []byte
	err = tx.QueryRow(r.Context(), `SELECT request_hash,operation,response FROM notice_operations WHERE client_id=$1 AND method=$2 AND normalized_route=$3 AND idempotency_key=$4 AND expires_at > now()`, h.clientID, r.Method, route, key).Scan(&storedHash, &storedOperation, &stored)
	if err == nil {
		if storedHash != requestHash || storedOperation != operation {
			writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for another request")
			return
		}
		var data any
		if json.Unmarshal(stored, &data) != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "stored operation is invalid")
			return
		}
		writeData(w, r, http.StatusOK, data)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice database is unavailable")
		return
	}
	data, err := apply(tx)
	if errors.Is(err, errInvalidPublicSourceOrigin) {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "source URL must use the approved public source origin")
		return
	}
	if errors.Is(err, errConflict) {
		writeError(w, r, http.StatusConflict, "NOTICE_CONFLICT", "Notice state or revision changed")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "NOTICE_NOT_FOUND", "Notice resource was not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusConflict, "NOTICE_CONFLICT", "Notice operation conflicts with existing data")
		return
	}
	payload, _ := json.Marshal(data)
	if _, err := tx.Exec(r.Context(), `INSERT INTO notice_operations (client_id,method,normalized_route,idempotency_key,operation,request_hash,response) VALUES ($1,$2,$3,$4,$5,$6,$7)`, h.clientID, r.Method, route, key, operation, requestHash, payload); err != nil {
		writeError(w, r, http.StatusConflict, "NOTICE_CONFLICT", "Notice operation conflicts with idempotency history")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice operation result is unknown")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func (h *service) operationStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.require(w, r, "notice.read"); !ok {
		return
	}
	operation, key := chi.URLParam(r, "operation"), r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	method, route, ok := operationRoute(operation)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "operation is invalid")
		return
	}
	var storedOperation string
	var stored []byte
	err := h.database.QueryRow(r.Context(), `SELECT operation,response FROM notice_operations WHERE client_id=$1 AND method=$2 AND normalized_route=$3 AND idempotency_key=$4 AND expires_at > now()`, h.clientID, method, route, key).Scan(&storedOperation, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		writeData(w, r, http.StatusOK, map[string]any{"operation": operation, "status": "unknown"})
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notice database is unavailable")
		return
	}
	if storedOperation != operation {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "operation does not match Idempotency-Key history")
		return
	}
	var data any
	if json.Unmarshal(stored, &data) != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "stored operation is invalid")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func operationRoute(operation string) (string, string, bool) {
	routes := map[string]string{"source_create": contract.SourceCreateRoute, "version_create": contract.VersionCreateRoute, "review": contract.ReviewRoute, "distribution": contract.DistributionRoute}
	route, ok := routes[operation]
	return http.MethodPost, route, ok
}
