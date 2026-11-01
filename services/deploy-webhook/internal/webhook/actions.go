package webhook

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"henukit.dev/deploy-webhook/internal/state"
)

const releaseWorkflowPath = ".github/workflows/deploy-henukit.yml"

var actionsSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// NewActions receives notifications only. Its store never executes deployment
// commands or grants approval; the watcher independently queries GitHub.
func NewActions(config Config, notifications Queue, logger *slog.Logger) (*Handler, error) {
	handler, err := New(config, notifications, logger)
	if err != nil {
		return nil, err
	}
	handler.actions = true
	return handler, nil
}

func (h *Handler) handleActions(writer http.ResponseWriter, eventType, delivery string, body []byte) {
	if eventType != "workflow_run" {
		writeJSON(writer, http.StatusAccepted, map[string]any{"accepted": true, "ignored": "unsupported event"})
		return
	}
	var payload struct {
		Action     string `json:"action"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Workflow struct {
			Path string `json:"path"`
		} `json:"workflow"`
		Run struct {
			ID         int64  `json:"id"`
			Attempt    int    `json:"run_attempt"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			Event      string `json:"event"`
			Branch     string `json:"head_branch"`
			SHA        string `json:"head_sha"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		} `json:"workflow_run"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": "invalid JSON payload"})
		return
	}
	if payload.Action != "completed" || payload.Run.Status != "completed" || payload.Run.Conclusion != "success" ||
		payload.Run.Event != "push" || payload.Run.Branch != h.config.Branch ||
		payload.Repository.FullName != h.config.Repository || payload.Run.Repository.FullName != h.config.Repository ||
		payload.Workflow.Path != releaseWorkflowPath {
		h.logger.Info("github_actions_ignored", "delivery", delivery, "reason", "release_workflow_mismatch")
		writeJSON(writer, http.StatusAccepted, map[string]any{"accepted": true, "ignored": "release workflow mismatch"})
		return
	}
	if !actionsSHA.MatchString(payload.Run.SHA) || allZero(payload.Run.SHA) || payload.Run.ID <= 0 || payload.Run.Attempt <= 0 {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": "invalid workflow run identity"})
		return
	}
	result, err := h.queue.Enqueue(state.Event{
		Delivery: delivery, Repository: payload.Repository.FullName,
		Ref: "refs/heads/" + h.config.Branch, After: strings.ToLower(payload.Run.SHA), ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		h.logger.Error("github_actions_notification_failed", "delivery", delivery, "error", err)
		writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"error": "notification state unavailable"})
		return
	}
	h.logger.Info("github_actions_notified", "delivery", delivery, "after", strings.ToLower(payload.Run.SHA),
		"run_id", payload.Run.ID, "run_attempt", payload.Run.Attempt, "notified", result.Queued, "duplicate", result.Duplicate)
	writeJSON(writer, http.StatusAccepted, map[string]any{
		"accepted": true, "notified": result.Queued, "duplicate": result.Duplicate, "delivery": delivery,
	})
}
