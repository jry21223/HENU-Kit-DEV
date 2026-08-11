package httpapi

import (
	"errors"
	"net/http"

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/practice"
)

// getQuizCraftCatalog maps the Core owner contract into the generated Portal
// Gateway response: only the identifiers and chapter labels needed for setup,
// plus the honest published availability fact, cross the browser boundary.
func (h *Handler) getQuizCraftCatalog(w http.ResponseWriter, r *http.Request) {
	if h.quizCraftCatalog == nil {
		writeJSON(w, http.StatusNotFound, contract.ErrorEnvelope{Error: "not found", Message: "内容不存在或已下架", RequestID: requestIDOf(w, r)})
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

	banks := make([]contract.QuizCraftCatalogBank, 0, len(result.Data))
	for _, bank := range result.Data {
		chapters := make([]contract.QuizCraftCatalogChapter, 0, len(bank.Chapters))
		for _, chapter := range bank.Chapters {
			chapters = append(chapters, contract.QuizCraftCatalogChapter{ID: chapter.ID, Name: chapter.Name})
		}
		banks = append(banks, contract.QuizCraftCatalogBank{
			BankID:        bank.BankID,
			BankVersionID: bank.BankVersionID,
			Name:          bank.Name,
			QuestionCount: bank.QuestionCount,
			// The Core contract contains only published bank versions. A
			// published version is available to start; it is never invented
			// from Portal-owned fallback data.
			Available: true,
			Chapters:  chapters,
		})
	}
	writeJSON(w, http.StatusOK, contract.QuizCraftCatalogResponse{
		Banks:     banks,
		RequestID: requestIDOf(w, r),
	})
}

func (h *Handler) writeQuizCraftCatalogFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, practice.ErrInvalidCatalog) {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "quizcraft_catalog_invalid_response", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
		return
	}
	// Authentication, authorization, and transport failures are deployment
	// facts. Keep them a 503 at the browser boundary without exposing Core
	// diagnostics or replacing them with legacy/mock catalog data.
	writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "quizcraft_catalog_unavailable", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
}
