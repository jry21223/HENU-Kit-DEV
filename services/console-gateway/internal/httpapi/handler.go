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
	foodapi "henukit.dev/console-gateway/internal/food"
	libraryapi "henukit.dev/console-gateway/internal/library"
	noticeapi "henukit.dev/console-gateway/internal/notice"
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
	CheckNotice(context.Context, string, string) error
	CheckLibrary(context.Context, string, string) error
	CheckFood(context.Context, string, string) error
}

type noticeClient interface {
	Snapshot(context.Context, string) (json.RawMessage, error)
	CreateSource(context.Context, string, string, []byte) (json.RawMessage, error)
	CreateVersion(context.Context, string, string, string, []byte) (json.RawMessage, error)
	Review(context.Context, string, string, string, []byte) (json.RawMessage, error)
	Distribute(context.Context, string, string, string, []byte) (json.RawMessage, error)
	Operation(context.Context, string, string, string) (json.RawMessage, error)
}

type libraryClient interface {
	Workspace(context.Context, string) (json.RawMessage, error)
	Command(context.Context, string, string, []byte) (json.RawMessage, error)
	Operation(context.Context, string, string, string) (json.RawMessage, error)
}

type foodClient interface {
	Workspace(context.Context, string) (json.RawMessage, error)
	Command(context.Context, string, string, []byte) (json.RawMessage, error)
	Operation(context.Context, string, string, string) (json.RawMessage, error)
}

type overviewClient interface {
	Fetch(context.Context, string) contract.ConsoleOverview
}

type Handler struct {
	platformOrigin, clientID, redirectURI string
	platform                              platformClient
	notice                                noticeClient
	library                               libraryClient
	food                                  foodClient
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

func New(platformOrigin, clientID, redirectURI string, platform platformClient, notice noticeClient, overview overviewClient, redisClient *redis.Client, codec *session.Codec, logger *slog.Logger, ownerClients ...any) (http.Handler, error) {
	origin, err := url.Parse(platformOrigin)
	redirect, redirectErr := url.Parse(redirectURI)
	originOK := err == nil && origin.Host != "" && origin.User == nil && (origin.Path == "" || origin.Path == "/") && origin.RawQuery == "" && origin.Fragment == "" && (origin.Scheme == "https" || origin.Hostname() == "localhost" || origin.Hostname() == "127.0.0.1" || origin.Hostname() == "platform-core")
	redirectOK := redirectErr == nil && redirect.Host != "" && (redirect.Scheme == "https" || redirect.Hostname() == "localhost" || redirect.Hostname() == "127.0.0.1")
	if !originOK || !redirectOK || clientID == "" || platform == nil || overview == nil || redisClient == nil || codec == nil {
		return nil, errors.New("invalid console gateway handler configuration")
	}
	if logger == nil {
		logger = slog.Default()
	}
	var library libraryClient
	var food foodClient
	if len(ownerClients) > 0 && ownerClients[0] != nil {
		library, _ = ownerClients[0].(libraryClient)
	}
	if len(ownerClients) > 1 && ownerClients[1] != nil {
		food, _ = ownerClients[1].(foodClient)
	}
	handler := &Handler{platformOrigin: strings.TrimRight(platformOrigin, "/"), clientID: clientID, redirectURI: redirectURI, platform: platform, notice: notice, library: library, food: food, overview: overview, redis: redisClient, codec: codec, logger: logger, now: time.Now}
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
	router.Get(contract.NoticeSnapshotRoute, handler.getNotices)
	router.Post(contract.NoticeSourceRoute, handler.createNoticeSource)
	router.Post(contract.NoticeVersionRoute, handler.createNoticeVersion)
	router.Post(contract.NoticeReviewRoute, handler.reviewNoticeVersion)
	router.Post(contract.NoticeDistributionRoute, handler.distributeNoticeVersion)
	router.Get(contract.NoticeOperationRoute, handler.getNoticeOperation)
	router.Get(contract.LibraryWorkspaceRoute, handler.getLibraryWorkspace)
	router.Post(contract.LibraryCommandRoute, handler.executeLibraryCommand)
	router.Get(contract.LibraryOperationRoute, handler.getLibraryOperation)
	router.Get(contract.FoodWorkspaceRoute, handler.getFoodWorkspace)
	router.Post(contract.FoodCommandRoute, handler.executeFoodCommand)
	router.Get(contract.FoodOperationRoute, handler.getFoodOperation)
	router.Post(contract.LogoutRoute, handler.logout)
	return router, nil
}

func foodPermission(operation string) (string, bool) {
	switch {
	case operation == "submission_approve" || operation == "submission_reject":
		return "food.review", true
	case operation == "anomaly_resolve" || operation == "anomaly_dismiss":
		return "food.anomaly", true
	case operation == "tier_adjustment_confirm" || operation == "tier_adjustment_reject":
		return "food.tier_adjust", true
	default:
		return "", false
	}
}

func (h *Handler) getFoodWorkspace(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeFood(writer, request, "food.read")
	if !ok {
		return
	}
	data, err := h.food.Workspace(foodapi.WithRequestID(request.Context(), requestID(request)), value.UserID)
	h.writeFoodResult(writer, request, data, err)
}

func (h *Handler) executeFoodCommand(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Kind            string          `json:"kind"`
		ResourceID      string          `json:"resource_id"`
		ExpectedVersion int             `json:"expected_version"`
		Payload         json.RawMessage `json:"payload"`
	}
	body, ok := decodeOperationInput(writer, request, &input)
	if !ok {
		return
	}
	permission, valid := foodPermission(input.Kind)
	if !valid {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Food operation is invalid")
		return
	}
	value, ok := h.authorizeFood(writer, request, permission)
	if !ok {
		return
	}
	data, err := h.food.Command(foodapi.WithRequestID(request.Context(), requestID(request)), value.UserID, request.Header.Get("Idempotency-Key"), body)
	h.writeFoodResult(writer, request, data, err)
}

