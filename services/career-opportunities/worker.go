package career

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

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
	return tx.Commit(ctx)
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
