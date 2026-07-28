package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/accountportfolio"
	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/platformcore"
	"henukit.dev/portal-gateway/internal/session"
)

// Handler is the Portal Gateway HTTP handler.
type Handler struct {
	sessionCodec       *session.Codec
	platform           *platformcore.Client
	portalAPI          *http.Client
	portalAPIURL       string
	accountPortfolio   *accountportfolio.Client
	redis              *redis.Client
	portalOrigin       string
	platformCoreURL    string
	publicPlatformURL  string
	clientID           string
	redirectURI        string
	localOAuthCookie   string
	localSessionCookie string
	trustedProxies     []*net.IPNet
}

var accountIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

// New creates a Handler from config.
func New(cfg config.Config, rdb *redis.Client) (*Handler, error) {
	codec, err := session.NewCodec(cfg.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("session.NewCodec: %w", err)
	}
	trustedProxies := make([]*net.IPNet, 0, len(cfg.TrustedProxyCIDRs))
	for _, value := range cfg.TrustedProxyCIDRs {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q", value)
		}
		trustedProxies = append(trustedProxies, network)
	}
	var portfolio *accountportfolio.Client
	if strings.TrimSpace(cfg.AccountPortfolioURL) != "" {
		var err error
		portfolio, err = accountportfolio.NewClient(
			cfg.AccountPortfolioURL,
			cfg.AccountPortfolioAuth.ClientID,
			cfg.AccountPortfolioAuth.ClientSecret,
			cfg.AccountPortfolioAuth.KeyID,
		)
		if err != nil {
			return nil, fmt.Errorf("accountportfolio.NewClient: %w", err)
		}
	}
	return &Handler{
		sessionCodec:       codec,
		platform:           platformcore.NewClient(cfg.PlatformCoreURL, cfg.PortalRedirectURI, cfg.PlatformClientID, cfg.PlatformSecret, cfg.PlatformKeyID),
		portalAPI:          &http.Client{Timeout: 10 * time.Second},
		portalAPIURL:       cfg.PortalAPIURL,
		accountPortfolio:   portfolio,
		redis:              rdb,
		portalOrigin:       cfg.PortalOrigin,
		platformCoreURL:    cfg.PlatformCoreURL,
		publicPlatformURL:  firstNonEmpty(cfg.PlatformCorePublicURL, cfg.PlatformCoreURL),
		clientID:           cfg.PlatformClientID,
		redirectURI:        cfg.PortalRedirectURI,
		localOAuthCookie:   firstNonEmpty(cfg.LocalOAuthCookieName, "henukit_portal_oauth_local"),
		localSessionCookie: firstNonEmpty(cfg.LocalSessionCookieName, "henukit_portal_session_local"),
		trustedProxies:     trustedProxies,
	}, nil
}

// Router builds the chi router with all Portal Gateway routes.
func (h *Handler) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(requestID)

	r.Get("/api/v1/healthz", h.healthz)
	r.Get("/api/v1/auth/login", h.login)
	r.Get("/api/v1/auth/callback", h.callback)
	r.Get("/api/v1/session", h.getSession)
	r.Post("/api/v1/session/logout", h.logout)
	r.Get("/api/v1/account/summary", h.accountSummary)
	r.Get("/api/v1/account/points", h.accountPoints)
	r.Get("/api/v1/account/membership", h.accountMembership)
	r.Get("/api/v1/account/notifications", h.accountNotifications)
	r.Post("/api/v1/account/notifications/{notification_id}/read", h.accountNotificationRead)
	r.Get("/api/v1/account/tickets", h.accountTickets)
	r.Post("/api/v1/account/tickets", h.accountTicketCreate)
	r.Get("/api/v1/account/tickets/{ticket_id}", h.accountTicket)
	r.Post("/api/v1/account/tickets/{ticket_id}/follow-ups", h.accountTicketFollowUp)
	r.Get("/api/v1/account/membership-orders", h.accountMembershipOrders)

	// Product data — proxy to portal-api (public, no auth required)
	r.Get("/api/v1/library/*", h.proxyToPortalAPI)
	r.Get("/api/v1/food/*", h.proxyToPortalAPI)
	r.Get("/api/v1/practice/*", h.proxyToPortalAPI)
	r.Get("/api/v1/campus/*", h.proxyToPortalAPI)
	r.Get("/api/v1/notices", h.proxyToPortalAPI)

	return r
}

// --- Middleware ---

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = uuid()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r)
	})
}

