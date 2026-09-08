package career

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"henukit.dev/career/internal/contract"
)

var (
	errCreateIdempotencyConflict = errors.New("career create idempotency conflict")
	errSearchRateLimited         = errors.New("career search rate limited")
	errSearchAlreadyActive       = errors.New("career search already active")
)

type createSearchInput struct {
	Profile map[string]any `json:"profile"`
}

type searchWire struct {
	ID           string            `json:"id"`
	Status       string            `json:"status"`
	Stage        string            `json:"stage,omitempty"`
	UserID       string            `json:"user_id"`
	HasEmail     bool              `json:"has_email"`
	DigestStatus string            `json:"digest_status,omitempty"`
	ErrorCode    string            `json:"error_code,omitempty"`
	ErrorMsg     string            `json:"error_message,omitempty"`
	CreatedAt    string            `json:"created_at"`
	Result       *searchResultWire `json:"result,omitempty"`
}

type searchResultWire struct {
	SourceCount  int                `json:"source_count"`
	JobCount     int                `json:"job_count"`
	MatchedCount int                `json:"matched_count"`
	Summary      string             `json:"summary"`
	Sources      []searchSourceWire `json:"sources"`
	Jobs         []browserJob       `json:"jobs"`
}

type searchSourceWire struct {
	Key      string `json:"key"`
	Status   string `json:"status"`
	Found    int    `json:"found"`
	Fetched  int    `json:"fetched,omitempty"`
	Rejected int    `json:"rejected,omitempty"`
}

