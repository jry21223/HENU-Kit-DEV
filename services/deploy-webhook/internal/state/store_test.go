package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStoreQueueClaimCompleteAndDedupe(t *testing.T) {
	store := newTestStore(t, 10)
	event := Event{
		Delivery: "delivery-1", Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main",
		After: "1111111111111111111111111111111111111111", ReceivedAt: time.Unix(1, 0).UTC(),
	}
	result, err := store.Enqueue(event)
	if err != nil || !result.Queued || result.Duplicate {
		t.Fatalf("enqueue = %+v, %v", result, err)
	}
	result, err = store.Enqueue(event)
	if err != nil || !result.Duplicate || result.Queued {
		t.Fatalf("duplicate enqueue = %+v, %v", result, err)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != event.Delivery {
		t.Fatalf("claimed = %+v", claimed)
	}
	started := time.Now().UTC()
	if err := store.Complete(Attempt{
		Event: *claimed, StartedAt: started, FinishedAt: started.Add(time.Second), Succeeded: true,
	}); err != nil {
		t.Fatal(err)
	}
	result, err = store.Enqueue(event)
	if err != nil || !result.Duplicate {
		t.Fatalf("processed duplicate = %+v, %v", result, err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 0 || snapshot.Running != nil || snapshot.LastSuccess == nil {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestStoreFailureDeduplicatesOrdinaryRedeliveryButAllowsExplicitRetry(t *testing.T) {
	store := newTestStore(t, 10)
	event := Event{
		Delivery: "delivery-failure", Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main",
		After: "2222222222222222222222222222222222222222", ReceivedAt: time.Now().UTC(),
	}
	if _, err := store.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	if err := store.Complete(Attempt{
		Event: *claimed, StartedAt: started, FinishedAt: started.Add(time.Second),
		Succeeded: false, ExitCode: 2, Error: "failed",
	}); err != nil {
		t.Fatal(err)
	}
	result, err := store.Enqueue(event)
	if err != nil || result.Queued || !result.Duplicate {
		t.Fatalf("ordinary failed redelivery = %+v, %v", result, err)
	}
	result, err = store.RetryFailedSHA(event.After)
	if err != nil || !result.Queued || result.Duplicate {
		t.Fatalf("explicit failed retry = %+v, %v", result, err)
	}
	retried, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if retried == nil || retried.Delivery != event.Delivery {
		t.Fatalf("retried event = %+v, want %q", retried, event.Delivery)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.LastFailure == nil || snapshot.LastFailure.ExitCode != 2 {
		t.Fatalf("last failure = %+v", snapshot.LastFailure)
	}
}

func TestStoreClaimsEventsFIFO(t *testing.T) {
	store := newTestStore(t, 10)
	for index, delivery := range []string{"delivery-a", "delivery-b", "delivery-c"} {
		_, err := store.Enqueue(Event{
			Delivery: delivery, Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main",
			After:      "3333333333333333333333333333333333333333",
			ReceivedAt: time.Unix(int64(index+1), 0).UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, expected := range []string{"delivery-a", "delivery-b", "delivery-c"} {
		claimed, err := store.Claim()
		if err != nil {
			t.Fatal(err)
		}
		if claimed.Delivery != expected {
			t.Fatalf("claimed %q, want %q", claimed.Delivery, expected)
		}
		now := time.Now().UTC()
		if err := store.Complete(Attempt{Event: *claimed, StartedAt: now, FinishedAt: now.Add(time.Millisecond), Succeeded: true}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStoreRecoversInterruptedEvent(t *testing.T) {
	store := newTestStore(t, 10)
	event := Event{
		Delivery: "delivery-recover", Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main",
		After: "4444444444444444444444444444444444444444", ReceivedAt: time.Now().UTC(),
	}
	if _, err := store.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(); err != nil {
		t.Fatal(err)
	}
	if err := store.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != event.Delivery {
		t.Fatalf("recovered claim = %+v", claimed)
	}
}

func TestStoreRecoveryDoesNotAutomaticallyRetryFailedCrashWindow(t *testing.T) {
	store := newTestStore(t, 10)
	event := Event{
		Delivery: "delivery-failed-crash-window", Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main",
		After: "4545454545454545454545454545454545454545", ReceivedAt: time.Unix(45, 0).UTC(),
	}
	if _, err := store.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim()
	if err != nil || claimed == nil {
		t.Fatalf("claim = %+v, %v", claimed, err)
	}
	started := time.Unix(46, 0).UTC()
	attempt := Attempt{
		Event: *claimed, StartedAt: started, FinishedAt: started.Add(time.Second),
		Succeeded: false, ExitCode: 78, Error: "approval required",
	}
	failedName := fmt.Sprintf("%020d-%s.json", attempt.FinishedAt.UnixNano(), attempt.Event.Delivery)
	if err := writeJSONAtomic(filepath.Join(store.failedDir(), failedName), attempt); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(store.lastFailurePath(), attempt); err != nil {
		t.Fatal(err)
	}

	if err := store.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Running != nil || snapshot.QueueDepth != 0 {
		t.Fatalf("recovered snapshot = %+v, want failed terminal state without automatic retry", snapshot)
	}
	if claimed, err := store.Claim(); err != nil || claimed != nil {
		t.Fatalf("automatic retry claim = %+v, %v", claimed, err)
	}
	if result, err := store.Enqueue(event); err != nil || !result.Duplicate || result.Queued {
		t.Fatalf("ordinary redelivery = %+v, %v", result, err)
	}
	result, err := store.RetryFailedSHA(event.After)
	if err != nil || !result.Queued || result.Duplicate {
		t.Fatalf("explicit retry = %+v, %v", result, err)
	}
	claimed, err = store.Claim()
	if err != nil || claimed == nil || claimed.Delivery != event.Delivery {
		t.Fatalf("explicit retry claim = %+v, %v", claimed, err)
	}
	retryGeneration := claimed.ReceivedAt
	if retryGeneration.Equal(event.ReceivedAt) {
		t.Fatalf("explicit retry generation = %s, want a fresh ReceivedAt", retryGeneration)
	}

	// A crash while the approved retry is running must not be confused with the
	// old failed attempt for the same GitHub delivery. Recovery may suppress the
	// original failed generation, but must requeue this explicitly approved one.
	if err := store.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if recovered == nil || recovered.Delivery != event.Delivery || !recovered.ReceivedAt.Equal(retryGeneration) {
		t.Fatalf("recovered approved retry = %+v, want generation %s", recovered, retryGeneration)
	}
}

func TestStoreRetriesNewestFailedDeliveryForSHA(t *testing.T) {
	store := newTestStore(t, 10)
	sha := "6666666666666666666666666666666666666666"
	for index, delivery := range []string{"delivery-old", "delivery-new"} {
		event := Event{
			Delivery: delivery, Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main",
			After: sha, ReceivedAt: time.Unix(int64(index+1), 0).UTC(),
		}
		if _, err := store.Enqueue(event); err != nil {
			t.Fatal(err)
		}
		claimed, err := store.Claim()
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now().UTC().Add(time.Duration(index) * time.Second)
		if err := store.Complete(Attempt{
			Event: *claimed, StartedAt: started, FinishedAt: started.Add(time.Millisecond),
			Succeeded: false, ExitCode: 78, Error: "approval required",
		}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := store.RetryFailedSHA(sha)
	if err != nil || !result.Queued || result.Duplicate {
		t.Fatalf("retry = %+v, %v", result, err)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != "delivery-new" {
		t.Fatalf("retried event = %+v", claimed)
	}
}

func TestStoreRetryRejectsUnknownSHA(t *testing.T) {
	store := newTestStore(t, 10)
	_, err := store.RetryFailedSHA("7777777777777777777777777777777777777777")
	if !errors.Is(err, ErrFailedNotFound) {
		t.Fatalf("retry error = %v, want ErrFailedNotFound", err)
	}
}

func TestStoreHonorsQueueLimit(t *testing.T) {
	store := newTestStore(t, 1)
	for index, delivery := range []string{"delivery-one", "delivery-two"} {
		_, err := store.Enqueue(Event{
			Delivery: delivery, Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main",
			After: "5555555555555555555555555555555555555555", ReceivedAt: time.Unix(int64(index+1), 0).UTC(),
		})
		if index == 0 && err != nil {
			t.Fatal(err)
		}
		if index == 1 && !errors.Is(err, ErrQueueFull) {
			t.Fatalf("second enqueue error = %v, want ErrQueueFull", err)
		}
	}
}

func TestLatestQueueCoalescesBurstIntoOneNewestRerun(t *testing.T) {
	store, err := NewWithQueueMode(t.TempDir(), 1, 24*time.Hour, QueueModeLatest)
	if err != nil {
		t.Fatal(err)
	}

	first := Event{
		Delivery: "delivery-first", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "1111111111111111111111111111111111111111", ReceivedAt: time.Unix(1, 0).UTC(),
	}
	second := Event{
		Delivery: "delivery-second", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "2222222222222222222222222222222222222222", ReceivedAt: time.Unix(2, 0).UTC(),
	}
	third := Event{
		Delivery: "delivery-third", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "3333333333333333333333333333333333333333", ReceivedAt: time.Unix(3, 0).UTC(),
	}
	fourth := Event{
		Delivery: "delivery-fourth", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "4444444444444444444444444444444444444444", ReceivedAt: time.Unix(4, 0).UTC(),
	}

	if result, err := store.Enqueue(first); err != nil || !result.Queued || result.Coalesced {
		t.Fatalf("first enqueue = %+v, %v", result, err)
	}
	if result, err := store.Enqueue(second); err != nil || !result.Queued || !result.Coalesced {
		t.Fatalf("pre-run coalescing = %+v, %v", result, err)
	}

	running, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if running == nil || running.Delivery != second.Delivery {
		t.Fatalf("running event = %+v, want newest pre-run delivery", running)
	}

	if result, err := store.Enqueue(third); err != nil || !result.Queued || result.Coalesced {
		t.Fatalf("first rerun enqueue = %+v, %v", result, err)
	}
	if result, err := store.Enqueue(fourth); err != nil || !result.Queued || !result.Coalesced {
		t.Fatalf("rerun coalescing = %+v, %v", result, err)
	}
	if result, err := store.Enqueue(fourth); err != nil || !result.Duplicate || result.Queued {
		t.Fatalf("duplicate newest delivery = %+v, %v", result, err)
	}

	now := time.Now().UTC()
	if err := store.Complete(Attempt{
		Event: *running, StartedAt: now, FinishedAt: now.Add(time.Millisecond), Succeeded: true,
	}); err != nil {
		t.Fatal(err)
	}
	runAgain, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if runAgain == nil || runAgain.Delivery != fourth.Delivery {
		t.Fatalf("queued rerun = %+v, want latest delivery", runAgain)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 0 {
		t.Fatalf("queue depth = %d, want 0 after claiming sole rerun", snapshot.QueueDepth)
	}
}

func TestLatestQueueRecoveryKeepsTheNewerQueuedRerun(t *testing.T) {
	store, err := NewWithQueueMode(t.TempDir(), 1, 24*time.Hour, QueueModeLatest)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := Event{
		Delivery: "delivery-interrupted", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReceivedAt: time.Unix(1, 0).UTC(),
	}
	newer := Event{
		Delivery: "delivery-newer", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ReceivedAt: time.Unix(2, 0).UTC(),
	}
	if _, err := store.Enqueue(interrupted); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue(newer); err != nil {
		t.Fatal(err)
	}

	if err := store.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 1 || snapshot.Running != nil {
		t.Fatalf("recovered snapshot = %+v, want one queued rerun", snapshot)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != newer.Delivery {
		t.Fatalf("recovered claim = %+v, want newer queued delivery", claimed)
	}
	if result, err := store.Enqueue(interrupted); err != nil || !result.Duplicate {
		t.Fatalf("superseded interrupted redelivery = %+v, %v", result, err)
	}
}

func TestLatestQueueRejectsTamperedPersistedDeliveryBeforeWritingMarker(t *testing.T) {
	store, err := NewWithQueueMode(t.TempDir(), 1, 24*time.Hour, QueueModeLatest)
	if err != nil {
		t.Fatal(err)
	}
	first := Event{
		Delivery: "delivery-first", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "1111111111111111111111111111111111111111", ReceivedAt: time.Unix(1, 0).UTC(),
	}
	if _, err := store.Enqueue(first); err != nil {
		t.Fatal(err)
	}
	queued, err := os.ReadDir(store.queueDir())
	if err != nil || len(queued) != 1 {
		t.Fatalf("queued files = %v, %v", queued, err)
	}
	tampered := `{"delivery":"../../escaped","repository":"jry21223/HENU-Final-Review","ref":"refs/heads/main","after":"2222222222222222222222222222222222222222"}`
	if err := os.WriteFile(filepath.Join(store.queueDir(), queued[0].Name()), []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}

	second := Event{
		Delivery: "delivery-second", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "3333333333333333333333333333333333333333", ReceivedAt: time.Unix(2, 0).UTC(),
	}
	if _, err := store.Enqueue(second); err == nil || !strings.Contains(err.Error(), "delivery ID is invalid") {
		t.Fatalf("enqueue after tampering error = %v, want invalid delivery", err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), "escaped.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("tampered marker escaped coalesced directory: %v", err)
	}
}

func TestLatestQueueConcurrentEnqueueKeepsExactlyOneNewestDelivery(t *testing.T) {
	store, err := NewWithQueueMode(t.TempDir(), 1, 24*time.Hour, QueueModeLatest)
	if err != nil {
		t.Fatal(err)
	}
	const deliveries = 24
	start := make(chan struct{})
	errorsByWorker := make(chan error, deliveries)
	var workers sync.WaitGroup
	for index := 0; index < deliveries; index++ {
		index := index
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, enqueueErr := store.Enqueue(Event{
				Delivery:   fmt.Sprintf("delivery-%02d", index),
				Repository: "jry21223/HENU-Final-Review",
				Ref:        "refs/heads/main",
				After:      fmt.Sprintf("%040x", index+1),
				ReceivedAt: time.Unix(0, int64(index+1)).UTC(),
			})
			errorsByWorker <- enqueueErr
		}()
	}
	close(start)
	workers.Wait()
	close(errorsByWorker)
	for enqueueErr := range errorsByWorker {
		if enqueueErr != nil {
			t.Fatalf("concurrent enqueue: %v", enqueueErr)
		}
	}

	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 1 {
		t.Fatalf("queue depth = %d, want exactly one latest delivery", snapshot.QueueDepth)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != "delivery-23" {
		t.Fatalf("claimed = %+v, want newest received delivery", claimed)
	}
}

func TestLatestQueueApprovedRetryReplacesTheSinglePendingRerun(t *testing.T) {
	store, err := NewWithQueueMode(t.TempDir(), 1, 24*time.Hour, QueueModeLatest)
	if err != nil {
		t.Fatal(err)
	}
	failed := Event{
		Delivery: "delivery-failed", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: strings.Repeat("a", 40), ReceivedAt: time.Unix(1, 0).UTC(),
	}
	if _, err := store.Enqueue(failed); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim()
	if err != nil || claimed == nil {
		t.Fatalf("claim failed delivery = %+v, %v", claimed, err)
	}
	started := time.Now().UTC()
	if err := store.Complete(Attempt{
		Event: *claimed, StartedAt: started, FinishedAt: started.Add(time.Second), Succeeded: false, ExitCode: 1,
	}); err != nil {
		t.Fatal(err)
	}
	pending := Event{
		Delivery: "delivery-pending", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: strings.Repeat("b", 40), ReceivedAt: time.Unix(2, 0).UTC(),
	}
	if _, err := store.Enqueue(pending); err != nil {
		t.Fatal(err)
	}
	result, err := store.RetryFailedSHA(failed.After)
	if err != nil || !result.Queued || !result.Coalesced {
		t.Fatalf("approved retry = %+v, %v", result, err)
	}
	retried, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if retried == nil || retried.Delivery != failed.Delivery {
		t.Fatalf("retried event = %+v, want failed delivery", retried)
	}
}

func newTestStore(t *testing.T, maxQueue int) *Store {
	t.Helper()
	store, err := New(t.TempDir(), maxQueue, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
