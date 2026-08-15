package career

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunWorkerLoopRetriesTransientErrorAndResetsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transient := errors.New("temporary database failure")
	calls := 0
	var delays []time.Duration

	step := func(context.Context) (bool, error) {
		calls++
		switch calls {
		case 1, 2:
			return false, transient
		case 3:
			// A successful busy step resets the retry backoff without sleeping.
			return true, nil
		case 4:
			return false, transient
		default:
			t.Fatalf("unexpected worker step %d", calls)
			return false, nil
		}
	}

	sleep := func(ctx context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		if calls == 4 {
			cancel()
			return ctx.Err()
		}
		return nil
	}

	err := runWorkerLoop(ctx, step, sleep)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runWorkerLoop error = %v, want context.Canceled", err)
	}
	if calls != 4 {
		t.Fatalf("step calls = %d, want 4", calls)
	}
	want := []time.Duration{workerRetryMin, 2 * workerRetryMin, workerRetryMin}
	if len(delays) != len(want) {
		t.Fatalf("retry delays = %v, want %v", delays, want)
	}
	for index := range want {
		if delays[index] != want[index] {
			t.Fatalf("retry delay[%d] = %s, want %s", index, delays[index], want[index])
		}
	}
}

func TestRunWorkerLoopStopsImmediatelyWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stepCalls := 0
	sleepCalls := 0
	step := func(context.Context) (bool, error) {
		stepCalls++
		return false, context.Canceled
	}
	sleep := func(context.Context, time.Duration) error {
		sleepCalls++
		return nil
	}

	err := runWorkerLoop(ctx, step, sleep)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runWorkerLoop error = %v, want context.Canceled", err)
	}
	if stepCalls != 1 {
		t.Fatalf("step calls = %d, want 1", stepCalls)
	}
	if sleepCalls != 0 {
		t.Fatalf("sleep calls = %d, want 0", sleepCalls)
	}
}
