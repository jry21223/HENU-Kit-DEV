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
	"mime"
	"mime/multipart"
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
	"henukit.dev/portal-gateway/internal/career"
	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/foodposts"
	"henukit.dev/portal-gateway/internal/librarydownload"
	"henukit.dev/portal-gateway/internal/platformcore"
	"henukit.dev/portal-gateway/internal/practice"
	"henukit.dev/portal-gateway/internal/session"
)

// Handler is the Portal Gateway HTTP handler.
type Handler struct {
	sessionCodec       *session.Codec
	platform           *platformcore.Client
	displayNames       *practice.DisplayNamesResolver
	quizCraft          *practice.Client
	portalAPI          *http.Client
	portalAPIURL       string
	libraryDownloads   *librarydownload.Client
	accountPortfolio   *accountportfolio.Client
	foodPosts          *foodposts.Client
	career             *career.Client
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

// oauthFlowTTL is the OAuth flow window shared by the Redis state and the
// oauth cookie MaxAge. The flow must survive the full email-code login: a
// signed-out browser lands on Platform Core's own login page after authorize,
// then the user reads the code from mail and enters it. A 5-minute window made
// the callback fail with missing oauth cookie for slow users; 30 minutes
// covers the verification code TTL plus reading mail while staying short
// enough to keep replay exposure bounded. The consumed marker written by a
// successful callback lives for the same window so replays stay classifiable.
const oauthFlowTTL = 30 * time.Minute

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
	var libraryDownloads *librarydownload.Client
	if strings.TrimSpace(cfg.LibraryDownloadURL) != "" {
		libraryDownloads, err = librarydownload.NewClient(
			cfg.LibraryDownloadURL,
			cfg.LibraryDownloadAuth.ClientID,
			cfg.LibraryDownloadAuth.ClientSecret,
			cfg.LibraryDownloadAuth.KeyID,
		)
		if err != nil {
			return nil, fmt.Errorf("librarydownload.NewClient: %w", err)
		}
	}
	var foodPosts *foodposts.Client
	if strings.TrimSpace(cfg.FoodPostsURL) != "" {
		foodPosts, err = foodposts.NewClient(
			cfg.FoodPostsURL,
			cfg.FoodPostCreateAuth.ClientID, cfg.FoodPostCreateAuth.ClientSecret, cfg.FoodPostCreateAuth.KeyID,
			cfg.FoodPostReadAuth.ClientID, cfg.FoodPostReadAuth.ClientSecret, cfg.FoodPostReadAuth.KeyID,
		)
		if err != nil {
			return nil, fmt.Errorf("foodposts.NewClient: %w", err)
		}
	}
	var careerClient *career.Client
	if strings.TrimSpace(cfg.CareerURL) != "" {
		careerClient, err = career.NewClient(
			cfg.CareerURL,
			cfg.CareerAuth.ClientID, cfg.CareerAuth.ClientSecret, cfg.CareerAuth.KeyID,
		)
		if err != nil {
			return nil, fmt.Errorf("career.NewClient: %w", err)
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
	platform := platformcore.NewClient(cfg.PlatformCoreURL, cfg.PortalRedirectURI, cfg.PlatformClientID, cfg.PlatformSecret, cfg.PlatformKeyID)
	// The ranking nickname synthesis resolves display names through Platform
	// Core's read-only boundary with a process-internal TTL cache and
	// singleflight (ADR-0038); a Platform Core outage degrades ranking
	// nicknames to 游客x instead of failing the read.
	displayNames := practice.NewDisplayNamesResolver(func(ctx context.Context, requestID string, userIDs []string) (map[string]string, error) {
		return platform.DisplayNames(ctx, userIDs, requestID)
	})
	return &Handler{
		sessionCodec:       codec,
		platform:           platform,
		displayNames:       displayNames,
		quizCraft:          quizCraft,
		portalAPI:          &http.Client{Timeout: 10 * time.Second},
		portalAPIURL:       cfg.PortalAPIURL,
		libraryDownloads:   libraryDownloads,
		accountPortfolio:   portfolio,
		foodPosts:          foodPosts,
		career:             careerClient,
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
	// QuizCraft V2 read routes are registered unconditionally (ADR-0036). The
	// PORTAL_ENABLE_QUIZCRAFT_* flags no longer decide whether a route exists —
	// they decide whether the matching read client exists. Each handler fails
	// closed on its own when its client is nil: public reads (catalog,
	// rankings) answer an honest 404 and actor-bound reads (stats, favorites,
	// feedback status) answer an honest 503, never a legacy portal-api or mock
	// fallback.
	r.Get("/api/v1/practice/catalog", h.getQuizCraftCatalog)
	r.Get(practice.OverallRankingPath, h.getQuizCraftOverallRanking)
	r.Get(practice.BankRankingPath, h.getQuizCraftBankRanking)

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
	// Favorites are signed-in learning state: writes are explicit signed
	// commands, reads are actor-bound like personal stats.
	r.Get("/api/v1/practice/favorites", h.favoritesOverview)
	r.Get("/api/v1/practice/banks/{bank_id}/favorites", h.favoritesList)
	r.Put("/api/v1/practice/banks/{bank_id}/favorites/{question_id}", h.favoriteQuestion)
	r.Delete("/api/v1/practice/banks/{bank_id}/favorites/{question_id}", h.unfavoriteQuestion)
	r.Post("/api/v1/practice/banks/{bank_id}/favorites/practice-sessions", h.createFavoritesSession)

	// The owner-backed download command must shadow the public-data wildcard.
	// Browser callers select only a material ID, never a storage key or URL.
	r.Get(contract.LibraryDownloadRoute, h.libraryDownload)
	// The complete owner snapshot shadows Portal API's legacy mock catalog. It
	// carries no browser filters so the global facts remain filter-independent.
	r.Get("/api/v1/library/materials", h.libraryCatalog)
	r.Get("/api/v1/library/materials/{material_id}", h.libraryMaterial)

	// Product data — proxy to portal-api (public, no auth required)
	r.Get("/api/v1/library/*", h.proxyToPortalAPI)
	// The Food Post boundary shadows the legacy food wildcard: public reads and
	// the signed-in create command go to the Food service with independent
	// credentials and never fall back to the Portal API proxy. The exact routes
	// are registered before the wildcard, and /posts/mine before /posts/{post_id}.
	r.Post("/api/v1/food/posts", h.createFoodPost)
	r.Get("/api/v1/food/posts", h.listFoodPosts)
	r.Get("/api/v1/food/posts/mine", h.myFoodPosts)
	r.Get("/api/v1/food/posts/{post_id}", h.foodPostDetail)
	r.Get("/api/v1/food/posts/{post_id}/images/{position}", h.foodPostImage)
	r.Get("/api/v1/food/venues", h.foodVenues)
	r.Get("/api/v1/food/*", h.proxyToPortalAPI)
	// The Career boundary shadows any future legacy career wildcard: exact
	// actor-bound routes to the Career service, gated on Account Portfolio
	// lifetime membership. Never proxied to the Portal API.
	r.Post("/api/v1/career/searches", h.createCareerSearch)
	r.Get("/api/v1/career/searches", h.listCareerSearches)
	r.Get("/api/v1/career/searches/{search_id}", h.careerSearchStatus)
	r.Get("/api/v1/career/profile", h.getCareerProfile)
	r.Put("/api/v1/career/profile", h.updateCareerProfile)
	r.Post("/api/v1/career/profile/extractions", h.createCareerExtraction)
	r.Get("/api/v1/career/profile/extractions/{extraction_id}", h.careerExtractionStatus)
	// This private V2 route is never proxied to legacy Portal API data. Before
	// #166 enables the V2 client it returns an honest unavailable response.
	r.Get("/api/v1/practice/stats", h.personalPracticeStats)
	// Legacy practice read endpoints removed by ADR-0036 no longer proxy to
	// portal-api and have no mock/legacy fallback: the Portal UI has migrated
	// to the V2 catalog (/api/v1/practice/catalog) and the Core ranking
	// contract (/api/v1/rankings/*). These exact routes fail closed with an
	// honest 404 plus a migration hint so a stale client never receives a
	// fabricated success response.
	r.Get("/api/v1/practice/banks", h.practiceLegacyGone)
	r.Get("/api/v1/practice/schools", h.practiceLegacyGone)
	r.Get("/api/v1/practice/lists/{list_id}", h.practiceLegacyGone)
	r.Get("/api/v1/practice/leaderboard", h.practiceLegacyGone)
	r.Get("/api/v1/practice/*", h.proxyToPortalAPI)
	r.Get("/api/v1/campus/*", h.proxyToPortalAPI)
	r.Get("/api/v1/notices", h.proxyToPortalAPI)

	return r
}

type browserLibraryMaterial struct {
	ID                string     `json:"id"`
	Type              string     `json:"type"`
	Subject           string     `json:"subject"`
	Title             string     `json:"title"`
	Author            string     `json:"author"`
	Intro             string     `json:"intro"`
	TOC               []string   `json:"toc"`
	Pages             [][]string `json:"pages"`
	Price             int64      `json:"price"`
	PreviewPages      int64      `json:"previewPages"`
	Downloads         int64      `json:"downloads"`
	DownloadAvailable bool       `json:"downloadAvailable"`
	FileSize          int64      `json:"fileSize"`
}

func (h *Handler) libraryCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "资料库筛选请在页面内完成。")
		return
	}
	if h.libraryDownloads == nil {
		writeError(w, r, http.StatusServiceUnavailable, "LIBRARY_TEMPORARILY_UNAVAILABLE", "资料库暂时无法加载，请稍后重试。")
		return
	}
	catalog, err := h.libraryDownloads.Catalog(r.Context(), requestIDOf(w, r))
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "LIBRARY_TEMPORARILY_UNAVAILABLE", "资料库暂时无法加载，请稍后重试。")
		return
	}
	materials := make([]browserLibraryMaterial, 0, len(catalog.Materials))
	for _, material := range catalog.Materials {
		materials = append(materials, toBrowserLibraryMaterial(material))
	}
	releaseID := any(nil)
	if catalog.ReleaseID != nil {
		releaseID = *catalog.ReleaseID
	}
	countingSince := any(nil)
	if catalog.CountingSince != nil {
		countingSince = catalog.CountingSince.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"materials": materials,
		"statistics": map[string]any{
			"releaseId": releaseID, "materialCount": catalog.MaterialCount,
			"downloadStarts": catalog.DownloadStarts,
			"countingSince":  countingSince,
			"asOf":           catalog.AsOf.UTC().Format(time.RFC3339),
		},
		"request_id": requestIDOf(w, r),
	})
}

