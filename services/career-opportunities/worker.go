package career

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// staleClaimAfter is how long a 'running' search may stay unclaimed before the
// worker treats it as a crashed attempt and reclaims it. This is what keeps the
// guarantee "never permanently stuck in running" even when the process dies
// between claiming and finishing.
const staleClaimAfter = 15 * time.Minute

const (
	workerIdleDelay   = time.Second
	workerRetryMin    = 250 * time.Millisecond
	workerRetryMax    = 5 * time.Second
)

type worker struct {
	h *service
}

type workerStepFunc func(context.Context) (bool, error)
type workerSleepFunc func(context.Context, time.Duration) error

// WorkResult is what a WorkFunc returns for one completed search.
type WorkResult struct {
	Payload      any
	SourceCount  int
	JobCount     int
	MatchedCount int
	Summary      string
}

// Step claims and completes at most one queued search, returning false when
// nothing was queued. It is safe to call concurrently: only one caller wins
// the claim for a given row (FOR UPDATE SKIP LOCKED), and a replayed completion
// on an already-finished search is a no-op that never writes a second result.
func (w *worker) Step(ctx context.Context) (bool, error) {
	searchID, profile, found, err := w.h.claimOne(ctx)
	if err != nil || !found {
		return found, err
	}
	err = w.h.finish(ctx, searchID, profile)
	return true, err
}

// Run drives queued searches to completion forever until ctx is cancelled.
// A transient dependency or transaction failure must not permanently kill the
// background worker while the HTTP service continues to accept new searches.
// Retry delay grows exponentially to a small cap and resets after any
// successful Step so recovery is prompt without creating a hot error loop.
func (w *worker) Run(ctx context.Context) error {
	return runWorkerLoop(ctx, w.Step, sleepWorker)
}

func runWorkerLoop(ctx context.Context, step workerStepFunc, sleep workerSleepFunc) error {
	retryDelay := workerRetryMin
	for {
		done, err := step(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("career worker step failed; retrying in %s: %v", retryDelay, err)
			if err := sleep(ctx, retryDelay); err != nil {
				return err
			}
			retryDelay *= 2
			if retryDelay > workerRetryMax {
				retryDelay = workerRetryMax
			}
			continue
		}

		retryDelay = workerRetryMin
		if !done {
			if err := sleep(ctx, workerIdleDelay); err != nil {
				return err
			}
		}
	}
}