// browserJob is the intentional display contract. Full duties and
// requirements remain in the durable owner result for matching/audit, but the
// Portal only needs this bounded subset and must not receive 150 long job
// descriptions in one status response.
type browserJob struct {
	SourceKey    string   `json:"source_key"`
	Company      string   `json:"company"`
	Title        string   `json:"title"`
	Location     string   `json:"location"`
	JobType      string   `json:"job_type,omitempty"`
	URL          string   `json:"url"`
	PublishedAt  string   `json:"published_at,omitempty"`
	MatchScore   int      `json:"match_score"`
	MatchReasons []string `json:"match_reasons"`
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
	if errors.Is(err, errSearchRateLimited) {
		w.Header().Set("Retry-After", "3600")
		writeError(w, r, http.StatusTooManyRequests, "SEARCH_RATE_LIMITED", "本小时扫描次数已用完，请稍后再试")
		return
	}
	if errors.Is(err, errSearchAlreadyActive) {
		w.Header().Set("Retry-After", "30")
		writeError(w, r, http.StatusTooManyRequests, "SEARCH_ALREADY_ACTIVE", "已有扫描任务正在进行，请等待完成后再试")
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
	var active int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM career_searches WHERE user_id=$1 AND status IN ('queued','running')`, value.userID).Scan(&active); err != nil {
		return searchWire{}, err
	}
	if active >= h.searchActiveLimit {
		return searchWire{}, errSearchAlreadyActive
	}
	if err := h.consumeSearchQuota(r.Context(), value.userID); err != nil {
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

const searchRateScript = `
local current = redis.call('INCR', KEYS[1])
if current == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end
return current`

// consumeSearchQuota is fail-closed: if Redis cannot durably account for the
// task, no crawler or digest work is created. The key hashes the actor ID so a
// Redis key listing does not expose a user identifier.
func (h *service) consumeSearchQuota(ctx context.Context, userID string) error {
	identity := sha256.Sum256([]byte(userID))
	window := h.now().UTC().Format("2006010215")
	key := "career:search-hour:" + hex.EncodeToString(identity[:]) + ":" + window
	count, err := h.redis.Eval(ctx, searchRateScript, []string{key}, int((2 * time.Hour).Seconds())).Int()
	if err != nil {
		return err
	}
	if count > h.searchRateLimit {
		return errSearchRateLimited
	}
	return nil
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
	rows, err := h.database.Query(r.Context(), `
		SELECT s.id,s.status,COALESCE(s.stage,''),s.email_sent_at,COALESCE(d.status,''),s.created_at,
		       COALESCE(r.source_count,0),COALESCE(r.job_count,0),
		       COALESCE(r.matched_count,0),COALESCE(r.summary,''),COALESCE(r.payload,'{}'::jsonb),r.search_id IS NOT NULL
		FROM career_searches s
		LEFT JOIN career_search_results r ON r.search_id=s.id
		LEFT JOIN career_digest_deliveries d ON d.search_id=s.id
		WHERE s.user_id=$1 ORDER BY s.created_at DESC LIMIT 50`, value.userID)
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
		var sourceCount, jobCount, matchedCount int
		var summary string
		var payload []byte
		var hasResult bool
		if err := rows.Scan(&item.ID, &item.Status, &stage, &emailSentAt, &item.DigestStatus, &createdAt, &sourceCount, &jobCount, &matchedCount, &summary, &payload, &hasResult); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career searches are unavailable")
			return
		}
		item.UserID = value.userID
		item.Stage = stage
		item.HasEmail = emailSentAt != nil
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		if hasResult {
			item.Result, err = decodeSearchResult(payload, sourceCount, jobCount, matchedCount, summary)
			if err != nil {
				writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career searches are unavailable")
				return
			}
			if len(item.Result.Jobs) > 3 {
				item.Result.Jobs = item.Result.Jobs[:3]
			}
		}
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
	if err != nil {
		return searchWire{}, false, err
	}
	if !found {
		return searchWire{}, false, nil
	}
	return item, true, nil
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
	var payload []byte
	var sourceCount, jobCount, matchedCount int
	var summary string
	var hasResult bool
	query := `
		SELECT s.id,s.status,COALESCE(s.stage,''),s.user_id,s.email_sent_at,COALESCE(d.status,''),
		       COALESCE(s.error_code,''),COALESCE(s.error_message,''),s.created_at,
		       COALESCE(r.payload,'{}'::jsonb),COALESCE(r.source_count,0),COALESCE(r.job_count,0),
		       COALESCE(r.matched_count,0),COALESCE(r.summary,''),r.search_id IS NOT NULL
		FROM career_searches s LEFT JOIN career_search_results r ON r.search_id=s.id
		LEFT JOIN career_digest_deliveries d ON d.search_id=s.id
		WHERE s.id=$1`
	args := []any{id}
	if userID != "" {
		query += ` AND s.user_id=$2`
		args = append(args, userID)
	}
	err := h.database.QueryRow(r.Context(), query, args...).Scan(
		&item.ID, &item.Status, &stage, &item.UserID, &emailSentAt, &item.DigestStatus, &errorCode, &errorMsg, &createdAt,
		&payload, &sourceCount, &jobCount, &matchedCount, &summary, &hasResult,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return searchWire{}, false, nil
	}
	if err != nil {
		return searchWire{}, false, err
	}
	item.Stage = stage
	item.HasEmail = emailSentAt != nil
	if item.Status == "failed" {
		item.ErrorCode = errorCode
		item.ErrorMsg = errorMsg
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	if hasResult {
		item.Result, err = decodeSearchResult(payload, sourceCount, jobCount, matchedCount, summary)
		if err != nil {
			return searchWire{}, false, err
		}
	}
	return item, true, nil
}

func decodeSearchResult(payload []byte, sourceCount, jobCount, _ int, _ string) (*searchResultWire, error) {
	var stored struct {
		Jobs    []Job `json:"jobs"`
		Sources map[string]struct {
			Status   string `json:"status"`
			Found    int    `json:"found"`
			Fetched  int    `json:"fetched"`
			Rejected int    `json:"rejected"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(payload, &stored); err != nil {
		return nil, err
	}
	jobs := make([]browserJob, 0, len(stored.Jobs))
	matchedCount := 0
	for _, job := range stored.Jobs {
		reasons := job.MatchReasons
		if reasons == nil {
			reasons = []string{}
		}
		jobs = append(jobs, browserJob{
			SourceKey: job.SourceKey, Company: job.Company, Title: job.Title,
			Location: job.Location, JobType: job.JobType, URL: job.URL,
			PublishedAt: job.PublishedAt, MatchScore: job.MatchScore, MatchReasons: reasons,
		})
		if careerJobIsRelevant(job) {
			matchedCount++
		}
	}
	sort.SliceStable(jobs, func(left, right int) bool {
		return jobs[left].MatchScore > jobs[right].MatchScore
	})
	sourceKeys := make([]string, 0, len(stored.Sources))
	for key := range stored.Sources {
		sourceKeys = append(sourceKeys, key)
	}
	sort.Strings(sourceKeys)
	sources := make([]searchSourceWire, 0, len(sourceKeys))
	succeeded := 0
	for _, key := range sourceKeys {
		state := stored.Sources[key]
		if state.Status == "success" {
			succeeded++
		}
		sources = append(sources, searchSourceWire{
			Key: key, Status: state.Status, Found: state.Found, Fetched: state.Fetched, Rejected: state.Rejected,
		})
	}
	if len(sources) == 0 {
		succeeded = sourceCount
	}
	summary := careerScanSummary(sourceCount, succeeded, jobCount, matchedCount)
	return &searchResultWire{
		SourceCount: sourceCount, JobCount: jobCount, MatchedCount: matchedCount,
		Summary: summary, Sources: sources, Jobs: jobs,
	}, nil
}
