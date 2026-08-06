package httpapi

import (
	"bytes"
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
	"henukit.dev/portal-gateway/internal/notice"
	"henukit.dev/portal-gateway/internal/platformcore"
	"henukit.dev/portal-gateway/internal/practice"
	"henukit.dev/portal-gateway/internal/session"
)

// Handler is the Portal Gateway HTTP handler.
type Handler struct {
	sessionCodec       *session.Codec
	platform           *platformcore.Client
	quizCraft          *practice.Client
	portalAPI          *http.Client
	portalAPIURL       string
	accountPortfolio   *accountportfolio.Client
	noticeClient       *notice.Client
	practiceCommands   *practice.CommandClient
	quizCraftCatalog   *practice.Client
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

var (
	accountIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
	portalRequestIDPattern       = regexp.MustCompile(`^req_[A-Za-z0-9_-]{1,116}$`)
)

const quizCraftCatalogPath = "/api/v1/practice/catalog"

// noticeReadPermission is the Portal-scoped permission the Gateway verifies
// with Platform Core before any signed Notice owner read.
const noticeReadPermission = "portal.notice.read"

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
	var noticeClient *notice.Client
	if strings.TrimSpace(cfg.NoticeURL) != "" {
		noticeClient, err = notice.NewClient(
			cfg.NoticeURL,
			cfg.NoticeAuth.ClientID,
			cfg.NoticeAuth.ClientSecret,
			cfg.NoticeAuth.KeyID,
		)
		if err != nil {
			return nil, fmt.Errorf("notice.NewClient: %w", err)
		}
	}
	var practiceCommands *practice.CommandClient
	if cfg.PracticeCommandsEnabled {
		practiceCommands, err = practice.NewCommandClient(
			cfg.PracticeURL,
			cfg.PracticeCommandAuth.ClientID,
			cfg.PracticeCommandAuth.ClientSecret,
			cfg.PracticeCommandAuth.KeyID,
		)
		if err != nil {
			return nil, fmt.Errorf("practice.NewCommandClient: %w", err)
		}
	}
	var quizCraftCatalog *practice.Client
	if cfg.QuizCraftCatalogEnabled {
		quizCraftCatalog, err = practice.NewClient(
			cfg.PracticeURL,
			cfg.PracticeAuth.ClientID,
			cfg.PracticeAuth.ClientSecret,
			cfg.PracticeAuth.KeyID,
		)
		if err != nil {
			return nil, fmt.Errorf("practice.NewClient catalog: %w", err)
		}
	}
	var quizCraft *practice.Client
	if cfg.QuizCraftV2ReadsEnabled {
		quizCraft, err = practice.NewClient(
			cfg.QuizCraftCoreURL,
			cfg.QuizCraftCoreAuth.ClientID,
			cfg.QuizCraftCoreAuth.ClientSecret,
			cfg.QuizCraftCoreAuth.KeyID,
		)
		if err != nil {
			return nil, fmt.Errorf("QuizCraft V2 read client: %w", err)
		}
	}
	return &Handler{
		sessionCodec:       codec,
		platform:           platformcore.NewClient(cfg.PlatformCoreURL, cfg.PortalRedirectURI, cfg.PlatformClientID, cfg.PlatformSecret, cfg.PlatformKeyID),
		quizCraft:          quizCraft,
		portalAPI:          &http.Client{Timeout: 10 * time.Second},
		portalAPIURL:       cfg.PortalAPIURL,
		accountPortfolio:   portfolio,
		noticeClient:       noticeClient,
		practiceCommands:   practiceCommands,
		quizCraftCatalog:   quizCraftCatalog,
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
	r.Post("/api/v1/account/membership-orders", h.accountMembershipOrderCreate)
	if h.quizCraftCatalog != nil {
		// This exact handler is registered only for a local or explicitly
		// coordinated cutover configuration. The default wildcard path below
		// still fails closed for this exact V2 route.
		r.Get(quizCraftCatalogPath, h.getQuizCraftCatalog)
	}
	if h.quizCraft != nil {
		// #164 prepared these clients behind the V2 read gate. #166 is the
		// only release allowed to make the public read routes reachable.
		r.Get(practice.OverallRankingPath, h.getQuizCraftOverallRanking)
		r.Get(practice.BankRankingPath, h.getQuizCraftBankRanking)
	}

	// This is the sole browser-visible QuizCraft write boundary. It is not a
	// generic proxy and stays unavailable until the explicit #166 cutover gate
	// has provisioned independent command credentials on both services.
	r.Post("/api/v1/practice/sessions", h.createPracticeSession)
	r.Post("/api/v1/practice/sessions/{session_id}/answers", h.submitPracticeAnswer)
	// Corrections are the one user-created QuizCraft command after sessions and
	// answers. Like them it is an explicit signed command, never a wildcard
	// proxy write; unlike them it is signed-in-only. The status read is the
	// matching actor-bound read and must never fall back to the generic proxy.
	r.Post("/api/v1/practice/feedback", h.createPracticeFeedback)
	r.Get("/api/v1/practice/feedback/{feedback_id}/status", h.getPracticeFeedbackStatus)

	// Product data — proxy to portal-api (public, no auth required)
	r.Get("/api/v1/library/*", h.proxyToPortalAPI)
	r.Get("/api/v1/food/*", h.proxyToPortalAPI)
	// This private V2 route is never proxied to legacy Portal API data. Before
	// #166 enables the V2 client it returns an honest unavailable response.
	r.Get("/api/v1/practice/stats", h.personalPracticeStats)
	r.Get("/api/v1/practice/*", h.proxyToPortalAPI)
	r.Get("/api/v1/campus/*", h.proxyToPortalAPI)

	// The notices read is not legacy aggregation data: it is a signed,
	// actor-bound read from the Notice owner. See getNotices.
	r.Get("/api/v1/notices", h.getNotices)

	return r
}

// --- Middleware ---

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if !portalRequestIDPattern.MatchString(id) {
			id = "req_" + strings.ReplaceAll(uuid(), "-", "")
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
		writeError(w, r, http.StatusBadRequest, "missing code or state", "登录没有成功，请重新登录；如果反复失败请稍后再试")
		return
	}

	cookies := h.browserCookies(r)
	cookie, err := r.Cookie(cookies.oauth)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "missing oauth cookie", "登录没有成功，请重新登录；如果反复失败请稍后再试")
		return
	}
	browserNonce, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid oauth cookie", "登录没有成功，请重新登录；如果反复失败请稍后再试")
		return
	}
	stateBytes, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil || len(stateBytes) != 32 {
		writeError(w, r, http.StatusBadRequest, "invalid or expired state", "登录没有成功，请重新登录；如果反复失败请稍后再试")
		return
	}

	stateHash := sha256Hex(stateBytes)
	browserHash := sha256Hex(browserNonce)
	key := fmt.Sprintf("portal:oauth-state:%s:%s", stateHash, browserHash)

	data, err := h.redis.GetDel(r.Context(), key).Bytes()
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid or expired state", "登录没有成功，请重新登录；如果反复失败请稍后再试")
		return
	}

	var stored map[string]string
	if err := json.Unmarshal(data, &stored); err != nil {
		writeError(w, r, http.StatusInternalServerError, "corrupt state", "登录没有成功，请重新登录；如果反复失败请稍后再试")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: cookies.oauth, Value: "", Path: "/",
		HttpOnly: true, Secure: cookies.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0),
	})

	idempotencyKey := hex.EncodeToString(stateBytes[:16])
	result, err := h.platform.ExchangeCode(r.Context(), code, stored["verifier"], idempotencyKey)
	if err != nil {
		code := "exchange_error"
		status := http.StatusInternalServerError
		if err == platformcore.ErrUnauthorized {
			code = "exchange failed"
			status = http.StatusUnauthorized
		}
		// Redacted: never log code, verifier, cookies, email, or secrets.
		log.Printf("portal-gateway oauth exchange failed request_id=%s category=%s", requestIDOf(w, r), code)
		writeError(w, r, status, code, "登录没有成功，请重新登录；如果反复失败请稍后再试")
		return
	}

	encoded, err := h.sessionCodec.Encode(session.Value{
		UserID: result.UserID, DisplayName: result.DisplayName,
		ExchangeToken: result.SessionExchangeToken, ExpiresAt: result.ExpiresAt,
	})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "session encode error", "登录没有成功，请重新登录；如果反复失败请稍后再试")
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
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", Message: "登录已过期，请重新登录"})
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

