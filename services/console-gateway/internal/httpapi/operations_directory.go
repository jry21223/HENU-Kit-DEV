package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	accountapi "henukit.dev/console-gateway/internal/accountportfolio"
	noticeapi "henukit.dev/console-gateway/internal/notice"
)

func sameOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || r.Host == "" {
		return false
	}
	if origin == "https://"+r.Host {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return false
	}
	host := r.Host
	if splitHost, _, err := net.SplitHostPort(r.Host); err == nil {
		host = splitHost
	}
	host = strings.Trim(strings.ToLower(strings.TrimRight(host, ".")), "[]")
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return origin == "http://"+r.Host
	}
	return false
}

func (h *Handler) listSources(w http.ResponseWriter, r *http.Request) {
	value, ok := h.authorizeNotice(w, r, "notice.read")
	if !ok {
		return
	}
	if r.URL.RawQuery != "" {
		invalidDirectoryRequest(w, r)
		return
	}
	data, err := h.notice.Sources(noticeapi.WithRequestID(r.Context(), requestID(r)), value.UserID)
	h.writeNoticeResult(w, r, data, err)
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	value, ok := h.readSession(w, r)
	if !ok {
		return
	}
	if err := h.platform.CheckPlatformOperations(r.Context(), value.ExchangeToken); err != nil {
		h.handlePlatformSessionError(w, r, err)
		return
	}
	if r.URL.RawQuery != "" {
		invalidDirectoryRequest(w, r)
		return
	}
	data, err := h.platform.Roles(r.Context(), value.ExchangeToken)
	if err != nil {
		h.handlePlatformSessionError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, data)
}

func (h *Handler) updateInbox(w http.ResponseWriter, r *http.Request) {
	value, ok := h.readSession(w, r)
	if !ok {
		return
	}
	if !sameOriginAllowed(r) {
		writeError(w, r, http.StatusForbidden, "ORIGIN_DENIED", "请求来源无效，请从控制台重新提交")
		return
	}
	if err := h.platform.CheckPlatformOperationsWrite(r.Context(), value.ExchangeToken); err != nil {
		h.handlePlatformSessionError(w, r, err)
		return
	}
	itemID, err := uuid.Parse(chi.URLParam(r, "item_id"))
	if err != nil || r.URL.RawQuery != "" {
		invalidDirectoryRequest(w, r)
		return
	}
	var input struct {
		ExpectedVersion    int64   `json:"expected_version"`
		Status             string  `json:"status"`
		Priority           *string `json:"priority,omitempty"`
		SourceProductCode  string  `json:"source_product_code,omitempty"`
		SourceResourceType string  `json:"source_resource_type,omitempty"`
		SourceResourceID   string  `json:"source_resource_id,omitempty"`
	}
	body, ok := decodeOperationInput(w, r, &input)
	if !ok {
		return
	}
	if input.ExpectedVersion < 1 || !validInboxUpdate(input.Status, input.Priority) {
		invalidDirectoryRequest(w, r)
		return
	}
	data, err := h.platform.UpdateInbox(r.Context(), value.ExchangeToken, itemID.String(), r.Header.Get("Idempotency-Key"), body)
	if err != nil {
		h.handlePlatformSessionError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, data)
}

func validInboxUpdate(status string, priority *string) bool {
	switch status {
	case "open", "in_progress", "blocked", "resolved", "cancelled", "archived":
	default:
		return false
	}
	if priority == nil {
		return true
	}
	switch *priority {
	case "low", "normal", "high", "urgent":
		return true
	default:
		return false
	}
}

func (h *Handler) getAccountPoints(w http.ResponseWriter, r *http.Request) {
	value, ok := h.authorizeAccount(w, r, "account.points.adjust")
	if !ok {
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "user_id"))
	query, valid := accountReadQuery(r.URL.RawQuery, false)
	if err != nil || !valid {
		invalidDirectoryRequest(w, r)
		return
	}
	data, lookupErr := h.account.Points(accountapi.WithRequestID(r.Context(), requestID(r)), value.UserID, userID.String(), query)
	h.writeAccountPointResult(w, r, data, lookupErr)
}

func (h *Handler) listMembershipOrders(w http.ResponseWriter, r *http.Request) {
	value, ok := h.authorizeAccount(w, r, "account.orders.read")
	if !ok {
		return
	}
	query, valid := accountReadQuery(r.URL.RawQuery, true)
	if !valid {
		invalidDirectoryRequest(w, r)
		return
	}
	data, err := h.account.MembershipOrders(accountapi.WithRequestID(r.Context(), requestID(r)), value.UserID, query)
	if err == nil {
		data = h.enrichOrderIdentitiesBestEffort(r.Context(), value.ExchangeToken, data)
	}
	h.writeAccountMembershipResult(w, r, data, err)
}

func (h *Handler) enrichOrderIdentitiesBestEffort(ctx context.Context, exchangeToken string, data json.RawMessage) json.RawMessage {
	var envelope struct {
		Orders []map[string]any `json:"orders"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Orders) == 0 {
		return data
	}
	userIDs := make([]string, 0, len(envelope.Orders))
	seen := make(map[string]struct{}, len(envelope.Orders))
	for _, order := range envelope.Orders {
		if rawID, ok := order["user_id"].(string); ok && rawID != "" {
			if _, exists := seen[rawID]; !exists {
				seen[rawID] = struct{}{}
				userIDs = append(userIDs, rawID)
			}
		}
	}
	if len(userIDs) == 0 {
		return data
	}
	raw, err := h.platform.UserIdentities(ctx, exchangeToken, "account.orders.read", userIDs)
	if err != nil {
		return data
	}
	var result struct {
		Identities []map[string]any `json:"identities"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return data
	}
	identities := make(map[string]map[string]any, len(result.Identities))
	for _, identity := range result.Identities {
		if id, ok := identity["id"].(string); ok {
			identities[id] = identity
		}
	}
	for _, order := range envelope.Orders {
		rawID, ok := order["user_id"].(string)
		if !ok {
			continue
		}
		if identity := identities[rawID]; identity != nil {
			if name, ok := identity["display_name"].(string); ok && strings.TrimSpace(name) != "" {
				order["display_name"] = name
			}
			if email, ok := identity["email"].(string); ok && strings.TrimSpace(email) != "" {
				order["email"] = email
			}
		}
	}
	enriched, err := json.Marshal(envelope)
	if err != nil {
		return data
	}
	return enriched
}

func accountReadQuery(raw string, orders bool) (url.Values, bool) {
	query, err := url.ParseQuery(raw)
	if err != nil {
		return nil, false
	}
	maximum := 50
	if orders {
		maximum = 100
	}
	for name, values := range query {
		if len(values) != 1 || values[0] == "" {
			return nil, false
		}
		switch name {
		case "limit":
			limit, err := strconv.Atoi(values[0])
			if err != nil || limit < 1 || limit > maximum {
				return nil, false
			}
		case "user_id":
			userID, err := uuid.Parse(values[0])
			if !orders || err != nil {
				return nil, false
			}
			query.Set(name, userID.String())
		case "status":
			if !orders {
				return nil, false
			}
			switch values[0] {
			case "all", "created", "pending_payment", "paid", "closed", "refunded":
			default:
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return query, true
}

func invalidDirectoryRequest(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效，请检查后重试")
}
