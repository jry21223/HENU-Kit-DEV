package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/contract"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

const (
	sessionCookie   = "__Host-henukit_console_session"
	oauthFlowCookie = "__Host-henukit_console_oauth"
	stateTTL        = 5 * time.Minute
)

type platformClient interface {
	ExchangeCode(context.Context, string, string, string, string) (platformcore.Exchange, error)
	CheckOverview(context.Context, string) error
	CheckPlatformOperations(context.Context, string) error
	CheckPlatformOperationsWrite(context.Context, string) error
	PlatformOperations(context.Context, string) (json.RawMessage, error)
	RevokeSession(context.Context, string, string, string, []byte) (json.RawMessage, error)
	UpdateAccess(context.Context, string, string, string, []byte) (json.RawMessage, error)
	OperationStatus(context.Context, string, string, string) (json.RawMessage, error)
}

type overviewClient interface {
	Fetch(context.Context, string) contract.ConsoleOverview
}

type Handler struct {
	platformOrigin, clientID, redirectURI string
	platform                              platformClient
	overview                              overviewClient
	redis                                 *redis.Client
	codec                                 *session.Codec
	logger                                *slog.Logger
	now                                   func() time.Time
}

type flowState struct {
	Verifier string `json:"verifier"`
	ReturnTo string `json:"return_to"`
}

func New(platformOrigin, clientID, redirectURI string, platform platformClient, overview overviewClient, redisClient *redis.Client, codec *session.Codec, logger *slog.Logger) (http.Handler, error) {
	origin, err := url.Parse(platformOrigin)
	redirect, redirectErr := url.Parse(redirectURI)
	if err != nil || redirectErr != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" || redirect.Scheme != "https" || redirect.Host == "" || clientID == "" || platform == nil || overview == nil || redisClient == nil || codec == nil {
		return nil, errors.New("invalid console gateway handler configuration")
	}
	if logger == nil {
		logger = slog.Default()
	}
	handler := &Handler{platformOrigin: strings.TrimRight(platformOrigin, "/"), clientID: clientID, redirectURI: redirectURI, platform: platform, overview: overview, redis: redisClient, codec: codec, logger: logger, now: time.Now}
	router := chi.NewRouter()
	router.Use(handler.requestContext)
	router.Use(securityHeaders)
	router.Get(contract.HealthRoute, handler.health)
	router.Get(contract.LoginRoute, handler.login)
	router.Get(contract.CallbackRoute, handler.callback)
	router.Get(contract.SessionRoute, handler.getSession)
	router.Get(contract.OverviewRoute, handler.getOverview)
	router.Get(contract.OperationsRoute, handler.getPlatformOperations)
	router.Post(contract.RevokeSessionRoute, handler.revokePlatformSession)
	router.Post(contract.UpdateAccessRoute, handler.updatePlatformAccess)
	router.Get(contract.OperationStatusRoute, handler.getPlatformOperationStatus)
	router.Post(contract.LogoutRoute, handler.logout)
	return router, nil
}

func (h *Handler) getPlatformOperations(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return
	}
	data, err := h.platform.PlatformOperations(request.Context(), value.ExchangeToken)
	if err != nil {
		h.handlePlatformSessionError(writer, request, err)
		return
	}
	var snapshot contract.PlatformOperationsSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		h.unavailable(writer, request, err)
		return
	}
	permissions := []string{"platform.operations.read"}
	if err := h.platform.CheckPlatformOperationsWrite(request.Context(), value.ExchangeToken); err == nil {
		permissions = append(permissions, "platform.operations.write")
	} else if !errors.Is(err, platformcore.ErrForbidden) {
		h.handlePlatformSessionError(writer, request, err)
		return
	}
	snapshot.AccessContext = contract.ConsoleAccessContext{Permissions: permissions, Scopes: []contract.ConsoleScope{{Kind: "platform"}}, VerifiedAt: h.now().UTC()}
	writeJSON(writer, request, http.StatusOK, snapshot)
}

func (h *Handler) revokePlatformSession(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return
	}
	var input contract.RevokePlatformSessionRequest
	body, ok := decodeOperationInput(writer, request, &input)
	if !ok || !input.ExpectedActive {
		if ok {
			writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "expected_active must be true")
		}
		return
	}
	data, err := h.platform.RevokeSession(request.Context(), value.ExchangeToken, chi.URLParam(request, "session_id"), request.Header.Get("Idempotency-Key"), body)
	if err != nil {
		h.handlePlatformSessionError(writer, request, err)
		return
	}
	writeJSON(writer, request, http.StatusOK, data)
}

func (h *Handler) updatePlatformAccess(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return
	}
	var input contract.UpdatePlatformAccessRequest
	body, ok := decodeOperationInput(writer, request, &input)
	if !ok {
		return
	}
	data, err := h.platform.UpdateAccess(request.Context(), value.ExchangeToken, chi.URLParam(request, "user_id"), request.Header.Get("Idempotency-Key"), body)
	if err != nil {
		h.handlePlatformSessionError(writer, request, err)
		return
	}
	writeJSON(writer, request, http.StatusOK, data)
}