// accountMembershipOrderCreate is the ADR-0019 write exception. It forwards
// only a user creating their own order; close, refund, and every other order
// command stay on the separately authenticated Console path.
func (h *Handler) accountMembershipOrderCreate(w http.ResponseWriter, r *http.Request) {
	h.accountCommand(w, r, http.StatusCreated, true, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
		return h.accountPortfolio.CreateMembershipOrder(ctx, actorUserID, requestID, idempotencyKey, raw)
	})
}

func (h *Handler) accountRead(w http.ResponseWriter, r *http.Request, read accountRead) {
	// Account facts are private and must not be stored by a browser or
	// intermediary while the Portal Session remains active.
	setPrivateResponseHeaders(w)
	value, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", Message: "登录已过期，请重新登录", RequestID: requestIDOf(w, r)})
		return
	}
	if h.accountPortfolio == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "account_portfolio_unavailable", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
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
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", Message: "登录已过期，请重新登录", RequestID: requestIDOf(w, r)})
		return
	}
	if h.accountPortfolio == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "account_portfolio_unavailable", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !validAccountIdempotencyKey(idempotencyKey) {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "account_idempotency_key_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
		return
	}
	var raw []byte
	if bodyRequired {
		raw, err = readAccountCommandBody(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "account_command_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
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
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "account_command_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
	case errors.Is(err, accountportfolio.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", Message: "登录已过期，请重新登录", RequestID: requestIDOf(w, r)})
	case errors.Is(err, accountportfolio.ErrNotFound):
		writeJSON(w, http.StatusNotFound, contract.ErrorEnvelope{Error: "account_resource_not_found", Message: "内容不存在或已下架", RequestID: requestIDOf(w, r)})
	case errors.Is(err, accountportfolio.ErrConflict):
		writeJSON(w, http.StatusConflict, contract.ErrorEnvelope{Error: "account_command_conflict", Message: "操作内容有更新，请刷新后重试", RequestID: requestIDOf(w, r)})
	case errors.Is(err, accountportfolio.ErrInvalid):
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "account_portfolio_invalid_response", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
	case errors.Is(err, accountportfolio.ErrPaymentProviderUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "membership_payment_unavailable", Message: "支付服务暂时不可用，请稍后再试", RequestID: requestIDOf(w, r)})
	default:
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "account_portfolio_unavailable", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
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

type practiceCommand func(context.Context, string, string, string, []byte, *http.Cookie) (practice.CommandResult, error)

func (h *Handler) createPracticeSession(w http.ResponseWriter, r *http.Request) {
	h.practiceCommand(w, r, http.StatusCreated, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (practice.CommandResult, error) {
		return h.practiceCommands.CreateSession(ctx, actorUserID, requestID, idempotencyKey, raw, anonymousCookie)
	})
}

func (h *Handler) submitPracticeAnswer(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")
	h.practiceCommand(w, r, http.StatusOK, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (practice.CommandResult, error) {
		return h.practiceCommands.SubmitAnswer(ctx, sessionID, actorUserID, requestID, idempotencyKey, raw, anonymousCookie)
	})
}

// createPracticeFeedback is the signed-in-only correction command. Unlike
// sessions and answers it never downgrades to a guest actor: a correction has
// no durable anonymous identity, and the ticket requires an explicit login
// path instead of a silent anonymous write.
func (h *Handler) createPracticeFeedback(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	if h.practiceCommands == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "practice_commands_unavailable", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
		return
	}
	value, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", Message: "请先登录后再提交纠错", RequestID: requestIDOf(w, r)})
		return
	}
	if !practice.ValidUUID(value.UserID) {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", Message: "登录已过期，请重新登录", RequestID: requestIDOf(w, r)})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !practice.ValidIdempotencyKey(idempotencyKey) {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "practice_idempotency_key_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
		return
	}
	raw, err := readGatewayPracticeCommandBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "practice_command_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
		return
	}
	result, err := h.practiceCommands.CreateFeedback(r.Context(), value.UserID, requestIDOf(w, r), idempotencyKey, raw, coreAnonymousCookie(r))
	if err != nil {
		h.writePracticeCommandFailure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write(result.Raw)
}

