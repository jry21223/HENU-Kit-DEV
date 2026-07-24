package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"henukit.dev/deploy-webhook/internal/state"
)

type Runner struct {
	store   *state.Store
	command string
	timeout time.Duration
	logger  *slog.Logger
}

func New(store *state.Store, command string, timeout time.Duration, logger *slog.Logger) (*Runner, error) {
	if store == nil {
		return nil, errors.New("state store is required")
	}
	command = filepath.Clean(strings.TrimSpace(command))
	if command == "." || !filepath.IsAbs(command) {
		return nil, errors.New("deploy command must be an absolute path")
	}
	info, err := os.Stat(command)
	if err != nil {
		return nil, fmt.Errorf("stat deploy command: %w", err)
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return nil, errors.New("deploy command must be an executable file")
	}
	if timeout <= 0 || timeout > 24*time.Hour {
		return nil, errors.New("deploy timeout must be between 1ns and 24h")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{store: store, command: command, timeout: timeout, logger: logger}, nil
}

// RunAll processes queued deliveries serially. A failed deployment is recorded
// and removed from the active queue so a later push can deliver a fix. Timeout
// or parent cancellation stops the current run and leaves later events queued.
func (r *Runner) RunAll(parent context.Context) error {
	var firstFailure error
	for {
		event, err := r.store.Claim()
		if err != nil {
			return err
		}
		if event == nil {
			return firstFailure
		}

		startedAt := time.Now().UTC()
		ctx, cancel := context.WithTimeout(parent, r.timeout)
		exitCode, runErr := r.runOne(ctx, *event)
		cancel()
		finishedAt := time.Now().UTC()
		attempt := state.Attempt{
			Event:      *event,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Succeeded:  runErr == nil,
			ExitCode:   exitCode,
		}
		if runErr != nil {
			attempt.Error = runErr.Error()
		}
		if completeErr := r.store.Complete(attempt); completeErr != nil {
			return fmt.Errorf("record deployment result: %w", completeErr)
		}
		if runErr != nil {
			r.logger.Error(
				"deployment_failed",
				"delivery", event.Delivery,
				"sha", event.After,
				"exit_code", exitCode,
				"duration", finishedAt.Sub(startedAt),
				"error", runErr,
			)
			if firstFailure == nil {
				firstFailure = runErr
			}
			if errors.Is(parent.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return firstFailure
			}
			continue
		}
		r.logger.Info(
			"deployment_completed",
			"delivery", event.Delivery,
			"sha", event.After,
			"duration", finishedAt.Sub(startedAt),
		)
	}
}

func (r *Runner) runOne(ctx context.Context, event state.Event) (int, error) {
	r.logger.Info(
		"deployment_started",
		"delivery", event.Delivery,
		"repository", event.Repository,
		"ref", event.Ref,
		"before", event.Before,
		"after", event.After,
	)
	command := exec.CommandContext(
		ctx,
		r.command,
		"--sha", event.After,
		"--delivery", event.Delivery,
		"--repository", event.Repository,
		"--ref", event.Ref,
	)
	command.Env = append(
		os.Environ(),
		"HENUKIT_DEPLOY_SHA="+event.After,
		"HENUKIT_DEPLOY_DELIVERY="+event.Delivery,
		"HENUKIT_DEPLOY_REPOSITORY="+event.Repository,
		"HENUKIT_DEPLOY_REF="+event.Ref,
	)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.WaitDelay = 10 * time.Second

	err := command.Run()
	if err == nil {
		return 0, nil
	}
	if ctx.Err() != nil {
		return -1, fmt.Errorf("deploy command context ended: %w", ctx.Err())
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), fmt.Errorf("deploy command exited with %d", exitError.ExitCode())
	}
	return -1, fmt.Errorf("start deploy command: %w", err)
}
