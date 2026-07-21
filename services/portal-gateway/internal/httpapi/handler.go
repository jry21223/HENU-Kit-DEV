package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/food"
	"henukit.dev/portal-gateway/internal/library"
	"henukit.dev/portal-gateway/internal/notice"
	"henukit.dev/portal-gateway/internal/platformcore"
	"henukit.dev/portal-gateway/internal/practice"
	"henukit.dev/portal-gateway/internal/session"
)

// Handler is the Portal Gateway HTTP handler.
type Handler struct {
	sessionCodec    *session.Codec
	platform        *platformcore.Client
	libraryClient   *library.Client
	foodClient      *food.Client
	practiceClient  *practice.Client
	noticeClient    *notice.Client
	redis           *redis.Client
	portalOrigin    string
	platformCoreURL string
	clientID        string
	redirectURI     string
}

// New creates a Handler from config.
func New(cfg config.Config, rdb *redis.Client) (*Handler, error) {
	codec, err := session.NewCodec(cfg.SessionKey)
	if err != nil {
		return nil, fmt.Errorf("session.NewCodec: %w", err)
	}
	return &Handler{
		sessionCodec:    codec,
		platform:        platformcore.NewClient(cfg.PlatformCoreURL, cfg.PortalRedirectURI, cfg.PlatformClientID, cfg.PlatformSecret, cfg.PlatformKeyID),
		libraryClient:   library.NewClient(cfg.LibraryURL, cfg.LibraryAuth.ClientID, cfg.LibraryAuth.ClientSecret, cfg.LibraryAuth.KeyID),
		foodClient:      food.NewClient(cfg.FoodURL, cfg.FoodAuth.ClientID, cfg.FoodAuth.ClientSecret, cfg.FoodAuth.KeyID),
		practiceClient:  practice.NewClient(cfg.PracticeURL, cfg.PracticeAuth.ClientID, cfg.PracticeAuth.ClientSecret, cfg.PracticeAuth.KeyID),
		noticeClient:    notice.NewClient(cfg.NoticeURL, cfg.NoticeAuth.ClientID, cfg.NoticeAuth.ClientSecret, cfg.NoticeAuth.KeyID),
		redis:           rdb,
		portalOrigin:    cfg.PortalOrigin,
		platformCoreURL: cfg.PlatformCoreURL,
		clientID:        cfg.PlatformClientID,
		redirectURI:     cfg.PortalRedirectURI,
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

	// Read-only product proxies (authenticated)
	r.Route("/api/v1/library", func(r chi.Router) {
		r.Get("/courses", h.libraryCourses)
	})
	r.Route("/api/v1/food", func(r chi.Router) {
		r.Get("/venues", h.foodVenues)
	})
	r.Route("/api/v1/practice", func(r chi.Router) {
		r.Get("/banks", h.practiceBanks)
	})
	r.Route("/api/v1/notices", func(r chi.Router) {
		r.Get("/", h.noticeList)
	})

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
		next.ServeHTTP(w, r.WithContext(r.Context()))
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

	// Store state in Redis with 5-minute TTL
	payload, _ := json.Marshal(map[string]string{
		"verifier":  base64.RawURLEncoding.EncodeToString(verifier),
		"return_to": returnTo,
	})
	key := fmt.Sprintf("portal:oauth-state:%s:%s", stateHash, browserHash)
	h.redis.Set(r.Context(), key, payload, 5*time.Minute)

	// Set OAuth flow cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-henukit_portal_oauth",
		Value:    base64.RawURLEncoding.EncodeToString(browserNonce),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})

	// Build PKCE challenge
	challenge := sha256Sum(verifier)
	codeChallenge := base64.RawURLEncoding.EncodeToString(challenge)

	redirectURL := fmt.Sprintf(
		"%s/api/v1/oauth/authorize?response_type=code&client_id=%s&redirect_uri=%s&state=%s&code_challenge=%s&code_challenge_method=S256",
		h.platformCoreURL,
		h.clientID,
		h.redirectURI,
		base64.RawURLEncoding.EncodeToString(state),
		codeChallenge,
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

	// Read browser nonce from OAuth cookie
	cookie, err := r.Cookie("__Host-henukit_portal_oauth")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "missing oauth cookie"})
		return
	}
	browserNonce, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "invalid oauth cookie"})
		return
	}

	stateHash := sha256Hex([]byte(state))
	browserHash := sha256Hex(browserNonce)
	key := fmt.Sprintf("portal:oauth-state:%s:%s", stateHash, browserHash)

	// GETDEL: single-use state
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

	// Clear OAuth cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-henukit_portal_oauth",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})

	// Exchange code for session
	stateBytes, _ := base64.RawURLEncoding.DecodeString(state)
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

	// Encode session cookie
	encoded, err := h.sessionCodec.Encode(session.Value{
		UserID:        result.UserID,
		ExchangeToken: result.SessionExchangeTkn,
		ExpiresAt:     result.ExpiresAt,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, contract.ErrorEnvelope{Error: "session encode error"})
		return
	}

	maxAge := int(time.Until(result.ExpiresAt).Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-henukit_portal_session",
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
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

	writeJSON(w, http.StatusOK, contract.PortalSession{
		UserID:    v.UserID,
		ExpiresAt: v.ExpiresAt,
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "__Host-henukit_portal_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed_out"})
}