func (h *Handler) libraryMaterial(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "资料请求无效，请返回资料库重新选择。")
		return
	}
	if h.libraryDownloads == nil {
		writeError(w, r, http.StatusServiceUnavailable, "LIBRARY_TEMPORARILY_UNAVAILABLE", "资料库暂时无法加载，请稍后重试。")
		return
	}
	catalog, err := h.libraryDownloads.Catalog(r.Context(), requestIDOf(w, r))
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "LIBRARY_TEMPORARILY_UNAVAILABLE", "资料库暂时无法加载，请稍后重试。")
		return
	}
	id := chi.URLParam(r, "material_id")
	for _, material := range catalog.Materials {
		if material.ID == id {
			writeJSON(w, http.StatusOK, map[string]any{"material": toBrowserLibraryMaterial(material), "request_id": requestIDOf(w, r)})
			return
		}
	}
	writeError(w, r, http.StatusNotFound, "MATERIAL_NOT_AVAILABLE", "资料不存在或已下架，请返回资料库重新选择。")
}

func toBrowserLibraryMaterial(material librarydownload.PublicMaterial) browserLibraryMaterial {
	return browserLibraryMaterial{
		ID: material.ID, Type: material.Type, Subject: material.Subject, Title: material.Title,
		Author: "资料库收录", Intro: "", TOC: []string{}, Pages: [][]string{}, Price: 0,
		PreviewPages: 0, Downloads: material.Downloads, DownloadAvailable: material.DownloadAvailable,
		FileSize: material.FileSize,
	}
}

