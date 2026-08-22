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
const digestRetryAfter = time.Minute
const resumeExtractionTimeout = 60 * time.Second

const (
	workerIdleDelay = time.Second
	workerRetryMin  = 250 * time.Millisecond
	workerRetryMax  = 5 * time.Second
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

// Step claims and completes at most one queued job (a search or a resume
// extraction), returning false when nothing was queued. It is safe to call
// concurrently: only one caller wins the claim for a given row (FOR UPDATE
// SKIP LOCKED), and a replayed completion on an already-finished job is a
// no-op that never writes a second result.
func (w *worker) Step(ctx context.Context) (bool, error) {
	searchID, profile, found, err := w.h.claimOne(ctx)
	if err != nil {
		return found, err
	}
	if found {
		return true, w.h.finish(ctx, searchID, profile)
	}
	extractionID, fileName, content, found, err := w.h.claimOneExtraction(ctx)
	if err != nil {
		return found, err
	}
	if found {
		return true, w.h.finishExtraction(ctx, extractionID, fileName, content)
	}
	digestID, result, found, err := w.h.claimOneDigest(ctx)
	if err != nil || !found {
		return found, err
	}
	return true, w.h.retryDigest(ctx, digestID, result)
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
	tag, err := tx.Exec(ctx, `UPDATE career_searches SET status='completed',stage=NULL,error_code=NULL,error_message=NULL,completed_at=now(),failed_at=NULL WHERE id=$1 AND status='running'`, searchID)
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
	if _, err = tx.Exec(ctx, `INSERT INTO career_digest_deliveries(search_id,status) VALUES($1,'sending')`, searchID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// The result is durable. Digest enqueue happens after commit and is retried
	// independently; a transient mail failure never rolls the search back.
	if err := h.deliverDigest(ctx, searchID, result); err != nil {
		log.Printf("career digest enqueue failed for search %s: %v", searchID, err)
		if markErr := h.markDigestRetry(ctx, searchID); markErr != nil {
			log.Printf("career digest retry could not be recorded for search %s: %v", searchID, markErr)
		}
	}
	return nil
}

// claimOneDigest durably claims one completed search whose digest enqueue
// failed. A crashed sending claim is reclaimed after the same bounded
// delay. Platform Core's search-scoped idempotency key makes every replay safe.
func (h *service) claimOneDigest(ctx context.Context) (string, WorkResult, bool, error) {
	var id string
	var payload []byte
	var result WorkResult
	err := h.database.QueryRow(ctx, `
		WITH claimed AS (
			SELECT d.search_id FROM career_digest_deliveries d
			JOIN career_searches s ON s.id=d.search_id
			WHERE s.status='completed' AND s.email_sent_at IS NULL
			  AND d.status IN ('retry','sending')
			  AND d.attempted_at < now() - $1::interval
			ORDER BY s.completed_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE career_digest_deliveries d SET status='sending',attempted_at=now(),updated_at=now()
		FROM claimed
		JOIN career_search_results r ON r.search_id=claimed.search_id
		WHERE d.search_id=claimed.search_id
		RETURNING d.search_id,r.payload,r.source_count,r.job_count,r.matched_count,r.summary`, digestRetryAfter.String()).Scan(
		&id, &payload, &result.SourceCount, &result.JobCount, &result.MatchedCount, &result.Summary,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", WorkResult{}, false, nil
	}
	if err != nil {
		return "", WorkResult{}, false, err
	}
	if err := json.Unmarshal(payload, &result.Payload); err != nil {
		return "", WorkResult{}, false, err
	}
	return id, result, true, nil
}

func (h *service) retryDigest(ctx context.Context, searchID string, result WorkResult) error {
	if err := h.deliverDigest(ctx, searchID, result); err != nil {
		if markErr := h.markDigestRetry(ctx, searchID); markErr != nil {
			return errors.Join(err, markErr)
		}
		return err
	}
	return nil
}

func (h *service) markDigestRetry(ctx context.Context, searchID string) error {
	_, err := h.database.Exec(ctx, `UPDATE career_digest_deliveries SET status='retry',attempted_at=now(),updated_at=now() WHERE search_id=$1 AND status <> 'sent'`, searchID)
	return err
}

// claimOneExtraction atomically moves one queued resume extraction (or one
// stale 'running' row left by a crashed attempt) to running and returns its
// transient file bytes. FOR UPDATE SKIP LOCKED lets concurrent workers claim
// different rows without blocking.
func (h *service) claimOneExtraction(ctx context.Context) (string, string, []byte, bool, error) {
	var id, fileName string
	var content []byte
	err := h.database.QueryRow(ctx, `
		WITH claimed AS (
			SELECT id FROM career_resume_extractions
			WHERE status = 'queued'
			   OR (status = 'running' AND started_at < now() - $1::interval)
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE career_resume_extractions e
		SET status = 'running', started_at = now()
		FROM claimed WHERE e.id = claimed.id
		RETURNING e.id, e.file_name, e.file_content`, staleClaimAfter.String()).Scan(&id, &fileName, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, false, nil
	}
	if err != nil {
		return "", "", nil, false, err
	}
	return id, fileName, content, true, nil
}

// finishExtraction runs the AI extraction for one claimed job, records the
// outcome, and purges the transient file bytes in the same transaction. A
// task-wide deadline bounds PDF rendering plus the provider call so one
// adversarial document cannot monopolize the shared worker indefinitely. The
// transition guard makes a replayed completion a no-op.
func (h *service) finishExtraction(ctx context.Context, extractionID, fileName string, content []byte) error {
	extractionContext, cancel := context.WithTimeout(ctx, resumeExtractionTimeout)
	profile, err := h.extract(extractionContext, fileName, content)
	cancel()
	if err != nil {
		code := "EXTRACT_FAILED"
		if errors.Is(err, ErrAIUnconfigured) {
			code = "EXTRACT_AI_UNCONFIGURED"
		}
		return h.failExtraction(ctx, extractionID, code, err)
	}
	extracted, _ := json.Marshal(profile)
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE career_resume_extractions SET status='completed',extracted=$2,file_content=NULL,failed_at=NULL,completed_at=now() WHERE id=$1 AND status='running'`, extractionID, extracted)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// The job already left 'running' (a replayed completion): leave it
		// exactly as-is and never overwrite a second result.
		return tx.Commit(ctx)
	}
	return tx.Commit(ctx)
}

// failExtraction moves a claimed extraction to failed with a stable
// browser-safe code. The underlying cause is never surfaced to the browser
// (it may leak provider internals); it is only written to the server log.
func (h *service) failExtraction(ctx context.Context, extractionID, code string, cause error) error {
	log.Printf("career extraction %s failed (%s): %v", extractionID, code, cause)
	_, err := h.database.Exec(ctx, `UPDATE career_resume_extractions SET status='failed',file_content=NULL,completed_at=NULL,failed_at=now(),error_code=$2,error_message='resume extraction failed' WHERE id=$1 AND status='running'`, extractionID, code)
	return err
}

// deliverDigest posts one Opportunity Digest for a completed search when the
// owner enabled email notifications. The email_sent_at guard plus Platform
// Core's dedupe key make retries idempotent.
func (h *service) deliverDigest(ctx context.Context, searchID string, result WorkResult) error {
	if h.digestSender == nil {
		_, err := h.database.Exec(ctx, `UPDATE career_digest_deliveries SET status='skipped',updated_at=now() WHERE search_id=$1`, searchID)
		return err
	}
	var userID string
	var completedAt string
	if err := h.database.QueryRow(ctx, `SELECT user_id,to_char(completed_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"') FROM career_searches WHERE id=$1`, searchID).Scan(&userID, &completedAt); err != nil {
		return err
	}
	var enabled bool
	if err := h.database.QueryRow(ctx, `SELECT COALESCE((SELECT email_notification_enabled FROM career_profiles WHERE user_id=$1), true)`, userID).Scan(&enabled); err != nil {
		return err
	}
	if !enabled {
		_, err := h.database.Exec(ctx, `UPDATE career_digest_deliveries SET status='skipped',updated_at=now() WHERE search_id=$1`, searchID)
		return err
	}
	var emailSentAt any
	if err := h.database.QueryRow(ctx, `SELECT email_sent_at FROM career_searches WHERE id=$1`, searchID).Scan(&emailSentAt); err != nil {
		return err
	}
	if emailSentAt != nil {
		_, err := h.database.Exec(ctx, `UPDATE career_digest_deliveries SET status='sent',enqueued_at=COALESCE(enqueued_at,now()),updated_at=now() WHERE search_id=$1`, searchID)
		return err
	}
	digest := DigestRequest{
		UserID: userID, SearchID: searchID,
		CompletedAt:  completedAt,
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
		return err
	}
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `UPDATE career_searches SET email_sent_at=now() WHERE id=$1 AND email_sent_at IS NULL`, searchID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE career_digest_deliveries SET status='sent',enqueued_at=COALESCE(enqueued_at,now()),updated_at=now() WHERE search_id=$1`, searchID); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
		if result.Jobs[index].MatchScore >= 50 {
			order = append(order, index)
		}
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
