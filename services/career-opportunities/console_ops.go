package career

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type consoleQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type consoleCounts struct {
	Profiles          int `json:"profiles"`
	SearchesTotal     int `json:"searches_total"`
	SearchesQueued    int `json:"searches_queued"`
	SearchesRunning   int `json:"searches_running"`
	SearchesCompleted int `json:"searches_completed"`
	SearchesFailed    int `json:"searches_failed"`
	ExtractionsTotal  int `json:"extractions_total"`
	DigestsPending    int `json:"digests_pending"`
	DigestsSent       int `json:"digests_sent"`
}

type consoleSearchItem struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Status       string     `json:"status"`
	Stage        string     `json:"stage"`
	SourceCount  int        `json:"source_count"`
	JobCount     int        `json:"job_count"`
	MatchedCount int        `json:"matched_count"`
	ErrorCode    string     `json:"error_code"`
	ErrorMessage string     `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	FailedAt     *time.Time `json:"failed_at"`
	EmailSentAt  *time.Time `json:"email_sent_at"`
}

type consoleExtractionItem struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Status       string     `json:"status"`
	ErrorCode    string     `json:"error_code"`
	ErrorMessage string     `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	FailedAt     *time.Time `json:"failed_at"`
}

type consoleDigestItem struct {
	SearchID    string     `json:"search_id"`
	Status      string     `json:"status"`
	AttemptedAt *time.Time `json:"attempted_at"`
	EnqueuedAt  *time.Time `json:"enqueued_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type consoleWorkspacePayload struct {
	JobSourceEnabled bool                    `json:"job_source_enabled"`
	AIMode           string                  `json:"ai_mode"`
	GeneratedAt      time.Time               `json:"generated_at"`
	Counts           consoleCounts           `json:"counts"`
	Searches         []consoleSearchItem     `json:"searches"`
	Extractions      []consoleExtractionItem `json:"extractions"`
	DigestDeliveries []consoleDigestItem     `json:"digest_deliveries"`
}

func (h *service) requireConsolePermission(w http.ResponseWriter, r *http.Request, permission string) bool {
	if _, ok := h.requireActor(w, r); !ok {
		return false
	}
	if r.Header.Get("X-Scope-Kind") != "product" || r.Header.Get("X-Product-Code") != "career" {
		writeError(w, r, http.StatusForbidden, "SCOPE_DENIED", "Career product scope is required")
		return false
	}
	if r.Header.Get("X-Permission-Code") != permission {
		writeError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "required Career permission is missing")
		return false
	}
	return true
}

func loadConsoleCounts(ctx context.Context, db consoleQuerier) (consoleCounts, error) {
	var c consoleCounts
	if err := db.QueryRow(ctx, `SELECT count(*) FROM career_profiles`).Scan(&c.Profiles); err != nil {
		return c, err
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'queued'),
		       count(*) FILTER (WHERE status = 'running'),
		       count(*) FILTER (WHERE status = 'completed'),
		       count(*) FILTER (WHERE status = 'failed')
		FROM career_searches
	`).Scan(&c.SearchesTotal, &c.SearchesQueued, &c.SearchesRunning, &c.SearchesCompleted, &c.SearchesFailed); err != nil {
		return c, err
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM career_resume_extractions`).Scan(&c.ExtractionsTotal); err != nil {
		return c, err
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status IN ('sending', 'retry')),
		       count(*) FILTER (WHERE status = 'sent')
		FROM career_digest_deliveries
	`).Scan(&c.DigestsPending, &c.DigestsSent); err != nil {
		return c, err
	}
	return c, nil
}

func (h *service) loadConsoleCounts(ctx context.Context) (consoleCounts, error) {
	return loadConsoleCounts(ctx, h.database)
}

func (h *service) consoleSummary(w http.ResponseWriter, r *http.Request) {
	now := h.now()
	counts, err := h.loadConsoleCounts(r.Context())
	if err != nil {
		writeData(w, r, http.StatusOK, map[string]any{
			"id":             "career",
			"status":         "partial",
			"status_message": "求职雷达统计数据暂时不可读",
			"as_of":          now,
			"metrics":        []map[string]string{},
		})
		return
	}
	sourceLabel := "已启用"
	status := "ok"
	msg := fmt.Sprintf("已建档画像 %d 份，扫描完成 %d 次、失败 %d 次，待投递简报 %d 条", counts.Profiles, counts.SearchesCompleted, counts.SearchesFailed, counts.DigestsPending)
	if h.jobSourceDisabled {
		sourceLabel = "已暂停"
		status = "partial"
		msg = fmt.Sprintf("外部抓取源已暂停（画像 %d 份与历史记录正常服务，已完成扫描 %d 次）", counts.Profiles, counts.SearchesCompleted)
	}
	metrics := []map[string]string{
		{"label": "求职画像", "value": fmt.Sprintf("%d", counts.Profiles)},
		{"label": "扫描中/排队", "value": fmt.Sprintf("%d", counts.SearchesQueued+counts.SearchesRunning)},
		{"label": "扫描已完成", "value": fmt.Sprintf("%d", counts.SearchesCompleted)},
		{"label": "扫描失败", "value": fmt.Sprintf("%d", counts.SearchesFailed)},
		{"label": "待发简报", "value": fmt.Sprintf("%d", counts.DigestsPending)},
		{"label": "抓取数据源", "value": sourceLabel},
	}
	writeData(w, r, http.StatusOK, map[string]any{
		"id":             "career",
		"status":         status,
		"status_message": msg,
		"as_of":          now,
		"metrics":        metrics,
	})
}

