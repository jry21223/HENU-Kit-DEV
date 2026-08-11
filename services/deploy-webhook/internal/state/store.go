package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

var (
	ErrQueueFull       = errors.New("deployment queue is full")
	ErrAlreadyRunning  = errors.New("a deployment is already running")
	ErrFailedNotFound  = errors.New("no failed deployment found for SHA")
	fullSHAPathPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

const (
	genericFIFOMode            = "generic-fifo"
	materialsLatestArrivalMode = "materials-latest-arrival"
)

type queueModeMarker struct {
	Mode string `json:"mode"`
}

type Event struct {
	Delivery   string    `json:"delivery"`
	Repository string    `json:"repository"`
	Ref        string    `json:"ref"`
	Before     string    `json:"before,omitempty"`
	After      string    `json:"after"`
	ReceivedAt time.Time `json:"received_at"`
}

type Attempt struct {
	Event      Event     `json:"event"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Succeeded  bool      `json:"succeeded"`
	ExitCode   int       `json:"exit_code,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type Snapshot struct {
	QueueDepth  int      `json:"queue_depth"`
	Running     *Event   `json:"running,omitempty"`
	LastSuccess *Attempt `json:"last_success,omitempty"`
	LastFailure *Attempt `json:"last_failure,omitempty"`
}

type EnqueueResult struct {
	Queued    bool `json:"queued"`
	Duplicate bool `json:"duplicate"`
}

type Store struct {
	dir           string
	maxQueue      int
	retention     time.Duration
	latestArrival bool
	consumerOnly  bool
	ownerUID      int
	ownerGID      int
}

func New(dir string, maxQueue int, retention time.Duration) (*Store, error) {
	return newStore(dir, maxQueue, retention, false)
}

// NewMaterialsLatestArrival creates the materials-only queue policy. It keeps
// at most one waiting delivery, replacing it with each later accepted delivery
// while preserving the generic Store's FIFO default.
func NewMaterialsLatestArrival(dir string, retention time.Duration) (*Store, error) {
	return newStore(dir, 1, retention, true)
}

// NewMaterialsLatestArrivalPrivilegedConsumer opens an existing materials
// queue for the fixed root runner. The unprivileged receiver remains the queue
// owner; this constructor never initializes or takes ownership of queue state.
func NewMaterialsLatestArrivalPrivilegedConsumer(dir string, retention time.Duration) (*Store, error) {
	dir, err := validateStoreOptions(dir, 1, retention)
	if err != nil {
		return nil, err
	}
	if os.Geteuid() != 0 {
		return nil, errors.New("privileged materials consumer must run as root")
	}

	ownerUID, ownerGID, err := validatePrivilegedMaterialsState(dir)
	if err != nil {
		return nil, err
	}
	store := &Store{
		dir: dir, maxQueue: 1, retention: retention, latestArrival: true,
		consumerOnly: true, ownerUID: ownerUID, ownerGID: ownerGID,
	}
	if err := store.withLock(func() error { return nil }); err != nil {
		return nil, err
	}
	return store, nil
}

func newStore(dir string, maxQueue int, retention time.Duration, latestArrival bool) (*Store, error) {
	dir, err := validateStoreOptions(dir, maxQueue, retention)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, maxQueue: maxQueue, retention: retention, latestArrival: latestArrival}
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return nil, fmt.Errorf("create state directory %s: %w", s.dir, err)
	}
	if err := validateStateDirectory(s.dir); err != nil {
		return nil, err
	}
	if err := s.withLock(s.ensureQueueModeLocked); err != nil {
		return nil, err
	}
	paths := []string{s.queueDir(), s.processedDir(), s.failedDir()}
	if s.latestArrival {
		paths = append(paths, s.supersededDir())
	}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o750); err != nil {
			return nil, fmt.Errorf("create state directory %s: %w", path, err)
		}
		if err := validateStateDirectory(path); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func validateStoreOptions(dir string, maxQueue int, retention time.Duration) (string, error) {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "." || !filepath.IsAbs(dir) {
		return "", errors.New("state directory must be an absolute path")
	}
	if maxQueue <= 0 || maxQueue > 10000 {
		return "", errors.New("max queue must be between 1 and 10000")
	}
	if retention <= 0 {
		return "", errors.New("processed delivery retention must be positive")
	}
	return dir, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) Enqueue(event Event) (EnqueueResult, error) {
	var result EnqueueResult
	if s.consumerOnly {
		return result, errors.New("privileged materials consumer cannot enqueue deliveries")
	}
	err := s.withLock(func() error {
		if err := validateEvent(event); err != nil {
			return err
		}
		if err := s.cleanupTerminalLocked(time.Now().UTC()); err != nil {
			return err
		}
		duplicate, err := s.isDuplicateLocked(event.Delivery)
		if err != nil {
			return err
		}
		if duplicate {
			result.Duplicate = true
			return nil
		}
		if event.ReceivedAt.IsZero() {
			event.ReceivedAt = time.Now().UTC()
		}
		if s.latestArrival {
			var waiting Event
			if err := readJSON(s.latestArrivalQueuePath(), &waiting); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("read waiting materials delivery: %w", err)
			} else if err == nil && waiting.Delivery != event.Delivery {
				if err := s.markSupersededLocked(waiting); err != nil {
					return err
				}
			}
			if err := s.writeJSONAtomic(s.latestArrivalQueuePath(), event); err != nil {
				return err
			}
			result.Queued = true
			return nil
		}
		files, err := s.queueFiles()
		if err != nil {
			return err
		}
		if len(files) >= s.maxQueue {
			return ErrQueueFull
		}
		name := fmt.Sprintf("%020d-%s.json", event.ReceivedAt.UnixNano(), event.Delivery)
		if err := s.writeJSONAtomic(filepath.Join(s.queueDir(), name), event); err != nil {
			return err
		}
		result.Queued = true
		return nil
	})
	return result, err
}

