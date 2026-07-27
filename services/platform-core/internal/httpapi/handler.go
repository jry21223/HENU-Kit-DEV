package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/platform-core/internal/contract"
	"henukit.dev/platform-core/internal/identity"
	"henukit.dev/platform-core/internal/operationsinbox"
	"henukit.dev/platform-core/internal/platformoperations"
	"henukit.dev/platform-core/internal/store"
	"henukit.dev/platform-core/internal/verification"
)

type Handler struct {
	publicPathPrefix string
	flow             *identity.Service
	verification     *verification.Service
	inbox            *operationsinbox.Service
	platformOps      *platformoperations.Service
	queries          *store.Queries
	database         *pgxpool.Pool
	redis            *redis.Client
	cookieName       string
	localCookieName  string
	logger           *slog.Logger
	deliveryKeys     map[string][]byte
	deviceKey        []byte
	trustedProxies   []*net.IPNet
}

type browserCookieProfile struct {
	core, csrf, device string
	secure             bool
}

const explicitFormResponseHeader = "X-Henukit-Form-Response"

func New(flow *identity.Service, verificationFlow *verification.Service, inbox *operationsinbox.Service, platformOps *platformoperations.Service, queries *store.Queries, database *pgxpool.Pool, redisClient *redis.Client, cookieName, localCookieName string, deliveryKeys map[string][]byte, deviceKey []byte, trustedProxies []*net.IPNet, logger *slog.Logger) http.Handler {
	handler := &Handler{publicPathPrefix: strings.TrimRight(os.Getenv("PLATFORM_CORE_PUBLIC_PATH_PREFIX"), "/"), flow: flow, verification: verificationFlow, inbox: inbox, platformOps: platformOps, queries: queries, database: database, redis: redisClient, cookieName: cookieName, localCookieName: localCookieName, deliveryKeys: deliveryKeys, deviceKey: deviceKey, trustedProxies: trustedProxies, logger: logger}
	router := chi.NewRouter()
	router.Use(handler.requestAudit)
	router.Get("/api/v1/healthz", handler.health)
	router.Get("/api/v1/readyz", handler.ready)
	router.Get("/login", handler.loginPage)
	router.Post("/login/code", handler.loginRequestCode)
	router.Post("/login/verify", handler.loginVerifyCode)
	router.Post("/login/password", handler.loginPassword)
	router.Get("/register", handler.registerPage)
	router.Post("/register/code", handler.registerRequestCode)
	router.Post("/register", handler.registerAccount)
	router.Get("/recover", handler.recoverPage)
	router.Post("/recover/code", handler.recoverRequestCode)
	router.Post("/recover", handler.recoverPassword)
	router.Get("/account/security", handler.securityPage)
	router.Post("/account/security/code", handler.securityRequestCode)
	router.Post("/account/security/password", handler.securityChangePassword)
	router.Get(contract.AuthorizeRoute, handler.authorize)
	router.Post(contract.TokenRoute, handler.exchange)
	router.Post(contract.AuthorizationCheckRoute, handler.checkAuthorization)
	router.Post(contract.RequestVerificationCodeRoute, handler.requestVerificationCode)
	router.Post(contract.VerifyVerificationCodeRoute, handler.verifyVerificationCode)
	router.Post("/api/v1/sessions/revoke", handler.revokeCurrentSession)
	router.Post(contract.RecordMailDeliveryRoute, handler.recordMailDelivery)
	router.Get(contract.ListOperationsInboxRoute, handler.listOperationsInboxItems)
	router.Get(contract.GetOperationsInboxRoute, handler.getOperationsInboxItem)
	router.Post(contract.CreateOperationsInboxRoute, handler.createOperationsInboxItem)
	router.Post(contract.UpdateOperationsInboxRoute, handler.updateOperationsInboxItem)
	router.Get(contract.OperationsInboxOperationStatusRoute, handler.getOperationsInboxOperationStatus)
	router.Get(contract.PlatformOperationsRoute, handler.getPlatformOperations)
	router.Post(contract.RevokePlatformOperationSessionRoute, handler.revokePlatformOperationSession)
	router.Post(contract.UpdatePlatformOperationAccessRoute, handler.updatePlatformOperationAccess)
	router.Get(contract.PlatformOperationStatusRoute, handler.getPlatformOperationStatus)
	return router
}