// practiceCommand turns a browser command into exactly one signed Core
// command. It intentionally does not proxy headers, cookies, actor identity,
// or mock data. An invalid Portal Session is a 401, while an absent Portal
// Session is a genuine guest request.
func (h *Handler) practiceCommand(w http.ResponseWriter, r *http.Request, successStatus int, command practiceCommand) {
	setPrivateResponseHeaders(w)
	if h.practiceCommands == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "practice_commands_unavailable", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
		return
	}
	actorUserID, anonymousCookie, status, err := h.practiceCommandActor(r)
	if err != nil {
		writeJSON(w, status, contract.ErrorEnvelope{Error: "not authenticated", Message: "登录已过期，请重新登录", RequestID: requestIDOf(w, r)})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !practice.ValidIdempotencyKey(idempotencyKey) {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "practice_idempotency_key_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
		return
	}
	raw, err := readGatewayPracticeCommandBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "practice_command_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
		return
	}
	result, err := command(r.Context(), actorUserID, requestIDOf(w, r), idempotencyKey, raw, anonymousCookie)
	if err != nil {
		h.writePracticeCommandFailure(w, r, err)
		return
	}
	if result.AnonymousCookie != nil {
		// CommandClient accepted this only after checking every browser-visible
		// attribute. Do not append any other upstream Set-Cookie header.
		http.SetCookie(w, result.AnonymousCookie)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(successStatus)
	_, _ = w.Write(result.Raw)
}

