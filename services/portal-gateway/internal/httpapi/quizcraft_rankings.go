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
	if h.quizCraft == nil {
		// The route is registered unconditionally (ADR-0036); the V2 read
		// client is not, so this public read fails closed exactly like the
		// catalog when PORTAL_ENABLE_QUIZCRAFT_V2_READS is off.
		writeJSON(w, http.StatusNotFound, contract.ErrorEnvelope{Error: "not found", Message: "内容不存在或已下架", RequestID: requestIDOf(w, r)})
		return
	}
	period, ok := rankingPeriodOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid_ranking_period", Message: "排行榜暂时加载不出来，请稍后再试", RequestID: requestIDOf(w, r)})
		return
	}
	result, err := h.quizCraft.OverallRanking(r.Context(), practice.AnonymousCatalogActor, requestIDOf(w, r), period)
	if err != nil {
		h.writeQuizCraftRankingFailure(w, r, err)
		return
	}
	h.writeQuizCraftRanking(w, r, result)
}

func (h *Handler) getQuizCraftBankRanking(w http.ResponseWriter, r *http.Request) {
	if h.quizCraft == nil {
		writeJSON(w, http.StatusNotFound, contract.ErrorEnvelope{Error: "not found", Message: "内容不存在或已下架", RequestID: requestIDOf(w, r)})
		return
	}
	bankID := strings.TrimSpace(chi.URLParam(r, "bank_id"))
	if !validRankingBankID(bankID) {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid_bank_id", Message: "排行榜暂时加载不出来，请稍后再试", RequestID: requestIDOf(w, r)})
		return
	}
	period, ok := rankingPeriodOf(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid_ranking_period", Message: "排行榜暂时加载不出来，请稍后再试", RequestID: requestIDOf(w, r)})
		return
	}
	result, err := h.quizCraft.BankRanking(r.Context(), practice.AnonymousCatalogActor, requestIDOf(w, r), bankID, period)
	if err != nil {
		h.writeQuizCraftRankingFailure(w, r, err)
		return
	}
	h.writeQuizCraftRanking(w, r, result)
}

// quizCraftRankingResponse is the browser-facing ranking payload. It mirrors
// the internal practice.RankingEnvelope but rebuilds every entry without a
// user_id key: the external contract stays {rank, nickname, system_avatar,
// correct_answer_count} and the internal identity never reaches the browser
// (ADR-0036 privacy contract, ADR-0038).
type quizCraftRankingResponse struct {
	RequestID string               `json:"request_id"`
	Data      quizCraftRankingPage `json:"data"`
}

type quizCraftRankingPage struct {
	Scope   string                  `json:"scope"`
	BankID  string                  `json:"bank_id,omitempty"`
	Period  practice.RankingPeriod  `json:"period"`
	Metric  string                  `json:"metric"`
	Entries []quizCraftRankingEntry `json:"entries"`
}

type quizCraftRankingEntry struct {
	Rank               int64  `json:"rank"`
	Nickname           string `json:"nickname"`
	SystemAvatar       string `json:"system_avatar"`
	CorrectAnswerCount int64  `json:"correct_answer_count"`
}

// writeQuizCraftRanking synthesizes the external ranking response from the
// internal contract: it resolves display names through the Platform Core
// batch boundary (cached, singleflight, degraded to 游客x on failure), derives
// nickname/system_avatar from the identity key (user_id for signed-in
// learners, guest_key for guests — ADR-0038), and strips user_id and guest_key
// before the payload is written.
func (h *Handler) writeQuizCraftRanking(w http.ResponseWriter, r *http.Request, result practice.RankingEnvelope) {
	userIDs := make([]string, 0, len(result.Data.Entries))
	for _, entry := range result.Data.Entries {
		if entry.UserID != nil {
			userIDs = append(userIDs, *entry.UserID)
		}
	}
	var names map[string]string
	if h.displayNames != nil {
		names = h.displayNames.Resolve(r.Context(), requestIDOf(w, r), userIDs)
	} else {
		names = map[string]string{}
	}
	entries := make([]quizCraftRankingEntry, 0, len(result.Data.Entries))
	for _, entry := range result.Data.Entries {
		identityKey := ""
		switch {
		case entry.UserID != nil:
			identityKey = *entry.UserID
		case entry.GuestKey != nil:
			identityKey = *entry.GuestKey
		}
		entries = append(entries, quizCraftRankingEntry{
			Rank:               entry.Rank,
			Nickname:           practice.RankingNickname(identityKey, names[identityKey]),
			SystemAvatar:       practice.RankingSystemAvatar(identityKey),
			CorrectAnswerCount: entry.CorrectAnswerCount,
		})
	}
	writeJSON(w, http.StatusOK, quizCraftRankingResponse{
		RequestID: result.RequestID,
		Data: quizCraftRankingPage{
			Scope: result.Data.Scope, BankID: result.Data.BankID, Period: result.Data.Period,
			Metric: result.Data.Metric, Entries: entries,
		},
	})
}

func (h *Handler) writeQuizCraftRankingFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, practice.ErrInvalidRanking) {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "quizcraft_ranking_invalid_response", Message: "排行榜暂时加载不出来，请稍后再试", RequestID: requestIDOf(w, r)})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "quizcraft_ranking_unavailable", Message: "排行榜暂时加载不出来，请稍后再试", RequestID: requestIDOf(w, r)})
}