func (h *Handler) libraryDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if h.libraryDownloads == nil {
		writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "暂时无法生成下载链接，请稍后重试。")
		return
	}
	grant, err := h.libraryDownloads.Start(r.Context(), chi.URLParam(r, "material_id"), requestIDOf(w, r))
	if err != nil {
		switch {
		case errors.Is(err, librarydownload.ErrBadRequest):
			writeError(w, r, http.StatusNotFound, "MATERIAL_NOT_AVAILABLE", "资料不存在或已下架，请返回资料库重新选择。")
		case errors.Is(err, librarydownload.ErrNotFound):
			writeError(w, r, http.StatusNotFound, "MATERIAL_NOT_AVAILABLE", "资料不存在或已下架，请返回资料库重新选择。")
		case errors.Is(err, librarydownload.ErrInvalid):
			writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "暂时无法生成下载链接，请稍后重试。")
		default:
			writeError(w, r, http.StatusServiceUnavailable, "DOWNLOAD_TEMPORARILY_UNAVAILABLE", "暂时无法生成下载链接，请稍后重试。")
		}
		return
	}
	w.Header().Set("Location", grant.Location)
	w.WriteHeader(http.StatusSeeOther)
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
	// The OAuth flow must survive the full email-code login: a signed-out
	// browser lands on Platform Core's own login page after authorize, then
	// the user reads the code from mail and enters it. A 5-minute window made
	// the callback fail with missing oauth cookie for slow users; 30 minutes
	// covers the verification code TTL plus reading mail while staying short
	// enough to keep replay exposure bounded.
	key := fmt.Sprintf("portal:oauth-state:%s:%s", stateHash, browserHash)
	h.redis.Set(r.Context(), key, payload, oauthFlowTTL)

	cookies := h.browserCookies(r)
	http.SetCookie(w, &http.Cookie{
		Name:     cookies.oauth,
		Value:    base64.RawURLEncoding.EncodeToString(browserNonce),
		Path:     "/",
		HttpOnly: true,
		Secure:   cookies.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oauthFlowTTL.Seconds()),
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
		h.failCallbackToLogin(w, r, "missing_code_or_state")
		return
	}

	cookies := h.browserCookies(r)
	cookie, err := r.Cookie(cookies.oauth)
	if err != nil {
		h.failCallbackToLogin(w, r, "missing_oauth_cookie")
		return
	}
	browserNonce, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		h.failCallbackToLogin(w, r, "invalid_oauth_cookie")
		return
	}
	stateBytes, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil || len(stateBytes) != 32 {
		h.failCallbackToLogin(w, r, "invalid_state")
		return
	}

	stateHash := sha256Hex(stateBytes)
	browserHash := sha256Hex(browserNonce)
	key := fmt.Sprintf("portal:oauth-state:%s:%s", stateHash, browserHash)

	data, err := h.redis.GetDel(r.Context(), key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			// Redis going down is a dependency failure, not an expired or replayed
			// flow: fail closed with the honest service error.
			log.Printf("portal-gateway oauth state lookup failed request_id=%s", requestIDOf(w, r))
			writeError(w, r, http.StatusServiceUnavailable, "STATE_UNAVAILABLE", "登录暂时不可用，请稍后再试")
			return
		}
		// The single-use state is gone. A successful callback leaves a consumed
		// marker behind, so a miss with a marker present is a replay; a miss
		// without one is an expired flow window. This is what lets production
		// logs answer "slow email login" vs "back-button replay".
		markerKey := fmt.Sprintf("portal:oauth-consumed:%s", stateHash)
		if _, markerErr := h.redis.Get(r.Context(), markerKey).Result(); markerErr == nil {
			h.failCallbackToLogin(w, r, "replayed_callback")
			return
		}
		h.failCallbackToLogin(w, r, "expired_state")
		return
	}

	var stored map[string]string
	if err := json.Unmarshal(data, &stored); err != nil {
		h.failCallbackToLogin(w, r, "corrupt_state")
		return
	}

	// Best-effort consumed marker: a later replay of this state (back-button,
	// double navigation) classifies as replayed_callback instead of masking as
	// expired_state. Failure to write only degrades the classification.
	h.redis.Set(r.Context(), fmt.Sprintf("portal:oauth-consumed:%s", stateHash), "1", oauthFlowTTL)

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

