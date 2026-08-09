//go:build linux

package state

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const privilegedConsumerHelper = "HENUKIT_TEST_PRIVILEGED_CONSUMER_HELPER"

func TestPrivilegedMaterialsConsumerCrossUID(t *testing.T) {
	if helper := os.Getenv(privilegedConsumerHelper); helper != "" {
		runPrivilegedConsumerHelper(t, helper, os.Getenv("HENUKIT_TEST_STATE_DIR"))
		return
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root to exercise the production receiver/runner UID boundary")
	}

	parent := crossUIDParent(t)
	stateDir := filepath.Join(parent, "state")
	runAsNobody(t, "initialize", stateDir)

	if _, err := NewMaterialsLatestArrival(stateDir, time.Hour); err == nil {
		t.Fatal("ordinary constructor accepted a foreign-owned state directory")
	}

	store, err := NewMaterialsLatestArrivalPrivilegedConsumer(stateDir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(stateDir, "queue"), 0o770); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(); err == nil {
		t.Fatal("opened privileged consumer ignored an unsafe queue mutation")
	}
	if err := os.Chmod(filepath.Join(stateDir, "queue"), 0o750); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Claim()
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Delivery != "materials-first" {
		t.Fatalf("claimed = %+v", claimed)
	}
	started := time.Now().UTC()
	if err := store.Complete(Attempt{
		Event: *claimed, StartedAt: started, FinishedAt: started.Add(time.Second), Succeeded: true,
	}); err != nil {
		t.Fatal(err)
	}

	// The receiver must still be able to reopen the queue, read the root
	// consumer's terminal state, and accept the next delivery.
	runAsNobody(t, "reopen", stateDir)
}

func TestPrivilegedMaterialsConsumerRejectsUnsafeOrUninitializedState(t *testing.T) {
	if os.Getenv(privilegedConsumerHelper) != "" || os.Geteuid() != 0 {
		return
	}

	nonexistent := filepath.Join(crossUIDParent(t), "missing")
	if _, err := NewMaterialsLatestArrivalPrivilegedConsumer(nonexistent, time.Hour); err == nil {
		t.Fatal("privileged consumer initialized a missing state directory")
	}
	if _, err := os.Lstat(nonexistent); !os.IsNotExist(err) {
		t.Fatalf("missing state path was created: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, stateDir string)
	}{
		{
			name: "group-writable subdirectory",
			mutate: func(t *testing.T, stateDir string) {
				if err := os.Chmod(filepath.Join(stateDir, "queue"), 0o770); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "foreign-owned subdirectory",
			mutate: func(t *testing.T, stateDir string) {
				if err := os.Chown(filepath.Join(stateDir, "processed"), 0, 0); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked policy",
			mutate: func(t *testing.T, stateDir string) {
				policy := filepath.Join(stateDir, "queue-policy.json")
				if err := os.Rename(policy, policy+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(policy+".real", policy); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked lock",
			mutate: func(t *testing.T, stateDir string) {
				lock := filepath.Join(stateDir, ".lock")
				if err := os.Rename(lock, lock+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(lock+".real", lock); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := crossUIDParent(t)
			stateDir := filepath.Join(parent, "state")
			runAsNobody(t, "initialize", stateDir)
			test.mutate(t, stateDir)
			if _, err := NewMaterialsLatestArrivalPrivilegedConsumer(stateDir, time.Hour); err == nil {
				t.Fatal("privileged consumer accepted unsafe state")
			}
		})
	}
}

func runPrivilegedConsumerHelper(t *testing.T, action, stateDir string) {
	t.Helper()
	store, err := NewMaterialsLatestArrival(stateDir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	switch action {
	case "initialize":
		result, err := store.Enqueue(materialsConsumerTestEvent("materials-first", '1'))
		if err != nil || !result.Queued {
			t.Fatalf("enqueue = %+v, %v", result, err)
		}
		if _, err := NewMaterialsLatestArrivalPrivilegedConsumer(stateDir, time.Hour); err == nil {
			t.Fatal("non-root process opened the privileged consumer")
		}
	case "reopen":
		snapshot, err := store.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.LastSuccess == nil || snapshot.LastSuccess.Event.Delivery != "materials-first" {
			t.Fatalf("snapshot = %+v", snapshot)
		}
		result, err := store.Enqueue(materialsConsumerTestEvent("materials-second", '2'))
		if err != nil || !result.Queued {
			t.Fatalf("enqueue after privileged completion = %+v, %v", result, err)
		}
	default:
		t.Fatalf("unknown helper action %q", action)
	}
}

func runAsNobody(t *testing.T, action, stateDir string) {
	t.Helper()
	executable := filepath.Join(filepath.Dir(stateDir), "state-test")
	if _, err := os.Stat(executable); os.IsNotExist(err) {
		source, err := os.Open(os.Args[0])
		if err != nil {
			t.Fatal(err)
		}
		defer source.Close()
		target, err := os.OpenFile(executable, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(target, source); err != nil {
			target.Close()
			t.Fatal(err)
		}
		if err := target.Close(); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command(executable, "-test.run=^TestPrivilegedMaterialsConsumerCrossUID$")
	command.Env = append(os.Environ(),
		privilegedConsumerHelper+"="+action,
		"HENUKIT_TEST_STATE_DIR="+stateDir,
	)
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s helper failed: %v\n%s", action, err, output)
	}
}

func crossUIDParent(t *testing.T) string {
	t.Helper()
	parent, err := os.MkdirTemp("", "henukit-crossuid-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(parent) })
	if err := os.Chmod(parent, 0o777); err != nil {
		t.Fatal(err)
	}
	return parent
}

func materialsConsumerTestEvent(delivery string, digit byte) Event {
	return Event{
		Delivery: delivery, Repository: "jry21223/HENU-Final-Review", Ref: "refs/heads/main",
		After:      fmt.Sprintf("%c%s", digit, "000000000000000000000000000000000000000"),
		ReceivedAt: time.Now().UTC(),
	}
}