func sleepWorker(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// claimOne atomically moves one queued search (or one stale 'running' search
// left by a crashed attempt) to running and returns its frozen profile
// snapshot. FOR UPDATE SKIP LOCKED lets concurrent workers claim different rows
// without blocking. Only rows in 'queued', or 'running' ones older than
// staleClaimAfter, are ever claimed, so a healthy in-flight job is untouched.
func (h *service) claimOne(ctx context.Context) (string, any, bool, error) {
	var id, snapshot string
	err := h.database.QueryRow(ctx, `
		WITH claimed AS (
			SELECT id FROM career_searches
			WHERE status = 'queued'
			   OR (status = 'running' AND started_at < now() - $1::interval)
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE career_searches s
		SET status = 'running', stage = 'crawling', started_at = now()
		FROM claimed WHERE s.id = claimed.id
		RETURNING s.id, s.profile_snapshot`, staleClaimAfter.String()).Scan(&id, &snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	var profile any
	if err := json.Unmarshal([]byte(snapshot), &profile); err != nil {
		return "", nil, false, err
	}
	return id, profile, true, nil
}

// finish runs the work for one claimed search and records the outcome. It is
// guarded by a transition back to 'running': if the row already finished (a
// retried completion), the UPDATE matches zero rows and nothing happens, so a
// retry can never write a second result row.
func (h *service) finish(ctx context.Context, searchID string, profile any) error {
	result, err := h.work(ctx, profile)
	if err != nil {
		return h.fail(ctx, searchID, "CAREER_WORK_FAILED", err)
	}
	payload, _ := json.Marshal(result.Payload)
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE career_searches SET status='completed',stage='rendering',completed_at=now(),failed_at=NULL WHERE id=$1 AND status='running'`, searchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// The search already left 'running' (a replayed completion): leave it
		// exactly as-is and never mint a second result row.
		return tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `INSERT INTO career_search_results(search_id,payload,source_count,job_count,matched_count,summary) VALUES($1,$2,$3,$4,$5,$6)`, searchID, payload, result.SourceCount, result.JobCount, result.MatchedCount, result.Summary)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// The result is durable. The digest mail is best-effort after commit: a
	// failure must never roll back the search result, and email_sent_at stays
	// NULL so a later pass could retry.
	h.enqueueDigest(ctx, searchID, result)
	return nil
}

// enqueueDigest posts one Opportunity Digest for a completed search when the
// owner enabled email notifications. It is best-effort: every failure is
// logged and the search result is untouched. The email_sent_at guard plus the
// Platform Core dedupe key make replays idempotent.
func (h *service) enqueueDigest(ctx context.Context, searchID string, result WorkResult) {
	if h.digestSender == nil {
		return
	}
	var userID string
	var completedAt any
	if err := h.database.QueryRow(ctx, `SELECT user_id,to_char(completed_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM career_searches WHERE id=$1`, searchID).Scan(&userID, &completedAt); err != nil {
		log.Printf("career digest: cannot read search %s: %v", searchID, err)
		return
	}
	var enabled bool
	if err := h.database.QueryRow(ctx, `SELECT COALESCE((SELECT email_notification_enabled FROM career_profiles WHERE user_id=$1), true)`, userID).Scan(&enabled); err != nil {
		log.Printf("career digest: cannot read profile for search %s: %v", searchID, err)
		return
	}
	if !enabled {
		return
	}
	var emailSentAt any
	if err := h.database.QueryRow(ctx, `SELECT email_sent_at FROM career_searches WHERE id=$1`, searchID).Scan(&emailSentAt); err != nil {
		log.Printf("career digest: cannot read email guard for search %s: %v", searchID, err)
		return
	}
	if emailSentAt != nil {
		return
	}
	digest := DigestRequest{
		UserID: userID, SearchID: searchID,
		CompletedAt:  completedAt.(string),
		SourceCount:  result.SourceCount,
		JobCount:     result.JobCount,
		MatchedCount: result.MatchedCount,
		Summary:      result.Summary,
		TopJobs:      digestTopJobs(result.Payload, 5),
		RequestID:    "req_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
	}
	if h.digestResultURL != "" {
		digest.CareerURL = strings.TrimRight(h.digestResultURL, "/") + "?search=" + searchID
	}
	if err := h.digestSender.SendDigest(ctx, digest); err != nil {
		// Best-effort by design: the search result stays completed and
		// email_sent_at stays NULL so a later pass could retry.
		log.Printf("career digest enqueue failed for search %s: %v", searchID, err)
		return
	}
	if _, err := h.database.Exec(ctx, `UPDATE career_searches SET email_sent_at=now() WHERE id=$1 AND email_sent_at IS NULL`, searchID); err != nil {
		log.Printf("career digest sent for search %s but the guard could not be recorded: %v", searchID, err)
	}
}

// digestTopJobs extracts the top N matches by score from the persisted payload
// and maps them to the browser-safe digest subset. Unknown payload shapes yield
// an empty list instead of an error.
func digestTopJobs(payload any, top int) []DigestJob {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var result struct {
		Jobs []Job `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil
	}
	order := make([]int, 0, len(result.Jobs))
	for index := range result.Jobs {
		order = append(order, index)
	}
	// Stable selection by descending match score.
	for first := 0; first < len(order) && first < top; first++ {
		best := first
		for candidate := first + 1; candidate < len(order); candidate++ {
			if result.Jobs[order[candidate]].MatchScore > result.Jobs[order[best]].MatchScore {
				best = candidate
			}
		}
		order[first], order[best] = order[best], order[first]
	}
	jobs := make([]DigestJob, 0, min(len(order), top))
	for _, index := range order[:min(len(order), top)] {
		job := result.Jobs[index]
		jobs = append(jobs, DigestJob{
			Company: job.Company, Title: job.Title, Location: job.Location,
			URL: job.URL, MatchScore: job.MatchScore, MatchReasons: job.MatchReasons,
		})
	}
	return jobs
}

func validDigestResultURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// fail moves a claimed search to failed with a stable browser-safe code and
// message. The underlying cause is never surfaced to the browser (it may leak
// internal paths or connector details); it is only written to the server log.
// The transition is guarded: a search that already finished is never re-failed.
func (h *service) fail(ctx context.Context, searchID, code string, cause error) error {
	log.Printf("career search %s failed (%s): %v", searchID, code, cause)
	_, err := h.database.Exec(ctx, `UPDATE career_searches SET status='failed',stage=NULL,completed_at=NULL,failed_at=now(),error_code=$2,error_message='job execution failed' WHERE id=$1 AND status='running'`, searchID, code)
	return err
}
