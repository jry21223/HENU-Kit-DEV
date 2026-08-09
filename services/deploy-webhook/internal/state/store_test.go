package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func TestStoreFailureAllowsExplicitRedelivery(t *testing.T) {
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
	if err != nil || !result.Queued || result.Duplicate {
		t.Fatalf("redelivery = %+v, %v", result, err)
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

func TestMaterialsLatestArrivalStoreKeepsOnlyNewestWaitingDelivery(t *testing.T) {
	store := newMaterialsLatestArrivalTestStore(t)
	for index, delivery := range []string{"materials-a", "materials-b", "materials-c"} {
		result, err := store.Enqueue(Event{
			Delivery: delivery, Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
			After:      string(rune('a'+index)) + "333333333333333333333333333333333333333",
			ReceivedAt: time.Unix(int64(index+1), 0).UTC(),
		})
		if err != nil || !result.Queued || result.Duplicate {
			t.Fatalf("enqueue %q = %+v, %v", delivery, result, err)
		}
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 1 || snapshot.Running != nil {
		t.Fatalf("snapshot = %+v, want one waiting delivery and no running delivery", snapshot)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != "materials-c" {
		t.Fatalf("claimed = %+v, want newest delivery", claimed)
	}
}

func TestMaterialsLatestArrivalStoreKeepsRunningDeliveryAndReplacesWaitingDelivery(t *testing.T) {
	store := newMaterialsLatestArrivalTestStore(t)
	first := Event{
		Delivery: "materials-a", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReceivedAt: time.Unix(1, 0).UTC(),
	}
	if _, err := store.Enqueue(first); err != nil {
		t.Fatal(err)
	}
	running, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if running == nil || running.Delivery != first.Delivery {
		t.Fatalf("running = %+v", running)
	}
	for index, delivery := range []string{"materials-b", "materials-c"} {
		if _, err := store.Enqueue(Event{
			Delivery: delivery, Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
			After:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			ReceivedAt: time.Unix(int64(index+2), 0).UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Running == nil || snapshot.Running.Delivery != "materials-a" || snapshot.QueueDepth != 1 {
		t.Fatalf("snapshot = %+v, want materials-a running and one waiting delivery", snapshot)
	}
	if _, err := store.Claim(); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second claim error = %v, want ErrAlreadyRunning", err)
	}
	now := time.Now().UTC()
	if err := store.Complete(Attempt{Event: *running, StartedAt: now, FinishedAt: now.Add(time.Millisecond), Succeeded: true}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != "materials-c" {
		t.Fatalf("claimed = %+v, want newest waiting delivery", claimed)
	}
}

func TestMaterialsLatestArrivalStoreKeepsSupersededDeliveryDeduplicated(t *testing.T) {
	store := newMaterialsLatestArrivalTestStore(t)
	first := Event{
		Delivery: "materials-superseded", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReceivedAt: time.Unix(1, 0).UTC(),
	}
	newest := Event{
		Delivery: "materials-newest", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ReceivedAt: time.Unix(2, 0).UTC(),
	}
	if _, err := store.Enqueue(first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue(newest); err != nil {
		t.Fatal(err)
	}
	result, err := store.Enqueue(first)
	if err != nil || !result.Duplicate || result.Queued {
		t.Fatalf("superseded redelivery = %+v, %v", result, err)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != newest.Delivery {
		t.Fatalf("claimed = %+v, want newest delivery", claimed)
	}
}

func TestMaterialsLatestArrivalStoreRecoveryKeepsNewerWaitingDelivery(t *testing.T) {
	store := newMaterialsLatestArrivalTestStore(t)
	interrupted := Event{
		Delivery: "materials-interrupted", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReceivedAt: time.Unix(1, 0).UTC(),
	}
	newest := Event{
		Delivery: "materials-newest", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ReceivedAt: time.Unix(2, 0).UTC(),
	}
	if _, err := store.Enqueue(interrupted); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue(newest); err != nil {
		t.Fatal(err)
	}
	if err := store.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Running != nil || snapshot.QueueDepth != 1 {
		t.Fatalf("snapshot = %+v, want only one waiting delivery after recovery", snapshot)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != newest.Delivery {
		t.Fatalf("recovered claim = %+v, want newer waiting delivery", claimed)
	}
	result, err := store.Enqueue(interrupted)
	if err != nil || !result.Duplicate || result.Queued {
		t.Fatalf("interrupted redelivery = %+v, %v", result, err)
	}
}

func TestMaterialsLatestArrivalStoreCoalescesConcurrentDeliveries(t *testing.T) {
	store := newMaterialsLatestArrivalTestStore(t)
	const deliveries = 32
	start := make(chan struct{})
	errs := make(chan error, deliveries)
	var group sync.WaitGroup
	for index := 0; index < deliveries; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			_, err := store.Enqueue(Event{
				Delivery:   fmt.Sprintf("materials-concurrent-%d", index),
				Repository: "jry21223/HENU-Final-Review",
				Ref:        "refs/heads/main",
				After:      "cccccccccccccccccccccccccccccccccccccccc",
				ReceivedAt: time.Unix(int64(index+1), 0).UTC(),
			})
			errs <- err
		}(index)
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 1 || snapshot.Running != nil {
		t.Fatalf("snapshot = %+v, want one waiting delivery and no running delivery", snapshot)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil {
		t.Fatal("claimed = nil, want one of the accepted concurrent deliveries")
	}
}

func TestMaterialsLatestArrivalStoreDoesNotClaimSupersededWaitingDelivery(t *testing.T) {
	store := newMaterialsLatestArrivalTestStore(t)
	event := Event{
		Delivery: "materials-crash-window", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReceivedAt: time.Unix(1, 0).UTC(),
	}
	if _, err := store.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	if err := store.withLock(func() error { return store.markSupersededLocked(event) }); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 0 || snapshot.Running != nil {
		t.Fatalf("snapshot = %+v, want superseded delivery to be absent from status", snapshot)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed != nil {
		t.Fatalf("claimed = %+v, want superseded delivery to remain terminal", claimed)
	}
	snapshot, err = store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 0 || snapshot.Running != nil {
		t.Fatalf("snapshot = %+v, want no runnable delivery", snapshot)
	}
}

func TestMaterialsLatestArrivalStoreRecoveryDoesNotRequeueSupersededRunningDelivery(t *testing.T) {
	store := newMaterialsLatestArrivalTestStore(t)
	event := Event{
		Delivery: "materials-superseded-running", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReceivedAt: time.Unix(1, 0).UTC(),
	}
	if _, err := store.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(); err != nil {
		t.Fatal(err)
	}
	if err := store.withLock(func() error { return store.markSupersededLocked(event) }); err != nil {
		t.Fatal(err)
	}
	if err := store.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 0 || snapshot.Running != nil {
		t.Fatalf("snapshot = %+v, want superseded running delivery to remain terminal", snapshot)
	}
}

func TestGenericStoreRejectsMaterialsLatestArrivalStateDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewMaterialsLatestArrival(dir, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir, 10, 24*time.Hour); err == nil {
		t.Fatal("generic FIFO store accepted a materials latest-arrival state directory")
	}
}

func TestMaterialsLatestArrivalStoreRejectsGenericStateDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := New(dir, 10, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMaterialsLatestArrival(dir, 24*time.Hour); err == nil {
		t.Fatal("materials latest-arrival store accepted an unmarked generic FIFO state directory")
	}
}

func TestGenericStoreAcceptsLegacyUnmarkedStateDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"queue", "processed", "failed"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New(dir, 10, 24*time.Hour); err != nil {
		t.Fatalf("generic FIFO store rejected legacy state directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "queue-policy.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy state directory unexpectedly gained a queue policy marker: %v", err)
	}
}

func TestMaterialsLatestArrivalStoreRejectsLegacyUnmarkedGenericStateDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "queue"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMaterialsLatestArrival(dir, 24*time.Hour); err == nil {
		t.Fatal("materials latest-arrival store accepted a legacy unmarked generic FIFO state directory")
	}
}

func TestGenericStorePersistsQueueMode(t *testing.T) {
	dir := t.TempDir()
	if _, err := New(dir, 10, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	var marker queueModeMarker
	if err := readJSON(filepath.Join(dir, "queue-policy.json"), &marker); err != nil {
		t.Fatalf("read persisted queue mode: %v", err)
	}
	if marker.Mode != genericFIFOMode {
		t.Fatalf("queue mode = %q, want %q", marker.Mode, genericFIFOMode)
	}
}

func TestStoreConstructorsAtomicallyClaimQueueMode(t *testing.T) {
	const attempts = 100
	for attempt := 0; attempt < attempts; attempt++ {
		dir := filepath.Join(t.TempDir(), "state")
		start := make(chan struct{})
		type result struct {
			materials bool
			err       error
		}
		results := make(chan result, 2)
		var wait sync.WaitGroup
		for _, materials := range []bool{false, true} {
			wait.Add(1)
			go func(materials bool) {
				defer wait.Done()
				<-start
				if materials {
					_, err := NewMaterialsLatestArrival(dir, 24*time.Hour)
					results <- result{materials: true, err: err}
					return
				}
				_, err := New(dir, 10, 24*time.Hour)
				results <- result{materials: false, err: err}
			}(materials)
		}
		close(start)
		wait.Wait()
		close(results)

		var genericOK, materialsOK bool
		for result := range results {
			if result.err != nil {
				continue
			}
			if result.materials {
				materialsOK = true
			} else {
				genericOK = true
			}
		}
		if genericOK == materialsOK {
			t.Fatalf("attempt %d constructor successes: generic=%t materials=%t, want exactly one mode", attempt, genericOK, materialsOK)
		}

		var marker queueModeMarker
		if err := readJSON(filepath.Join(dir, "queue-policy.json"), &marker); err != nil {
			t.Fatalf("attempt %d read persisted queue mode: %v", attempt, err)
		}
		wantMode := genericFIFOMode
		if materialsOK {
			wantMode = materialsLatestArrivalMode
		}
		if marker.Mode != wantMode {
			t.Fatalf("attempt %d queue mode = %q, want %q", attempt, marker.Mode, wantMode)
		}
	}
}

func TestMaterialsLatestArrivalStoreRejectsManualRetry(t *testing.T) {
	store := newMaterialsLatestArrivalTestStore(t)
	if _, err := store.RetryFailedSHA("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("materials latest-arrival store accepted a generic manual retry")
	}
}

func TestMaterialsLatestArrivalStoreRecoveryRejectsCorruptDeliveryBeforeWritingMarker(t *testing.T) {
	store := newMaterialsLatestArrivalTestStore(t)
	if err := store.writeJSONAtomic(store.runningPath(), Event{
		Delivery: "../../outside-marker", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReceivedAt: time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Enqueue(Event{
		Delivery: "materials-newest", Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ReceivedAt: time.Unix(2, 0).UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecoverInterrupted(); err == nil {
		t.Fatal("recovery accepted a corrupt running delivery")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(store.Dir()), "outside-marker.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt delivery wrote outside the state directory: %v", err)
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

func newTestStore(t *testing.T, maxQueue int) *Store {
	t.Helper()
	store, err := New(t.TempDir(), maxQueue, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func newMaterialsLatestArrivalTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewMaterialsLatestArrival(t.TempDir(), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