// failCallbackToLogin redirects a failed or replayed OAuth callback back into
// the login entry so the browser recovers through a fresh flow instead of a
// raw JSON error page. The category is logged (redacted, with the request id)
// so production failures can be classified as flow-expiry vs callback-replay.
func (h *Handler) failCallbackToLogin(w http.ResponseWriter, r *http.Request, category string) {
	log.Printf("portal-gateway oauth callback failed request_id=%s category=%s", requestIDOf(w, r), category)
	// Relative Location keeps the browser on its current host and scheme; the
	// login entry defaults return_to to "/" and issues a fresh flow.
	http.Redirect(w, r, "/api/v1/auth/login?return_to="+url.QueryEscape("/"), http.StatusFound)
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
	h.practiceCommand(w, r, http.StatusCreated, true, true, "登录已过期，请重新登录", func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (practice.CommandResult, error) {
		return h.practiceCommands.CreateSession(ctx, actorUserID, requestID, idempotencyKey, raw, anonymousCookie)
	})
}

func (h *Handler) submitPracticeAnswer(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")
	h.practiceCommand(w, r, http.StatusOK, true, true, "登录已过期，请重新登录", func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (practice.CommandResult, error) {
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
// or mock data. An invalid Portal Session is a 401; an absent Portal Session
// is a genuine guest request when guestAllowed, otherwise also a 401.
// readBody=false skips raw-body validation for body-less commands.
func (h *Handler) practiceCommand(w http.ResponseWriter, r *http.Request, successStatus int, guestAllowed, readBody bool, unauthorizedMessage string, command practiceCommand) {
	setPrivateResponseHeaders(w)
	if h.practiceCommands == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "practice_commands_unavailable", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
		return
	}
	actorUserID, anonymousCookie, status, err := h.practiceCommandActor(r, guestAllowed)
	if err != nil {
		writeJSON(w, status, contract.ErrorEnvelope{Error: "not authenticated", Message: unauthorizedMessage, RequestID: requestIDOf(w, r)})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !practice.ValidIdempotencyKey(idempotencyKey) {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "practice_idempotency_key_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
		return
	}
	var raw []byte
	if readBody {
		raw, err = readGatewayPracticeCommandBody(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "practice_command_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
			return
		}
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

// practiceCommandActor resolves the actor for a browser practice command. A
// valid Portal Session cookie yields the signed-in actor; an invalid one is a
// 401. An absent cookie is a genuine guest request when guestAllowed,
// otherwise also a 401 (signed-in-only commands like favorites).
func (h *Handler) practiceCommandActor(r *http.Request, guestAllowed bool) (string, *http.Cookie, int, error) {
	if _, err := r.Cookie(h.browserCookies(r).session); err == nil {
		value, sessionErr := h.readSession(r)
		if sessionErr != nil || !practice.ValidUUID(value.UserID) {
			return "", nil, http.StatusUnauthorized, errors.New("invalid Portal Session")
		}
		return value.UserID, coreAnonymousCookie(r), 0, nil
	} else if !errors.Is(err, http.ErrNoCookie) {
		return "", nil, http.StatusUnauthorized, errors.New("invalid Portal Session")
	}
	if !guestAllowed {
		return "", nil, http.StatusUnauthorized, errors.New("Portal Session required")
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
		h.writePracticeReadPermissionError(w, r, err)
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

// practiceLegacyGone answers the practice read endpoints portal-api used to
// own (banks/schools/lists/leaderboard). ADR-0036 routes every Portal
// practice read to the QuizCraft Core contract instead; a stale caller gets
// an honest 404 with a migration hint, never a mock or legacy fallback.
func (h *Handler) practiceLegacyGone(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, contract.ErrorEnvelope{
		Error:     "not found",
		Message:   "练习目录与排行榜已迁移到新数据源，请刷新页面或升级客户端。",
		RequestID: requestIDOf(w, r),
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
		h.writePracticeReadPermissionError(w, r, err)
		return
	}
	status, err := h.quizCraft.FeedbackStatus(r.Context(), value.UserID, requestIDOf(w, r), feedbackID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "practice feedback status is temporarily unavailable", "反馈状态暂时不可用，请稍后再试")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// favoritesRead runs the shared actor-bound favorites read skeleton: private
// response headers, service gate, session, permission, then the typed read
// and JSON write. It mirrors personalPracticeStats/getPracticeFeedbackStatus
// but stays scoped to favorites so the main-line handlers remain untouched.
func (h *Handler) favoritesRead(w http.ResponseWriter, r *http.Request, read func(userID, requestID string) (any, error)) {
	setPrivateResponseHeaders(w)
	if h.quizCraft == nil {
		writeError(w, r, http.StatusServiceUnavailable, "practice favorites are not enabled", "收藏暂时不可用，请稍后再试")
		return
	}
	value, err := h.readSession(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "not authenticated", "登录已过期，请重新登录")
		return
	}
	if err := h.platform.CheckPermission(r.Context(), value.ExchangeToken, practice.CatalogReadPermission); err != nil {
		h.writePracticeReadPermissionError(w, r, err)
		return
	}
	envelope, err := read(value.UserID, requestIDOf(w, r))
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "practice favorites are temporarily unavailable", "收藏暂时不可用，请稍后再试")
		return
	}
	writeJSON(w, http.StatusOK, envelope)
}

// favoritesOverview lists the signed-in user's per-bank favorite folders.
func (h *Handler) favoritesOverview(w http.ResponseWriter, r *http.Request) {
	h.favoritesRead(w, r, func(userID, requestID string) (any, error) {
		return h.quizCraft.FavoritesOverview(r.Context(), userID, requestID)
	})
}

// favoritesList reads one bank's favorite references for the signed-in user.
func (h *Handler) favoritesList(w http.ResponseWriter, r *http.Request) {
	bankID := chi.URLParam(r, "bank_id")
	h.favoritesRead(w, r, func(userID, requestID string) (any, error) {
		return h.quizCraft.FavoriteList(r.Context(), userID, requestID, bankID)
	})
}

// favoriteQuestion and unfavoriteQuestion are signed-in-only favorites writes.
// Like corrections they never downgrade to a guest actor.
func (h *Handler) favoriteQuestion(w http.ResponseWriter, r *http.Request) {
	h.favoritesCommand(w, r, http.StatusOK, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (practice.CommandResult, error) {
		return h.practiceCommands.FavoriteQuestion(ctx, chi.URLParam(r, "bank_id"), chi.URLParam(r, "question_id"), actorUserID, requestID, idempotencyKey, anonymousCookie)
	})
}

func (h *Handler) unfavoriteQuestion(w http.ResponseWriter, r *http.Request) {
	h.favoritesCommand(w, r, http.StatusOK, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (practice.CommandResult, error) {
		return h.practiceCommands.UnfavoriteQuestion(ctx, chi.URLParam(r, "bank_id"), chi.URLParam(r, "question_id"), actorUserID, requestID, idempotencyKey, anonymousCookie)
	})
}

// createFavoritesSession starts practice from one bank's available favorites.
func (h *Handler) createFavoritesSession(w http.ResponseWriter, r *http.Request) {
	h.favoritesCommand(w, r, http.StatusCreated, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte, anonymousCookie *http.Cookie) (practice.CommandResult, error) {
		return h.practiceCommands.CreateFavoritesSession(ctx, chi.URLParam(r, "bank_id"), actorUserID, requestID, idempotencyKey, anonymousCookie)
	})
}

// favoritesCommand is the signed-in-only favorites write path: it shares the
// practiceCommand skeleton but never downgrades to a guest actor and carries
// no raw browser body. PUT/DELETE mutations answer 200; the POST session
// creation answers 201 like createPracticeSession.
func (h *Handler) favoritesCommand(w http.ResponseWriter, r *http.Request, successStatus int, command practiceCommand) {
	h.practiceCommand(w, r, successStatus, false, false, "请先登录后再操作收藏", command)
}

// writePracticeReadPermissionError maps Platform Core permission outcomes for
// actor-bound practice reads (stats, feedback status, favorites).
func (h *Handler) writePracticeReadPermissionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, platformcore.ErrUnauthorized):
		writeError(w, r, http.StatusUnauthorized, "not authenticated", "登录已过期，请重新登录")
	case errors.Is(err, platformcore.ErrForbidden):
		writeError(w, r, http.StatusForbidden, "practice access denied", "暂无练习权限，请联系管理员")
	default:
		writeError(w, r, http.StatusServiceUnavailable, "practice authorization is temporarily unavailable", "服务暂时不可用，请稍后再来")
	}
}

// --- Food Posts ---

// createFoodPost forwards a signed-in actor's create command. The actor and
// display-name snapshot come exclusively from the verified Portal Session; a
// missing session is a 401, never an anonymous downgrade. The browser body is
// re-signed byte-for-byte with the independent create credential.
func (h *Handler) createFoodPost(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	value, err := h.readSession(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "not authenticated", "登录已过期，请重新登录")
		return
	}
	if h.foodPosts == nil {
		writeError(w, r, http.StatusServiceUnavailable, "food_posts_unavailable", "投稿服务暂时不可用，请稍后再试")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !foodposts.ValidIdempotencyKey(idempotencyKey) {
		writeError(w, r, http.StatusBadRequest, "food_post_idempotency_key_invalid", "请求内容不完整，请检查后重试")
		return
	}
	raw, err := readFoodPostBody(r)
	if err != nil {
		if errors.Is(err, errFoodPostBodyTooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "food_post_body_too_large", "投稿内容过大，请减少或压缩图片后重试")
			return
		}
		writeError(w, r, http.StatusBadRequest, "food_post_invalid", "请求内容不完整，请检查后重试")
		return
	}
	data, err := h.foodPosts.CreatePost(r.Context(), value.UserID, value.DisplayName, requestIDOf(w, r), idempotencyKey, raw)
	if err != nil {
		h.writeFoodPostsFailure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) listFoodPosts(w http.ResponseWriter, r *http.Request) {
	h.foodPostsRead(w, r, func(ctx context.Context, requestID string) (json.RawMessage, error) {
		return h.foodPosts.ListPosts(ctx, requestID, r.URL.RawQuery)
	})
}

func (h *Handler) myFoodPosts(w http.ResponseWriter, r *http.Request) {
	setPrivateResponseHeaders(w)
	value, err := h.readSession(r)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "not authenticated", "登录已过期，请重新登录")
		return
	}
	h.foodPostsRead(w, r, func(ctx context.Context, requestID string) (json.RawMessage, error) {
		return h.foodPosts.MyPosts(ctx, value.UserID, requestID)
	})
}