func (h *Handler) getFoodOperation(writer http.ResponseWriter, request *http.Request) {
	operation, key := chi.URLParam(request, "operation"), request.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	permission, valid := foodPermission(operation)
	if !valid {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Food operation is invalid")
		return
	}
	value, ok := h.authorizeFood(writer, request, permission)
	if !ok {
		return
	}
	data, err := h.food.Operation(foodapi.WithRequestID(request.Context(), requestID(request)), value.UserID, operation, key)
	h.writeFoodResult(writer, request, data, err)
}

func (h *Handler) authorizeFood(writer http.ResponseWriter, request *http.Request, permission string) (session.Value, bool) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return session.Value{}, false
	}
	if h.food == nil {
		h.unavailable(writer, request, errors.New("food API is not configured"))
		return session.Value{}, false
	}
	if err := h.platform.CheckFood(request.Context(), value.ExchangeToken, permission); err != nil {
		h.handlePlatformSessionError(writer, request, err)
		return session.Value{}, false
	}
	return value, true
}

func (h *Handler) writeFoodResult(writer http.ResponseWriter, request *http.Request, data json.RawMessage, err error) {
	if err == nil {
		writeJSON(writer, request, http.StatusOK, data)
		return
	}
	switch {
	case errors.Is(err, foodapi.ErrUnauthorized):
		writeError(writer, request, http.StatusUnauthorized, "CONSOLE_SESSION_EXPIRED", "Food rejected the verified actor")
	case errors.Is(err, foodapi.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "ACCESS_DENIED", "Food permission or Scope is missing")
	case errors.Is(err, foodapi.ErrConflict):
		writeError(writer, request, http.StatusConflict, "FOOD_CONFLICT", "Food version or idempotency history conflicts")
	case errors.Is(err, foodapi.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Food request is invalid")
	default:
		h.unavailable(writer, request, err)
	}
}

func (h *Handler) getLibraryWorkspace(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeLibrary(writer, request, "library.read")
	if !ok {
		return
	}
	data, err := h.library.Workspace(libraryapi.WithRequestID(request.Context(), requestID(request)), value.UserID)
	h.writeLibraryResult(writer, request, data, err)
}

func (h *Handler) executeLibraryCommand(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Kind            string          `json:"kind"`
		ResourceID      string          `json:"resource_id,omitempty"`
		ExpectedVersion string          `json:"expected_version,omitempty"`
		Payload         json.RawMessage `json:"payload"`
	}
	body, ok := decodeOperationInput(writer, request, &input)
	if !ok {
		return
	}
	permission := "library.manage"
	if strings.HasPrefix(input.Kind, "submission_") || strings.HasPrefix(input.Kind, "correction_") {
		permission = "library.review"
	}
	value, ok := h.authorizeLibrary(writer, request, permission)
	if !ok {
		return
	}
	data, err := h.library.Command(libraryapi.WithRequestID(request.Context(), requestID(request)), value.UserID, request.Header.Get("Idempotency-Key"), body)
	h.writeLibraryResult(writer, request, data, err)
}

