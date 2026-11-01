package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActionsNotificationsCoalesceAndDeduplicateWithoutClaimingDeployment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notifications")
	s, err := NewActionsNotifications(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		r, err := s.Enqueue(Event{Delivery: fmt.Sprintf("actions-%d", i), Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main", After: strings.Repeat("a", 40), ReceivedAt: time.Now().UTC()})
		if err != nil || !r.Queued {
			t.Fatalf("notification %d result=%v err=%v", i, r, err)
		}
	}
	snapshot, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != "actions-notification" || snapshot.Notification == nil || snapshot.Notification.Delivery != "actions-299" || snapshot.QueueDepth != 0 || snapshot.LastSuccess != nil {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	before, err := os.ReadFile(filepath.Join(dir, "latest-notification.json"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Enqueue(Event{Delivery: "actions-250", Repository: "jry21223/HENU-Kit-DEV", Ref: "refs/heads/main", After: strings.Repeat("a", 40)})
	if err != nil || !r.Duplicate || r.Queued {
		t.Fatalf("duplicate=%v err=%v", r, err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "latest-notification.json"))
	if string(before) != string(after) {
		t.Fatal("duplicate rewrote notification")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("notification state grew to %d files", len(entries))
	}
	if _, err := New(dir, 100, time.Hour); err == nil {
		t.Fatal("generic runner opened notification state")
	}
}

func TestActionsNotificationsRejectQueueReuseAndSymlinkState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fifo")
	if _, err := New(dir, 100, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := NewActionsNotifications(dir); err == nil {
		t.Fatal("reused generic queue")
	}
	dir = filepath.Join(t.TempDir(), "unmarked")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewActionsNotifications(dir); err == nil {
		t.Fatal("reused unmarked state")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewActionsNotifications(link); err == nil {
		t.Fatal("accepted symbolic link state")
	}
}