func (h *Handler) revokeCurrentSession(writer http.ResponseWriter, request *http.Request) {
	if !h.sameOriginBrowserRequest(request) {
		writeError(writer, request, http.StatusForbidden, "ORIGIN_REJECTED", "session revocation origin is not allowed")
		return
	}
	var body struct {
		AllSessions bool   `json:"all_sessions"`
		SessionID   string `json:"session_id"`
	}
	if err := decodeStrictJSON(writer, request, &body); err != nil || body.SessionID != "" {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "session revocation request is invalid")
		return
	}
	profile := h.browserCookies(request)
	cookie, err := request.Cookie(profile.core)
	if err != nil || len(cookie.Value) < 32 {
		writeError(writer, request, http.StatusUnauthorized, "CORE_SESSION_REQUIRED", "Core Session is required")
		return
	}
	tokenHash := sha256.Sum256([]byte(cookie.Value))
	tx, err := h.database.Begin(request.Context())
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()
	var sessionID, userID string
	var expiresAt time.Time
	if err := tx.QueryRow(request.Context(), `SELECT id::text,user_id::text,expires_at FROM sessions WHERE kind='core' AND token_hash=$1 AND revoked_at IS NULL FOR UPDATE`, tokenHash[:]).Scan(&sessionID, &userID, &expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(writer, request, http.StatusUnauthorized, "CORE_SESSION_REQUIRED", "Core Session is invalid")
			return
		}
		h.writeFlowError(writer, request, err)
		return
	}
	if !time.Now().Before(expiresAt) {
		writeError(writer, request, http.StatusUnauthorized, "CORE_SESSION_EXPIRED", "Core Session has expired")
		return
	}
	if body.AllSessions {
		_, err = tx.Exec(request.Context(), `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, userID)
	} else {
		_, err = tx.Exec(request.Context(), `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 OR parent_session_id=$1`, sessionID)
	}
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	if err := tx.Commit(request.Context()); err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	http.SetCookie(writer, h.expiredBrowserCookie(profile.core, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(userID)
	writeSuccess(writer, request, http.StatusOK, map[string]bool{"revoked": true})
}

func (h *Handler) getPlatformOperations(writer http.ResponseWriter, request *http.Request) {
	decision, err := h.authorizeInbox(request, nil, "platform.operations.read", "platform", "", "", "")
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	snapshot, err := h.platformOps.Snapshot(request.Context())
	if err != nil {
		h.logger.Error("platform_operations_snapshot_error", "request_id", requestIDFrom(request.Context()), "error", err)
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, snapshot)
}

func (h *Handler) revokePlatformOperationSession(writer http.ResponseWriter, request *http.Request) {
	rawBody, body, ok := decodeInboxBody[struct {
		ExpectedActive bool `json:"expected_active"`
	}](writer, request)
	if !ok || !body.ExpectedActive {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Platform Operation request is invalid")
		return
	}
	decision, err := h.authorizeInbox(request, rawBody, "platform.operations.write", "platform", "", "", "")
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	hash := sha256.Sum256(append([]byte(chi.URLParam(request, "session_id")+"\x00"), rawBody...))
	result, err := h.platformOps.RevokeSession(request.Context(), platformoperations.WriteInput{
		ServiceID: request.Header.Get(contract.ServiceIDHeader), ActorUserID: decision.ActorUserID,
		RequestID: requestIDFrom(request.Context()), IdempotencyKey: request.Header.Get(contract.IdempotencyKeyHeader),
		RequestHash: hash[:], ResourceID: chi.URLParam(request, "session_id"),
	})
	if err != nil {
		h.writePlatformOperationError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, result)
}

func (h *Handler) updatePlatformOperationAccess(writer http.ResponseWriter, request *http.Request) {
	type scope struct {
		Kind         string `json:"kind"`
		ProductCode  string `json:"product_code"`
		ResourceType string `json:"resource_type"`
		ResourceID   string `json:"resource_id"`
	}
	type grant struct {
		RoleCode string `json:"role_code"`
		Scope    scope  `json:"scope"`
	}
	rawBody, body, ok := decodeInboxBody[struct {
		ExpectedRevision int64   `json:"expected_revision"`
		Status           string  `json:"status"`
		Grants           []grant `json:"grants"`
	}](writer, request)
	if !ok {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Platform Operation request is invalid")
		return
	}
	decision, err := h.authorizeInbox(request, rawBody, "platform.operations.write", "platform", "", "", "")
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	grants := make([]platformoperations.GrantInput, 0, len(body.Grants))
	for _, item := range body.Grants {
		grants = append(grants, platformoperations.GrantInput{RoleCode: item.RoleCode, Scope: platformoperations.ScopeInput{Kind: item.Scope.Kind, ProductCode: item.Scope.ProductCode, ResourceType: item.Scope.ResourceType, ResourceID: item.Scope.ResourceID}})
	}
	hash := sha256.Sum256(append([]byte(chi.URLParam(request, "user_id")+"\x00"), rawBody...))
	result, err := h.platformOps.UpdateAccess(request.Context(), platformoperations.AccessUpdateInput{
		WriteInput: platformoperations.WriteInput{
			ServiceID: request.Header.Get(contract.ServiceIDHeader), ActorUserID: decision.ActorUserID,
			RequestID: requestIDFrom(request.Context()), IdempotencyKey: request.Header.Get(contract.IdempotencyKeyHeader),
			RequestHash: hash[:], ResourceID: chi.URLParam(request, "user_id"),
		},
		ExpectedRevision: body.ExpectedRevision, Status: body.Status, Grants: grants,
	})
	if err != nil {
		h.writePlatformOperationError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, result)
}

func (h *Handler) getPlatformOperationStatus(writer http.ResponseWriter, request *http.Request) {
	decision, err := h.authorizeInbox(request, nil, "platform.operations.write", "platform", "", "", "")
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	result, err := h.platformOps.OperationStatus(request.Context(), request.Header.Get(contract.ServiceIDHeader), decision.ActorUserID, chi.URLParam(request, "operation"), request.Header.Get(contract.IdempotencyKeyHeader))
	if err != nil {
		h.writePlatformOperationError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, result)
}

func (h *Handler) writePlatformOperationError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, platformoperations.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Platform Operation request is invalid")
	case errors.Is(err, platformoperations.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "PLATFORM_OPERATION_RESOURCE_NOT_FOUND", "Platform Operation resource was not found")
	case errors.Is(err, platformoperations.ErrConflict):
		writeError(writer, request, http.StatusConflict, "VERSION_CONFLICT", "Platform Operation resource state conflicts with the request")
	case errors.Is(err, platformoperations.ErrIdempotencyConflict):
		writeError(writer, request, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key conflicts with another request")
	default:
		h.logger.Error("platform_operation_error", "request_id", requestIDFrom(request.Context()), "error", err)
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
	}
}

func (h *Handler) getOperationsInboxItem(writer http.ResponseWriter, request *http.Request) {
	itemID := chi.URLParam(request, "item_id")
	productCode := request.URL.Query().Get("source_product_code")
	resourceType := request.URL.Query().Get("source_resource_type")
	resourceID := request.URL.Query().Get("source_resource_id")
	decision, err := h.authorizeInbox(request, nil, "platform.operations_inbox.read", "resource", productCode, resourceType, resourceID)
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	item, err := h.inbox.Get(request.Context(), itemID)
	if err != nil {
		h.writeInboxError(writer, request, err)
		return
	}
	if item.SourceProductCode != productCode || item.SourceResourceType != resourceType || item.SourceResourceID != resourceID {
		h.writeInboxError(writer, request, operationsinbox.ErrNotFound)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, inboxContractItem(item))
}

func (h *Handler) getOperationsInboxOperationStatus(writer http.ResponseWriter, request *http.Request) {
	operation := chi.URLParam(request, "operation")
	productCode := request.URL.Query().Get("source_product_code")
	resourceType := request.URL.Query().Get("source_resource_type")
	resourceID := request.URL.Query().Get("source_resource_id")
	decision, err := h.authorizeInbox(request, nil, "platform.operations_inbox.write", "resource", productCode, resourceType, resourceID)
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	item, found, err := h.inbox.OperationStatus(request.Context(), request.Header.Get(contract.ServiceIDHeader), decision.ActorUserID, operation, request.Header.Get(contract.IdempotencyKeyHeader))
	if err != nil {
		h.writeInboxError(writer, request, err)
		return
	}
	if found && (item.SourceProductCode != productCode || item.SourceResourceType != resourceType || item.SourceResourceID != resourceID) {
		found = false
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	status := contract.OperationsInboxOperationStatus{Status: "unknown"}
	if found {
		contractItem := inboxContractItem(item)
		status.Status, status.Item = "succeeded", &contractItem
	}
	writeSuccess(writer, request, http.StatusOK, status)
}

func (h *Handler) listOperationsInboxItems(writer http.ResponseWriter, request *http.Request) {
	productCode, status := request.URL.Query().Get("product_code"), request.URL.Query().Get("status")
	page, pageSize := 1, 20
	var err error
	if rawPage := request.URL.Query().Get("page"); rawPage != "" {
		page, err = strconv.Atoi(rawPage)
		if err != nil {
			h.writeInboxError(writer, request, operationsinbox.ErrInvalid)
			return
		}
	}
	if rawPageSize := request.URL.Query().Get("page_size"); rawPageSize != "" {
		pageSize, err = strconv.Atoi(rawPageSize)
		if err != nil {
			h.writeInboxError(writer, request, operationsinbox.ErrInvalid)
			return
		}
	}
	decision, err := h.authorizeInbox(request, nil, "platform.operations_inbox.read", "product", productCode, "", "")
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	items, hasMore, err := h.inbox.List(request.Context(), productCode, status, page, pageSize)
	if err != nil {
		h.writeInboxError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	result := make([]contract.OperationsInboxItem, 0, len(items))
	for _, item := range items {
		result = append(result, inboxContractItem(item))
	}
	writeSuccess(writer, request, http.StatusOK, map[string]any{"items": result, "page": page, "page_size": pageSize, "has_more": hasMore})
}

func (h *Handler) createOperationsInboxItem(writer http.ResponseWriter, request *http.Request) {
	rawBody, body, ok := decodeInboxBody[contract.CreateOperationsInboxItemRequest](writer, request)
	if !ok {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Operations Inbox request is invalid")
		return
	}
	decision, err := h.authorizeInbox(request, rawBody, "platform.operations_inbox.write", "resource", body.SourceProductCode, body.SourceResourceType, body.SourceResourceID)
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	hash := sha256.Sum256(rawBody)
	item, err := h.inbox.Create(request.Context(), operationsinbox.CreateInput{
		ServiceID: request.Header.Get(contract.ServiceIDHeader), ActorUserID: decision.ActorUserID, RequestID: requestIDFrom(request.Context()),
		IdempotencyKey: request.Header.Get(contract.IdempotencyKeyHeader), RequestHash: hash[:],
		SourceProductCode: body.SourceProductCode, SourceResourceType: body.SourceResourceType,
		SourceResourceID: body.SourceResourceID, SourceResourceURL: body.SourceResourceURL,
		OwnerUserID: body.OwnerUserID, Priority: body.Priority, SLADueAt: body.SlaDueAt,
		Status: optionalString(body.Status),
	})
	if err != nil {
		h.writeInboxError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusCreated, inboxContractItem(item))
}

func (h *Handler) updateOperationsInboxItem(writer http.ResponseWriter, request *http.Request) {
	itemID := chi.URLParam(request, "item_id")
	rawBody, body, ok := decodeInboxBody[contract.UpdateOperationsInboxItemRequest](writer, request)
	if !ok {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Operations Inbox request is invalid")
		return
	}
	if body.ClearOwner != nil && !*body.ClearOwner || body.ClearSla != nil && !*body.ClearSla {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Operations Inbox request is invalid")
		return
	}
	decision, err := h.authorizeInbox(request, rawBody, "platform.operations_inbox.write", "resource", body.SourceProductCode, body.SourceResourceType, body.SourceResourceID)
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	existing, err := h.inbox.Get(request.Context(), itemID)
	if err != nil {
		h.writeInboxError(writer, request, err)
		return
	}
	if existing.SourceProductCode != body.SourceProductCode || existing.SourceResourceType != body.SourceResourceType || existing.SourceResourceID != body.SourceResourceID {
		writeError(writer, request, http.StatusNotFound, "OPERATIONS_INBOX_ITEM_NOT_FOUND", "Operations Inbox item was not found")
		return
	}
	hash := sha256.Sum256(append([]byte(itemID+"\x00"), rawBody...))
	item, err := h.inbox.Update(request.Context(), operationsinbox.UpdateInput{
		ServiceID: request.Header.Get(contract.ServiceIDHeader), ItemID: itemID, ActorUserID: decision.ActorUserID, RequestID: requestIDFrom(request.Context()),
		IdempotencyKey: request.Header.Get(contract.IdempotencyKeyHeader), RequestHash: hash[:], ExpectedVersion: body.ExpectedVersion,
		OwnerUserID: body.OwnerUserID, ClearOwner: body.ClearOwner != nil && *body.ClearOwner,
		Priority: body.Priority, SLADueAt: body.SlaDueAt, ClearSLA: body.ClearSla != nil && *body.ClearSla, Status: body.Status,
	})
	if err != nil {
		h.writeInboxError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, inboxContractItem(item))
}

func (h *Handler) authorizeInbox(request *http.Request, rawBody []byte, permission, scopeKind, productCode, resourceType, resourceID string) (identity.AuthorizationDecision, error) {
	audit := auditFrom(request.Context())
	audit.serviceID, audit.keyID = request.Header.Get(contract.ServiceIDHeader), request.Header.Get(contract.KeyIDHeader)
	clientID, clientSecret, ok := request.BasicAuth()
	if !ok || clientID != audit.serviceID {
		return identity.AuthorizationDecision{}, identity.ErrUnauthorized
	}
	timestamp, nonce, signature := request.Header.Get(contract.TimestampHeader), request.Header.Get(contract.NonceHeader), request.Header.Get(contract.SignatureHeader)
	sessionToken := request.Header.Get(contract.SessionExchangeTokenHeader)
	if audit.serviceID == "" || audit.keyID == "" || timestamp == "" || nonce == "" || signature == "" || len(sessionToken) < 32 {
		return identity.AuthorizationDecision{}, identity.ErrInvalid
	}
	bodyHash := sha256.Sum256(rawBody)
	pathAndQuery := request.URL.EscapedPath()
	if request.URL.RawQuery != "" {
		pathAndQuery += "?" + request.URL.RawQuery
	}
	return h.flow.CheckAuthorization(request.Context(), identity.AuthorizationCheckInput{
		HTTPMethod:   request.Method,
		SessionToken: sessionToken, ClientSecret: clientSecret, PermissionCode: permission,
		ScopeKind: scopeKind, ProductCode: productCode, ResourceType: resourceType, ResourceID: resourceID,
		ServiceID: audit.serviceID, KeyID: audit.keyID, Timestamp: timestamp, Nonce: nonce,
		Signature: signature, BodyHash: bodyHash[:], PathAndQuery: pathAndQuery, RequestID: audit.requestID,
	})
}

func decodeInboxBody[T any](writer http.ResponseWriter, request *http.Request) ([]byte, T, bool) {
	var body T
	request.Body = http.MaxBytesReader(writer, request.Body, 32<<10)
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, body, false
	}
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, body, false
	}
	return rawBody, body, true
}

func inboxContractItem(item operationsinbox.Item) contract.OperationsInboxItem {
	return contract.OperationsInboxItem{
		ID: item.ID, SourceProductCode: item.SourceProductCode, SourceResourceType: item.SourceResourceType,
		SourceResourceID: item.SourceResourceID, SourceResourceURL: item.SourceResourceURL,
		OwnerUserID: item.OwnerUserID, Priority: item.Priority, SlaDueAt: item.SLADueAt,
		Status: item.Status, Version: item.Version, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func (h *Handler) writeInboxError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, operationsinbox.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Operations Inbox request is invalid")
	case errors.Is(err, operationsinbox.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "OPERATIONS_INBOX_ITEM_NOT_FOUND", "Operations Inbox item was not found")
	case errors.Is(err, operationsinbox.ErrConflict):
		writeError(writer, request, http.StatusConflict, "VERSION_CONFLICT", "Operations Inbox item version or source reference conflicts")
	case errors.Is(err, operationsinbox.ErrIdempotencyConflict):
		writeError(writer, request, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key conflicts with another request")
	default:
		h.logger.Error("operations_inbox_error", "request_id", requestIDFrom(request.Context()), "error", err)
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
	}
}

func (h *Handler) recordMailDelivery(writer http.ResponseWriter, request *http.Request) {
	keyID, timestamp, nonce, signature := request.Header.Get(contract.KeyIDHeader), request.Header.Get(contract.TimestampHeader), request.Header.Get(contract.NonceHeader), request.Header.Get(contract.SignatureHeader)
	audit := auditFrom(request.Context())
	audit.serviceID, audit.keyID = "mail-provider", keyID
	secret, knownKey := h.deliveryKeys[keyID]
	unixTime, parseErr := strconv.ParseInt(timestamp, 10, 64)
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	rawBody, readErr := io.ReadAll(request.Body)
	if !knownKey || parseErr != nil || readErr != nil || len(nonce) < 16 || len(nonce) > 200 || time.Since(time.Unix(unixTime, 0)).Abs() > 5*time.Minute {
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "delivery receipt authentication failed")
		return
	}
	bodyHash := sha256.Sum256(rawBody)
	canonical := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", request.Method, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(bodyHash[:]))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	providedSignature, decodeErr := base64.RawURLEncoding.DecodeString(signature)
	if decodeErr != nil || !hmac.Equal(providedSignature, mac.Sum(nil)) {
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "delivery receipt authentication failed")
		return
	}
	stored, err := h.redis.SetNX(request.Context(), "platform-core:mail-delivery:nonce:"+keyID+":"+nonce, "1", 10*time.Minute).Result()
	if err != nil {
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	if !stored {
		writeError(writer, request, http.StatusConflict, "NONCE_ALREADY_USED", "delivery receipt was already submitted")
		return
	}
	var body contract.RecordMailDeliveryRequest
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF || body.Status != "delivered" || len(body.MessageID) > 200 || body.MessageID == "" {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "delivery receipt is invalid")
		return
	}
	_, err = h.queries.RecordMailDeliveryReceipt(request.Context(), store.RecordMailDeliveryReceiptParams{
		MessageID: body.MessageID, RequestID: requestIDFrom(request.Context()), ActorID: keyID,
	})
	if err != nil {
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	writeSuccess(writer, request, http.StatusAccepted, contract.MailDeliveryAccepted{Accepted: true})
}

var accountLoginTemplate = template.Must(template.New("account-login").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>HENU Kit 账号中心</title><style>
body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f5f7fb;color:#172033;font:16px/1.5 system-ui,sans-serif}.card{width:min(390px,calc(100% - 32px));box-sizing:border-box;background:#fff;border:1px solid #dfe5ef;border-radius:18px;padding:28px;box-shadow:0 14px 40px #27364d18}h1{margin:0 0 8px;font-size:24px}p{color:#5d687a}label{display:block;margin:20px 0 6px;font-weight:650}input{width:100%;box-sizing:border-box;border:1px solid #bdc7d6;border-radius:10px;padding:12px;font:inherit}button{width:100%;margin-top:18px;border:0;border-radius:10px;padding:12px;background:#2457d6;color:#fff;font:inherit;font-weight:700}.error{color:#b42318}.hint{font-size:13px}
</style></head><body><main class="card"><h1>HENU Kit 账号中心</h1><p class="hint">学生自主运营 · 非河南大学官方项目</p>
{{if .Error}}<p class="error" role="alert">{{.Error}}</p>{{else if .CodeRequested}}<p>验证码已进入发送队列，请查收学校邮箱。</p>{{else}}<p>使用河南大学邮箱登录。</p>{{end}}
<form method="post" action="{{.PathPrefix}}{{if .CodeRequested}}/login/verify{{else}}/login/code{{end}}">
<input type="hidden" name="csrf_token" value="{{.CSRFToken}}"><input type="hidden" name="return_to" value="{{.ReturnTo}}">
<label for="email">学校邮箱</label><input id="email" name="email" type="email" value="{{.Email}}" autocomplete="email" required {{if .CodeRequested}}readonly{{end}}>
{{if .CodeRequested}}<label for="code">6 位验证码</label><input id="code" name="code" inputmode="numeric" pattern="[0-9]{6}" autocomplete="one-time-code" required autofocus>{{end}}
<button type="submit">{{if .CodeRequested}}登录并继续{{else}}发送验证码{{end}}</button>
	</form><p class="hint">当前仅允许 henu.edu.cn 邮箱。会话绝对有效期为 15 天。</p></main></body></html>`))

