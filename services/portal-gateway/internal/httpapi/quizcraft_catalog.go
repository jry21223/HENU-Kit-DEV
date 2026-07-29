package httpapi

import (
	"errors"
	"net/http"

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/practice"
)

// quizCraftCatalogResponse is deliberately local to the dark Portal handoff.
// It is not added to the public OpenAPI contract before the #166 cutover.
// Browser clients receive only the stable identifiers needed to start a
// practice session and the honest published availability state.
type quizCraftCatalogResponse struct {
	Banks     []quizCraftCatalogBank `json:"banks"`
	RequestID string                 `json:"request_id"`
}

type quizCraftCatalogBank struct {
	BankID        string `json:"bank_id"`
	BankVersionID string `json:"bank_version_id"`
	Name          string `json:"name"`
	QuestionCount int    `json:"question_count"`
	Available     bool   `json:"available"`
}

func (h *Handler) getQuizCraftCatalog(w http.ResponseWriter, r *http.Request) {
	if h.quizCraftCatalog == nil {
		writeJSON(w, http.StatusNotFound, contract.ErrorEnvelope{Error: "not found", RequestID: requestIDOf(w, r)})
		return
	}

	// The catalog is public and contains no user-specific result. The existing
	// read-signature contract does not bind an actor header, so never derive a
	// Core actor from a browser session here. Use the explicit anonymous actor
	// for every request rather than presenting an unauthenticated identity as
	// trusted audit data.
	result, err := h.quizCraftCatalog.Banks(r.Context(), requestIDOf(w, r))
	if err != nil {
		h.writeQuizCraftCatalogFailure(w, r, err)
		return
	}

	banks := make([]quizCraftCatalogBank, 0, len(result.Data))
	for _, bank := range result.Data {
		banks = append(banks, quizCraftCatalogBank{
			BankID:        bank.BankID,
			BankVersionID: bank.BankVersionID,
			Name:          bank.Name,
			QuestionCount: bank.QuestionCount,
			// The Core contract contains only published bank versions. A
			// published version is available to start; it is never invented
			// from Portal-owned fallback data.
			Available: true,
		})
	}
	writeJSON(w, http.StatusOK, quizCraftCatalogResponse{
		Banks:     banks,
		RequestID: requestIDOf(w, r),
	})
}

func (h *Handler) writeQuizCraftCatalogFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, practice.ErrInvalidCatalog) {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "quizcraft_catalog_invalid_response", RequestID: requestIDOf(w, r)})
		return
	}
	// Authentication, authorization, and transport failures are deployment
	// facts. Keep them a 503 at the browser boundary without exposing Core
	// diagnostics or replacing them with legacy/mock catalog data.
	writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "quizcraft_catalog_unavailable", RequestID: requestIDOf(w, r)})
}