func (h *Handler) writePracticeCommandFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, practice.ErrPracticeCommandBadRequest):
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "practice_command_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
	case errors.Is(err, practice.ErrPracticeCommandForbidden):
		writeJSON(w, http.StatusForbidden, contract.ErrorEnvelope{Error: "practice_session_forbidden", Message: "暂无练习权限，请联系管理员", RequestID: requestIDOf(w, r)})
	case errors.Is(err, practice.ErrPracticeCommandNotFound):
		writeJSON(w, http.StatusNotFound, contract.ErrorEnvelope{Error: "practice_session_not_found", Message: "练习记录不存在，请刷新后重试", RequestID: requestIDOf(w, r)})
	case errors.Is(err, practice.ErrPracticeCommandConflict):
		writeJSON(w, http.StatusConflict, contract.ErrorEnvelope{Error: "practice_command_conflict", Message: "操作内容有更新，请刷新后重试", RequestID: requestIDOf(w, r)})
	case errors.Is(err, practice.ErrPracticeCommandInvalid):
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "practice_command_invalid_response", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
	default:
		// Authentication failures are a deployment/configuration fault between
		// Gateway and Core, not a browser authentication state.
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "practice_commands_unavailable", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
	}
}

func (h *Handler) practiceCommandActor(r *http.Request) (string, *http.Cookie, int, error) {
	if _, err := r.Cookie(h.browserCookies(r).session); err == nil {
		value, sessionErr := h.readSession(r)
		if sessionErr != nil || !practice.ValidUUID(value.UserID) {
			return "", nil, http.StatusUnauthorized, errors.New("invalid Portal Session")
		}
		return value.UserID, coreAnonymousCookie(r), 0, nil
	} else if !errors.Is(err, http.ErrNoCookie) {
		return "", nil, http.StatusUnauthorized, errors.New("invalid Portal Session")
	}
	return "", coreAnonymousCookie(r), 0, nil
}

