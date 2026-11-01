package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const actionsNotificationMode = "actions-notification"
const recentActionsDeliveries = 256

// ActionsNotifications stores a bounded wake-up hint, never a deployment queue.
// The root watcher must independently authorize each release through GitHub.
type ActionsNotifications struct{ base *Store }

type actionsNotification struct {
	Event  Event    `json:"notification"`
	Recent []string `json:"recent_deliveries"`
}

func NewActionsNotifications(dir string) (*ActionsNotifications, error) {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if !filepath.IsAbs(dir) {
		return nil, errors.New("notification state directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	if err := validateStateDirectory(dir); err != nil {
		return nil, err
	}
	s := &ActionsNotifications{base: &Store{dir: dir}}
	err := s.base.withLock(func() error {
		marker := s.base.queueModeMarkerPath()
		var policy queueModeMarker
		if err := readNotificationJSON(marker, &policy); err == nil {
			if policy.Mode != actionsNotificationMode {
				return errors.New("notification state cannot reuse a deployment queue")
			}
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Name() != ".lock" {
				return errors.New("notification state must not reuse an unmarked directory")
			}
		}
		return s.base.writeJSONAtomic(marker, queueModeMarker{Mode: actionsNotificationMode})
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ActionsNotifications) withLock(fn func() error) error {
	if err := validateStateDirectory(s.base.dir); err != nil {
		return err
	}
	return s.base.withLock(func() error {
		var marker queueModeMarker
		if err := readNotificationJSON(s.base.queueModeMarkerPath(), &marker); err != nil {
			return err
		}
		if marker.Mode != actionsNotificationMode {
			return errors.New("notification state policy changed")
		}
		return fn()
	})
}

func (s *ActionsNotifications) Enqueue(event Event) (EnqueueResult, error) {
	var result EnqueueResult
	err := s.withLock(func() error {
		if err := validateEvent(event); err != nil {
			return err
		}
		if len(event.Delivery) > 128 || len(event.Repository) > 256 || len(event.Ref) > 256 || len(event.After) != 40 || !fullSHAPathPattern.MatchString(event.After) || strings.Trim(event.After, "0") == "" {
			return errors.New("invalid notification identity")
		}
		path := filepath.Join(s.base.dir, "latest-notification.json")
		var previous actionsNotification
		if err := readNotificationJSON(path, &previous); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		for _, delivery := range previous.Recent {
			if delivery == event.Delivery {
				result.Duplicate = true
				return nil
			}
		}
		if event.ReceivedAt.IsZero() {
			event.ReceivedAt = time.Now().UTC()
		}
		recent := append(previous.Recent, event.Delivery)
		if len(recent) > recentActionsDeliveries {
			recent = recent[len(recent)-recentActionsDeliveries:]
		}
		if err := s.base.writeJSONAtomic(path, actionsNotification{Event: event, Recent: recent}); err != nil {
			return err
		}
		result.Queued = true
		return nil
	})
	return result, err
}

func (s *ActionsNotifications) Snapshot() (Snapshot, error) {
	result := Snapshot{Mode: actionsNotificationMode}
	err := s.withLock(func() error {
		var current actionsNotification
		if err := readNotificationJSON(filepath.Join(s.base.dir, "latest-notification.json"), &current); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		result.Notification = &current.Event
		return nil
	})
	return result, err
}

// Reject links and oversized state before reading it back into an HTTP status.
func readNotificationJSON(path string, value any) error {
	before, err := os.Lstat(path)
	if err != nil {
		return err
	}
	meta, ok := before.Sys().(*syscall.Stat_t)
	if !before.Mode().IsRegular() || !ok || meta.Uid != uint32(os.Geteuid()) || before.Mode().Perm()&0o077 != 0 || before.Size() > 64*1024 {
		return fmt.Errorf("notification state must be a private bounded regular file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) {
		return errors.New("notification state changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 64*1024 {
		return errors.New("notification state is oversized")
	}
	return json.Unmarshal(data, value)
}