var accountRegisterTemplate = template.Must(template.New("account-register").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>注册 HENU Kit</title></head><body><main><h1>注册 HENU Kit</h1>
{{if .Error}}<p role="alert">{{.Error}}</p>{{else if .CodeRequested}}<p>验证码已进入发送队列，请查收学校邮箱。</p>{{end}}
<form method="post" action="{{.PathPrefix}}{{if .CodeRequested}}/register{{else}}/register/code{{end}}">
<input type="hidden" name="csrf_token" value="{{.CSRFToken}}"><input type="hidden" name="return_to" value="{{.ReturnTo}}">
<label for="email">学校邮箱</label><input id="email" name="email" type="email" value="{{.Email}}" required {{if .CodeRequested}}readonly{{end}}>
{{if .CodeRequested}}
<label for="display_name">展示名</label><input id="display_name" name="display_name" maxlength="80" required>
<label for="code">6 位验证码</label><input id="code" name="code" inputmode="numeric" pattern="[0-9]{6}" required>
<label for="password">密码</label><input id="password" name="password" type="password" minlength="10" maxlength="128" required>
{{end}}
<button type="submit">{{if .CodeRequested}}注册并登录{{else}}发送验证码{{end}}</button>
</form><p>学生自主运营 · 非河南大学官方项目</p></main></body></html>`))

var accountRecoverTemplate = template.Must(template.New("account-recover").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>找回密码</title></head>
<body><main><h1>找回密码</h1>{{if .Error}}<p role="alert">{{.Error}}</p>{{else if .CodeRequested}}<p>验证码已进入发送队列。</p>{{end}}
<form method="post" action="{{.PathPrefix}}{{if .CodeRequested}}/recover{{else}}/recover/code{{end}}"><input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
<label>学校邮箱<input name="email" type="email" value="{{.Email}}" required {{if .CodeRequested}}readonly{{end}}></label>
{{if .CodeRequested}}<label>验证码<input name="code" inputmode="numeric" pattern="[0-9]{6}" required></label>
<label>新密码<input name="password" type="password" minlength="10" maxlength="128" required></label>{{end}}
<button type="submit">{{if .CodeRequested}}重置密码并登录{{else}}发送验证码{{end}}</button></form></main></body></html>`))

