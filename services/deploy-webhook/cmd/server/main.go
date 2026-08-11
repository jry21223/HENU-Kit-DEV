package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"henukit.dev/deploy-webhook/internal/runner"
	"henukit.dev/deploy-webhook/internal/state"
	"henukit.dev/deploy-webhook/internal/webhook"
)

const usage = `usage: henukit-deploy-webhook [serve|run|retry <sha>]

serve        receive and persist verified GitHub webhook deliveries
run          process all persisted deliveries with the fixed deploy command
retry <sha>  requeue the newest failed delivery for an approved full SHA
`

type commonConfig struct {
	StateDir  string
	MaxQueue  int
	Retention time.Duration
	QueueMode state.QueueMode
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	var err error
	switch command {
	case "serve":
		err = serve(logger)
	case "run":
		err = runPending(logger)
	case "retry":
		if len(os.Args) != 3 {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		err = retryFailed(logger, os.Args[2])
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usage)
		return
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		logger.Error("deploy_webhook_exit", "command", command, "error", err)
		os.Exit(1)
	}
}

func serve(logger *slog.Logger) error {
	common, err := loadCommonConfig()
	if err != nil {
		return err
	}
	store, err := state.NewWithQueueMode(common.StateDir, common.MaxQueue, common.Retention, common.QueueMode)
	if err != nil {
		return err
	}
	secretPath, err := requiredEnv("HENUKIT_WEBHOOK_SECRET_FILE")
	if err != nil {
		return err
	}
	secret, err := loadSecret(secretPath)
	if err != nil {
		return err
	}
	maxBody, err := int64Env("HENUKIT_WEBHOOK_MAX_BODY_BYTES", 1024*1024)
	if err != nil {
		return err
	}
	handler, err := webhook.New(webhook.Config{
		Path:         envOr("HENUKIT_WEBHOOK_PATH", "/webhooks/github"),
		Repository:   envOr("HENUKIT_WEBHOOK_REPOSITORY", "jry21223/HENU-Kit-DEV"),
		Branch:       envOr("HENUKIT_WEBHOOK_BRANCH", "main"),
		Secret:       secret,
		MaxBodyBytes: maxBody,
	}, store, logger)
	if err != nil {
		return err
	}
	address := envOr("HENUKIT_WEBHOOK_LISTEN_ADDR", "127.0.0.1:10087")
	if err := validateLoopbackAddress(address); err != nil {
		return err
	}
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}

	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serveError := make(chan error, 1)
	go func() {
		logger.Info(
			"deploy_webhook_listen",
			"address", address,
			"path", envOr("HENUKIT_WEBHOOK_PATH", "/webhooks/github"),
			"repository", envOr("HENUKIT_WEBHOOK_REPOSITORY", "jry21223/HENU-Kit-DEV"),
			"branch", envOr("HENUKIT_WEBHOOK_BRANCH", "main"),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveError <- err
		}
		close(serveError)
	}()

	select {
	case err := <-serveError:
		if err != nil {
			return err
		}
		return nil
	case <-shutdown.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

func runPending(logger *slog.Logger) error {
	common, err := loadCommonConfig()
	if err != nil {
		return err
	}
	store, err := state.NewWithQueueMode(common.StateDir, common.MaxQueue, common.Retention, common.QueueMode)
	if err != nil {
		return err
	}
	if err := store.RecoverInterrupted(); err != nil {
		return fmt.Errorf("recover interrupted deployment: %w", err)
	}
	timeout, err := durationEnv("HENUKIT_DEPLOY_TIMEOUT", 45*time.Minute)
	if err != nil {
		return err
	}
	deploymentRunner, err := runner.New(
		store,
		envOr("HENUKIT_DEPLOY_COMMAND", "/usr/local/libexec/henukit/henukit-deploy"),
		timeout,
		logger,
	)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return deploymentRunner.RunAll(ctx)
}

func retryFailed(logger *slog.Logger, sha string) error {
	common, err := loadCommonConfig()
	if err != nil {
		return err
	}
	store, err := state.NewWithQueueMode(common.StateDir, common.MaxQueue, common.Retention, common.QueueMode)
	if err != nil {
		return err
	}
	result, err := store.RetryFailedSHA(strings.ToLower(strings.TrimSpace(sha)))
	if err != nil {
		return err
	}
	logger.Info("deployment_retry_queued", "sha", strings.ToLower(strings.TrimSpace(sha)), "queued", result.Queued, "duplicate", result.Duplicate)
	return nil
}

func loadCommonConfig() (commonConfig, error) {
	maxQueue, err := intEnv("HENUKIT_WEBHOOK_MAX_QUEUE", 100)
	if err != nil {
		return commonConfig{}, err
	}
	retention, err := durationEnv("HENUKIT_WEBHOOK_PROCESSED_RETENTION", 30*24*time.Hour)
	if err != nil {
		return commonConfig{}, err
	}
	queueMode := state.QueueMode(strings.ToLower(envOr("HENUKIT_WEBHOOK_QUEUE_MODE", string(state.QueueModeFIFO))))
	if queueMode != state.QueueModeFIFO && queueMode != state.QueueModeLatest {
		return commonConfig{}, fmt.Errorf("HENUKIT_WEBHOOK_QUEUE_MODE must be %q or %q", state.QueueModeFIFO, state.QueueModeLatest)
	}
	return commonConfig{
		StateDir:  envOr("HENUKIT_WEBHOOK_STATE_DIR", "/var/lib/henukit-deploy-webhook"),
		MaxQueue:  maxQueue,
		Retention: retention,
		QueueMode: queueMode,
	}, nil
}

func loadSecret(path string) ([]byte, error) {
	linkInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("lstat webhook secret file: %w", err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("webhook secret path must not be a symbolic link")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat webhook secret file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("webhook secret path must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("webhook secret file must not be readable or writable by group/other")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read webhook secret: %w", err)
	}
	value = bytes.TrimRight(value, "\r\n")
	if len(value) < 32 {
		return nil, errors.New("webhook secret must contain at least 32 bytes")
	}
	return value, nil
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func intEnv(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func int64Env(name string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func validateLoopbackAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse HENUKIT_WEBHOOK_LISTEN_ADDR: %w", err)
	}
	if port == "" {
		return errors.New("webhook listen port is required")
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("webhook receiver must listen on a loopback address")
	}
	return nil
}