// --- Handlers ---

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	returnTo := r.URL.Query().Get("return_to")
	if returnTo == "" || returnTo[0] != '/' {
		returnTo = "/"
	}

	state := randomBytes(32)
	// RFC 7636: code_verifier is a high-entropy string; code_challenge is
	// BASE64URL(SHA256(ASCII(code_verifier))). Hash the same string we store
	// and later send as code_verifier — never the raw pre-encoding bytes.
	verifier := base64.RawURLEncoding.EncodeToString(randomBytes(32))
	browserNonce := randomBytes(32)

	stateHash := sha256Hex(state)
	browserHash := sha256Hex(browserNonce)

	payload, _ := json.Marshal(map[string]string{
		"verifier":  verifier,
		"return_to": returnTo,
	})
	key := fmt.Sprintf("portal:oauth-state:%s:%s", stateHash, browserHash)
	h.redis.Set(r.Context(), key, payload, 5*time.Minute)

	cookies := h.browserCookies(r)
	http.SetCookie(w, &http.Cookie{
		Name:     cookies.oauth,
		Value:    base64.RawURLEncoding.EncodeToString(browserNonce),
		Path:     "/",
		HttpOnly: true,
		Secure:   cookies.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})

	codeChallenge := s256Challenge(verifier)

	redirectURL := fmt.Sprintf(
		"%s/api/v1/oauth/authorize?response_type=code&client_id=%s&redirect_uri=%s&state=%s&code_challenge=%s&code_challenge_method=S256",
		firstNonEmpty(h.publicPlatformURL, h.platformCoreURL), h.clientID, url.QueryEscape(h.redirectURI),
		base64.RawURLEncoding.EncodeToString(state), codeChallenge,
	)

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeError(w, r, http.StatusBadRequest, "missing code or state")
		return
	}

	cookies := h.browserCookies(r)
	cookie, err := r.Cookie(cookies.oauth)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "missing oauth cookie")
		return
	}
	browserNonce, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid oauth cookie")
		return
	}
	stateBytes, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil || len(stateBytes) != 32 {
		writeError(w, r, http.StatusBadRequest, "invalid or expired state")
		return
	}

	stateHash := sha256Hex(stateBytes)
	browserHash := sha256Hex(browserNonce)
	key := fmt.Sprintf("portal:oauth-state:%s:%s", stateHash, browserHash)

	data, err := h.redis.GetDel(r.Context(), key).Bytes()
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid or expired state")
		return
	}

	var stored map[string]string
	if err := json.Unmarshal(data, &stored); err != nil {
		writeError(w, r, http.StatusInternalServerError, "corrupt state")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: cookies.oauth, Value: "", Path: "/",
		HttpOnly: true, Secure: cookies.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0),
	})

	idempotencyKey := hex.EncodeToString(stateBytes[:16])
	result, err := h.platform.ExchangeCode(r.Context(), code, stored["verifier"], idempotencyKey)
	if err != nil {
		category := "exchange_error"
		status := http.StatusInternalServerError
		message := "exchange error"
		if err == platformcore.ErrUnauthorized {
			category = "unauthorized"
			status = http.StatusUnauthorized
			message = "exchange failed"
		}
		// Redacted: never log code, verifier, cookies, email, or secrets.
		log.Printf("portal-gateway oauth exchange failed request_id=%s category=%s", requestIDOf(w, r), category)
		writeError(w, r, status, message)
		return
	}

	encoded, err := h.sessionCodec.Encode(session.Value{
		UserID: result.UserID, DisplayName: result.DisplayName,
		ExchangeToken: result.SessionExchangeToken, ExpiresAt: result.ExpiresAt,
	})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "session encode error")
		return
	}

	maxAge := int(time.Until(result.ExpiresAt).Seconds())
	http.SetCookie(w, &http.Cookie{
		Name: cookies.session, Value: encoded, Path: "/",
		HttpOnly: true, Secure: cookies.secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge,
	})

	returnTo := stored["return_to"]
	if returnTo == "" {
		returnTo = "/"
	}
	http.Redirect(w, r, h.portalOrigin+returnTo, http.StatusFound)
}