func (h *Handler) foodPostDetail(w http.ResponseWriter, r *http.Request) {
	h.foodPostsRead(w, r, func(ctx context.Context, requestID string) (json.RawMessage, error) {
		return h.foodPosts.Post(ctx, requestID, chi.URLParam(r, "post_id"))
	})
}

// foodPostImage relays one stored photo's bytes and cache headers. Food's 404
// and error body pass through unchanged, like every other Food Post read.
func (h *Handler) foodPostImage(w http.ResponseWriter, r *http.Request) {
	if h.foodPosts == nil {
		writeError(w, r, http.StatusServiceUnavailable, "food_posts_unavailable", "投稿服务暂时不可用，请稍后再试")
		return
	}
	image, err := h.foodPosts.PostImage(r.Context(), requestIDOf(w, r), chi.URLParam(r, "post_id"), chi.URLParam(r, "position"))
	if err != nil {
		h.writeFoodPostsFailure(w, r, err)
		return
	}
	if image.ContentType != "" {
		w.Header().Set("Content-Type", image.ContentType)
	}
	if image.ETag != "" {
		w.Header().Set("ETag", image.ETag)
	}
	if image.CacheControl != "" {
		w.Header().Set("Cache-Control", image.CacheControl)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image.Bytes)
}

func (h *Handler) foodVenues(w http.ResponseWriter, r *http.Request) {
	h.foodPostsRead(w, r, func(ctx context.Context, requestID string) (json.RawMessage, error) {
		return h.foodPosts.Venues(ctx, requestID, r.URL.RawQuery)
	})
}