var accountSecurityTemplate = template.Must(template.New("account-security").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>账号安全</title></head>
<body><main><h1>账号安全</h1>{{if .Error}}<p role="alert">{{.Error}}</p>{{else if .CodeRequested}}<p>验证码已进入发送队列。</p>{{end}}
<form method="post" action="{{.PathPrefix}}{{if .CodeRequested}}/account/security/password{{else}}/account/security/code{{end}}"><input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
<label>学校邮箱<input name="email" type="email" value="{{.Email}}" required {{if .CodeRequested}}readonly{{end}}></label>
{{if .CodeRequested}}<label>当前密码<input name="current_password" type="password" required></label>
<label>验证码<input name="code" inputmode="numeric" pattern="[0-9]{6}" required></label>
<label>新密码<input name="new_password" type="password" minlength="10" maxlength="128" required></label>{{end}}
<button type="submit">{{if .CodeRequested}}更改密码{{else}}发送验证码{{end}}</button></form></main></body></html>`))

type accountLoginView struct {
	CSRFToken, ReturnTo, Email, Error, PathPrefix string
	CodeRequested                                 bool
}

type accountRegisterView struct {
	CSRFToken, ReturnTo, Email, Error, PathPrefix string
	CodeRequested                                 bool
}

type accountCredentialView struct {
	CSRFToken, Email, Error, PathPrefix string
	CodeRequested                       bool
}

func (h *Handler) registerPage(writer http.ResponseWriter, request *http.Request) {
	returnTo := safeRegistrationReturnTo(request.URL.Query().Get("return_to"))
	csrfToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "register_csrf")
		return
	}
	_, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "register_device")
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.csrf, csrfToken, 10*60, time.Time{}, profile.secure))
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	h.renderRegister(writer, request, accountRegisterView{CSRFToken: csrfToken, ReturnTo: returnTo})
}

func (h *Handler) registerRequestCode(writer http.ResponseWriter, request *http.Request) {
	csrfToken, returnTo, email, ok := h.parseRegistrationForm(writer, request)
	if !ok {
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "register_device")
		return
	}
	idempotencyToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "register_request_idempotency")
		return
	}
	_, err = h.verification.Request(request.Context(), verification.RequestInput{
		Email: email, Purpose: "register", ClientID: "account-center", DeviceID: deviceID,
		ClientIP: h.clientIP(request), IdempotencyKey: "register_request_" + idempotencyToken,
		RequestID: requestIDFrom(request.Context()),
	})
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	if err != nil {
		h.renderRegister(writer, request, accountRegisterView{CSRFToken: csrfToken, ReturnTo: returnTo, Email: email, Error: "无法发送验证码，请检查邮箱或稍后重试。"})
		return
	}
	h.renderRegister(writer, request, accountRegisterView{CSRFToken: csrfToken, ReturnTo: returnTo, Email: email, CodeRequested: true})
}

func (h *Handler) registerAccount(writer http.ResponseWriter, request *http.Request) {
	csrfToken, returnTo, email, ok := h.parseRegistrationForm(writer, request)
	if !ok {
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "register_device")
		return
	}
	idempotencyToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "register_idempotency")
		return
	}
	registered, err := h.verification.Register(request.Context(), verification.RegisterInput{
		Email: email, Code: strings.TrimSpace(request.FormValue("code")),
		DisplayName: request.FormValue("display_name"), Password: request.FormValue("password"),
		IdempotencyKey: "register_" + idempotencyToken, DeviceID: deviceID, ClientIP: h.clientIP(request),
	})
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	if errors.Is(err, verification.ErrRandomSource) {
		h.writeRandomSourceError(writer, request, "registration")
		return
	}
	if err != nil || registered.SessionToken == "" {
		message := "注册失败，请检查验证码和注册信息后重试。"
		if errors.Is(err, verification.ErrAlreadyRegistered) {
			message = "该邮箱已注册，请登录或找回密码。"
		}
		h.renderRegister(writer, request, accountRegisterView{
			CSRFToken: csrfToken, ReturnTo: returnTo, Email: email, CodeRequested: true, Error: message,
		})
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.core, registered.SessionToken, max(1, int(time.Until(registered.SessionExpiresAt).Seconds())), registered.SessionExpiresAt, profile.secure))
	http.SetCookie(writer, h.expiredBrowserCookie(profile.csrf, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(registered.UserID)
	http.Redirect(writer, request, returnTo, http.StatusSeeOther)
}

func (h *Handler) renderRegister(writer http.ResponseWriter, request *http.Request, view accountRegisterView) {
	view.PathPrefix = h.publicPathPrefix
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if err := accountRegisterTemplate.Execute(writer, view); err != nil {
		h.logger.Error("account_register_template_error", "request_id", requestIDFrom(request.Context()), "error", err)
	}
}

func (h *Handler) recoverPage(writer http.ResponseWriter, request *http.Request) {
	csrfToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "recover_csrf")
		return
	}
	_, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "recover_device")
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.csrf, csrfToken, 10*60, time.Time{}, profile.secure))
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	h.renderCredentialPage(writer, request, accountRecoverTemplate, accountCredentialView{CSRFToken: csrfToken})
}

func (h *Handler) recoverRequestCode(writer http.ResponseWriter, request *http.Request) {
	csrfToken, _, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "recover_device")
		return
	}
	idempotencyToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "recover_request_idempotency")
		return
	}
	_, err = h.verification.Request(request.Context(), verification.RequestInput{
		Email: email, Purpose: "security", ClientID: "account-center", DeviceID: deviceID,
		ClientIP: h.clientIP(request), IdempotencyKey: "recover_request_" + idempotencyToken,
		RequestID: requestIDFrom(request.Context()),
	})
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	view := accountCredentialView{CSRFToken: csrfToken, Email: email, CodeRequested: true}
	if err != nil {
		view.CodeRequested, view.Error = false, "无法发送验证码，请检查邮箱或稍后重试。"
	}
	h.renderCredentialPage(writer, request, accountRecoverTemplate, view)
}

func (h *Handler) recoverPassword(writer http.ResponseWriter, request *http.Request) {
	csrfToken, _, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "recover_device")
		return
	}
	idempotencyToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "recover_idempotency")
		return
	}
	recovered, err := h.verification.RecoverPassword(request.Context(), verification.PasswordRecoveryInput{
		Email: email, Code: strings.TrimSpace(request.FormValue("code")), Password: request.FormValue("password"),
		IdempotencyKey: "recover_" + idempotencyToken, DeviceID: deviceID, ClientIP: h.clientIP(request),
	})
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	if errors.Is(err, verification.ErrRandomSource) {
		h.writeRandomSourceError(writer, request, "recover")
		return
	}
	if err != nil || recovered.SessionToken == "" {
		h.renderCredentialPage(writer, request, accountRecoverTemplate, accountCredentialView{
			CSRFToken: csrfToken, Email: email, CodeRequested: true,
			Error: "无法重置密码，请检查验证码和新密码后重试。",
		})
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.core, recovered.SessionToken, max(1, int(time.Until(recovered.SessionExpiresAt).Seconds())), recovered.SessionExpiresAt, profile.secure))
	http.SetCookie(writer, h.expiredBrowserCookie(profile.csrf, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(recovered.UserID)
	http.Redirect(writer, request, "/account/security", http.StatusSeeOther)
}

func (h *Handler) securityPage(writer http.ResponseWriter, request *http.Request) {
	profile := h.browserCookies(request)
	coreCookie, err := request.Cookie(profile.core)
	if err != nil {
		h.redirectToLogin(writer, request)
		return
	}
	session, err := h.verification.CoreSession(request.Context(), coreCookie.Value)
	if err != nil {
		h.redirectToLogin(writer, request)
		return
	}
	csrfToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "security_csrf")
		return
	}
	_, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "security_device")
		return
	}
	http.SetCookie(writer, h.browserCookie(profile.csrf, csrfToken, 10*60, time.Time{}, profile.secure))
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(session.UserID)
	h.renderCredentialPage(writer, request, accountSecurityTemplate, accountCredentialView{CSRFToken: csrfToken})
}

func (h *Handler) securityRequestCode(writer http.ResponseWriter, request *http.Request) {
	csrfToken, _, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return
	}
	profile := h.browserCookies(request)
	coreCookie, err := request.Cookie(profile.core)
	if err != nil {
		h.redirectToLogin(writer, request)
		return
	}
	session, err := h.verification.CoreSession(request.Context(), coreCookie.Value)
	if err != nil {
		h.redirectToLogin(writer, request)
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "security_device")
		return
	}
	idempotencyToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "security_request_idempotency")
		return
	}
	_, err = h.verification.Request(request.Context(), verification.RequestInput{
		Email: email, Purpose: "security", ClientID: "account-center", DeviceID: deviceID,
		ClientIP: h.clientIP(request), IdempotencyKey: "security_request_" + idempotencyToken,
		RequestID: requestIDFrom(request.Context()),
	})
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(session.UserID)
	view := accountCredentialView{CSRFToken: csrfToken, Email: email, CodeRequested: true}
	if err != nil {
		view.CodeRequested, view.Error = false, "无法发送验证码，请检查邮箱或稍后重试。"
	}
	h.renderCredentialPage(writer, request, accountSecurityTemplate, view)
}

func (h *Handler) securityChangePassword(writer http.ResponseWriter, request *http.Request) {
	csrfToken, _, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return
	}
	profile := h.browserCookies(request)
	coreCookie, err := request.Cookie(profile.core)
	if err != nil {
		if request.Header.Get(explicitFormResponseHeader) == "status" {
			writeError(writer, request, http.StatusUnauthorized, "CORE_SESSION_REQUIRED", "Core Session is required")
			return
		}
		h.redirectToLogin(writer, request)
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "security_device")
		return
	}
	idempotencyToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "security_change_idempotency")
		return
	}
	changed, err := h.verification.ChangePassword(request.Context(), verification.PasswordChangeInput{
		Email: email, Code: strings.TrimSpace(request.FormValue("code")),
		CurrentPassword: request.FormValue("current_password"), NewPassword: request.FormValue("new_password"),
		CoreSessionToken: coreCookie.Value, IdempotencyKey: "security_change_" + idempotencyToken,
		DeviceID: deviceID, ClientIP: h.clientIP(request),
	})
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	if err != nil {
		message := "无法更改密码，请检查当前密码、验证码和新密码。"
		if errors.Is(err, verification.ErrChallengeRequired) {
			message = "密码尝试过多，请先使用邮箱验证码登录后再试。"
		}
		h.renderCredentialPage(writer, request, accountSecurityTemplate, accountCredentialView{
			CSRFToken: csrfToken, Email: email, CodeRequested: true, Error: message,
		})
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(changed.UserID)
	if request.Header.Get(explicitFormResponseHeader) == "status" {
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(writer, request, "/account/security?password_changed=1", http.StatusSeeOther)
}

func (h *Handler) renderCredentialPage(writer http.ResponseWriter, request *http.Request, page *template.Template, view accountCredentialView) {
	view.PathPrefix = h.publicPathPrefix
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if err := page.Execute(writer, view); err != nil {
		h.logger.Error("account_credential_template_error", "request_id", requestIDFrom(request.Context()), "error", err)
	}
}

func (h *Handler) loginPage(writer http.ResponseWriter, request *http.Request) {
	returnTo := safeAccountReturnTo(request.URL.Query().Get("return_to"))
	csrfToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "login_csrf")
		return
	}
	_, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "login_device")
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.csrf, csrfToken, 10*60, time.Time{}, profile.secure))
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	h.renderLogin(writer, request, accountLoginView{CSRFToken: csrfToken, ReturnTo: returnTo, PathPrefix: h.publicPathPrefix})
}

func (h *Handler) loginRequestCode(writer http.ResponseWriter, request *http.Request) {
	csrfToken, returnTo, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "login_device")
		return
	}
	idempotencyToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "login_request_idempotency")
		return
	}
	accepted, err := h.verification.Request(request.Context(), verification.RequestInput{
		Email: email, Purpose: "login", ClientID: "account-center", DeviceID: deviceID, ClientIP: h.clientIP(request),
		IdempotencyKey: "login_request_" + idempotencyToken, RequestID: requestIDFrom(request.Context()),
	})
	if err != nil {
		if errors.Is(err, verification.ErrRandomSource) {
			h.writeRandomSourceError(writer, request, "login_verification_request")
			return
		}
		if deviceCookie != nil {
			http.SetCookie(writer, deviceCookie)
		}
		h.renderLogin(writer, request, accountLoginView{CSRFToken: csrfToken, ReturnTo: returnTo, Email: email, PathPrefix: h.publicPathPrefix, Error: "无法发送验证码，请检查邮箱或稍后重试。"})
		return
	}
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	writer.Header().Set("X-Verification-Expires", accepted.ExpiresAt.Format(time.RFC3339))
	h.renderLogin(writer, request, accountLoginView{CSRFToken: csrfToken, ReturnTo: returnTo, Email: email, PathPrefix: h.publicPathPrefix, CodeRequested: true})
}

func (h *Handler) loginVerifyCode(writer http.ResponseWriter, request *http.Request) {
	csrfToken, returnTo, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return
	}
	code := strings.TrimSpace(request.FormValue("code"))
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "login_device")
		return
	}
	idempotencyToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "login_verify_idempotency")
		return
	}
	verified, err := h.verification.Verify(request.Context(), verification.VerifyInput{
		Email: email, Code: code, Purpose: "login", IdempotencyKey: "login_verify_" + idempotencyToken,
		DeviceID: deviceID, ClientIP: h.clientIP(request),
	})
	if errors.Is(err, verification.ErrRandomSource) {
		h.writeRandomSourceError(writer, request, "login_verification")
		return
	}
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	if err != nil || verified.SessionToken == "" {
		h.renderLogin(writer, request, accountLoginView{CSRFToken: csrfToken, ReturnTo: returnTo, Email: email, PathPrefix: h.publicPathPrefix, CodeRequested: true, Error: "验证码无效、已过期或登录暂不可用。"})
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.core, verified.SessionToken, max(1, int(time.Until(verified.SessionExpiresAt).Seconds())), verified.SessionExpiresAt, profile.secure))
	http.SetCookie(writer, h.expiredBrowserCookie(profile.csrf, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(verified.UserID)
	http.Redirect(writer, request, returnTo, http.StatusSeeOther)
}

func (h *Handler) loginPassword(writer http.ResponseWriter, request *http.Request) {
	csrfToken, returnTo, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "password_login_device")
		return
	}
	loggedIn, err := h.verification.PasswordLogin(request.Context(), verification.PasswordLoginInput{
		Email: email, Password: request.FormValue("password"),
		DeviceID: deviceID, ClientIP: h.clientIP(request),
	})
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	if errors.Is(err, verification.ErrRandomSource) {
		h.writeRandomSourceError(writer, request, "password_login")
		return
	}
	if err != nil || loggedIn.SessionToken == "" {
		message := "邮箱或密码错误，或登录暂不可用。"
		if errors.Is(err, verification.ErrChallengeRequired) {
			message = "密码尝试过多，请改用邮箱验证码登录。"
		}
		h.renderLogin(writer, request, accountLoginView{
			CSRFToken: csrfToken, ReturnTo: returnTo, Email: email,
			Error: message,
		})
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.core, loggedIn.SessionToken, max(1, int(time.Until(loggedIn.SessionExpiresAt).Seconds())), loggedIn.SessionExpiresAt, profile.secure))
	http.SetCookie(writer, h.expiredBrowserCookie(profile.csrf, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(loggedIn.UserID)
	http.Redirect(writer, request, returnTo, http.StatusSeeOther)
}

func (h *Handler) parseLoginForm(writer http.ResponseWriter, request *http.Request) (string, string, string, bool) {
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	if err := request.ParseForm(); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "login form is invalid")
		return "", "", "", false
	}
	csrfToken := request.FormValue("csrf_token")
	cookie, err := request.Cookie(h.browserCookies(request).csrf)
	if err != nil || len(csrfToken) < 32 || !hmac.Equal([]byte(cookie.Value), []byte(csrfToken)) {
		writeError(writer, request, http.StatusForbidden, "CSRF_REJECTED", "login form expired; reload and try again")
		return "", "", "", false
	}
	return csrfToken, safeAccountReturnTo(request.FormValue("return_to")), strings.TrimSpace(request.FormValue("email")), true
}

func (h *Handler) parseRegistrationForm(writer http.ResponseWriter, request *http.Request) (string, string, string, bool) {
	csrfToken, _, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return "", "", "", false
	}
	return csrfToken, safeRegistrationReturnTo(request.FormValue("return_to")), email, true
}

func (h *Handler) renderLogin(writer http.ResponseWriter, request *http.Request, view accountLoginView) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if err := accountLoginTemplate.Execute(writer, view); err != nil {
		h.logger.Error("account_login_template_error", "request_id", requestIDFrom(request.Context()), "error", err)
	}
}

func (h *Handler) redirectToLogin(writer http.ResponseWriter, request *http.Request) {
	location := h.publicPathPrefix + "/login?return_to=" + url.QueryEscape(request.URL.RequestURI())
	writer.Header().Set("Cache-Control", "no-store")
	http.Redirect(writer, request, location, http.StatusFound)
}

func safeAccountReturnTo(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path != contract.AuthorizeRoute || parsed.Fragment != "" {
		return "/"
	}
	return parsed.RequestURI()
}

func safeRegistrationReturnTo(value string) string {
	if value == "" {
		return "/account/security"
	}
	if safe := safeAccountReturnTo(value); safe != "/" {
		return safe
	}
	return "/account/security"
}

func randomBrowserToken() (string, error) {
	value := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (h *Handler) requestVerificationCode(writer http.ResponseWriter, request *http.Request) {
	idempotencyKey := request.Header.Get(contract.IdempotencyKeyHeader)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "verification request is invalid")
		return
	}
	var body contract.RequestVerificationCodeRequest
	if err := decodeStrictJSON(writer, request, &body); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "verification request is invalid")
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "verification_request_device")
		return
	}
	accepted, err := h.verification.Request(request.Context(), verification.RequestInput{
		Email: body.Email, Purpose: body.Purpose, ClientID: optionalString(body.ClientID),
		DeviceID: deviceID, ClientIP: h.clientIP(request),
		IdempotencyKey: idempotencyKey, RequestID: requestIDFrom(request.Context()),
	})
	if err != nil {
		if errors.Is(err, verification.ErrRandomSource) {
			h.writeRandomSourceError(writer, request, "verification_request")
			return
		}
		if deviceCookie != nil {
			http.SetCookie(writer, deviceCookie)
		}
		h.writeFlowError(writer, request, err)
		return
	}
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	writeSuccess(writer, request, http.StatusAccepted, contract.VerificationCodeAccepted{ExpiresAt: accepted.ExpiresAt, ResendAfter: accepted.ResendAfter})
}

func (h *Handler) verifyVerificationCode(writer http.ResponseWriter, request *http.Request) {
	if !h.sameOriginBrowserRequest(request) {
		writeError(writer, request, http.StatusForbidden, "ORIGIN_REJECTED", "verification request origin is not allowed")
		return
	}
	idempotencyKey := request.Header.Get(contract.IdempotencyKeyHeader)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 200 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "verification request is invalid")
		return
	}
	var body contract.VerifyVerificationCodeRequest
	if err := decodeStrictJSON(writer, request, &body); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "verification request is invalid")
		return
	}
	deviceID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "verification_verify_device")
		return
	}
	verified, err := h.verification.Verify(request.Context(), verification.VerifyInput{
		Email: body.Email, Code: body.Code, Purpose: body.Purpose, IdempotencyKey: idempotencyKey,
		DeviceID: deviceID, ClientIP: h.clientIP(request),
	})
	if err != nil {
		if errors.Is(err, verification.ErrRandomSource) {
			h.writeRandomSourceError(writer, request, "verification_verify")
			return
		}
		if deviceCookie != nil {
			http.SetCookie(writer, deviceCookie)
		}
		h.writeFlowError(writer, request, err)
		return
	}
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	response := contract.VerificationCodeVerified{VerificationID: verified.VerificationID}
	if verified.SessionToken != "" {
		profile := h.browserCookies(request)
		http.SetCookie(writer, h.browserCookie(profile.core, verified.SessionToken, max(1, int(time.Until(verified.SessionExpiresAt).Seconds())), verified.SessionExpiresAt, profile.secure))
	}
	if verified.UserID != "" {
		response.User = &contract.PlatformUser{UserID: verified.UserID, EmailVerified: verified.EmailVerified, Status: verified.UserStatus, CreatedAt: verified.UserCreatedAt}
		response.SessionExpiresAt = &verified.SessionExpiresAt
		auditFrom(request.Context()).subjectUserID = maskSubject(verified.UserID)
	}
	writeSuccess(writer, request, http.StatusOK, response)
}

func (h *Handler) sameOriginBrowserRequest(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	expectedScheme := "http"
	if h.externallyHTTPS(request) {
		expectedScheme = "https"
	}
	return parsed.Scheme == expectedScheme && parsed.Host == request.Host
}

func decodeStrictJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func (h *Handler) health(writer http.ResponseWriter, request *http.Request) {
	writeSuccess(writer, request, http.StatusOK, map[string]bool{"alive": true})
}

func (h *Handler) ready(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	if err := h.database.Ping(ctx); err != nil {
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	if err := h.redis.Ping(ctx).Err(); err != nil {
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	writeSuccess(writer, request, http.StatusOK, map[string]bool{"ready": true})
}

func (h *Handler) authorize(writer http.ResponseWriter, request *http.Request) {
	query, err := contract.ParseAuthorizeOAuthClientQuery(request.URL.Query())
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
		return
	}
	callback, err := url.Parse(query.RedirectURI)
	localHTTP := err == nil && callback.Scheme == "http" && (callback.Hostname() == "localhost" || callback.Hostname() == "127.0.0.1")
	if err != nil || callback.Host == "" || callback.User != nil || callback.Fragment != "" || (callback.Scheme != "https" && !localHTTP) {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
		return
	}
	profile := h.browserCookies(request)
	cookie, err := request.Cookie(profile.core)
	if err != nil {
		h.redirectToLogin(writer, request)
		return
	}
	authorization, err := h.flow.Authorize(request.Context(), identity.AuthorizeInput{
		CoreSessionToken: cookie.Value,
		ClientID:         query.ClientID, RedirectURI: query.RedirectURI, CodeChallenge: query.CodeChallenge,
	})
	if err != nil {
		if errors.Is(err, identity.ErrUnauthorized) {
			h.redirectToLogin(writer, request)
			return
		}
		h.writeFlowError(writer, request, err)
		return
	}
	http.SetCookie(writer, h.browserCookie(profile.core, cookie.Value, max(1, int(time.Until(authorization.SessionExpires).Seconds())), authorization.SessionExpires, profile.secure))
	callbackQuery := callback.Query()
	callbackQuery.Set("code", authorization.Code)
	callbackQuery.Set("state", query.State)
	callback.RawQuery = callbackQuery.Encode()
	auditFrom(request.Context()).subjectUserID = maskSubject(authorization.UserID)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Pragma", "no-cache")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(writer, request, callback.String(), http.StatusFound)
}

func (h *Handler) exchange(writer http.ResponseWriter, request *http.Request) {
	audit := auditFrom(request.Context())
	audit.serviceID, audit.keyID = request.Header.Get(contract.ServiceIDHeader), request.Header.Get(contract.KeyIDHeader)
	headers, err := contract.ParseExchangeHeaders(request.Header)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "token request headers are invalid")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "token request is invalid")
		return
	}
	var body contract.ExchangeAuthorizationCodeRequest
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.GrantType != "authorization_code" {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "token request is invalid")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "token request is invalid")
		return
	}
	basicClientID, clientSecret, ok := request.BasicAuth()
	if !ok || basicClientID != body.ClientID {
		writeError(writer, request, http.StatusUnauthorized, "CLIENT_AUTH_FAILED", "client authentication failed")
		return
	}
	bodyHash := sha256.Sum256(rawBody)
	pathAndQuery := request.URL.EscapedPath()
	if request.URL.RawQuery != "" {
		pathAndQuery += "?" + request.URL.RawQuery
	}
	exchange, err := h.flow.Exchange(request.Context(), identity.ExchangeInput{
		ClientID: body.ClientID, ClientSecret: clientSecret, Code: body.Code,
		RedirectURI: body.RedirectURI, CodeVerifier: body.CodeVerifier,
		ServiceID: headers.ServiceID, KeyID: headers.KeyID,
		Timestamp: headers.Timestamp, Nonce: headers.Nonce,
		Signature: headers.Signature, BodyHash: bodyHash[:], IdempotencyKey: headers.IdempotencyKey,
		PathAndQuery: pathAndQuery,
	})
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	audit.subjectUserID = maskSubject(exchange.UserID)
	writeSuccess(writer, request, http.StatusOK, contract.ExchangeAuthorizationCodeResponse{
		User: contract.PlatformUser{
			UserID: exchange.UserID, DisplayName: exchange.DisplayName,
			EmailVerified: exchange.EmailVerified, Status: exchange.UserStatus, CreatedAt: exchange.UserCreatedAt,
		},
		SessionExchangeToken: exchange.SessionExchangeToken, ExpiresAt: exchange.ExpiresAt,
	})
}

func (h *Handler) checkAuthorization(writer http.ResponseWriter, request *http.Request) {
	audit := auditFrom(request.Context())
	audit.serviceID, audit.keyID = request.Header.Get(contract.ServiceIDHeader), request.Header.Get(contract.KeyIDHeader)
	timestamp, nonce, signature := request.Header.Get(contract.TimestampHeader), request.Header.Get(contract.NonceHeader), request.Header.Get(contract.SignatureHeader)
	if audit.serviceID == "" || audit.keyID == "" || timestamp == "" || nonce == "" || signature == "" {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "authorization request headers are invalid")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
		return
	}
	var body contract.AuthorizationCheckRequest
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
		return
	}
	clientID, clientSecret, ok := request.BasicAuth()
	if !ok || clientID != audit.serviceID {
		writeError(writer, request, http.StatusUnauthorized, "CLIENT_AUTH_FAILED", "client authentication failed")
		return
	}
	bodyHash := sha256.Sum256(rawBody)
	pathAndQuery := request.URL.EscapedPath()
	if request.URL.RawQuery != "" {
		pathAndQuery += "?" + request.URL.RawQuery
	}
	decision, err := h.flow.CheckAuthorization(request.Context(), identity.AuthorizationCheckInput{
		SessionToken: body.SessionExchangeToken, ClientSecret: clientSecret,
		PermissionCode: body.PermissionCode, ScopeKind: body.Scope.Kind,
		ProductCode: optionalString(body.Scope.ProductCode), ResourceType: optionalString(body.Scope.ResourceType), ResourceID: optionalString(body.Scope.ResourceID),
		ServiceID: audit.serviceID, KeyID: audit.keyID, Timestamp: timestamp, Nonce: nonce, Signature: signature,
		BodyHash: bodyHash[:], PathAndQuery: pathAndQuery, RequestID: audit.requestID,
	})
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	audit.subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, contract.AuthorizationDecision{
		Allowed: true, ActorUserID: decision.ActorUserID, PermissionCode: body.PermissionCode,
		Scope: body.Scope, GrantID: decision.GrantID, AuthorizationRevision: decision.AuthorizationRevision,
		CheckedAt: decision.CheckedAt,
	})
}

func (h *Handler) writeFlowError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, verification.ErrRandomSource):
		h.writeRandomSourceError(writer, request, "verification")
	case errors.Is(err, verification.ErrCodeExpired):
		writeError(writer, request, http.StatusBadRequest, "VERIFICATION_CODE_EXPIRED", "verification code expired")
	case errors.Is(err, verification.ErrCodeAlreadyUsed):
		writeError(writer, request, http.StatusConflict, "VERIFICATION_CODE_ALREADY_USED", "verification code was already used")
	case errors.Is(err, verification.ErrCodeInvalid):
		writeError(writer, request, http.StatusBadRequest, "VERIFICATION_CODE_INVALID", "verification code is invalid")
	case errors.Is(err, verification.ErrRegistrationRequired):
		writeError(writer, request, http.StatusConflict, "REGISTRATION_REQUIRED", "email identity must be registered before login")
	case errors.Is(err, verification.ErrAlreadyRegistered):
		writeError(writer, request, http.StatusConflict, "ACCOUNT_ALREADY_REGISTERED", "email identity is already registered")
	case errors.Is(err, verification.ErrAuthentication):
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "authentication failed")
	case errors.Is(err, verification.ErrChallengeRequired):
		writeError(writer, request, http.StatusTooManyRequests, "EMAIL_CODE_LOGIN_REQUIRED", "email-code login is required")
	case errors.Is(err, verification.ErrIdempotency):
		writeError(writer, request, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key conflicts with another request")
	case errors.Is(err, verification.ErrDependency):
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
	case errors.Is(err, verification.ErrRateLimited):
		writeError(writer, request, http.StatusTooManyRequests, "RATE_LIMITED", "verification attempts are temporarily limited")
	case errors.Is(err, verification.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "verification request is invalid")
	case errors.Is(err, identity.ErrUnauthorized):
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "authentication failed")
	case errors.Is(err, identity.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "PERMISSION_DENIED", "permission or scope is not granted")
	case errors.Is(err, identity.ErrCallback):
		writeError(writer, request, http.StatusBadRequest, "CALLBACK_NOT_REGISTERED", "callback is not registered")
	case errors.Is(err, identity.ErrCodeUsed):
		writeError(writer, request, http.StatusConflict, "AUTH_CODE_ALREADY_USED", "authorization code was already used")
	case errors.Is(err, identity.ErrCodeBusy):
		writeError(writer, request, http.StatusConflict, "AUTH_CODE_IN_USE", "authorization code exchange is already in progress")
	case errors.Is(err, identity.ErrCodeExpired):
		writeError(writer, request, http.StatusBadRequest, "AUTH_CODE_EXPIRED", "authorization code expired")
	case errors.Is(err, identity.ErrNonceReplay):
		writeError(writer, request, http.StatusConflict, "NONCE_ALREADY_USED", "request nonce was already used")
	case errors.Is(err, identity.ErrSignature):
		writeError(writer, request, http.StatusUnauthorized, "SIGNATURE_INVALID", "request signature is invalid")
	case errors.Is(err, identity.ErrTimestamp):
		writeError(writer, request, http.StatusUnauthorized, "TIMESTAMP_INVALID", "request timestamp is invalid")
	case errors.Is(err, identity.ErrIdempotency):
		writeError(writer, request, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key conflicts with another request")
	case errors.Is(err, identity.ErrIdempotencyBusy):
		writeError(writer, request, http.StatusConflict, "IDEMPOTENCY_IN_USE", "idempotent request is still in progress")
	case errors.Is(err, identity.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
	case errors.Is(err, identity.ErrDependency):
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
	default:
		writeError(writer, request, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}

func (h *Handler) writeRandomSourceError(writer http.ResponseWriter, request *http.Request, operation string) {
	h.logger.Error("security_random_source_unavailable", "request_id", requestIDFrom(request.Context()), "operation", operation)
	writeError(writer, request, http.StatusServiceUnavailable, "RANDOM_SOURCE_UNAVAILABLE", "temporarily unavailable")
}

func writeSuccess(writer http.ResponseWriter, request *http.Request, status int, data any) {
	writeJSON(writer, status, contract.SuccessEnvelope[any]{Data: data, RequestID: requestIDFrom(request.Context())})
}

func writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	auditFrom(request.Context()).errorCode = code
	writeJSON(writer, status, contract.ErrorEnvelope{Error: contract.ErrorObject{Code: code, Message: message}, RequestID: requestIDFrom(request.Context())})
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func requestID() string {
	bytes := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "req_unavailable"
	}
	return "req_" + strings.TrimRight(base64.RawURLEncoding.EncodeToString(bytes), "=")
}

type contextKey string

const requestContextKey contextKey = "request-audit"

type auditContext struct {
	requestID, errorCode, serviceID, keyID, subjectUserID string
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (h *Handler) requestAudit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		id := request.Header.Get("X-Request-Id")
		if !validRequestID(id) {
			id = requestID()
		}
		audit := &auditContext{requestID: id}
		request = request.WithContext(context.WithValue(request.Context(), requestContextKey, audit))
		writer.Header().Set("X-Request-Id", id)
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		h.logger.Info("http_request",
			"request_id", id, "method", request.Method, "path", request.URL.Path,
			"status", recorder.status, "error_code", audit.errorCode,
			"service_id", audit.serviceID, "key_id", audit.keyID,
			"subject_user_id", audit.subjectUserID, "duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func validRequestID(value string) bool {
	if !strings.HasPrefix(value, "req_") || len(value) < 8 || len(value) > 100 {
		return false
	}
	for _, character := range value[4:] {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '-') {
			return false
		}
	}
	return true
}

func auditFrom(ctx context.Context) *auditContext {
	audit, _ := ctx.Value(requestContextKey).(*auditContext)
	if audit == nil {
		return &auditContext{requestID: requestID()}
	}
	return audit
}

func requestIDFrom(ctx context.Context) string { return auditFrom(ctx).requestID }

func maskSubject(value string) string {
	if len(value) < 8 {
		return ""
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func remoteIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil && host != "" {
		return host
	}
	return remoteAddress
}

func (h *Handler) clientIP(request *http.Request) string {
	peer := net.ParseIP(remoteIP(request.RemoteAddr))
	if peer == nil || !h.isTrustedProxy(peer) {
		return remoteIP(request.RemoteAddr)
	}
	chain := strings.Split(request.Header.Get("X-Forwarded-For"), ",")
	addresses := make([]net.IP, 0, len(chain)+1)
	for _, value := range chain {
		parsed := net.ParseIP(strings.TrimSpace(value))
		if parsed == nil {
			return peer.String()
		}
		addresses = append(addresses, parsed)
	}
	addresses = append(addresses, peer)
	index := len(addresses) - 1
	for index > 0 && h.isTrustedProxy(addresses[index]) {
		index--
	}
	return addresses[index].String()
}

func (h *Handler) isTrustedProxy(address net.IP) bool {
	for _, network := range h.trustedProxies {
		if network.Contains(address) {
			return true
		}
	}
	return false
}

func (h *Handler) externallyHTTPS(request *http.Request) bool {
	if request.TLS != nil {
		return true
	}
	peer := net.ParseIP(remoteIP(request.RemoteAddr))
	return peer != nil && h.isTrustedProxy(peer) &&
		strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Forwarded-Proto")), "https")
}

func (h *Handler) browserCookies(request *http.Request) browserCookieProfile {
	if h.externallyHTTPS(request) {
		return browserCookieProfile{
			core: h.cookieName, csrf: "__Host-henukit_login_csrf",
			device: "__Host-henukit_device", secure: true,
		}
	}
	return browserCookieProfile{
		core: h.localCookieName, csrf: h.localCookieName + "_csrf",
		device: h.localCookieName + "_device",
	}
}

func (h *Handler) browserCookie(name, value string, maxAge int, expires time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name: name, Value: value, Path: "/", Expires: expires, MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	}
}

func (h *Handler) expiredBrowserCookie(name string, secure bool) *http.Cookie {
	return h.browserCookie(name, "", -1, time.Unix(1, 0), secure)
}

func (h *Handler) deviceID(request *http.Request) (string, *http.Cookie, error) {
	profile := h.browserCookies(request)
	if cookie, err := request.Cookie(profile.device); err == nil {
		parts := strings.Split(cookie.Value, ".")
		if len(parts) == 2 {
			identifier, decodeIDErr := base64.RawURLEncoding.DecodeString(parts[0])
			signature, decodeSignatureErr := base64.RawURLEncoding.DecodeString(parts[1])
			mac := hmac.New(sha256.New, h.deviceKey)
			_, _ = mac.Write(identifier)
			if decodeIDErr == nil && decodeSignatureErr == nil && len(identifier) == 16 && hmac.Equal(signature, mac.Sum(nil)) {
				return parts[0], nil, nil
			}
		}
	}
	identifier := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, identifier); err != nil {
		return "", nil, err
	}
	mac := hmac.New(sha256.New, h.deviceKey)
	_, _ = mac.Write(identifier)
	value := base64.RawURLEncoding.EncodeToString(identifier) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(identifier), h.browserCookie(profile.device, value, 365*24*60*60, time.Time{}, profile.secure), nil
}