func (h *Handler) getSession(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	v, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated"})
		return
	}
	var displayName *string
	if v.DisplayName != "" {
		displayName = &v.DisplayName
	}
	writeJSON(w, http.StatusOK, contract.PortalSession{
		UserID: v.UserID, DisplayName: displayName, ExpiresAt: v.ExpiresAt,
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	cookies := h.browserCookies(r)
	http.SetCookie(w, &http.Cookie{
		Name: cookies.session, Value: "", Path: "/",
		HttpOnly: true, Secure: cookies.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed_out"})
}

type accountRead func(context.Context, string, string) (json.RawMessage, error)
type accountCommand func(context.Context, string, string, string, []byte) (json.RawMessage, error)

type pointsPage struct {
	cursor string
	limit  int
}

func accountPointsPage(r *http.Request) (pointsPage, error) {
	query := r.URL.Query()
	for key := range query {
		if key != "cursor" && key != "limit" {
			return pointsPage{}, errors.New("unknown point page query")
		}
	}
	if len(query["cursor"]) > 1 || len(query["limit"]) > 1 {
		return pointsPage{}, errors.New("duplicate point page query")
	}
	page := pointsPage{limit: 20}
	if values, exists := query["cursor"]; exists {
		page.cursor = values[0]
		if page.cursor == "" || len(page.cursor) > 512 || strings.TrimSpace(page.cursor) != page.cursor {
			return pointsPage{}, errors.New("invalid point cursor")
		}
	}
	if values, exists := query["limit"]; exists {
		if values[0] == "" || strings.TrimSpace(values[0]) != values[0] {
			return pointsPage{}, errors.New("invalid point page limit")
		}
		limit, err := strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > 50 {
			return pointsPage{}, errors.New("invalid point page limit")
		}
		page.limit = limit
	}
	return page, nil
}

func (h *Handler) accountSummary(w http.ResponseWriter, r *http.Request) {
	h.accountRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.accountPortfolio.Summary(ctx, actorUserID, requestID)
	})
}

func (h *Handler) accountPoints(w http.ResponseWriter, r *http.Request) {
	h.accountRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		page, err := accountPointsPage(r)
		if err != nil {
			return nil, accountportfolio.ErrBadRequest
		}
		return h.accountPortfolio.PointsPage(ctx, actorUserID, requestID, page.cursor, page.limit)
	})
}

func (h *Handler) accountMembership(w http.ResponseWriter, r *http.Request) {
	h.accountRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.accountPortfolio.Membership(ctx, actorUserID, requestID)
	})
}

func (h *Handler) accountNotifications(w http.ResponseWriter, r *http.Request) {
	h.accountRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.accountPortfolio.Notifications(ctx, actorUserID, requestID)
	})
}

func (h *Handler) accountTickets(w http.ResponseWriter, r *http.Request) {
	h.accountRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.accountPortfolio.Tickets(ctx, actorUserID, requestID)
	})
}

func (h *Handler) accountTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticket_id")
	h.accountRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.accountPortfolio.Ticket(ctx, actorUserID, requestID, ticketID)
	})
}

func (h *Handler) accountTicketCreate(w http.ResponseWriter, r *http.Request) {
	h.accountCommand(w, r, http.StatusCreated, true, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
		return h.accountPortfolio.CreateTicket(ctx, actorUserID, requestID, idempotencyKey, raw)
	})
}

func (h *Handler) accountTicketFollowUp(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticket_id")
	h.accountCommand(w, r, http.StatusOK, true, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
		return h.accountPortfolio.FollowUp(ctx, actorUserID, requestID, ticketID, idempotencyKey, raw)
	})
}

func (h *Handler) accountNotificationRead(w http.ResponseWriter, r *http.Request) {
	notificationID := chi.URLParam(r, "notification_id")
	h.accountCommand(w, r, http.StatusOK, false, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
		return h.accountPortfolio.MarkNotificationRead(ctx, actorUserID, requestID, notificationID, idempotencyKey)
	})
}

func (h *Handler) accountMembershipOrders(w http.ResponseWriter, r *http.Request) {
	h.accountRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.accountPortfolio.MembershipOrders(ctx, actorUserID, requestID)
	})
}

func (h *Handler) accountRead(w http.ResponseWriter, r *http.Request, read accountRead) {
	// Account facts are private and must not be stored by a browser or
	// intermediary while the Portal Session remains active.
	setPrivateResponseHeaders(w)
	value, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", RequestID: requestIDOf(w, r)})
		return
	}
	if h.accountPortfolio == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "account_portfolio_unavailable", RequestID: requestIDOf(w, r)})
		return
	}
	data, err := read(r.Context(), value.UserID, requestIDOf(w, r))
	if err != nil {
		h.writeAccountFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}{Data: data, RequestID: requestIDOf(w, r)})
}

