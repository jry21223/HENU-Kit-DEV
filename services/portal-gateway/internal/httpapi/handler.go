package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

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
	return &Handler{
		sessionCodec:       codec,
		platform:           platformcore.NewClient(cfg.PlatformCoreURL, cfg.PortalRedirectURI, cfg.PlatformClientID, cfg.PlatformSecret, cfg.PlatformKeyID),
		portalAPI:          &http.Client{Timeout: 10 * time.Second},
		portalAPIURL:       cfg.PortalAPIURL,
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
	verifier := randomBytes(32)
	browserNonce := randomBytes(32)

	stateHash := sha256Hex(state)
	browserHash := sha256Hex(browserNonce)

	payload, _ := json.Marshal(map[string]string{
		"verifier":  base64.RawURLEncoding.EncodeToString(verifier),
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

	challenge := sha256Sum(verifier)
	codeChallenge := base64.RawURLEncoding.EncodeToString(challenge)

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
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "missing code or state"})
		return
	}

	cookies := h.browserCookies(r)
	cookie, err := r.Cookie(cookies.oauth)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "missing oauth cookie"})
		return
	}
	browserNonce, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid oauth cookie"})
		return
	}
	stateBytes, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil || len(stateBytes) != 32 {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid or expired state"})
		return
	}

	stateHash := sha256Hex(stateBytes)
	browserHash := sha256Hex(browserNonce)
	key := fmt.Sprintf("portal:oauth-state:%s:%s", stateHash, browserHash)

	data, err := h.redis.GetDel(r.Context(), key).Bytes()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid or expired state"})
		return
	}

	var stored map[string]string
	if err := json.Unmarshal(data, &stored); err != nil {
		writeJSON(w, http.StatusInternalServerError, contract.ErrorEnvelope{Error: "corrupt state"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name: cookies.oauth, Value: "", Path: "/",
		HttpOnly: true, Secure: cookies.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0),
	})

	idempotencyKey := hex.EncodeToString(stateBytes[:16])
	result, err := h.platform.ExchangeCode(r.Context(), code, stored["verifier"], idempotencyKey)
	if err != nil {
		if err == platformcore.ErrUnauthorized {
			writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "exchange failed"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, contract.ErrorEnvelope{Error: "exchange error"})
		return
	}

	encoded, err := h.sessionCodec.Encode(session.Value{
		UserID: result.UserID, ExchangeToken: result.SessionExchangeToken, ExpiresAt: result.ExpiresAt,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, contract.ErrorEnvelope{Error: "session encode error"})
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
	v, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated"})
		return
	}
	writeJSON(w, http.StatusOK, contract.PortalSession{UserID: v.UserID, ExpiresAt: v.ExpiresAt})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	cookies := h.browserCookies(r)
	http.SetCookie(w, &http.Cookie{
		Name: cookies.session, Value: "", Path: "/",
		HttpOnly: true, Secure: cookies.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed_out"})
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

func sha256Sum(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
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