func (h *service) consoleWorkspace(w http.ResponseWriter, r *http.Request) {
	if !h.requireConsolePermission(w, r, "career.read") {
		return
	}
	h.writeConsoleWorkspace(w, r, h.database)
}

func (h *service) writeConsoleWorkspace(w http.ResponseWriter, r *http.Request, db consoleQuerier) {
	counts, err := loadConsoleCounts(r.Context(), db)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career database unavailable")
		return
	}
	mode := h.aiMode
	if mode == "" {
		mode = "unconfigured"
	}
	payload := consoleWorkspacePayload{
		JobSourceEnabled: !h.jobSourceDisabled,
		AIMode:           mode,
		GeneratedAt:      h.now(),
		Counts:           counts,
		Searches:         []consoleSearchItem{},
		Extractions:      []consoleExtractionItem{},
		DigestDeliveries: []consoleDigestItem{},
	}

	sRows, err := db.Query(r.Context(), `
		SELECT s.id::text, s.user_id::text, s.status, COALESCE(s.stage, ''),
		       COALESCE(r.source_count, 0), COALESCE(r.job_count, 0), COALESCE(r.matched_count, 0),
		       COALESCE(s.error_code, ''), COALESCE(s.error_message, ''),
		       s.created_at, s.started_at, s.completed_at, s.failed_at, s.email_sent_at
		FROM career_searches s
		LEFT JOIN career_search_results r ON r.search_id = s.id
		ORDER BY s.created_at DESC
		LIMIT 50
	`)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to load career searches")
		return
	}
	defer sRows.Close()
	for sRows.Next() {
		var item consoleSearchItem
		if err := sRows.Scan(
			&item.ID, &item.UserID, &item.Status, &item.Stage,
			&item.SourceCount, &item.JobCount, &item.MatchedCount,
			&item.ErrorCode, &item.ErrorMessage,
			&item.CreatedAt, &item.StartedAt, &item.CompletedAt, &item.FailedAt, &item.EmailSentAt,
		); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to scan career searches")
			return
		}
		payload.Searches = append(payload.Searches, item)
	}
	if err := sRows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to read career searches")
		return
	}

	eRows, err := db.Query(r.Context(), `
		SELECT id::text, user_id::text, status,
		       COALESCE(error_code, ''), COALESCE(error_message, ''),
		       created_at, completed_at, failed_at
		FROM career_resume_extractions
		ORDER BY created_at DESC
		LIMIT 30
	`)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to load career resume extractions")
		return
	}
	defer eRows.Close()
	for eRows.Next() {
		var item consoleExtractionItem
		if err := eRows.Scan(
			&item.ID, &item.UserID, &item.Status,
			&item.ErrorCode, &item.ErrorMessage,
			&item.CreatedAt, &item.CompletedAt, &item.FailedAt,
		); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to scan career resume extractions")
			return
		}
		payload.Extractions = append(payload.Extractions, item)
	}
	if err := eRows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to read career resume extractions")
		return
	}

	dRows, err := db.Query(r.Context(), `
		SELECT search_id::text, status, attempted_at, enqueued_at, created_at, updated_at
		FROM career_digest_deliveries
		ORDER BY updated_at DESC
		LIMIT 30
	`)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to load career digest deliveries")
		return
	}
	defer dRows.Close()
	for dRows.Next() {
		var item consoleDigestItem
		if err := dRows.Scan(
			&item.SearchID, &item.Status, &item.AttemptedAt, &item.EnqueuedAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to scan career digest deliveries")
			return
		}
		payload.DigestDeliveries = append(payload.DigestDeliveries, item)
	}
	if err := dRows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to read career digest deliveries")
		return
	}

	writeData(w, r, http.StatusOK, payload)
}

func (h *service) retryConsoleDigest(w http.ResponseWriter, r *http.Request) {
	if !h.requireConsolePermission(w, r, "career.manage") {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	searchID := strings.TrimSpace(chi.URLParam(r, "search_id"))
	if _, err := uuid.Parse(searchID); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_SEARCH_ID", "search_id must be a valid UUID")
		return
	}
	var searchStatus string
	err := h.database.QueryRow(r.Context(), `SELECT status FROM career_searches WHERE id = $1::uuid`, searchID).Scan(&searchStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "SEARCH_NOT_FOUND", "career search was not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "database query failed")
		return
	}
	if searchStatus != "completed" {
		writeError(w, r, http.StatusConflict, "SEARCH_NOT_COMPLETED", "only completed searches can retry digest delivery")
		return
	}
	var updatedAt time.Time
	err = h.database.QueryRow(r.Context(), `
		INSERT INTO career_digest_deliveries (search_id, status, updated_at)
		VALUES ($1::uuid, 'retry', now())
		ON CONFLICT (search_id) DO UPDATE SET status = 'retry', updated_at = now()
		RETURNING updated_at
	`, searchID).Scan(&updatedAt)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "failed to schedule digest retry")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{
		"search_id":  searchID,
		"status":     "retry",
		"updated_at": updatedAt,
	})
}