func (h *Handler) accountCommand(w http.ResponseWriter, r *http.Request, successStatus int, bodyRequired bool, command accountCommand) {
	setPrivateResponseHeaders(w)
	value, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", RequestID: requestIDOf(w, r)})
		return
	}
	if h.accountPortfolio == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "account_portfolio_unavailable", RequestID: requestIDOf(w, r)})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !validAccountIdempotencyKey(idempotencyKey) {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "account_idempotency_key_invalid", RequestID: requestIDOf(w, r)})
		return
	}
	var raw []byte
	if bodyRequired {
		raw, err = readAccountCommandBody(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "account_command_invalid", RequestID: requestIDOf(w, r)})
			return
		}
	}
	data, err := command(r.Context(), value.UserID, requestIDOf(w, r), idempotencyKey, raw)
	if err != nil {
		h.writeAccountFailure(w, r, err)
		return
	}
	writeJSON(w, successStatus, struct {
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}{Data: data, RequestID: requestIDOf(w, r)})
}

func (h *Handler) writeAccountFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, accountportfolio.ErrBadRequest):
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "account_command_invalid", RequestID: requestIDOf(w, r)})
	case errors.Is(err, accountportfolio.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", RequestID: requestIDOf(w, r)})
	case errors.Is(err, accountportfolio.ErrNotFound):
		writeJSON(w, http.StatusNotFound, contract.ErrorEnvelope{Error: "account_resource_not_found", RequestID: requestIDOf(w, r)})
	case errors.Is(err, accountportfolio.ErrConflict):
		writeJSON(w, http.StatusConflict, contract.ErrorEnvelope{Error: "account_command_conflict", RequestID: requestIDOf(w, r)})
	case errors.Is(err, accountportfolio.ErrInvalid):
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "account_portfolio_invalid_response", RequestID: requestIDOf(w, r)})
	default:
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "account_portfolio_unavailable", RequestID: requestIDOf(w, r)})
	}
}

func readAccountCommandBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("request body is required")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10+1))
	if err != nil || len(raw) == 0 || len(raw) > 64<<10 {
		return nil, fmt.Errorf("request body is invalid")
	}
	return raw, nil
}

func validAccountIdempotencyKey(value string) bool {
	return len(value) >= 8 && len(value) <= 200 && accountIdempotencyKeyPattern.MatchString(value)
}

// --- Proxy to portal-api ---

func (h *Handler) proxyToPortalAPI(w http.ResponseWriter, r *http.Request) {
	targetURL := h.portalAPIURL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "proxy_error"})
		return
	}
	req.Header.Set("X-Request-Id", w.Header().Get("X-Request-Id"))

	resp, err := h.portalAPI.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "portal_api_unavailable"})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// --- Helpers ---

func (h *Handler) readSession(r *http.Request) (session.Value, error) {
	cookie, err := r.Cookie(h.browserCookies(r).session)
	if err != nil {
		return session.Value{}, err
	}
	v, err := h.sessionCodec.Decode(cookie.Value)
	if err != nil {
		return session.Value{}, err
	}
	if time.Now().After(v.ExpiresAt) {
		return session.Value{}, fmt.Errorf("session expired")
	}
	return v, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func setPrivateResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	writeJSON(w, status, contract.ErrorEnvelope{
		Error:     message,
		RequestID: requestIDOf(w, r),
	})
}

func requestIDOf(w http.ResponseWriter, r *http.Request) string {
	if id := strings.TrimSpace(w.Header().Get("X-Request-Id")); id != "" {
		return id
	}
	if id := strings.TrimSpace(r.Header.Get("X-Request-Id")); id != "" {
		return id
	}
	return ""
}

// s256Challenge is BASE64URL(SHA256(ASCII(code_verifier))) per RFC 7636 / Platform Core.
func s256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func uuid() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

type browserCookieProfile struct {
	oauth   string
	session string
	secure  bool
}

func (h *Handler) browserCookies(r *http.Request) browserCookieProfile {
	if h.externallyHTTPS(r) {
		return browserCookieProfile{
			oauth: "__Host-henukit_portal_oauth", session: "__Host-henukit_portal_session", secure: true,
		}
	}
	return browserCookieProfile{
		oauth: h.localOAuthCookie, session: h.localSessionCookie,
	}
}

func (h *Handler) externallyHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	peer := net.ParseIP(remoteIP(r.RemoteAddr))
	return peer != nil && h.isTrustedProxy(peer) &&
		strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (h *Handler) isTrustedProxy(address net.IP) bool {
	for _, network := range h.trustedProxies {
		if network.Contains(address) {
			return true
		}
	}
	return false
}

func remoteIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil && host != "" {
		return host
	}
	return remoteAddress
}