// foodPostsRead is the shared public-read skeleton: an unconfigured client is
// an honest 503 and a Food failure is never replaced by the legacy wildcard.
func (h *Handler) foodPostsRead(w http.ResponseWriter, r *http.Request, read func(ctx context.Context, requestID string) (json.RawMessage, error)) {
	if h.foodPosts == nil {
		writeError(w, r, http.StatusServiceUnavailable, "food_posts_unavailable", "投稿服务暂时不可用，请稍后再试")
		return
	}
	data, err := read(r.Context(), requestIDOf(w, r))
	if err != nil {
		h.writeFoodPostsFailure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// writeFoodPostsFailure forwards a Food non-2xx verbatim and otherwise writes
// an honest Gateway error. It never falls back to the Portal API proxy.
func (h *Handler) writeFoodPostsFailure(w http.ResponseWriter, r *http.Request, err error) {
	var upstream *foodposts.UpstreamError
	switch {
	case errors.As(err, &upstream):
		contentType := upstream.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(upstream.StatusCode)
		_, _ = w.Write(upstream.Body)
	case errors.Is(err, foodposts.ErrUnconfigured):
		writeError(w, r, http.StatusServiceUnavailable, "food_posts_unavailable", "投稿服务暂时不可用，请稍后再试")
	case errors.Is(err, foodposts.ErrBadRequest):
		writeError(w, r, http.StatusBadRequest, "food_post_invalid", "请求内容不完整，请检查后重试")
	default:
		writeError(w, r, http.StatusBadGateway, "food_service_unavailable", "服务暂时不可用，请稍后再来")
	}
}

var errFoodPostBodyTooLarge = errors.New("food post body exceeds the gateway cap")

// createCareerSearch forwards a signed-in Lifetime actor's create command.
func (h *Handler) createCareerSearch(w http.ResponseWriter, r *http.Request) {
	h.careerWrite(w, r, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
		return h.career.CreateSearch(ctx, actorUserID, requestID, idempotencyKey, raw)
	})
}

func (h *Handler) listCareerSearches(w http.ResponseWriter, r *http.Request) {
	h.careerRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.career.ListSearches(ctx, actorUserID, requestID)
	})
}

