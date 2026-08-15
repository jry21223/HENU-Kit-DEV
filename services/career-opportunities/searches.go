package career

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

	"henukit.dev/career/internal/contract"
)

var errCreateIdempotencyConflict = errors.New("career create idempotency conflict")

type createSearchInput struct {
	Profile map[string]any `json:"profile"`
}

type searchWire struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Stage     string `json:"stage,omitempty"`
	UserID    string `json:"user_id"`
	HasEmail  bool   `json:"has_email"`
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_message,omitempty"`
	CreatedAt string `json:"created_at"`
}

func (h *service) createSearch(w http.ResponseWriter, r *http.Request) {
	value, ok := h.requireActor(w, r)
	if !ok {
		return
	}
	var input createSearchInput
	body, ok := decode(w, r, &input)
	if !ok {
		return
	}
	if !validProfile(input.Profile) {
		writeError(w, r, http.StatusBadRequest, "INVALID_PROFILE", "career profile is invalid")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	digest := sha256.Sum256(append([]byte(r.Method+"\n"+contract.CreateSearchRoute+"\n"), body...))
	search, err := h.storeSearch(r, value, key, input.Profile, hex.EncodeToString(digest[:]))
	if errors.Is(err, errCreateIdempotencyConflict) {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for another request")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career search creation is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"search": search})
}

func validProfile(profile map[string]any) bool {
	return len(profile) != 0
}

// storeSearch creates one queued search inside a transaction. The advisory
// lock on the client+actor dimension keeps the idempotency ledger correct
// under concurrent creates from the same actor. The profile snapshot is frozen
// here and never re-read by the worker.
func (h *service) storeSearch(r *http.Request, value actor, key string, profile map[string]any, hash string) (searchWire, error) {
	tx, err := h.database.BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		return searchWire{}, err
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	lock := strings.Join([]string{h.clientID, value.userID}, "\n")
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lock); err != nil {
		return searchWire{}, err
	}
	var storedHash, storedSearchID string
	err = tx.QueryRow(r.Context(), `SELECT request_hash,search_id FROM career_search_operations WHERE client_id=$1 AND actor_user_id=$2 AND idempotency_key=$3`, h.clientID, value.userID, key).Scan(&storedHash, &storedSearchID)
	if err == nil {
		if storedHash != hash {
			return searchWire{}, errCreateIdempotencyConflict
		}
		if err = tx.Commit(r.Context()); err != nil {
			return searchWire{}, err
		}
		return h.loadSearchByID(r, storedSearchID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return searchWire{}, err
	}
	id := uuid.New()
	snapshot, _ := json.Marshal(profile)
	if _, err = tx.Exec(r.Context(), `INSERT INTO career_searches(id,user_id,status,profile_snapshot) VALUES($1,$2,'queued',$3)`, id, value.userID, snapshot); err != nil {
		return searchWire{}, err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO career_search_operations(id,client_id,actor_user_id,idempotency_key,request_hash,request_id,search_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), h.clientID, value.userID, key, hash, requestID(r), id); err != nil {
		return searchWire{}, err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return searchWire{}, err
	}
	return h.loadSearchByID(r, id.String())
}

func (h *service) searchStatus(w http.ResponseWriter, r *http.Request) {
	value, ok := h.requireActor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "search_id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(w, r, http.StatusNotFound, "SEARCH_NOT_FOUND", "career search does not exist")
		return
	}
	search, found, err := h.loadSearch(r, id, value.userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career search is unavailable")
		return
	}
	if !found {
		writeError(w, r, http.StatusNotFound, "SEARCH_NOT_FOUND", "career search does not exist")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"search": search})
}

func (h *service) listSearches(w http.ResponseWriter, r *http.Request) {
	value, ok := h.requireActor(w, r)
	if !ok {
		return
	}
	rows, err := h.database.Query(r.Context(), `SELECT id,status,COALESCE(stage,''),email_sent_at,created_at FROM career_searches WHERE user_id=$1 ORDER BY created_at DESC`, value.userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career searches are unavailable")
		return
	}
	defer rows.Close()
	searches := []searchWire{}
	for rows.Next() {
		var item searchWire
		var stage string
		var emailSentAt any
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.Status, &stage, &emailSentAt, &createdAt); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career searches are unavailable")
			return
		}
		item.UserID = value.userID
		item.Stage = stage
		item.HasEmail = emailSentAt != nil
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		searches = append(searches, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career searches are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"searches": searches})
}

func (h *service) loadSearch(r *http.Request, id, userID string) (searchWire, bool, error) {
	item, found, err := h.querySearch(r, id, userID)
	if !found {
		return searchWire{}, false, nil
	}
	return item, true, err
}

func (h *service) loadSearchByID(r *http.Request, id string) (searchWire, error) {
	item, found, err := h.querySearch(r, id, "")
	if err != nil {
		return searchWire{}, err
	}
	if !found {
		return searchWire{}, errors.New("career search not found")
	}
	return item, nil
}

// querySearch loads one search, optionally scoped to a single owner. With
// userID set it is an actor-scoped read; with userID empty it is the internal
// idempotent-replay read.
func (h *service) querySearch(r *http.Request, id, userID string) (searchWire, bool, error) {
	var item searchWire
	var stage string
	var emailSentAt any
	var errorCode, errorMsg string
	var createdAt time.Time
	query := `SELECT id,status,COALESCE(stage,''),user_id,email_sent_at,COALESCE(error_code,''),COALESCE(error_message,''),created_at FROM career_searches WHERE id=$1`
	args := []any{id}
	if userID != "" {
		query += ` AND user_id=$2`
		args = append(args, userID)
	}
	err := h.database.QueryRow(r.Context(), query, args...).Scan(&item.ID, &item.Status, &stage, &item.UserID, &emailSentAt, &errorCode, &errorMsg, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchWire{}, false, nil
	}
	if err != nil {
		return searchWire{}, false, err
	}
	item.Stage = stage
	item.HasEmail = emailSentAt != nil
	item.ErrorCode = errorCode
	item.ErrorMsg = errorMsg
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return item, true, nil
}
