package runner

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"henukit.dev/deploy-webhook/internal/state"
)

func TestRunnerPassesValidatedValuesAsArgumentsWithoutShell(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "args.txt")
	script := filepath.Join(dir, "deploy")
	content := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" > \"$OUTPUT\"\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OUTPUT", output)
	store := newRunnerStore(t)
	event := state.Event{
		Delivery: "delivery;touch-pwned", Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main",
		Before: "1111111111111111111111111111111111111111", After: "2222222222222222222222222222222222222222",
		ReceivedAt: time.Now().UTC(),
	}
	if _, err := store.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	runner, err := New(store, script, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.RunAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	arguments, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(arguments)), "\n")
	want := []string{
		"--sha", event.After,
		"--delivery", event.Delivery,
		"--repository", event.Repository,
		"--ref", event.Ref,
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("arguments = %#v, want %#v", lines, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned")); !os.IsNotExist(err) {
		t.Fatalf("shell injection side effect exists: %v", err)
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.LastSuccess == nil || snapshot.LastSuccess.Event.Delivery != event.Delivery {
		t.Fatalf("last success = %+v", snapshot.LastSuccess)
	}
}

func TestRunnerRecordsFailureAndDrainsLaterFix(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "counter")
	script := filepath.Join(dir, "deploy")
	content := `#!/bin/sh
set -eu
count=0
if [ -f "$COUNTER" ]; then count="$(cat "$COUNTER")"; fi
count=$((count + 1))
printf '%s' "$count" > "$COUNTER"
if [ "$count" -eq 1 ]; then exit 7; fi
exit 0
`
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COUNTER", counter)
	store := newRunnerStore(t)
	for index, delivery := range []string{"delivery-fail", "delivery-fix"} {
		if _, err := store.Enqueue(state.Event{
			Delivery: delivery, Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main",
			After: strings.Repeat(string(rune('a'+index)), 40), ReceivedAt: time.Unix(int64(index+1), 0).UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	runner, err := New(store, script, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.RunAll(context.Background()); err == nil {
		t.Fatal("RunAll returned nil after a failed delivery")
	}
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QueueDepth != 0 || snapshot.LastFailure == nil || snapshot.LastSuccess == nil {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.LastFailure.ExitCode != 7 || snapshot.LastSuccess.Event.Delivery != "delivery-fix" {
		t.Fatalf("unexpected results: failure=%+v success=%+v", snapshot.LastFailure, snapshot.LastSuccess)
	}
}

func newRunnerStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.New(t.TempDir(), 10, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