func (h *Handler) careerSearchStatus(w http.ResponseWriter, r *http.Request) {
	searchID := chi.URLParam(r, "search_id")
	h.careerRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.career.Search(ctx, actorUserID, requestID, searchID)
	})
}

func (h *Handler) getCareerProfile(w http.ResponseWriter, r *http.Request) {
	h.careerRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.career.Profile(ctx, actorUserID, requestID)
	})
}

func (h *Handler) updateCareerProfile(w http.ResponseWriter, r *http.Request) {
	h.careerProfileWrite(w, r, func(ctx context.Context, actorUserID, requestID string, raw []byte) (json.RawMessage, error) {
		return h.career.UpdateProfile(ctx, actorUserID, requestID, raw)
	})
}

// createCareerExtraction forwards a signed-in Lifetime actor's resume upload.
// The multipart body is passed through byte-for-byte; Career owns the file
// validation, so the Gateway only enforces the upload-sized body cap.
func (h *Handler) createCareerExtraction(w http.ResponseWriter, r *http.Request) {
	h.careerExtractionUpload(w, r, func(ctx context.Context, actorUserID, requestID string, raw []byte) (json.RawMessage, error) {
		return h.career.CreateExtraction(ctx, actorUserID, requestID, careerUploadFileName(raw, r.Header.Get("Content-Type")), raw)
	})
}

func (h *Handler) careerExtractionStatus(w http.ResponseWriter, r *http.Request) {
	extractionID := chi.URLParam(r, "extraction_id")
	h.careerRead(w, r, func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
		return h.career.Extraction(ctx, actorUserID, requestID, extractionID)
	})
}

// careerRead gates a Career read on a verified Lifetime membership. An
// anonymous caller is a 401; a signed-in free user is a 403; a membership
// dependency failure fails closed (503) rather than downgrading to allow.
func (h *Handler) careerRead(w http.ResponseWriter, r *http.Request, read func(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error)) {
	setPrivateResponseHeaders(w)
	value, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", Message: "登录已过期，请重新登录", RequestID: requestIDOf(w, r)})
		return
	}
	if !h.requireLifetime(w, r, value.UserID) {
		return
	}
	if h.career == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "career_unavailable", Message: "求职雷达服务暂时不可用，请稍后再试", RequestID: requestIDOf(w, r)})
		return
	}
	data, err := read(r.Context(), value.UserID, requestIDOf(w, r))
	if err != nil {
		h.writeCareerFailure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// careerWrite is careerRead plus create-command handling: it requires a
// validated Idempotency-Key and re-signs the browser body byte-for-byte.
func (h *Handler) careerWrite(w http.ResponseWriter, r *http.Request, write func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error)) {
	h.careerWriteNoKey(w, r, false, func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
		return write(ctx, actorUserID, requestID, idempotencyKey, raw)
	})
}

// careerProfileWrite is like careerWrite but does not require an
// Idempotency-Key: a profile PUT is a full replace and is naturally idempotent.
func (h *Handler) careerProfileWrite(w http.ResponseWriter, r *http.Request, write func(ctx context.Context, actorUserID, requestID string, raw []byte) (json.RawMessage, error)) {
	h.careerWriteNoKey(w, r, true, func(ctx context.Context, actorUserID, requestID, _ string, raw []byte) (json.RawMessage, error) {
		return write(ctx, actorUserID, requestID, raw)
	})
}