// --- Product proxies ---

func (h *Handler) libraryCourses(w http.ResponseWriter, r *http.Request) {
	v, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated"})
		return
	}
	if err := h.platform.CheckPermission(r.Context(), v.ExchangeToken, "portal.library.read"); err != nil {
		h.handlePermError(w, err)
		return
	}
	result, err := h.libraryClient.Courses(r.Context(), v.UserID, w.Header().Get("X-Request-Id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "library_unavailable", Detail: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) foodVenues(w http.ResponseWriter, r *http.Request) {
	v, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated"})
		return
	}
	if err := h.platform.CheckPermission(r.Context(), v.ExchangeToken, "portal.food.read"); err != nil {
		h.handlePermError(w, err)
		return
	}
	campus := r.URL.Query().Get("campus")
	result, err := h.foodClient.Venues(r.Context(), campus, v.UserID, w.Header().Get("X-Request-Id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "food_unavailable", Detail: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) practiceBanks(w http.ResponseWriter, r *http.Request) {
	v, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated"})
		return
	}
	if err := h.platform.CheckPermission(r.Context(), v.ExchangeToken, "portal.practice.read"); err != nil {
		h.handlePermError(w, err)
		return
	}
	result, err := h.practiceClient.Banks(r.Context(), v.UserID, w.Header().Get("X-Request-Id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "practice_unavailable", Detail: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) noticeList(w http.ResponseWriter, r *http.Request) {
	v, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated"})
		return
	}
	if err := h.platform.CheckPermission(r.Context(), v.ExchangeToken, "portal.notice.read"); err != nil {
		h.handlePermError(w, err)
		return
	}
	result, err := h.noticeClient.List(r.Context(), v.UserID, w.Header().Get("X-Request-Id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "notice_unavailable", Detail: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- Helpers ---

func (h *Handler) readSession(r *http.Request) (session.Value, error) {
	cookie, err := r.Cookie("__Host-henukit_portal_session")
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

func (h *Handler) handlePermError(w http.ResponseWriter, err error) {
	if err == platformcore.ErrUnauthorized {
		// Clear session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "__Host-henukit_portal_session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			MaxAge:   -1,
			Expires:  time.Unix(1, 0),
		})
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "session expired"})
		return
	}
	if err == platformcore.ErrForbidden {
		writeJSON(w, http.StatusForbidden, contract.ErrorEnvelope{Error: "forbidden"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, contract.ErrorEnvelope{Error: "auth_check_error"})
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
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func sha256Sum(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

