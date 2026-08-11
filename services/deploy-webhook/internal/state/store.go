package state

import (
	"crypto/sha256"
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
	deliveryPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,127}$`)
	repositoryPattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	refPattern         = regexp.MustCompile(`^refs/heads/[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

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
	Coalesced bool `json:"coalesced"`
}

type QueueMode string

const (
	QueueModeFIFO   QueueMode = "fifo"
	QueueModeLatest QueueMode = "latest"
)

type Store struct {
	dir       string
	maxQueue  int
	retention time.Duration
	queueMode QueueMode
}

func New(dir string, maxQueue int, retention time.Duration) (*Store, error) {
	return NewWithQueueMode(dir, maxQueue, retention, QueueModeFIFO)
}

func NewWithQueueMode(dir string, maxQueue int, retention time.Duration, queueMode QueueMode) (*Store, error) {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "." || !filepath.IsAbs(dir) {
		return nil, errors.New("state directory must be an absolute path")
	}
	if maxQueue <= 0 || maxQueue > 10000 {
		return nil, errors.New("max queue must be between 1 and 10000")
	}
	if retention <= 0 {
		return nil, errors.New("processed delivery retention must be positive")
	}
	if queueMode != QueueModeFIFO && queueMode != QueueModeLatest {
		return nil, fmt.Errorf("unsupported queue mode %q", queueMode)
	}
	s := &Store{dir: dir, maxQueue: maxQueue, retention: retention, queueMode: queueMode}
	for _, path := range []string{s.dir, s.queueDir(), s.processedDir(), s.failedDir(), s.coalescedDir()} {
		if err := os.MkdirAll(path, 0o750); err != nil {
			return nil, fmt.Errorf("create state directory %s: %w", path, err)
		}
		if err := validateStateDirectory(path); err != nil {
			return nil, err
		}
	}
	if s.queueMode == QueueModeLatest {
		if err := s.withLock(func() error {
			files, err := s.queueFiles()
			if err != nil || len(files) <= 1 {
				return err
			}
			return s.markQueuedFilesCoalesced(files[:len(files)-1])
		}); err != nil {
			return nil, fmt.Errorf("normalize latest-only queue: %w", err)
		}
	}
	return s, nil
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) Enqueue(event Event) (EnqueueResult, error) {
	var result EnqueueResult
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
		files, err := s.queueFiles()
		if err != nil {
			return err
		}
		if s.queueMode == QueueModeFIFO && len(files) >= s.maxQueue {
			return ErrQueueFull
		}
		if event.ReceivedAt.IsZero() {
			event.ReceivedAt = time.Now().UTC()
		}
		if s.queueMode == QueueModeLatest && len(files) > 0 {
			newest, err := s.readQueuedEvent(files[len(files)-1])
			if err != nil {
				return err
			}
			if newest.ReceivedAt.After(event.ReceivedAt) {
				if err := s.markCoalesced(event); err != nil {
					return err
				}
				result.Coalesced = true
				return nil
			}
			if err := s.validateQueuedFiles(files); err != nil {
				return err
			}
		}
		name := fmt.Sprintf("%020d-%s.json", event.ReceivedAt.UnixNano(), event.Delivery)
		if err := writeJSONAtomic(filepath.Join(s.queueDir(), name), event); err != nil {
			return err
		}
		if s.queueMode == QueueModeLatest {
			if err := s.markQueuedFilesCoalesced(files); err != nil {
				return err
			}
			result.Coalesced = len(files) > 0
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
			if err := validateEvent(attempt.Event); err != nil {
				return fmt.Errorf("validate failed event %s: %w", name, err)
			}
			if strings.ToLower(attempt.Event.After) != sha {
				continue
			}
			event := attempt.Event
			duplicate, err := s.isRetryDuplicateLocked(event.Delivery)
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
			if s.queueMode == QueueModeFIFO && len(files) >= s.maxQueue {
				return ErrQueueFull
			}
			if s.queueMode == QueueModeLatest {
				if err := s.validateQueuedFiles(files); err != nil {
					return err
				}
			}
			event.ReceivedAt = time.Now().UTC()
			queueName := fmt.Sprintf("%020d-%s.json", event.ReceivedAt.UnixNano(), event.Delivery)
			if err := writeJSONAtomic(filepath.Join(s.queueDir(), queueName), event); err != nil {
				return err
			}
			if s.queueMode == QueueModeLatest {
				if err := s.markQueuedFilesCoalesced(files); err != nil {
					return err
				}
				result.Coalesced = len(files) > 0
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
			return fmt.Errorf("validate interrupted event: %w", err)
		}
		processed, err := s.isTerminal(event.Delivery)
		if err != nil {
			return err
		}
		failed, err := s.hasFailedGeneration(event)
		if err != nil {
			return err
		}
		queued, err := s.hasQueuedDelivery(event.Delivery)
		if err != nil {
			return err
		}
		if processed || failed || queued {
			return os.Remove(path)
		}
		if s.queueMode == QueueModeLatest {
			files, err := s.queueFiles()
			if err != nil {
				return err
			}
			if len(files) > 0 {
				if err := s.markCoalesced(event); err != nil {
					return err
				}
				if err := os.Remove(path); err != nil {
					return err
				}
				return syncDir(s.dir)
			}
		}
		if event.ReceivedAt.IsZero() {
			event.ReceivedAt = time.Now().UTC()
		}
		name := fmt.Sprintf("%020d-%s.json", event.ReceivedAt.UnixNano(), event.Delivery)
		if err := writeJSONAtomic(filepath.Join(s.queueDir(), name), event); err != nil {
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
			event, err := s.readQueuedEvent(files[0])
			if err != nil {
				return err
			}
			processed, err := s.isTerminal(event.Delivery)
			if err != nil {
				return err
			}
			if processed {
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
		if err := validateEvent(running); err != nil {
			return fmt.Errorf("validate running event: %w", err)
		}
		if err := validateEvent(attempt.Event); err != nil {
			return fmt.Errorf("validate completed event: %w", err)
		}
		if running.Delivery != attempt.Event.Delivery || running.After != attempt.Event.After {
			return errors.New("attempt does not match running event")
		}
		if attempt.StartedAt.IsZero() || attempt.FinishedAt.IsZero() || attempt.FinishedAt.Before(attempt.StartedAt) {
			return errors.New("attempt timestamps are invalid")
		}
		if attempt.Succeeded {
			if err := writeJSONAtomic(filepath.Join(s.processedDir(), deliveryMarkerName(attempt.Event.Delivery)), attempt); err != nil {
				return err
			}
			if err := writeJSONAtomic(s.lastSuccessPath(), attempt); err != nil {
				return err
			}
		} else {
			name := fmt.Sprintf("%020d-%s.json", attempt.FinishedAt.UnixNano(), attempt.Event.Delivery)
			if err := writeJSONAtomic(filepath.Join(s.failedDir(), name), attempt); err != nil {
				return err
			}
			if err := writeJSONAtomic(s.lastFailurePath(), attempt); err != nil {
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
		snapshot.QueueDepth = len(files)
		var running Event
		if err := readJSON(s.runningPath(), &running); err == nil {
			if err := validateEvent(running); err != nil {
				return fmt.Errorf("validate running event: %w", err)
			}
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
	file, err := os.OpenFile(filepath.Join(s.dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return fn()
}

func (s *Store) queueFiles() ([]string, error) {
	entries, err := os.ReadDir(s.queueDir())
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func (s *Store) isDuplicateLocked(delivery string) (bool, error) {
	processed, err := s.isTerminal(delivery)
	if err != nil || processed {
		return processed, err
	}
	failed, err := s.hasFailedDelivery(delivery)
	if err != nil || failed {
		return failed, err
	}
	return s.isActiveLocked(delivery)
}

// isRetryDuplicateLocked intentionally excludes failed attempts. A failed
// delivery is terminal for ordinary webhook intake, but RetryFailedSHA is the
// single explicit approval path that may enqueue that same delivery again.
func (s *Store) isRetryDuplicateLocked(delivery string) (bool, error) {
	terminal, err := s.isTerminal(delivery)
	if err != nil || terminal {
		return terminal, err
	}
	return s.isActiveLocked(delivery)
}

func (s *Store) isActiveLocked(delivery string) (bool, error) {
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

func (s *Store) hasFailedDelivery(delivery string) (bool, error) {
	if !deliveryPattern.MatchString(delivery) {
		return false, errors.New("delivery ID is invalid")
	}
	return s.failedAttemptMatches(func(attempt Attempt) bool {
		return attempt.Event.Delivery == delivery
	})
}

func (s *Store) hasFailedGeneration(event Event) (bool, error) {
	if err := validateEvent(event); err != nil {
		return false, err
	}
	return s.failedAttemptMatches(func(attempt Attempt) bool {
		failedEvent := attempt.Event
		return failedEvent.Delivery == event.Delivery &&
			failedEvent.Repository == event.Repository &&
			failedEvent.Ref == event.Ref &&
			failedEvent.Before == event.Before &&
			failedEvent.After == event.After &&
			failedEvent.ReceivedAt.Equal(event.ReceivedAt)
	})
}

func (s *Store) failedAttemptMatches(match func(Attempt) bool) (bool, error) {
	entries, err := os.ReadDir(s.failedDir())
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var attempt Attempt
		path := filepath.Join(s.failedDir(), entry.Name())
		if err := readJSON(path, &attempt); err != nil {
			return false, fmt.Errorf("read failed attempt %s: %w", path, err)
		}
		if err := validateEvent(attempt.Event); err != nil {
			return false, fmt.Errorf("validate failed attempt %s: %w", path, err)
		}
		if match(attempt) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) hasQueuedDelivery(delivery string) (bool, error) {
	if !deliveryPattern.MatchString(delivery) {
		return false, errors.New("delivery ID is invalid")
	}
	files, err := s.queueFiles()
	if err != nil {
		return false, err
	}
	for _, name := range files {
		event, err := s.readQueuedEvent(name)
		if err != nil {
			return false, err
		}
		if event.Delivery == delivery {
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
	if err := validateEvent(event); err != nil {
		return "", fmt.Errorf("validate running event: %w", err)
	}
	return event.Delivery, nil
}

func (s *Store) isProcessed(delivery string) (bool, error) {
	if !deliveryPattern.MatchString(delivery) {
		return false, errors.New("delivery ID is invalid")
	}
	return markerExists(s.processedDir(), delivery, "processed")
}

func (s *Store) markQueuedFilesCoalesced(files []string) error {
	for _, name := range files {
		path := filepath.Join(s.queueDir(), name)
		event, err := s.readQueuedEvent(name)
		if err != nil {
			return err
		}
		if err := s.markCoalesced(event); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove superseded event %s: %w", path, err)
		}
	}
	return syncDir(s.queueDir())
}

func (s *Store) markCoalesced(event Event) error {
	if err := validateEvent(event); err != nil {
		return fmt.Errorf("validate superseded event: %w", err)
	}
	if err := writeJSONAtomic(filepath.Join(s.coalescedDir(), deliveryMarkerName(event.Delivery)), event); err != nil {
		return fmt.Errorf("record superseded delivery %s: %w", event.Delivery, err)
	}
	return nil
}

func (s *Store) isTerminal(delivery string) (bool, error) {
	processed, err := s.isProcessed(delivery)
	if err != nil || processed {
		return processed, err
	}
	return markerExists(s.coalescedDir(), delivery, "coalesced")
}

func (s *Store) cleanupTerminalLocked(now time.Time) error {
	cutoff := now.Add(-s.retention)
	for _, dir := range []string{s.processedDir(), s.failedDir(), s.coalescedDir()} {
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

func validateEvent(event Event) error {
	if !deliveryPattern.MatchString(event.Delivery) {
		return errors.New("delivery ID is invalid")
	}
	if !repositoryPattern.MatchString(event.Repository) {
		return errors.New("event repository is invalid")
	}
	if !refPattern.MatchString(event.Ref) || strings.Contains(event.Ref, "..") || strings.Contains(event.Ref, "//") {
		return errors.New("event ref is invalid")
	}
	if !fullSHAPathPattern.MatchString(event.After) {
		return errors.New("event after SHA is invalid")
	}
	if event.Before != "" && !fullSHAPathPattern.MatchString(event.Before) && strings.Trim(event.Before, "0") != "" {
		return errors.New("event before SHA is invalid")
	}
	return nil
}

func (s *Store) readQueuedEvent(name string) (Event, error) {
	var event Event
	path := filepath.Join(s.queueDir(), name)
	if err := readJSON(path, &event); err != nil {
		return Event{}, fmt.Errorf("read queued event %s: %w", path, err)
	}
	if err := validateEvent(event); err != nil {
		return Event{}, fmt.Errorf("validate queued event %s: %w", path, err)
	}
	return event, nil
}

func (s *Store) validateQueuedFiles(files []string) error {
	for _, name := range files {
		if _, err := s.readQueuedEvent(name); err != nil {
			return err
		}
	}
	return nil
}

func deliveryMarkerName(delivery string) string {
	digest := sha256.Sum256([]byte(delivery))
	return fmt.Sprintf("%x.json", digest)
}

func markerExists(dir, delivery, kind string) (bool, error) {
	// The hashed name is the canonical containment boundary. The literal name
	// is checked read-only for upgrade compatibility after strict validation;
	// all new writes use only the hashed form.
	for _, name := range []string{deliveryMarkerName(delivery), delivery + ".json"} {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return false, fmt.Errorf("%s delivery marker is not a regular file: %s", kind, path)
		}
		return true, nil
	}
	return false, nil
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

func writeJSONAtomic(path string, value any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
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

func (s *Store) queueDir() string        { return filepath.Join(s.dir, "queue") }
func (s *Store) processedDir() string    { return filepath.Join(s.dir, "processed") }
func (s *Store) failedDir() string       { return filepath.Join(s.dir, "failed") }
func (s *Store) coalescedDir() string    { return filepath.Join(s.dir, "coalesced") }
func (s *Store) runningPath() string     { return filepath.Join(s.dir, "running.json") }
func (s *Store) lastSuccessPath() string { return filepath.Join(s.dir, "last-success.json") }
func (s *Store) lastFailurePath() string { return filepath.Join(s.dir, "last-failure.json") }
