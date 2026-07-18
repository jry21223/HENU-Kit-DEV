package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/platform-core/internal/identity"
)

type Handler struct {
	flow       *identity.Service
	database   *pgxpool.Pool
	redis      *redis.Client
	cookieName string
}

func New(flow *identity.Service, database *pgxpool.Pool, redisClient *redis.Client, cookieName string) http.Handler {
	handler := &Handler{flow: flow, database: database, redis: redisClient, cookieName: cookieName}
	router := chi.NewRouter()
	router.Get("/api/v1/healthz", handler.health)
	router.Get("/api/v1/readyz", handler.ready)
	router.Get("/api/v1/oauth/authorize", handler.authorize)
	router.Post("/api/v1/oauth/token", handler.exchange)
	return router
}

func (h *Handler) health(writer http.ResponseWriter, _ *http.Request) {
	writeSuccess(writer, http.StatusOK, map[string]bool{"alive": true})
}

func (h *Handler) ready(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	if err := h.database.Ping(ctx); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	if err := h.redis.Ping(ctx).Err(); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	writeSuccess(writer, http.StatusOK, map[string]bool{"ready": true})
}

func (h *Handler) authorize(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	if query.Get("response_type") != "code" || query.Get("code_challenge_method") != "S256" || len(query.Get("state")) < 8 {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
		return
	}
	callback, err := url.Parse(query.Get("redirect_uri"))
	if err != nil || callback.Scheme != "https" || callback.Host == "" || callback.User != nil || callback.Fragment != "" {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
		return
	}
	cookie, err := request.Cookie(h.cookieName)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "SESSION_REQUIRED", "a valid Core Session is required")
		return
	}
	authorization, err := h.flow.Authorize(request.Context(), identity.AuthorizeInput{
		CoreSessionToken: cookie.Value,
		ClientID:         query.Get("client_id"), RedirectURI: query.Get("redirect_uri"), CodeChallenge: query.Get("code_challenge"),
	})
	if err != nil {
		h.writeFlowError(writer, err)
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: h.cookieName, Value: cookie.Value, Path: "/", Expires: authorization.SessionExpires,
		MaxAge: max(1, int(time.Until(authorization.SessionExpires).Seconds())), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
	callbackQuery := callback.Query()
	callbackQuery.Set("code", authorization.Code)
	callbackQuery.Set("state", query.Get("state"))
	callback.RawQuery = callbackQuery.Encode()
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Pragma", "no-cache")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(writer, request, callback.String(), http.StatusFound)
}

type exchangeRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	RedirectURI  string `json:"redirect_uri"`
	ClientID     string `json:"client_id"`
	CodeVerifier string `json:"code_verifier"`
}

func (h *Handler) exchange(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	var body exchangeRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.GrantType != "authorization_code" {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", "token request is invalid")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", "token request is invalid")
		return
	}
	basicClientID, clientSecret, ok := request.BasicAuth()
	if !ok || basicClientID != body.ClientID {
		writeError(writer, http.StatusUnauthorized, "CLIENT_AUTH_FAILED", "client authentication failed")
		return
	}
	exchange, err := h.flow.Exchange(request.Context(), identity.ExchangeInput{
		ClientID: body.ClientID, ClientSecret: clientSecret, Code: body.Code,
		RedirectURI: body.RedirectURI, CodeVerifier: body.CodeVerifier,
	})
	if err != nil {
		h.writeFlowError(writer, err)
		return
	}
	writeSuccess(writer, http.StatusOK, map[string]any{
		"user": map[string]any{
			"user_id": exchange.UserID, "email_verified": exchange.EmailVerified,
			"status": exchange.UserStatus, "created_at": exchange.UserCreatedAt,
		},
		"session_exchange_token": exchange.SessionExchangeToken,
		"expires_at":             exchange.ExpiresAt,
	})
}

func (h *Handler) writeFlowError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrUnauthorized):
		writeError(writer, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "authentication failed")
	case errors.Is(err, identity.ErrCallback):
		writeError(writer, http.StatusBadRequest, "CALLBACK_NOT_REGISTERED", "callback is not registered")
	case errors.Is(err, identity.ErrCodeUsed):
		writeError(writer, http.StatusConflict, "AUTH_CODE_ALREADY_USED", "authorization code was already used")
	case errors.Is(err, identity.ErrCodeBusy):
		writeError(writer, http.StatusConflict, "AUTH_CODE_IN_USE", "authorization code exchange is already in progress")
	case errors.Is(err, identity.ErrCodeExpired):
		writeError(writer, http.StatusBadRequest, "AUTH_CODE_EXPIRED", "authorization code expired")
	case errors.Is(err, identity.ErrInvalid):
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
	case errors.Is(err, identity.ErrDependency):
		writeError(writer, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
	default:
		writeError(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func writeSuccess(writer http.ResponseWriter, status int, data any) {
	writeJSON(writer, status, map[string]any{"data": data, "request_id": requestID()})
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, map[string]any{"error": map[string]string{"code": code, "message": message}, "request_id": requestID()})
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func requestID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "req_unavailable"
	}
	return "req_" + strings.TrimRight(base64.RawURLEncoding.EncodeToString(bytes), "=")
}