// RetryFailedSHA requeues the newest failed delivery for an exact full commit
// SHA. It is intended for the root approval helper after a gated release has
// been reviewed; successful deliveries remain deduplicated.
func (s *Store) RetryFailedSHA(sha string) (EnqueueResult, error) {
	var result EnqueueResult
	if s.consumerOnly {
		return result, errors.New("privileged materials consumer cannot retry deliveries")
	}
	if s.latestArrival {
		return result, errors.New("retry is not supported for the materials latest-arrival queue")
	}
	sha = strings.ToLower(strings.TrimSpace(sha))
	if !fullSHAPathPattern.MatchString(sha) {
		return result, errors.New("retry SHA must be a full lowercase hexadecimal SHA")
	}
	err := s.withLock(func() error {
		entries, err := os.ReadDir(s.failedDir())
		if err != nil {
			return err
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				names = append(names, entry.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
		for _, name := range names {
			var attempt Attempt
			if err := readJSON(filepath.Join(s.failedDir(), name), &attempt); err != nil {
				return err
			}
			if strings.ToLower(attempt.Event.After) != sha {
				continue
			}
			event := attempt.Event
			duplicate, err := s.isDuplicateLocked(event.Delivery)
			if err != nil {
				return err
			}
			if duplicate {
				result.Duplicate = true
				return nil
			}
			files, err := s.queueFiles()
			if err != nil {
				return err
			}
			if len(files) >= s.maxQueue {
				return ErrQueueFull
			}
			event.ReceivedAt = time.Now().UTC()
			queueName := fmt.Sprintf("%020d-%s.json", event.ReceivedAt.UnixNano(), event.Delivery)
			if err := s.writeJSONAtomic(filepath.Join(s.queueDir(), queueName), event); err != nil {
				return err
			}
			result.Queued = true
			return nil
		}
		return ErrFailedNotFound
	})
	return result, err
}

// RecoverInterrupted requeues a deployment that was claimed but did not reach a
// terminal state before a process or host restart.
func (s *Store) RecoverInterrupted() error {
	return s.withLock(func() error {
		path := s.runningPath()
		var event Event
		if err := readJSON(path, &event); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := validateEvent(event); err != nil {
			return fmt.Errorf("invalid running event: %w", err)
		}
		processed, err := s.isProcessed(event.Delivery)
		if err != nil {
			return err
		}
		if s.latestArrival {
			superseded, err := s.isSuperseded(event.Delivery)
			if err != nil {
				return err
			}
			if superseded {
				if err := os.Remove(path); err != nil {
					return err
				}
				return syncDir(s.dir)
			}
			var waiting Event
			err = readJSON(s.latestArrivalQueuePath(), &waiting)
			switch {
			case err == nil:
				if waiting.Delivery != event.Delivery {
					if err := s.markSupersededLocked(event); err != nil {
						return err
					}
				}
				if err := os.Remove(path); err != nil {
					return err
				}
				return syncDir(s.dir)
			case !errors.Is(err, fs.ErrNotExist):
				return fmt.Errorf("read waiting materials delivery: %w", err)
			}
			if processed {
				return os.Remove(path)
			}
			if event.ReceivedAt.IsZero() {
				event.ReceivedAt = time.Now().UTC()
			}
			if err := s.writeJSONAtomic(s.latestArrivalQueuePath(), event); err != nil {
				return err
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			return syncDir(s.dir)
		}
		queued, err := s.hasQueuedDelivery(event.Delivery)
		if err != nil {
			return err
		}
		if processed || queued {
			return os.Remove(path)
		}
		if event.ReceivedAt.IsZero() {
			event.ReceivedAt = time.Now().UTC()
		}
		name := fmt.Sprintf("%020d-%s.json", event.ReceivedAt.UnixNano(), event.Delivery)
		if err := s.writeJSONAtomic(filepath.Join(s.queueDir(), name), event); err != nil {
			return err
		}
		return os.Remove(path)
	})
}

// Claim moves the oldest queued event into the single running slot.
func (s *Store) Claim() (*Event, error) {
	var claimed *Event
	err := s.withLock(func() error {
		if _, err := os.Stat(s.runningPath()); err == nil {
			return ErrAlreadyRunning
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		for {
			files, err := s.queueFiles()
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return nil
			}
			source := filepath.Join(s.queueDir(), files[0])
			var event Event
			if err := readJSON(source, &event); err != nil {
				return fmt.Errorf("read queued event %s: %w", source, err)
			}
			if err := validateEvent(event); err != nil {
				return fmt.Errorf("invalid queued event %s: %w", source, err)
			}
			processed, err := s.isProcessed(event.Delivery)
			if err != nil {
				return err
			}
			superseded := false
			if s.latestArrival {
				superseded, err = s.isSuperseded(event.Delivery)
				if err != nil {
					return err
				}
			}
			if processed || superseded {
				if err := os.Remove(source); err != nil {
					return err
				}
				continue
			}
			if err := os.Rename(source, s.runningPath()); err != nil {
				return fmt.Errorf("claim queued event: %w", err)
			}
			claimed = &event
			return syncDir(s.dir)
		}
	})
	return claimed, err
}

func (s *Store) Complete(attempt Attempt) error {
	return s.withLock(func() error {
		var running Event
		if err := readJSON(s.runningPath(), &running); err != nil {
			return fmt.Errorf("read running event: %w", err)
		}
		if running.Delivery != attempt.Event.Delivery || running.After != attempt.Event.After {
			return errors.New("attempt does not match running event")
		}
		if attempt.StartedAt.IsZero() || attempt.FinishedAt.IsZero() || attempt.FinishedAt.Before(attempt.StartedAt) {
			return errors.New("attempt timestamps are invalid")
		}
		if attempt.Succeeded {
			if err := s.writeJSONAtomic(filepath.Join(s.processedDir(), attempt.Event.Delivery+".json"), attempt); err != nil {
				return err
			}
			if err := s.writeJSONAtomic(s.lastSuccessPath(), attempt); err != nil {
				return err
			}
		} else {
			name := fmt.Sprintf("%020d-%s.json", attempt.FinishedAt.UnixNano(), attempt.Event.Delivery)
			if err := s.writeJSONAtomic(filepath.Join(s.failedDir(), name), attempt); err != nil {
				return err
			}
			if err := s.writeJSONAtomic(s.lastFailurePath(), attempt); err != nil {
				return err
			}
		}
		if err := os.Remove(s.runningPath()); err != nil {
			return err
		}
		if err := s.cleanupTerminalLocked(time.Now().UTC()); err != nil {
			return err
		}
		return syncDir(s.dir)
	})
}

func (s *Store) Snapshot() (Snapshot, error) {
	var snapshot Snapshot
	err := s.withLock(func() error {
		files, err := s.queueFiles()
		if err != nil {
			return err
		}
		if s.latestArrival && len(files) == 1 {
			var waiting Event
			if err := readJSON(filepath.Join(s.queueDir(), files[0]), &waiting); err != nil {
				return err
			}
			superseded, err := s.isSuperseded(waiting.Delivery)
			if err != nil {
				return err
			}
			if superseded {
				if err := os.Remove(filepath.Join(s.queueDir(), files[0])); err != nil {
					return err
				}
				if err := syncDir(s.queueDir()); err != nil {
					return err
				}
				files = nil
			}
		}
		snapshot.QueueDepth = len(files)
		var running Event
		if err := readJSON(s.runningPath(), &running); err == nil {
			snapshot.Running = &running
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		var success Attempt
		if err := readJSON(s.lastSuccessPath(), &success); err == nil {
			snapshot.LastSuccess = &success
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		var failure Attempt
		if err := readJSON(s.lastFailurePath(), &failure); err == nil {
			snapshot.LastFailure = &failure
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	})
	return snapshot, err
}

func (s *Store) withLock(fn func() error) error {
	lockPath := filepath.Join(s.dir, ".lock")
	flags := os.O_RDWR
	if !s.consumerOnly {
		flags |= os.O_CREATE
	}
	before, err := os.Lstat(lockPath)
	if err != nil && (s.consumerOnly || !errors.Is(err, fs.ErrNotExist)) {
		return err
	}
	if err == nil && (before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular()) {
		return errors.New("state lock must be a regular file")
	}
	file, err := os.OpenFile(lockPath, flags, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if s.consumerOnly {
		after, err := file.Stat()
		if err != nil {
			return err
		}
		if before == nil || !os.SameFile(before, after) {
			return errors.New("state lock changed while opening privileged consumer")
		}
		if err := validatePrivilegedOwnedInfo(lockPath, after, s.ownerUID, s.ownerGID, false); err != nil {
			return err
		}
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN) //nolint:errcheck
	if s.consumerOnly {
		ownerUID, ownerGID, err := validatePrivilegedMaterialsState(s.dir)
		if err != nil {
			return err
		}
		if ownerUID != s.ownerUID || ownerGID != s.ownerGID {
			return errors.New("materials state owner changed while opening privileged consumer")
		}
		if err := s.requireQueueModeLocked(materialsLatestArrivalMode); err != nil {
			return err
		}
	}
	return fn()
}

// ensureQueueModeLocked atomically claims the queue policy for a new state
// directory. It must run under withLock: otherwise a generic constructor and a
// materials constructor can both observe an unmarked empty directory and
// return stores with incompatible queue semantics.
func (s *Store) ensureQueueModeLocked() error {
	markerPath := s.queueModeMarkerPath()
	info, err := os.Lstat(markerPath)
	if errors.Is(err, fs.ErrNotExist) {
		entries, err := os.ReadDir(s.dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			// withLock creates this file before calling us. It is not state
			// content and therefore must not make a fresh directory look used.
			if entry.Name() != ".lock" {
				if !s.latestArrival {
					// Existing generic FIFO state predates queue-policy.json and
					// remains supported without retroactively changing it.
					return nil
				}
				return errors.New("materials latest-arrival state directory must not reuse an unmarked queue")
			}
		}
		mode := genericFIFOMode
		if s.latestArrival {
			mode = materialsLatestArrivalMode
		}
		return s.writeJSONAtomic(markerPath, queueModeMarker{Mode: mode})
	}
	if err != nil {
		return fmt.Errorf("inspect queue mode marker: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("queue mode marker is not a regular file: %s", markerPath)
	}
	expectedMode := genericFIFOMode
	if s.latestArrival {
		expectedMode = materialsLatestArrivalMode
	}
	return s.requireQueueModeLocked(expectedMode)
}

func (s *Store) requireQueueModeLocked(expectedMode string) error {
	markerPath := s.queueModeMarkerPath()
	var marker queueModeMarker
	if err := readJSON(markerPath, &marker); err != nil {
		return fmt.Errorf("read queue mode marker: %w", err)
	}
	if marker.Mode != expectedMode {
		if !s.latestArrival && marker.Mode == materialsLatestArrivalMode {
			return errors.New("generic FIFO queue cannot use a materials latest-arrival state directory")
		}
		if s.latestArrival && marker.Mode == genericFIFOMode {
			return errors.New("materials latest-arrival state directory cannot use a generic FIFO queue")
		}
		return fmt.Errorf("unsupported queue mode: %q", marker.Mode)
	}
	return nil
}

func (s *Store) queueFiles() ([]string, error) {
	entries, err := os.ReadDir(s.queueDir())
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			if s.latestArrival && entry.Name() != "latest.json" {
				return nil, fmt.Errorf("materials queue contains an unexpected file: %s", entry.Name())
			}
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func (s *Store) isDuplicateLocked(delivery string) (bool, error) {
	processed, err := s.isProcessed(delivery)
	if err != nil || processed {
		return processed, err
	}
	if s.latestArrival {
		superseded, err := s.isSuperseded(delivery)
		if err != nil || superseded {
			return superseded, err
		}
	}
	queued, err := s.hasQueuedDelivery(delivery)
	if err != nil || queued {
		return queued, err
	}
	running, err := s.runningDelivery()
	if err != nil {
		return false, err
	}
	return running == delivery, nil
}

func (s *Store) hasQueuedDelivery(delivery string) (bool, error) {
	if s.latestArrival {
		var event Event
		if err := readJSON(s.latestArrivalQueuePath(), &event); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}
			return false, err
		}
		return event.Delivery == delivery, nil
	}
	files, err := s.queueFiles()
	if err != nil {
		return false, err
	}
	suffix := "-" + delivery + ".json"
	for _, name := range files {
		if strings.HasSuffix(name, suffix) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) runningDelivery() (string, error) {
	var event Event
	if err := readJSON(s.runningPath(), &event); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return event.Delivery, nil
}

func (s *Store) isProcessed(delivery string) (bool, error) {
	path := filepath.Join(s.processedDir(), delivery+".json")
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, fmt.Errorf("processed delivery marker is not a regular file: %s", path)
	}
	return true, nil
}

func (s *Store) isSuperseded(delivery string) (bool, error) {
	if err := validateDelivery(delivery); err != nil {
		return false, err
	}
	path := filepath.Join(s.supersededDir(), delivery+".json")
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, fmt.Errorf("superseded delivery marker is not a regular file: %s", path)
	}
	return true, nil
}

func (s *Store) markSupersededLocked(event Event) error {
	if err := validateEvent(event); err != nil {
		return err
	}
	if err := s.writeJSONAtomic(filepath.Join(s.supersededDir(), event.Delivery+".json"), event); err != nil {
		return fmt.Errorf("record superseded materials delivery: %w", err)
	}
	return nil
}

func (s *Store) cleanupTerminalLocked(now time.Time) error {
	cutoff := now.Add(-s.retention)
	dirs := []string{s.processedDir(), s.failedDir()}
	if s.latestArrival {
		dirs = append(dirs, s.supersededDir())
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.ModTime().Before(cutoff) {
				if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
		}
	}
	return nil
}

func validateStateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect state directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("state path must be a real directory: %s", path)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("state directory must not be writable by group or other: %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("state directory must be owned by the current service user: %s", path)
	}
	return nil
}

func validatePrivilegedMaterialsState(path string) (int, int, error) {
	root, err := os.Lstat(path)
	if err != nil {
		return 0, 0, fmt.Errorf("inspect existing materials state root %s: %w", path, err)
	}
	if root.Mode()&os.ModeSymlink != 0 || !root.IsDir() {
		return 0, 0, fmt.Errorf("materials state root must be a real directory: %s", path)
	}
	stat, ok := root.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("inspect materials state owner: %s", path)
	}
	ownerUID := int(stat.Uid)
	ownerGID := int(stat.Gid)
	if ownerUID == 0 {
		return 0, 0, errors.New("materials state must be initialized by a non-root receiver")
	}
	if err := validatePrivilegedOwnedInfo(path, root, ownerUID, ownerGID, true); err != nil {
		return 0, 0, err
	}

	for _, child := range []string{"queue", "processed", "failed", "superseded"} {
		childPath := filepath.Join(path, child)
		info, err := os.Lstat(childPath)
		if err != nil {
			return 0, 0, fmt.Errorf("inspect existing materials state directory %s: %w", childPath, err)
		}
		if err := validatePrivilegedOwnedInfo(childPath, info, ownerUID, ownerGID, true); err != nil {
			return 0, 0, err
		}
	}
	for _, child := range []string{".lock", "queue-policy.json"} {
		childPath := filepath.Join(path, child)
		info, err := os.Lstat(childPath)
		if err != nil {
			return 0, 0, fmt.Errorf("inspect existing materials state file %s: %w", childPath, err)
		}
		if err := validatePrivilegedOwnedInfo(childPath, info, ownerUID, ownerGID, false); err != nil {
			return 0, 0, err
		}
	}
	return ownerUID, ownerGID, nil
}

func validatePrivilegedOwnedInfo(path string, info os.FileInfo, ownerUID, ownerGID int, directory bool) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("privileged materials state path must not be a symbolic link: %s", path)
	}
	if directory && !info.IsDir() {
		return fmt.Errorf("privileged materials state path must be a directory: %s", path)
	}
	if !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("privileged materials state path must be a regular file: %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != ownerUID || int(stat.Gid) != ownerGID {
		return fmt.Errorf("privileged materials state path has a different owner: %s", path)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("privileged materials state path must not be writable by group or other: %s", path)
	}
	return nil
}

func validateEvent(event Event) error {
	if err := validateDelivery(event.Delivery); err != nil {
		return err
	}
	if strings.TrimSpace(event.Repository) == "" || strings.TrimSpace(event.Ref) == "" || strings.TrimSpace(event.After) == "" {
		return errors.New("event repository, ref, and after SHA are required")
	}
	return nil
}

func validateDelivery(delivery string) error {
	if strings.TrimSpace(delivery) == "" || strings.ContainsAny(delivery, `/\\`) {
		return errors.New("delivery ID is invalid")
	}
	return nil
}

func readJSON(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func (s *Store) writeJSONAtomic(path string, value any) error {
	dir := filepath.Dir(path)
	if s.consumerOnly {
		info, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if err := validatePrivilegedOwnedInfo(dir, info, s.ownerUID, s.ownerGID, true); err != nil {
			return err
		}
	} else {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if s.consumerOnly {
		if err := temp.Chown(s.ownerUID, s.ownerGID); err != nil {
			temp.Close()
			return err
		}
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func (s *Store) queueDir() string { return filepath.Join(s.dir, "queue") }
func (s *Store) latestArrivalQueuePath() string {
	return filepath.Join(s.queueDir(), "latest.json")
}
func (s *Store) queueModeMarkerPath() string { return filepath.Join(s.dir, "queue-policy.json") }
func (s *Store) processedDir() string        { return filepath.Join(s.dir, "processed") }
func (s *Store) failedDir() string           { return filepath.Join(s.dir, "failed") }
func (s *Store) supersededDir() string       { return filepath.Join(s.dir, "superseded") }
func (s *Store) runningPath() string         { return filepath.Join(s.dir, "running.json") }
func (s *Store) lastSuccessPath() string     { return filepath.Join(s.dir, "last-success.json") }
func (s *Store) lastFailurePath() string     { return filepath.Join(s.dir, "last-failure.json") }