func coreAnonymousCookie(r *http.Request) *http.Cookie {
	cookie, err := r.Cookie("quizcraft_anonymous")
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil
	}
	return &http.Cookie{Name: "quizcraft_anonymous", Value: cookie.Value}
}

func readGatewayPracticeCommandBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, errors.New("practice command body is required")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20+1))
	if err != nil || len(raw) == 0 || len(raw) > 2<<20 {
		return nil, errors.New("practice command body is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value any
	if err := decoder.Decode(&value); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("practice command body is not one JSON value")
	}
	return raw, nil
}

// personalPracticeStats returns only the signed-in user's Core-derived
// statistics. It intentionally has no mock or Portal API success fallback.
func (h *Handler) personalPracticeStats(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	if h.quizCraft == nil {
		writeError(w, r, http.StatusServiceUnavailable, "practice statistics are not enabled", "学习统计暂时不可用，请稍后再试")
		return
	}
	value, err := h.readSession(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "not authenticated", "登录已过期，请重新登录")
		return
	}
	if err := h.platform.CheckPermission(r.Context(), value.ExchangeToken, practice.CatalogReadPermission); err != nil {
		h.writePlatformPermissionError(w, r, err, "practice")
		return
	}
	stats, err := h.quizCraft.PersonalStats(r.Context(), value.UserID, requestIDOf(w, r))
	if err != nil {
		switch {
		case errors.Is(err, practice.ErrStatsUnauthorized):
			writeError(w, r, http.StatusUnauthorized, "not authenticated", "登录已过期，请重新登录")
		default:
			writeError(w, r, http.StatusServiceUnavailable, "practice statistics are temporarily unavailable", "学习统计暂时不可用，请稍后再试")
		}
		return
	}
	mastery := make([]contract.MasterySubject, 0, len(stats.Data.Mastery))
	for _, subject := range stats.Data.Mastery {
		mastery = append(mastery, contract.MasterySubject{
			BankID: subject.BankID, Label: subject.Label, Value: subject.Value,
			TotalQuestions: subject.TotalQuestions, CorrectQuestions: subject.CorrectQuestions,
		})
	}
	writeJSON(w, http.StatusOK, contract.PersonalPracticeStatsEnvelope{
		RequestID: stats.RequestID,
		Data: contract.PersonalPracticeStats{
			TotalAnswers: stats.Data.TotalAnswers, CorrectAnswers: stats.Data.CorrectAnswers,
			Accuracy: stats.Data.Accuracy, StreakDays: stats.Data.StreakDays, Mastery: mastery,
		},
	})
}

// getPracticeFeedbackStatus reads one signed-in user's persisted correction
// status. It is a narrow actor-bound read like personalPracticeStats, never
// the generic practice proxy: the wildcard read stays public product data.
func (h *Handler) getPracticeFeedbackStatus(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	if h.quizCraft == nil {
		writeError(w, r, http.StatusServiceUnavailable, "practice feedback status is not enabled", "反馈状态暂时不可用，请稍后再试")
		return
	}
	feedbackID := chi.URLParam(r, "feedback_id")
	value, err := h.readSession(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "not authenticated", "登录已过期，请重新登录")
		return
	}
	if err := h.platform.CheckPermission(r.Context(), value.ExchangeToken, practice.CatalogReadPermission); err != nil {
		h.writePlatformPermissionError(w, r, err, "practice")
		return
	}
	status, err := h.quizCraft.FeedbackStatus(r.Context(), value.UserID, requestIDOf(w, r), feedbackID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "practice feedback status is temporarily unavailable", "反馈状态暂时不可用，请稍后再试")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// getNotices returns the Notice owner's bounded snapshot for the signed-in