func (h *Handler) getPlatformOperationStatus(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return
	}
	key := request.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	data, err := h.platform.OperationStatus(request.Context(), value.ExchangeToken, chi.URLParam(request, "operation"), key)
	if err != nil {
		h.handlePlatformSessionError(writer, request, err)
		return
	}
	writeJSON(writer, request, http.StatusOK, data)
}

func decodeOperationInput(writer http.ResponseWriter, request *http.Request, target any) ([]byte, bool) {
	key := request.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return nil, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return nil, false
	}
	body, err := json.Marshal(target)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return nil, false
	}
	return body, true
}

func (h *Handler) handlePlatformSessionError(writer http.ResponseWriter, request *http.Request, err error) {
	if errors.Is(err, platformcore.ErrUnauthorized) {
		h.clearSession(writer)
	}
	h.writePlatformError(writer, request, err)
}

func (h *Handler) getOverview(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return
	}
	if err := h.platform.CheckOverview(request.Context(), value.ExchangeToken); err != nil {
		if errors.Is(err, platformcore.ErrUnauthorized) {
			h.clearSession(writer)
		}
		h.writePlatformError(writer, request, err)
		return
	}
	writeJSON(writer, request, http.StatusOK, h.overview.Fetch(request.Context(), requestID(request)))
}

func (h *Handler) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get("X-Request-Id")
		if len(requestID) > 100 || !requestIDPattern.MatchString(requestID) {
			requestID = "req_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		}
		writer.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), requestIDKey{}, requestID)))
	})
}

type requestIDKey struct{}

var requestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)

func (h *Handler) health(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, request, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) login(writer http.ResponseWriter, request *http.Request) {
	returnTo := request.URL.Query().Get("return_to")
	if returnTo == "" {
		returnTo = "/"
	}
	if !validReturnTo(returnTo) {
		writeError(writer, request, http.StatusBadRequest, "INVALID_RETURN_TO", "return_to must be a same-origin path")
		return
	}
	state, err := randomToken(32)
	if err != nil {
		h.unavailable(writer, request, err)
		return
	}
	verifier, err := randomToken(32)
	if err != nil {
		h.unavailable(writer, request, err)
		return
	}
	browserNonce, err := randomToken(32)
	if err != nil {
		h.unavailable(writer, request, err)
		return
	}
	payload, _ := json.Marshal(flowState{Verifier: verifier, ReturnTo: returnTo})
	stateHash := sha256.Sum256([]byte(state))
	browserHash := sha256.Sum256([]byte(browserNonce))
	stored, err := h.redis.SetNX(request.Context(), oauthStateKey(stateHash, browserHash), payload, stateTTL).Result()
	if err != nil || !stored {
		if err == nil {
			err = errors.New("oauth state collision")
		}
		h.unavailable(writer, request, err)
		return
	}
	http.SetCookie(writer, &http.Cookie{Name: oauthFlowCookie, Value: browserNonce, Path: "/", MaxAge: int(stateTTL.Seconds()), Expires: h.now().Add(stateTTL), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"response_type": {"code"}, "client_id": {h.clientID}, "redirect_uri": {h.redirectURI}, "state": {state},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"},
	}
	http.Redirect(writer, request, h.platformOrigin+"/api/v1/oauth/authorize?"+query.Encode(), http.StatusFound)
}

func (h *Handler) callback(writer http.ResponseWriter, request *http.Request) {
	code, state := request.URL.Query().Get("code"), request.URL.Query().Get("state")
	if len(code) < 16 || len(state) < 32 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_CALLBACK", "authorization callback is invalid")
		return
	}
	flowCookie, cookieErr := request.Cookie(oauthFlowCookie)
	if cookieErr != nil || len(flowCookie.Value) != 43 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_OAUTH_STATE", "authorization state is not bound to this browser")
		return
	}
	stateHash := sha256.Sum256([]byte(state))
	browserHash := sha256.Sum256([]byte(flowCookie.Value))
	payload, err := h.redis.GetDel(request.Context(), oauthStateKey(stateHash, browserHash)).Bytes()
	if errors.Is(err, redis.Nil) {
		writeError(writer, request, http.StatusBadRequest, "INVALID_OAUTH_STATE", "authorization state is invalid or already used")
		return
	}
	if err != nil {
		h.unavailable(writer, request, err)
		return
	}
	h.clearOAuthFlow(writer)
	var flow flowState
	if err := json.Unmarshal(payload, &flow); err != nil || !validReturnTo(flow.ReturnTo) || len(flow.Verifier) != 43 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_OAUTH_STATE", "authorization state is invalid")
		return
	}
	exchange, err := h.platform.ExchangeCode(request.Context(), code, h.redirectURI, flow.Verifier, "idem_console_"+hex.EncodeToString(stateHash[:16]))
	if err != nil {
		h.writeOAuthPlatformError(writer, request, err)
		return
	}
	encoded, err := h.codec.Encode(session.Value{UserID: exchange.UserID, ExchangeToken: exchange.ExchangeToken, ExpiresAt: exchange.ExpiresAt})
	if err != nil {
		h.unavailable(writer, request, err)
		return
	}
	maxAge := int(exchange.ExpiresAt.Sub(h.now()).Seconds())
	if maxAge < 1 {
		writeError(writer, request, http.StatusUnauthorized, "SESSION_EXPIRED", "Platform Session has expired")
		return
	}
	http.SetCookie(writer, &http.Cookie{Name: sessionCookie, Value: encoded, Path: "/", MaxAge: maxAge, Expires: exchange.ExpiresAt, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(writer, request, flow.ReturnTo, http.StatusFound)
}

func (h *Handler) getSession(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return
	}
	overviewErr := h.platform.CheckOverview(request.Context(), value.ExchangeToken)
	operationsErr := h.platform.CheckPlatformOperations(request.Context(), value.ExchangeToken)
	if errors.Is(overviewErr, platformcore.ErrUnauthorized) || errors.Is(operationsErr, platformcore.ErrUnauthorized) {
		h.clearSession(writer)
		h.writePlatformError(writer, request, platformcore.ErrUnauthorized)
		return
	}
	permissions := make([]string, 0, 2)
	if overviewErr == nil {
		permissions = append(permissions, "console.overview.read")
	}
	if operationsErr == nil {
		permissions = append(permissions, "platform.operations.read")
	}
	if len(permissions) == 0 {
		err := overviewErr
		if errors.Is(err, platformcore.ErrForbidden) {
			err = operationsErr
		}
		if errors.Is(err, platformcore.ErrUnauthorized) {
			h.clearSession(writer)
		}
		h.writePlatformError(writer, request, err)
		return
	}
	response := contract.ConsoleSession{
		AccessContext: contract.ConsoleAccessContext{
			Permissions: permissions, Scopes: []contract.ConsoleScope{{Kind: "platform"}}, VerifiedAt: h.now().UTC(),
		},
		ExpiresAt: value.ExpiresAt,
	}
	response.User.ID = value.UserID
	writeJSON(writer, request, http.StatusOK, response)
}

