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
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/platform-core/internal/contract"
	"henukit.dev/platform-core/internal/identity"
	"henukit.dev/platform-core/internal/operationsinbox"
	"henukit.dev/platform-core/internal/store"
	"henukit.dev/platform-core/internal/verification"
)

type Handler struct {
	flow           *identity.Service
	verification   *verification.Service
	inbox          *operationsinbox.Service
	queries        *store.Queries
	database       *pgxpool.Pool
	redis          *redis.Client
	cookieName     string
	logger         *slog.Logger
	deliveryKeys   map[string][]byte
	deviceKey      []byte
	trustedProxies []*net.IPNet
}

func New(flow *identity.Service, verificationFlow *verification.Service, inbox *operationsinbox.Service, queries *store.Queries, database *pgxpool.Pool, redisClient *redis.Client, cookieName string, deliveryKeys map[string][]byte, deviceKey []byte, trustedProxies []*net.IPNet, logger *slog.Logger) http.Handler {
	handler := &Handler{flow: flow, verification: verificationFlow, inbox: inbox, queries: queries, database: database, redis: redisClient, cookieName: cookieName, deliveryKeys: deliveryKeys, deviceKey: deviceKey, trustedProxies: trustedProxies, logger: logger}
	router := chi.NewRouter()
	router.Use(handler.requestAudit)
	router.Get("/api/v1/healthz", handler.health)
	router.Get("/api/v1/readyz", handler.ready)
	router.Get(contract.AuthorizeRoute, handler.authorize)
	router.Post(contract.TokenRoute, handler.exchange)
	router.Post(contract.AuthorizationCheckRoute, handler.checkAuthorization)
	router.Post(contract.RequestVerificationCodeRoute, handler.requestVerificationCode)
	router.Post(contract.VerifyVerificationCodeRoute, handler.verifyVerificationCode)
	router.Post(contract.RecordMailDeliveryRoute, handler.recordMailDelivery)
	router.Get(contract.ListOperationsInboxRoute, handler.listOperationsInboxItems)
	router.Post(contract.CreateOperationsInboxRoute, handler.createOperationsInboxItem)
	router.Post(contract.UpdateOperationsInboxRoute, handler.updateOperationsInboxItem)
	return router
}

func (h *Handler) listOperationsInboxItems(writer http.ResponseWriter, request *http.Request) {
	productCode, status := request.URL.Query().Get("product_code"), request.URL.Query().Get("status")
	decision, err := h.authorizeInbox(request, nil, "platform.operations_inbox.read", "product", productCode, "", "")
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	items, err := h.inbox.List(request.Context(), productCode, status)
	if err != nil {
		h.writeInboxError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	result := make([]contract.OperationsInboxItem, 0, len(items))
	for _, item := range items {
		result = append(result, inboxContractItem(item))
	}
	writeSuccess(writer, request, http.StatusOK, map[string]any{"items": result})
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
		ActorUserID: decision.ActorUserID, RequestID: requestIDFrom(request.Context()),
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
		ItemID: itemID, ActorUserID: decision.ActorUserID, RequestID: requestIDFrom(request.Context()),
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
	accepted, err := h.verification.Request(request.Context(), verification.RequestInput{
		Email: body.Email, Purpose: body.Purpose, ClientID: optionalString(body.ClientID),
		DeviceID: h.deviceID(writer, request), ClientIP: h.clientIP(request),
		IdempotencyKey: idempotencyKey, RequestID: requestIDFrom(request.Context()),
	})
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusAccepted, contract.VerificationCodeAccepted{ExpiresAt: accepted.ExpiresAt, ResendAfter: accepted.ResendAfter})
}

func (h *Handler) verifyVerificationCode(writer http.ResponseWriter, request *http.Request) {
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
	verified, err := h.verification.Verify(request.Context(), verification.VerifyInput{
		Email: body.Email, Code: body.Code, Purpose: body.Purpose, IdempotencyKey: idempotencyKey,
		DeviceID: h.deviceID(writer, request), ClientIP: h.clientIP(request),
	})
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	writeSuccess(writer, request, http.StatusOK, contract.VerificationCodeVerified{VerificationID: verified.VerificationID})
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
	if err != nil || callback.Scheme != "https" || callback.Host == "" || callback.User != nil || callback.Fragment != "" {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "authorization request is invalid")
		return
	}
	cookie, err := request.Cookie(h.cookieName)
	if err != nil {
		writeError(writer, request, http.StatusUnauthorized, "SESSION_REQUIRED", "a valid Core Session is required")
		return
	}
	authorization, err := h.flow.Authorize(request.Context(), identity.AuthorizeInput{
		CoreSessionToken: cookie.Value,
		ClientID:         query.ClientID, RedirectURI: query.RedirectURI, CodeChallenge: query.CodeChallenge,
	})
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: h.cookieName, Value: cookie.Value, Path: "/", Expires: authorization.SessionExpires,
		MaxAge: max(1, int(time.Until(authorization.SessionExpires).Seconds())), HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
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
		User:                 contract.PlatformUser{UserID: exchange.UserID, EmailVerified: exchange.EmailVerified, Status: exchange.UserStatus, CreatedAt: exchange.UserCreatedAt},
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
	case errors.Is(err, verification.ErrCodeExpired):
		writeError(writer, request, http.StatusBadRequest, "VERIFICATION_CODE_EXPIRED", "verification code expired")
	case errors.Is(err, verification.ErrCodeAlreadyUsed):
		writeError(writer, request, http.StatusConflict, "VERIFICATION_CODE_ALREADY_USED", "verification code was already used")
	case errors.Is(err, verification.ErrCodeInvalid):
		writeError(writer, request, http.StatusBadRequest, "VERIFICATION_CODE_INVALID", "verification code is invalid")
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
	if _, err := rand.Read(bytes); err != nil {
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

func (h *Handler) deviceID(writer http.ResponseWriter, request *http.Request) string {
	const name = "__Host-henukit_device"
	if cookie, err := request.Cookie(name); err == nil {
		parts := strings.Split(cookie.Value, ".")
		if len(parts) == 2 {
			identifier, decodeIDErr := base64.RawURLEncoding.DecodeString(parts[0])
			signature, decodeSignatureErr := base64.RawURLEncoding.DecodeString(parts[1])
			mac := hmac.New(sha256.New, h.deviceKey)
			_, _ = mac.Write(identifier)
			if decodeIDErr == nil && decodeSignatureErr == nil && len(identifier) == 16 && hmac.Equal(signature, mac.Sum(nil)) {
				return parts[0]
			}
		}
	}
	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); err != nil {
		return "device-unavailable"
	}
	mac := hmac.New(sha256.New, h.deviceKey)
	_, _ = mac.Write(identifier)
	value := base64.RawURLEncoding.EncodeToString(identifier) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(writer, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	return base64.RawURLEncoding.EncodeToString(identifier)
}