// Portal Session actor. It is an actor-bound read like personalPracticeStats:
// it never falls back to the aggregation layer, a cache, or mock success.
func (h *Handler) getNotices(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	if h.noticeClient == nil {
		writeError(w, r, http.StatusServiceUnavailable, "notice_unavailable", "通知服务暂时不可用，请稍后再来")
		return
	}
	value, err := h.readSession(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "not authenticated", "登录已过期，请重新登录")
		return
	}
	if err := h.platform.CheckPermission(r.Context(), value.ExchangeToken, noticeReadPermission); err != nil {
		h.writePlatformPermissionError(w, r, err, "notice")
		return
	}
	data, err := h.noticeClient.List(r.Context(), value.UserID, requestIDOf(w, r))
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "notice temporarily unavailable", "通知服务暂时不可用，请稍后再来")
		return
	}
	writeJSON(w, http.StatusOK, contract.NoticeFeedEnvelope{
		Data:      data,
		RequestID: requestIDOf(w, r),
	})
}

// writePlatformPermissionError maps Platform Core permission outcomes for
// actor-bound reads. The practice reads and the Notice read share one
// CheckPermission path, so the mapping is shared; the only variance is the
// resource named in the error payloads. (#269's learning-state read carries
// its own copy of this switch and should switch to this helper at cutover.)
func (h *Handler) writePlatformPermissionError(w http.ResponseWriter, r *http.Request, err error, resource string) {
	switch {
	case errors.Is(err, platformcore.ErrUnauthorized):
		writeError(w, r, http.StatusUnauthorized, "not authenticated", "登录已过期，请重新登录")
	case errors.Is(err, platformcore.ErrForbidden):
		writeError(w, r, http.StatusForbidden, resource+" access denied", "暂无"+resource+"权限，请联系管理员")
	default:
		writeError(w, r, http.StatusServiceUnavailable, resource+" authorization is temporarily unavailable", "服务暂时不可用，请稍后再来")
	}
}

// --- Proxy to portal-api ---

func (h *Handler) proxyToPortalAPI(w http.ResponseWriter, r *http.Request) {
	// The legacy wildcard must not accidentally make a successful V2 catalog
	// route public before #166. Keep the path externally indistinguishable
	// from an unregistered route and do not contact either upstream.
	if r.URL.Path == quizCraftCatalogPath {
		writeJSON(w, http.StatusNotFound, contract.ErrorEnvelope{Error: "not found", Message: "内容不存在或已下架", RequestID: requestIDOf(w, r)})
		return
	}
	targetURL := h.portalAPIURL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "proxy_error", Message: "服务暂时不可用，请稍后再来"})
		return
	}
	req.Header.Set("X-Request-Id", w.Header().Get("X-Request-Id"))
	// Let a cached food photo revalidate instead of transferring the bytes again.
	if match := r.Header.Get("If-None-Match"); match != "" {
		req.Header.Set("If-None-Match", match)
	}

	resp, err := h.portalAPI.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "portal_api_unavailable", Message: "服务暂时不可用，请稍后再来"})
		return
	}
	defer resp.Body.Close()

	// Most Portal API routes answer JSON, but food photos come back as image
	// bytes; forwarding them as application/json would stop a browser rendering
	// them. Carry the upstream content type, and the headers that let a photo be
	// cached and revalidated, rather than assuming a single shape.
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	for _, header := range []string{"Cache-Control", "ETag", "Content-Disposition", "X-Content-Type-Options"} {
		if value := resp.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
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

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, contract.ErrorEnvelope{
		Error:     code,
		Message:   message,
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
