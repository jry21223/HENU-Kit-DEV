package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"henukit.dev/deploy-webhook/internal/state"
)

var (
	deliveryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,127}$`)
	shaPattern      = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)
)

type Queue interface {
	Enqueue(event state.Event) (state.EnqueueResult, error)
	Snapshot() (state.Snapshot, error)
}

type Config struct {
	Path         string
	Repository   string
	Branch       string
	Secret       []byte
	MaxBodyBytes int64
}

type Handler struct {
	config Config
	queue  Queue
	logger *slog.Logger
}

func New(config Config, queue Queue, logger *slog.Logger) (*Handler, error) {
	config.Path = strings.TrimSpace(config.Path)
	config.Repository = strings.TrimSpace(config.Repository)
	config.Branch = strings.TrimSpace(config.Branch)
	if config.Path == "" || !strings.HasPrefix(config.Path, "/") {
		return nil, errors.New("webhook path must begin with /")
	}
	if config.Repository == "" || strings.Count(config.Repository, "/") != 1 {
		return nil, errors.New("repository must be in owner/name form")
	}
	if config.Branch == "" || strings.ContainsAny(config.Branch, "\r\n") {
		return nil, errors.New("branch is required")
	}
	if len(config.Secret) < 32 {
		return nil, errors.New("webhook secret must contain at least 32 bytes")
	}
	if config.MaxBodyBytes <= 0 || config.MaxBodyBytes > 25*1024*1024 {
		return nil, errors.New("maximum body size must be between 1 byte and 25 MiB")
	}
	if queue == nil {
		return nil, errors.New("queue is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{config: config, queue: queue, logger: logger}, nil
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")

	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/healthz":
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true})
		return
	case request.Method == http.MethodGet && request.URL.Path == "/readyz":
		if _, err := h.queue.Snapshot(); err != nil {
			h.logger.Error("webhook_state_unavailable", "error", err)
			writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"ready": false})
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ready": true})
		return
	case request.Method == http.MethodGet && request.URL.Path == "/statusz":
		snapshot, err := h.queue.Snapshot()
		if err != nil {
			h.logger.Error("webhook_status_unavailable", "error", err)
			writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"error": "status unavailable"})
			return
		}
		writeJSON(writer, http.StatusOK, snapshot)
		return
	case request.URL.Path != h.config.Path:
		writeJSON(writer, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	case request.Method != http.MethodPost:
		writer.Header().Set("Allow", http.MethodPost)
		writeJSON(writer, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(writer, http.StatusUnsupportedMediaType, map[string]any{"error": "application/json is required"})
		return
	}

	request.Body = http.MaxBytesReader(writer, request.Body, h.config.MaxBodyBytes)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeJSON(writer, http.StatusRequestEntityTooLarge, map[string]any{"error": "payload too large"})
			return
		}
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": "unable to read payload"})
		return
	}
	if len(body) == 0 {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": "payload is required"})
		return
	}

	delivery := strings.TrimSpace(request.Header.Get("X-GitHub-Delivery"))
	if !deliveryPattern.MatchString(delivery) {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": "invalid delivery id"})
		return
	}
	if !verifySignature(h.config.Secret, body, request.Header.Get("X-Hub-Signature-256")) {
		h.logger.Warn("github_webhook_rejected", "delivery", delivery, "reason", "invalid_signature")
		writeJSON(writer, http.StatusUnauthorized, map[string]any{"error": "invalid signature"})
		return
	}

	eventType := strings.TrimSpace(request.Header.Get("X-GitHub-Event"))
	if eventType == "ping" {
		h.logger.Info("github_webhook_ping", "delivery", delivery)
		writeJSON(writer, http.StatusOK, map[string]any{"accepted": true, "event": "ping"})
		return
	}
	if eventType != "push" {
		h.logger.Info("github_webhook_ignored", "delivery", delivery, "event", eventType, "reason", "unsupported_event")
		writeJSON(writer, http.StatusAccepted, map[string]any{"accepted": true, "ignored": "unsupported event"})
		return
	}

	var payload struct {
		Ref        string `json:"ref"`
		Before     string `json:"before"`
		After      string `json:"after"`
		Deleted    bool   `json:"deleted"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": "invalid JSON payload"})
		return
	}

	expectedRef := "refs/heads/" + h.config.Branch
	if payload.Repository.FullName != h.config.Repository || payload.Ref != expectedRef {
		h.logger.Info(
			"github_webhook_ignored",
			"delivery", delivery,
			"event", eventType,
			"repository", payload.Repository.FullName,
			"ref", payload.Ref,
			"reason", "repository_or_branch_mismatch",
		)
		writeJSON(writer, http.StatusAccepted, map[string]any{"accepted": true, "ignored": "repository or branch mismatch"})
		return
	}
	if payload.Deleted || allZero(payload.After) {
		h.logger.Info("github_webhook_ignored", "delivery", delivery, "reason", "deleted_ref")
		writeJSON(writer, http.StatusAccepted, map[string]any{"accepted": true, "ignored": "deleted ref"})
		return
	}
	if !shaPattern.MatchString(payload.After) || (payload.Before != "" && !allZero(payload.Before) && !shaPattern.MatchString(payload.Before)) {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": "invalid commit SHA"})
		return
	}

	queued, err := h.queue.Enqueue(state.Event{
		Delivery:   delivery,
		Repository: payload.Repository.FullName,
		Ref:        payload.Ref,
		Before:     strings.ToLower(payload.Before),
		After:      strings.ToLower(payload.After),
		ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, state.ErrQueueFull) {
			status = http.StatusServiceUnavailable
		}
		h.logger.Error("github_webhook_enqueue_failed", "delivery", delivery, "error", err)
		writeJSON(writer, status, map[string]any{"error": "deployment queue unavailable"})
		return
	}

	h.logger.Info(
		"github_webhook_accepted",
		"delivery", delivery,
		"repository", payload.Repository.FullName,
		"ref", payload.Ref,
		"after", strings.ToLower(payload.After),
		"queued", queued.Queued,
		"duplicate", queued.Duplicate,
		"coalesced", queued.Coalesced,
	)
	writeJSON(writer, http.StatusAccepted, map[string]any{
		"accepted":  true,
		"queued":    queued.Queued,
		"duplicate": queued.Duplicate,
		"coalesced": queued.Coalesced,
		"delivery":  delivery,
	})
}

func verifySignature(secret, body []byte, header string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	received, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil || len(received) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hmac.Equal(received, mac.Sum(nil))
}

func allZero(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character != '0' {
			return false
		}
	}
	return true
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}
