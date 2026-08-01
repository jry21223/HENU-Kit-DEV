package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/practice"
)

func rankingPeriodOf(r *http.Request) (practice.RankingPeriod, bool) {
	period := practice.RankingPeriod(strings.TrimSpace(r.URL.Query().Get("period")))
	switch period {
	case "", practice.RankingPeriodWeekly:
		return practice.RankingPeriodWeekly, true
	case practice.RankingPeriodLifetime:
		return practice.RankingPeriodLifetime, true
	default:
		return "", false
	}
}

func validRankingBankID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func (h *Handler) getQuizCraftOverallRanking(w http.ResponseWriter, r *http.Request) {
	period, ok := rankingPeriodOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid_ranking_period", RequestID: requestIDOf(w, r)})
		return
	}
	result, err := h.quizCraft.OverallRanking(r.Context(), practice.AnonymousCatalogActor, requestIDOf(w, r), period)
	if err != nil {
		h.writeQuizCraftRankingFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getQuizCraftBankRanking(w http.ResponseWriter, r *http.Request) {
	bankID := strings.TrimSpace(chi.URLParam(r, "bank_id"))
	if !validRankingBankID(bankID) {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid_bank_id", RequestID: requestIDOf(w, r)})
		return
	}
	period, ok := rankingPeriodOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid_ranking_period", RequestID: requestIDOf(w, r)})
		return
	}
	result, err := h.quizCraft.BankRanking(r.Context(), practice.AnonymousCatalogActor, requestIDOf(w, r), bankID, period)
	if err != nil {
		h.writeQuizCraftRankingFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) writeQuizCraftRankingFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, practice.ErrInvalidRanking) {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "quizcraft_ranking_invalid_response", RequestID: requestIDOf(w, r)})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "quizcraft_ranking_unavailable", RequestID: requestIDOf(w, r)})
}
