package library

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type commandInput struct {
	Kind            string          `json:"kind"`
	ResourceID      string          `json:"resource_id"`
	ExpectedVersion string          `json:"expected_version"`
	Payload         json.RawMessage `json:"payload"`
}

// ADR-0037 removed the Study Legacy adapter. The legacy admin write commands
// (course/material/submission/correction CRUD through services/api) and the
// operation ledger that recorded their outcomes are gone with it. These
// routes stay registered so callers get an honest 503 instead of 404; the
// catalog (T1) migration will replace them with library-owned commands.
func (h *service) command(w http.ResponseWriter, r *http.Request) {
	var input commandInput
	if _, ok := decode(w, r, &input); !ok {
		return
	}
	if _, ok := h.require(w, r, commandPermission(input.Kind)); !ok {
		return
	}
	writeError(w, r, http.StatusServiceUnavailable, "LIBRARY_COMMANDS_UNAVAILABLE", "legacy Study API adapter removed (ADR-0037); catalog migration (T1) pending")
}

func (h *service) operationStatus(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	operation := chi.URLParam(r, "operation")
	if _, ok := h.require(w, r, commandPermission(operation)); !ok {
		return
	}
	writeError(w, r, http.StatusServiceUnavailable, "LIBRARY_COMMANDS_UNAVAILABLE", "legacy Study API adapter removed (ADR-0037); catalog migration (T1) pending")
}

func commandPermission(kind string) string {
	if strings.HasPrefix(kind, "submission_") || strings.HasPrefix(kind, "correction_") {
		return "library.review"
	}
	return "library.manage"
}
