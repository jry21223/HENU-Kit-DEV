package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	accountapi "henukit.dev/console-gateway/internal/accountportfolio"
	"henukit.dev/console-gateway/internal/businessfacts"
	foodapi "henukit.dev/console-gateway/internal/food"
)

type foodFactsClient interface {
	Facts(context.Context, string) (json.RawMessage, error)
}
type accountTicketFactsClient interface {
	TicketFacts(context.Context, string, string) (json.RawMessage, error)
}

func (h *Handler) getFoodFacts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	actor, ok := h.authorizeFood(w, r, "food.read")
	if !ok {
		return
	}
	if r.URL.RawQuery != "" {
		writeError(w, r, 400, "INVALID_REQUEST", "美食业务概况仅支持全局范围")
		return
	}
	client, ok := h.food.(foodFactsClient)
	if !ok {
		h.unavailable(w, r, errors.New("Food facts are not configured"))
		return
	}
	data, err := client.Facts(foodapi.WithRequestID(ctx, requestID(r)), actor.UserID)
	if err == nil {
		err = businessfacts.ValidateFood(data)
	}
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	writeJSON(w, r, 200, data)
}
func (h *Handler) getAccountTicketFacts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	actor, ok := h.authorizeAccount(w, r, "account.tickets.read")
	if !ok {
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	status := "unresolved"
	valid := err == nil && len(query) <= 1
	for key, values := range query {
		if key != "status" || len(values) != 1 {
			valid = false
		} else {
			status = values[0]
		}
	}
	if !valid || !businessfacts.ValidTicketFilter(status) {
		writeError(w, r, 400, "INVALID_REQUEST", "工单筛选条件无效")
		return
	}
	client, ok := h.account.(accountTicketFactsClient)
	if !ok {
		h.unavailable(w, r, errors.New("Ticket facts are not configured"))
		return
	}
	data, err := client.TicketFacts(accountapi.WithRequestID(ctx, requestID(r)), actor.UserID, status)
	if err == nil {
		err = businessfacts.ValidateTickets(data, status)
	}
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	writeJSON(w, r, 200, data)
}
