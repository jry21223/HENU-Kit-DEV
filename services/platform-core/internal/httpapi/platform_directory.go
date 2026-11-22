package httpapi

import (
	"crypto/sha256"
	"net/http"

	"github.com/go-chi/chi/v5"
	"henukit.dev/platform-core/internal/contract"
	"henukit.dev/platform-core/internal/operationsinbox"
)

func (h *Handler) listPlatformOperationRoles(w http.ResponseWriter, r *http.Request) {
	decision, err := h.authorizeInbox(r, nil, "platform.operations.read", "platform", "", "", "")
	if err != nil {
		h.writeFlowError(w, r, err)
		return
	}
	roles, err := h.platformOps.ListRoles(r.Context())
	if err != nil {
		h.writePlatformOperationError(w, r, err)
		return
	}
	auditFrom(r.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(w, r, http.StatusOK, map[string]any{"roles": roles})
}

func (h *Handler) updatePlatformInboxItem(w http.ResponseWriter, r *http.Request) {
	raw, body, ok := decodeInboxBody[struct {
		ExpectedVersion int64 `json:"expected_version"`
		Status string `json:"status"`
		Priority *string `json:"priority,omitempty"`
		SourceProductCode string `json:"source_product_code,omitempty"`
		SourceResourceType string `json:"source_resource_type,omitempty"`
		SourceResourceID string `json:"source_resource_id,omitempty"`
	}](w, r)
	if !ok || body.Status == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Operations Inbox request is invalid")
		return
	}
	decision, err := h.authorizeInbox(r, raw, "platform.operations.write", "platform", "", "", "")
	if err != nil {
		h.writeFlowError(w, r, err)
		return
	}
	itemID := chi.URLParam(r, "item_id")
	status := body.Status
	if status == "archived" {
		status = "cancelled"
	}
	hash := sha256.Sum256(append([]byte("platform-inbox\x00"+itemID+"\x00"), raw...))
	item, err := h.inbox.Update(r.Context(), operationsinbox.UpdateInput{
		ServiceID: r.Header.Get(contract.ServiceIDHeader), ItemID: itemID, ActorUserID: decision.ActorUserID,
		RequestID: requestIDFrom(r.Context()), IdempotencyKey: r.Header.Get(contract.IdempotencyKeyHeader),
		RequestHash: hash[:], ExpectedVersion: body.ExpectedVersion, Status: &status, Priority: body.Priority,
	})
	if err != nil {
		h.writeInboxError(w, r, err)
		return
	}
	auditFrom(r.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(w, r, http.StatusOK, inboxContractItem(item))
}