func (h *Handler) getLibraryOperation(writer http.ResponseWriter, request *http.Request) {
	operation := chi.URLParam(request, "operation")
	permission := "library.manage"
	if strings.HasPrefix(operation, "submission_") || strings.HasPrefix(operation, "correction_") {
		permission = "library.review"
	}
	value, ok := h.authorizeLibrary(writer, request, permission)
	if !ok {
		return
	}
	key := request.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	data, err := h.library.Operation(libraryapi.WithRequestID(request.Context(), requestID(request)), value.UserID, operation, key)
	h.writeLibraryResult(writer, request, data, err)
}

func (h *Handler) authorizeLibrary(writer http.ResponseWriter, request *http.Request, permission string) (session.Value, bool) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return session.Value{}, false
	}
	if h.library == nil {
		h.unavailable(writer, request, errors.New("library API is not configured"))
		return session.Value{}, false
	}
	if err := h.platform.CheckLibrary(request.Context(), value.ExchangeToken, permission); err != nil {
		h.handlePlatformSessionError(writer, request, err)
		return session.Value{}, false
	}
	return value, true
}

func (h *Handler) writeLibraryResult(writer http.ResponseWriter, request *http.Request, data json.RawMessage, err error) {
	if err == nil {
		writeJSON(writer, request, http.StatusOK, data)
		return
	}
	switch {
	case errors.Is(err, libraryapi.ErrUnauthorized):
		writeError(writer, request, http.StatusUnauthorized, "CONSOLE_SESSION_EXPIRED", "Library rejected the verified actor")
	case errors.Is(err, libraryapi.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "ACCESS_DENIED", "Library permission or Scope is missing")
	case errors.Is(err, libraryapi.ErrConflict):
		writeError(writer, request, http.StatusConflict, "LIBRARY_CONFLICT", "Library version or idempotency history conflicts")
	case errors.Is(err, libraryapi.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Library request is invalid")
	default:
		h.unavailable(writer, request, err)
	}
}

func (h *Handler) getNotices(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeNotice(writer, request, "notice.read")
	if !ok {
		return
	}
	data, err := h.notice.Snapshot(noticeapi.WithRequestID(request.Context(), requestID(request)), value.UserID)
	h.writeNoticeResult(writer, request, data, err)
}

func (h *Handler) createNoticeSource(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeNotice(writer, request, "notice.manage")
	if !ok {
		return
	}
	var input map[string]any
	body, ok := decodeOperationInput(writer, request, &input)
	if !ok {
		return
	}
	data, err := h.notice.CreateSource(noticeapi.WithRequestID(request.Context(), requestID(request)), value.UserID, request.Header.Get("Idempotency-Key"), body)
	h.writeNoticeResult(writer, request, data, err)
}

func (h *Handler) createNoticeVersion(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeNotice(writer, request, "notice.manage")
	if !ok {
		return
	}
	var input map[string]any
	body, ok := decodeOperationInput(writer, request, &input)
	if !ok {
		return
	}
	data, err := h.notice.CreateVersion(noticeapi.WithRequestID(request.Context(), requestID(request)), value.UserID, chi.URLParam(request, "source_id"), request.Header.Get("Idempotency-Key"), body)
	h.writeNoticeResult(writer, request, data, err)
}

func (h *Handler) reviewNoticeVersion(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeNotice(writer, request, "notice.review")
	if !ok {
		return
	}
	var input map[string]any
	body, ok := decodeOperationInput(writer, request, &input)
	if !ok {
		return
	}
	data, err := h.notice.Review(noticeapi.WithRequestID(request.Context(), requestID(request)), value.UserID, chi.URLParam(request, "version_id"), request.Header.Get("Idempotency-Key"), body)
	h.writeNoticeResult(writer, request, data, err)
}

func (h *Handler) distributeNoticeVersion(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeNotice(writer, request, "notice.distribute")
	if !ok {
		return
	}
	var input map[string]any
	body, ok := decodeOperationInput(writer, request, &input)
	if !ok {
		return
	}
	data, err := h.notice.Distribute(noticeapi.WithRequestID(request.Context(), requestID(request)), value.UserID, chi.URLParam(request, "version_id"), request.Header.Get("Idempotency-Key"), body)
	h.writeNoticeResult(writer, request, data, err)
}

func (h *Handler) getNoticeOperation(writer http.ResponseWriter, request *http.Request) {
	value, ok := h.authorizeNotice(writer, request, "notice.read")
	if !ok {
		return
	}
	key := request.Header.Get("Idempotency-Key")
	if len(key) < 8 || len(key) > 200 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	data, err := h.notice.Operation(noticeapi.WithRequestID(request.Context(), requestID(request)), value.UserID, chi.URLParam(request, "operation"), key)
	h.writeNoticeResult(writer, request, data, err)
}

func (h *Handler) authorizeNotice(writer http.ResponseWriter, request *http.Request, permission string) (session.Value, bool) {
	value, ok := h.readSession(writer, request)
	if !ok {
		return session.Value{}, false
	}
	if h.notice == nil {
		h.unavailable(writer, request, errors.New("notice API is not configured"))
		return session.Value{}, false
	}
	if err := h.platform.CheckNotice(request.Context(), value.ExchangeToken, permission); err != nil {
		h.handlePlatformSessionError(writer, request, err)
		return session.Value{}, false
	}
	return value, true
}

func (h *Handler) writeNoticeResult(writer http.ResponseWriter, request *http.Request, data json.RawMessage, err error) {
	if err == nil {
		writeJSON(writer, request, http.StatusOK, data)
		return
	}
	switch {
	case errors.Is(err, noticeapi.ErrUnauthorized):
		writeError(writer, request, http.StatusUnauthorized, "CONSOLE_SESSION_EXPIRED", "Notice rejected the verified actor")
	case errors.Is(err, noticeapi.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "ACCESS_DENIED", "Notice permission or Scope is missing")
	case errors.Is(err, noticeapi.ErrConflict):
		writeError(writer, request, http.StatusConflict, "NOTICE_CONFLICT", "Notice state, revision, or idempotency history conflicts")
	case errors.Is(err, noticeapi.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "NOTICE_NOT_FOUND", "Notice resource was not found")
	case errors.Is(err, noticeapi.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Notice request is invalid")
	default:
		h.unavailable(writer, request, err)
	}
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
	noticeErr := h.platform.CheckNotice(request.Context(), value.ExchangeToken, "notice.read")
	noticeManageErr := h.platform.CheckNotice(request.Context(), value.ExchangeToken, "notice.manage")
	noticeReviewErr := h.platform.CheckNotice(request.Context(), value.ExchangeToken, "notice.review")
	noticeDistributeErr := h.platform.CheckNotice(request.Context(), value.ExchangeToken, "notice.distribute")
	libraryReadErr, libraryManageErr, libraryReviewErr := error(platformcore.ErrForbidden), error(platformcore.ErrForbidden), error(platformcore.ErrForbidden)
	if h.library != nil {
		libraryReadErr = h.platform.CheckLibrary(request.Context(), value.ExchangeToken, "library.read")
		libraryManageErr = h.platform.CheckLibrary(request.Context(), value.ExchangeToken, "library.manage")
		libraryReviewErr = h.platform.CheckLibrary(request.Context(), value.ExchangeToken, "library.review")
	}
	foodReadErr, foodReviewErr, foodAnomalyErr, foodTierErr := error(platformcore.ErrForbidden), error(platformcore.ErrForbidden), error(platformcore.ErrForbidden), error(platformcore.ErrForbidden)
	if h.food != nil {
		foodReadErr = h.platform.CheckFood(request.Context(), value.ExchangeToken, "food.read")
		foodReviewErr = h.platform.CheckFood(request.Context(), value.ExchangeToken, "food.review")
		foodAnomalyErr = h.platform.CheckFood(request.Context(), value.ExchangeToken, "food.anomaly")
		foodTierErr = h.platform.CheckFood(request.Context(), value.ExchangeToken, "food.tier_adjust")
	}
	if errors.Is(overviewErr, platformcore.ErrUnauthorized) || errors.Is(operationsErr, platformcore.ErrUnauthorized) || errors.Is(noticeErr, platformcore.ErrUnauthorized) || errors.Is(noticeManageErr, platformcore.ErrUnauthorized) || errors.Is(noticeReviewErr, platformcore.ErrUnauthorized) || errors.Is(noticeDistributeErr, platformcore.ErrUnauthorized) || errors.Is(libraryReadErr, platformcore.ErrUnauthorized) || errors.Is(libraryManageErr, platformcore.ErrUnauthorized) || errors.Is(libraryReviewErr, platformcore.ErrUnauthorized) || errors.Is(foodReadErr, platformcore.ErrUnauthorized) || errors.Is(foodReviewErr, platformcore.ErrUnauthorized) || errors.Is(foodAnomalyErr, platformcore.ErrUnauthorized) || errors.Is(foodTierErr, platformcore.ErrUnauthorized) {
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
	if noticeErr == nil {
		permissions = append(permissions, "notice.read")
	}
	if noticeManageErr == nil {
		permissions = append(permissions, "notice.manage")
	}
	if noticeReviewErr == nil {
		permissions = append(permissions, "notice.review")
	}
	if noticeDistributeErr == nil {
		permissions = append(permissions, "notice.distribute")
	}
	if libraryReadErr == nil {
		permissions = append(permissions, "library.read")
	}
	if libraryManageErr == nil {
		permissions = append(permissions, "library.manage")
	}
	if libraryReviewErr == nil {
		permissions = append(permissions, "library.review")
	}
	if foodReadErr == nil {
		permissions = append(permissions, "food.read")
	}
	if foodReviewErr == nil {
		permissions = append(permissions, "food.review")
	}
	if foodAnomalyErr == nil {
		permissions = append(permissions, "food.anomaly")
	}
	if foodTierErr == nil {
		permissions = append(permissions, "food.tier_adjust")
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
	if noticeErr == nil {
		noticeProduct := "notice"
		response.AccessContext.Scopes = append(response.AccessContext.Scopes, contract.ConsoleScope{Kind: "product", ProductCode: &noticeProduct})
	}
	if libraryReadErr == nil {
		libraryProduct := "library"
		response.AccessContext.Scopes = append(response.AccessContext.Scopes, contract.ConsoleScope{Kind: "product", ProductCode: &libraryProduct})
	}
	if foodReadErr == nil {
		foodProduct := "food"
		response.AccessContext.Scopes = append(response.AccessContext.Scopes, contract.ConsoleScope{Kind: "product", ProductCode: &foodProduct})
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
