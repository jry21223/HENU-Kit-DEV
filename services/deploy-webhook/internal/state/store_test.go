package state

import (
	"errors"
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