func (h *Handler) logout(writer http.ResponseWriter, _ *http.Request) {
	h.clearSession(writer)
	writer.WriteHeader(http.StatusNoContent)
}

func (h *Handler) readSession(writer http.ResponseWriter, request *http.Request) (session.Value, bool) {
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		writeError(writer, request, http.StatusUnauthorized, "CONSOLE_SESSION_REQUIRED", "Console Session is required")
		return session.Value{}, false
	}
	value, err := h.codec.Decode(cookie.Value)
	if err != nil || !h.now().Before(value.ExpiresAt) {
		h.clearSession(writer)
		writeError(writer, request, http.StatusUnauthorized, "CONSOLE_SESSION_EXPIRED", "Console Session is invalid or expired")
		return session.Value{}, false
	}
	return value, true
}

func (h *Handler) clearSession(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

func (h *Handler) clearOAuthFlow(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{Name: oauthFlowCookie, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

func oauthStateKey(stateHash, browserHash [sha256.Size]byte) string {
	return "console:oauth-state:" + hex.EncodeToString(stateHash[:]) + ":" + hex.EncodeToString(browserHash[:])
}

func (h *Handler) writePlatformError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, platformcore.ErrUnauthorized):
		writeError(writer, request, http.StatusUnauthorized, "CONSOLE_SESSION_EXPIRED", "Console Session is no longer authorized")
	case errors.Is(err, platformcore.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "ACCESS_DENIED", "Required permission or Scope is missing")
	case errors.Is(err, platformcore.ErrConflict):
		writeError(writer, request, http.StatusConflict, "PLATFORM_OPERATION_CONFLICT", "Platform Operation conflicts with current state or idempotency history")
	case errors.Is(err, platformcore.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "PLATFORM_RESOURCE_NOT_FOUND", "Platform resource was not found")
	case errors.Is(err, platformcore.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Platform Operation request is invalid")
	default:
		h.unavailable(writer, request, err)
	}
}

func (h *Handler) writeOAuthPlatformError(writer http.ResponseWriter, request *http.Request, err error) {
	if errors.Is(err, platformcore.ErrConflict) {
		writeError(writer, request, http.StatusConflict, "AUTHORIZATION_CONFLICT", "Authorization code could not be consumed")
		return
	}
	h.writePlatformError(writer, request, err)
}

func (h *Handler) unavailable(writer http.ResponseWriter, request *http.Request, err error) {
	h.logger.Error("console_gateway_dependency_error", "request_id", requestID(request), "error", err)
	writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Console authentication dependency is unavailable")
}

func validReturnTo(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && !strings.ContainsAny(value, "\\\r\n") && !parsed.IsAbs() && parsed.Host == ""
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}

func randomToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func requestID(request *http.Request) string {
	value, _ := request.Context().Value(requestIDKey{}).(string)
	return value
}

func writeJSON(writer http.ResponseWriter, request *http.Request, status int, data any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"data": data, "request_id": requestID(request)})
}

func writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}, "request_id": requestID(request)})
}