func (h *Handler) careerWriteNoKey(w http.ResponseWriter, r *http.Request, skipIdempotencyKey bool, write func(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error)) {
	setPrivateResponseHeaders(w)
	value, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", Message: "登录已过期，请重新登录", RequestID: requestIDOf(w, r)})
		return
	}
	if !h.requireLifetime(w, r, value.UserID) {
		return
	}
	if h.career == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "career_unavailable", Message: "求职雷达服务暂时不可用，请稍后再试", RequestID: requestIDOf(w, r)})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !skipIdempotencyKey && !career.ValidIdempotencyKey(idempotencyKey) {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "career_idempotency_key_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 128<<10+1))
	if err != nil || len(raw) == 0 {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "career_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
		return
	}
	if len(raw) > 128<<10 {
		writeJSON(w, http.StatusRequestEntityTooLarge, contract.ErrorEnvelope{Error: "career_body_too_large", Message: "请求内容过大，请精简后重试", RequestID: requestIDOf(w, r)})
		return
	}
	data, err := write(r.Context(), value.UserID, requestIDOf(w, r), idempotencyKey, raw)
	if err != nil {
		h.writeCareerFailure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// careerExtractionUpload gates and forwards one resume upload with the
// extraction-sized body cap, mirroring careerWriteNoKey without an
// Idempotency-Key: each upload is a distinct file, so replays are harmless.
func (h *Handler) careerExtractionUpload(w http.ResponseWriter, r *http.Request, write func(ctx context.Context, actorUserID, requestID string, raw []byte) (json.RawMessage, error)) {
	setPrivateResponseHeaders(w)
	value, err := h.readSession(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, contract.ErrorEnvelope{Error: "not authenticated", Message: "登录已过期，请重新登录", RequestID: requestIDOf(w, r)})
		return
	}
	if !h.requireLifetime(w, r, value.UserID) {
		return
	}
	if h.career == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "career_unavailable", Message: "求职雷达服务暂时不可用，请稍后再试", RequestID: requestIDOf(w, r)})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 11<<20+1))
	if err != nil || len(raw) == 0 {
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "career_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
		return
	}
	if len(raw) > 11<<20 {
		writeJSON(w, http.StatusRequestEntityTooLarge, contract.ErrorEnvelope{Error: "career_body_too_large", Message: "请求内容过大，请精简后重试", RequestID: requestIDOf(w, r)})
		return
	}
	data, err := write(r.Context(), value.UserID, requestIDOf(w, r), raw)
	if err != nil {
		h.writeCareerFailure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// careerUploadFileName reads the original file name from the raw multipart
// body, touching only the part headers (never the file bytes). An absent or
// unparseable name yields an empty string and Career rejects the upload with
// INVALID_FILE.
func careerUploadFileName(raw []byte, contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || params["boundary"] == "" {
		return ""
	}
	reader := multipart.NewReader(bytes.NewReader(raw), params["boundary"])
	for {
		part, err := reader.NextPart()
		if err != nil {
			return ""
		}
		if part.FormName() == "file" {
			return part.FileName()
		}
		_ = part.Close()
	}
}

// requireLifetime checks the current Account Portfolio membership for the
// actor and returns whether they may use Career benefits. It fails closed: a
// membership dependency failure is a 503, never an allow. The authoritative
// check happens server-side on every protected request, even when the Portal
// UI already shows VIP.
func (h *Handler) requireLifetime(w http.ResponseWriter, r *http.Request, actorUserID string) bool {
	if h.accountPortfolio == nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "membership_unavailable", Message: "会员服务暂时不可用，请稍后再试", RequestID: requestIDOf(w, r)})
		return false
	}
	data, err := h.accountPortfolio.Membership(r.Context(), actorUserID, requestIDOf(w, r))
	if err != nil {
		if errors.Is(err, accountportfolio.ErrUnauthorized) || errors.Is(err, accountportfolio.ErrNotFound) {
			// No valid membership for this actor: not a Lifetime member.
			writeJSON(w, http.StatusForbidden, contract.ErrorEnvelope{Error: "lifetime_required", Message: "求职雷达需要 Lifetime VIP 会员", RequestID: requestIDOf(w, r)})
			return false
		}
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "membership_unavailable", Message: "会员服务暂时不可用，请稍后再试", RequestID: requestIDOf(w, r)})
		return false
	}
	// The Account Portfolio client already unwraps the owner's {data,...}
	// envelope, so `data` is the membership payload {plan, lifetime}.
	var membership struct {
		Lifetime bool `json:"lifetime"`
	}
	if err := json.Unmarshal(data, &membership); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "membership_unavailable", Message: "会员服务暂时不可用，请稍后再试", RequestID: requestIDOf(w, r)})
		return false
	}
	if !membership.Lifetime {
		writeJSON(w, http.StatusForbidden, contract.ErrorEnvelope{Error: "lifetime_required", Message: "求职雷达需要 Lifetime VIP 会员", RequestID: requestIDOf(w, r)})
		return false
	}
	return true
}

// writeCareerFailure forwards a Career non-2xx verbatim and otherwise writes
// an honest Gateway error. It never falls back to the Portal API proxy.
func (h *Handler) writeCareerFailure(w http.ResponseWriter, r *http.Request, err error) {
	var upstream *career.UpstreamError
	switch {
	case errors.As(err, &upstream):
		contentType := upstream.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(upstream.StatusCode)
		_, _ = w.Write(upstream.Body)
	case errors.Is(err, career.ErrUnconfigured):
		writeJSON(w, http.StatusServiceUnavailable, contract.ErrorEnvelope{Error: "career_unavailable", Message: "求职雷达服务暂时不可用，请稍后再试", RequestID: requestIDOf(w, r)})
	case errors.Is(err, career.ErrBadRequest):
		writeJSON(w, http.StatusBadRequest, contract.ErrorEnvelope{Error: "career_invalid", Message: "请求内容不完整，请检查后重试", RequestID: requestIDOf(w, r)})
	default:
		writeJSON(w, http.StatusBadGateway, contract.ErrorEnvelope{Error: "career_service_unavailable", Message: "服务暂时不可用，请稍后再来", RequestID: requestIDOf(w, r)})
	}
}

// readFoodPostBody reads the raw create body byte-for-byte, capping it at
// 20MiB (six 2MiB photos plus base64 overhead). It deliberately does not
// parse the JSON: Food owns the shape validation.
func readFoodPostBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, errors.New("food post body is required")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 20<<20+1))
	if err != nil || len(raw) == 0 {
		return nil, errors.New("food post body is invalid")
	}
	if len(raw) > 20<<20 {
		return nil, errFoodPostBodyTooLarge
	}
	return raw, nil
}

// --- Proxy to portal-api ---

func (h *Handler) proxyToPortalAPI(w http.ResponseWriter, r *http.Request) {
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
