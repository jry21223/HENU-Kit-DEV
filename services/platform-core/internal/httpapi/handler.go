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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/platform-core/internal/careerdigestmail"
	"henukit.dev/platform-core/internal/contract"
	"henukit.dev/platform-core/internal/identity"
	"henukit.dev/platform-core/internal/oauthcontinuation"
	"henukit.dev/platform-core/internal/operationsinbox"
	"henukit.dev/platform-core/internal/platformoperations"
	"henukit.dev/platform-core/internal/store"
	"henukit.dev/platform-core/internal/verification"
	"henukit.dev/platform-core/internal/verificationmail"
)

type Handler struct {
	flow                 *identity.Service
	verification         *verification.Service
	continuations        *oauthcontinuation.Store
	inbox                *operationsinbox.Service
	platformOps          *platformoperations.Service
	queries              *store.Queries
	database             *pgxpool.Pool
	redis                *redis.Client
	cookieName           string
	localCookieName      string
	logger               *slog.Logger
	deliveryKeys         map[string][]byte
	deviceKey            []byte
	trustedProxies       []*net.IPNet
	digestMail           *careerdigestmail.Service
	careerDigestClientID string
	careerDigestKeys     map[string][]byte
}

type browserCookieProfile struct {
	core, csrf, device string
	secure             bool
}

const explicitFormResponseHeader = "X-Henukit-Form-Response"

func explicitAccountFormResponse(request *http.Request) bool {
	return request.Header.Get(explicitFormResponseHeader) == "status"
}

func writeExplicitAccountFormSuccess(writer http.ResponseWriter, request *http.Request) bool {
	if !explicitAccountFormResponse(request) {
		return false
	}
	writer.WriteHeader(http.StatusNoContent)
	return true
}

func New(flow *identity.Service, verificationFlow *verification.Service, continuations *oauthcontinuation.Store, inbox *operationsinbox.Service, platformOps *platformoperations.Service, queries *store.Queries, database *pgxpool.Pool, redisClient *redis.Client, cookieName, localCookieName string, deliveryKeys map[string][]byte, deviceKey []byte, trustedProxies []*net.IPNet, digestMail *careerdigestmail.Service, careerDigestClientID string, careerDigestKeys map[string][]byte, logger *slog.Logger) http.Handler {
	handler := &Handler{flow: flow, verification: verificationFlow, continuations: continuations, inbox: inbox, platformOps: platformOps, queries: queries, database: database, redis: redisClient, cookieName: cookieName, localCookieName: localCookieName, deliveryKeys: deliveryKeys, deviceKey: deviceKey, trustedProxies: trustedProxies, digestMail: digestMail, careerDigestClientID: careerDigestClientID, careerDigestKeys: careerDigestKeys, logger: logger}
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
	router.Get("/account/bootstrap", handler.accountBootstrap)
	router.Get("/account/security", handler.securityPage)
	router.Post("/account/security/code", handler.securityRequestCode)
	router.Post("/account/security/password", handler.securityChangePassword)
	router.Post("/account/continuation/resume", handler.resumeOAuthContinuation)
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
	router.Post(contract.PlatformOperationsAccountLookupRoute, handler.lookupPlatformOperationAccount)
	router.Post(contract.ConsoleUserIdentityResolutionRoute, handler.resolveConsoleUserIdentities)
	router.Post(contract.DisplayNamesRoute, handler.resolveUserDisplayNames)
	router.Post(contract.PlatformOperationsMembershipAccountsRoute, handler.listPlatformOperationMembershipAccounts)
	router.Post(contract.RevokePlatformOperationSessionRoute, handler.revokePlatformOperationSession)
	router.Post(contract.UpdatePlatformOperationAccessRoute, handler.updatePlatformOperationAccess)
	router.Get(contract.PlatformOperationStatusRoute, handler.getPlatformOperationStatus)
	router.Post(contract.CareerDigestMailEnqueueRoute, handler.enqueueCareerDigestMail)
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

func (h *Handler) lookupPlatformOperationAccount(writer http.ResponseWriter, request *http.Request) {
	rawBody, body, ok := decodeInboxBody[contract.PlatformOperationsAccountLookupRequest](writer, request)
	if !ok || len(body.Email) > 320 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Platform Operation request is invalid")
		return
	}
	decision, err := h.authorizeInbox(request, rawBody, "platform.operations.read", "platform", "", "", "")
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	result, err := h.platformOps.LookupAccount(request.Context(), request.Header.Get(contract.ServiceIDHeader), body.Email)
	if err != nil {
		h.writePlatformOperationError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, result)
}

func (h *Handler) resolveConsoleUserIdentities(writer http.ResponseWriter, request *http.Request) {
	rawBody, body, ok := decodeInboxBody[struct {
		UserIDs        []string `json:"user_ids"`
		PermissionCode string   `json:"permission_code"`
	}](writer, request)
	if !ok || len(body.UserIDs) == 0 || len(body.UserIDs) > 100 || !consoleTicketIdentityPermission(body.PermissionCode) {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Console identity request is invalid")
		return
	}
	decision, err := h.authorizeInbox(request, rawBody, body.PermissionCode, "product", "account-portfolio", "", "")
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	identities, err := h.platformOps.ResolveIdentities(request.Context(), body.UserIDs)
	if err != nil {
		h.writePlatformOperationError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, identities)
}

func consoleTicketIdentityPermission(permission string) bool {
	return permission == "account.tickets.read" || permission == "account.tickets.reply" || permission == "account.tickets.transition"
}

// resolveUserDisplayNames is the ADR-0038 read-only boundary that resolves a
// bounded batch of display names for an authenticated product service (the
// Portal Gateway's ranking nickname synthesis). It authenticates the calling
// service with the five-line HMAC credential — no session token, no browser
// identity — and returns only display names; email, status, and every other
// account field stay inside Platform Core. Unknown or unset ids resolve to
// null so the caller can batch-degrade without a 404.
func (h *Handler) resolveUserDisplayNames(writer http.ResponseWriter, request *http.Request) {
	audit := auditFrom(request.Context())
	audit.serviceID, audit.keyID = request.Header.Get(contract.ServiceIDHeader), request.Header.Get(contract.KeyIDHeader)
	rawBody, body, ok := decodeInboxBody[contract.ResolveUserDisplayNamesRequest](writer, request)
	if !ok || len(body.UserIDs) == 0 || len(body.UserIDs) > 100 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_BATCH_SIZE", "display-name batch must contain 1 to 100 user ids")
		return
	}
	clientID, clientSecret, ok := request.BasicAuth()
	if !ok || clientID != audit.serviceID {
		writeError(writer, request, http.StatusUnauthorized, "CLIENT_AUTH_FAILED", "client authentication failed")
		return
	}
	ids := make([]pgtype.UUID, 0, len(body.UserIDs))
	seen := make(map[uuid.UUID]struct{}, len(body.UserIDs))
	for _, rawID := range body.UserIDs {
		id, err := uuid.Parse(rawID)
		if err != nil {
			writeError(writer, request, http.StatusBadRequest, "INVALID_BATCH_SIZE", "display-name batch must contain only user ids")
			return
		}
		if _, exists := seen[id]; exists {
			writeError(writer, request, http.StatusBadRequest, "INVALID_BATCH_SIZE", "display-name batch must not repeat a user id")
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, pgtype.UUID{Bytes: id, Valid: true})
	}
	bodyHash := sha256.Sum256(rawBody)
	pathAndQuery := request.URL.EscapedPath()
	if request.URL.RawQuery != "" {
		pathAndQuery += "?" + request.URL.RawQuery
	}
	if err := h.flow.AuthenticateServiceRequest(request.Context(), identity.ServiceRequestCredentials{
		HTTPMethod: request.Method, ClientID: audit.serviceID, ClientSecret: clientSecret, KeyID: audit.keyID,
		Timestamp: request.Header.Get(contract.TimestampHeader), Nonce: request.Header.Get(contract.NonceHeader),
		Signature: request.Header.Get(contract.SignatureHeader), BodyHash: bodyHash[:], PathAndQuery: pathAndQuery,
		NonceNamespace: "display-names",
	}); err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	rows, err := h.queries.ListUserDisplayNames(request.Context(), ids)
	if err != nil {
		h.logger.Error("display_names_query_error", "request_id", requestIDFrom(request.Context()), "error", err)
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	found := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.DisplayName.Valid {
			if name := strings.TrimSpace(row.DisplayName.String); name != "" {
				found[row.ID.String()] = name
			}
		}
	}
	result := make(contract.DisplayNameMap, len(ids))
	for _, id := range ids {
		var name *string
		if value, exists := found[id.String()]; exists {
			name = &value
		}
		result[id.String()] = name
	}
	writeSuccess(writer, request, http.StatusOK, result)
}

func (h *Handler) listPlatformOperationMembershipAccounts(writer http.ResponseWriter, request *http.Request) {
	rawBody, body, ok := decodeInboxBody[struct {
		Query string `json:"query"`
		Page  int    `json:"page"`
	}](writer, request)
	if !ok || len([]rune(body.Query)) > 100 || body.Page < 1 || body.Page > 10_000 {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Membership account search is invalid")
		return
	}
	decision, err := h.authorizeInbox(request, rawBody, "account.membership.write", "product", "account-portfolio", "", "")
	if err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	result, err := h.platformOps.ListMembershipAccounts(request.Context(), body.Query, body.Page)
	if err != nil {
		h.writePlatformOperationError(writer, request, err)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(decision.ActorUserID)
	writeSuccess(writer, request, http.StatusOK, result)
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
	case errors.Is(err, platformoperations.ErrRateLimited):
		writeError(writer, request, http.StatusTooManyRequests, "RATE_LIMITED", "Platform Operation request is rate limited")
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

// enqueueCareerDigestMail is the #397 internal boundary: the Career worker
// enqueues one Opportunity Digest per completed search. The caller authenticates
// with the shared career-digest service credential (Basic + the five-line HMAC
// canonical form used across the repo); the browser never reaches this route
// and never supplies a recipient. The verified email is resolved and sealed
// server-side, and a replay for the same search is an idempotent no-op.
func (h *Handler) enqueueCareerDigestMail(writer http.ResponseWriter, request *http.Request) {
	if h.digestMail == nil {
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	audit := auditFrom(request.Context())
	audit.serviceID, audit.keyID = request.Header.Get(contract.ServiceIDHeader), request.Header.Get(contract.KeyIDHeader)
	clientID, clientSecret, basic := request.BasicAuth()
	secret, knownKey := h.careerDigestKeys[audit.keyID]
	rawTimestamp := request.Header.Get(contract.TimestampHeader)
	timestamp, parseErr := strconv.ParseInt(rawTimestamp, 10, 64)
	nonce := request.Header.Get(contract.NonceHeader)
	if !basic || clientID == "" || clientID != h.careerDigestClientID || audit.serviceID != clientID || !knownKey || parseErr != nil || !hmac.Equal([]byte(clientSecret), secret) || time.Since(time.Unix(timestamp, 0)).Abs() > 5*time.Minute {
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "career digest authentication failed")
		return
	}
	decodedNonce, decodeErr := base64.RawURLEncoding.DecodeString(nonce)
	if decodeErr != nil || len(decodedNonce) != 24 {
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "career digest authentication failed")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 32<<10)
	rawBody, readErr := io.ReadAll(request.Body)
	if readErr != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "career digest request is invalid")
		return
	}
	bodyHash := sha256.Sum256(rawBody)
	canonical := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", request.Method, request.URL.RequestURI(), rawTimestamp, nonce, hex.EncodeToString(bodyHash[:]))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	providedSignature, decodeErr := base64.RawURLEncoding.DecodeString(request.Header.Get(contract.SignatureHeader))
	if decodeErr != nil || !hmac.Equal(providedSignature, mac.Sum(nil)) {
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "career digest authentication failed")
		return
	}
	stored, err := h.redis.SetNX(request.Context(), "platform-core:career-digest:nonce:"+audit.keyID+":"+nonce, "1", 10*time.Minute).Result()
	if err != nil {
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	if !stored {
		writeError(writer, request, http.StatusConflict, "NONCE_ALREADY_USED", "career digest request was already submitted")
		return
	}
	var body contract.CareerDigestMailEnqueueRequest
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !validCareerDigestEnqueue(body) {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "career digest request is invalid")
		return
	}
	userID, err := uuid.Parse(body.UserID)
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "career digest request is invalid")
		return
	}
	topJobs := make([]verificationmail.Job, 0, len(body.TopJobs))
	for _, job := range body.TopJobs {
		topJobs = append(topJobs, verificationmail.Job{
			Company: job.Company, Title: job.Title, Location: job.Location,
			URL: job.URL, MatchScore: job.MatchScore, MatchReasons: job.MatchReasons,
		})
	}
	_, err = h.digestMail.Enqueue(request.Context(), careerdigestmail.EnqueueInput{
		UserID: userID, RequestID: requestIDFrom(request.Context()), SearchID: body.SearchID,
		CompletedAt: body.CompletedAt, SourceCount: body.SourceCount, JobCount: body.JobCount,
		MatchedCount: body.MatchedCount, Summary: body.Summary, CareerURL: body.CareerURL, TopJobs: topJobs,
	})
	if errors.Is(err, careerdigestmail.ErrNoVerifiedEmail) {
		writeError(writer, request, http.StatusNotFound, "VERIFIED_EMAIL_NOT_FOUND", "user has no verified email identity")
		return
	}
	if err != nil {
		h.logger.Error("career_digest_enqueue_error", "request_id", requestIDFrom(request.Context()), "error", err)
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(body.UserID)
	writeSuccess(writer, request, http.StatusAccepted, contract.CareerDigestMailEnqueued{Enqueued: true})
}

func validCareerDigestEnqueue(body contract.CareerDigestMailEnqueueRequest) bool {
	if body.UserID == "" || body.SearchID == "" || len(body.SearchID) > 100 || body.CompletedAt == "" ||
		body.Summary == "" || len(body.Summary) > 4000 ||
		body.SourceCount < 0 || body.JobCount < 0 || body.MatchedCount < 0 ||
		len(body.TopJobs) > 20 {
		return false
	}
	if _, err := time.Parse(time.RFC3339, body.CompletedAt); err != nil {
		return false
	}
	if body.CareerURL != "" && !validCareerDigestWebURL(body.CareerURL) {
		return false
	}
	for _, job := range body.TopJobs {
		if job.MatchScore < 0 || job.MatchScore > 100 ||
			len(job.Company) > 200 || len(job.Title) > 200 || len(job.Location) > 200 ||
			len(job.URL) > 1000 || len(job.MatchReasons) > 10 ||
			(job.URL != "" && !validCareerDigestWebURL(job.URL)) {
			return false
		}
		for _, reason := range job.MatchReasons {
			if len(reason) > 200 {
				return false
			}
		}
	}
	return true
}

func validCareerDigestWebURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func accountBrowserResponseHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func (h *Handler) accountBootstrap(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	flow := strings.TrimSpace(request.URL.Query().Get("flow"))
	if flow != "login" && flow != "register" && flow != "recover" && flow != "security" {
		writeError(writer, request, http.StatusBadRequest, "INVALID_ACCOUNT_FLOW", "account flow is invalid")
		return
	}
	profile := h.browserCookies(request)
	continuationHandle := strings.TrimSpace(request.URL.Query().Get("continuation"))
	var continuation *oauthcontinuation.Continuation
	if continuationHandle != "" {
		browserID, ok := h.existingDeviceID(request)
		if !ok {
			writeError(writer, request, http.StatusGone, "OAUTH_CONTINUATION_UNAVAILABLE", "OAuth continuation is unavailable")
			return
		}
		stored, err := h.continuations.Peek(request.Context(), continuationHandle, browserID)
		if err != nil {
			if errors.Is(err, oauthcontinuation.ErrDependency) {
				writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
			} else {
				writeError(writer, request, http.StatusGone, "OAUTH_CONTINUATION_UNAVAILABLE", "OAuth continuation is unavailable")
			}
			return
		}
		continuation = &stored
	}
	if flow == "security" {
		coreCookie, err := request.Cookie(profile.core)
		if err != nil || len(coreCookie.Value) < 32 {
			writeError(writer, request, http.StatusUnauthorized, "CORE_SESSION_REQUIRED", "Core Session is required")
			return
		}
		session, err := h.verification.CoreSession(request.Context(), coreCookie.Value)
		if err != nil {
			writeError(writer, request, http.StatusUnauthorized, "CORE_SESSION_REQUIRED", "Core Session is required")
			return
		}
		auditFrom(request.Context()).subjectUserID = maskSubject(session.UserID)
	}
	csrfToken, err := randomBrowserToken()
	if err != nil {
		h.writeRandomSourceError(writer, request, "account_bootstrap_csrf")
		return
	}
	_, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.writeRandomSourceError(writer, request, "account_bootstrap_device")
		return
	}
	http.SetCookie(writer, h.browserCookie(profile.csrf, csrfToken, 10*60, time.Time{}, profile.secure))
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	data := map[string]any{"flow": flow, "csrf_token": csrfToken}
	if continuation != nil {
		data["continuation"] = map[string]any{"available": true, "product_name": continuation.ProductName}
	}
	writeSuccess(writer, request, http.StatusOK, data)
}

func (h *Handler) resumeOAuthContinuation(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	if err := request.ParseForm(); err != nil {
		h.redirectOAuthContinuationFailure(writer, request, "expired")
		return
	}
	profile := h.browserCookies(request)
	csrfToken := request.FormValue("csrf_token")
	csrfCookie, err := request.Cookie(profile.csrf)
	if err != nil || len(csrfToken) < 32 || !hmac.Equal([]byte(csrfCookie.Value), []byte(csrfToken)) {
		h.redirectOAuthContinuationFailure(writer, request, "expired")
		return
	}
	coreCookie, err := request.Cookie(profile.core)
	if err != nil || len(coreCookie.Value) < 32 {
		h.redirectOAuthContinuationFailure(writer, request, "unavailable")
		return
	}
	if _, err := h.verification.CoreSession(request.Context(), coreCookie.Value); err != nil {
		h.redirectOAuthContinuationFailure(writer, request, "unavailable")
		return
	}
	browserID, ok := h.existingDeviceID(request)
	if !ok {
		h.redirectOAuthContinuationFailure(writer, request, "unavailable")
		return
	}
	continuation, err := h.continuations.Consume(request.Context(), strings.TrimSpace(request.FormValue("continuation")), browserID)
	if err != nil {
		switch {
		case errors.Is(err, oauthcontinuation.ErrDependency):
			h.redirectOAuthContinuationFailure(writer, request, "service")
		case errors.Is(err, oauthcontinuation.ErrConsumed):
			auditFrom(request.Context()).errorCode = "OAUTH_CONTINUATION_REPLAYED"
			h.redirectOAuthContinuationFailure(writer, request, "unavailable")
		default:
			h.redirectOAuthContinuationFailure(writer, request, "expired")
		}
		return
	}
	auditFrom(request.Context()).serviceID = continuation.ClientID
	authorization, err := h.flow.Authorize(request.Context(), identity.AuthorizeInput{
		CoreSessionToken: coreCookie.Value, ClientID: continuation.ClientID,
		RedirectURI: continuation.RedirectURI, CodeChallenge: continuation.CodeChallenge,
	})
	if err != nil {
		h.redirectOAuthContinuationFailure(writer, request, "unavailable")
		return
	}
	callback, err := url.Parse(continuation.RedirectURI)
	if err != nil {
		h.redirectOAuthContinuationFailure(writer, request, "unavailable")
		return
	}
	callbackQuery := callback.Query()
	callbackQuery.Set("code", authorization.Code)
	callbackQuery.Set("state", continuation.State)
	callback.RawQuery = callbackQuery.Encode()
	http.SetCookie(writer, h.browserCookie(profile.core, coreCookie.Value, max(1, int(time.Until(authorization.SessionExpires).Seconds())), authorization.SessionExpires, profile.secure))
	http.SetCookie(writer, h.expiredBrowserCookie(profile.csrf, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(authorization.UserID)
	writer.Header().Set("Pragma", "no-cache")
	http.Redirect(writer, request, callback.String(), http.StatusSeeOther)
}

func (h *Handler) redirectOAuthContinuationFailure(writer http.ResponseWriter, request *http.Request, category string) {
	accountBrowserResponseHeaders(writer)
	audit := auditFrom(request.Context())
	switch category {
	case "service":
		if audit.errorCode == "" {
			audit.errorCode = "OAUTH_CONTINUATION_DEPENDENCY_UNAVAILABLE"
		}
	case "unavailable":
		if audit.errorCode == "" {
			audit.errorCode = "OAUTH_CONTINUATION_UNAVAILABLE"
		}
	case "unsupported":
		if audit.errorCode == "" {
			audit.errorCode = "OAUTH_CONTINUATION_CLIENT_UNSUPPORTED"
		}
	default:
		category = "expired"
		if audit.errorCode == "" {
			audit.errorCode = "OAUTH_CONTINUATION_EXPIRED"
		}
	}
	writer.Header().Set("Pragma", "no-cache")
	target := "/account/login?" + url.Values{
		"continuation_error": {category}, "request_id": {requestIDFrom(request.Context())},
	}.Encode()
	http.Redirect(writer, request, target, http.StatusSeeOther)
}

func (h *Handler) registerPage(writer http.ResponseWriter, request *http.Request) {
	h.redirectToPortalAccountCenter(writer, request, "/account/login")
}

func (h *Handler) registerRequestCode(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	_, _, email, ok := h.parseRegistrationForm(writer, request)
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
		writeError(writer, request, http.StatusServiceUnavailable, "VERIFICATION_UNAVAILABLE", "verification code delivery is unavailable")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *Handler) registerAccount(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	_, returnTo, email, ok := h.parseRegistrationForm(writer, request)
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
		status, code, detail := http.StatusBadRequest, "REGISTRATION_FAILED", "registration was rejected"
		switch {
		case errors.Is(err, verification.ErrAlreadyRegistered):
			status, code, detail = http.StatusConflict, "ACCOUNT_ALREADY_REGISTERED", "account is already registered"
		case errors.Is(err, verification.ErrDependency):
			status, code, detail = http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "registration dependency is unavailable"
		}
		writeError(writer, request, status, code, detail)
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.core, registered.SessionToken, max(1, int(time.Until(registered.SessionExpiresAt).Seconds())), registered.SessionExpiresAt, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(registered.UserID)
	if writeExplicitAccountFormSuccess(writer, request) {
		return
	}
	http.SetCookie(writer, h.expiredBrowserCookie(profile.csrf, profile.secure))
	http.Redirect(writer, request, returnTo, http.StatusSeeOther)
}

func (h *Handler) recoverPage(writer http.ResponseWriter, request *http.Request) {
	h.redirectToPortalAccountCenter(writer, request, "/account/recover")
}

func (h *Handler) recoverRequestCode(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	_, _, email, ok := h.parseLoginForm(writer, request)
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
	if err != nil {
		writeError(writer, request, http.StatusServiceUnavailable, "VERIFICATION_UNAVAILABLE", "verification code delivery is unavailable")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *Handler) recoverPassword(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	_, _, email, ok := h.parseLoginForm(writer, request)
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
		status, code, detail := http.StatusBadRequest, "RECOVERY_FAILED", "password recovery was rejected"
		if errors.Is(err, verification.ErrDependency) {
			status, code, detail = http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "password recovery dependency is unavailable"
		}
		writeError(writer, request, status, code, detail)
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.core, recovered.SessionToken, max(1, int(time.Until(recovered.SessionExpiresAt).Seconds())), recovered.SessionExpiresAt, profile.secure))
	http.SetCookie(writer, h.expiredBrowserCookie(profile.csrf, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(recovered.UserID)
	if writeExplicitAccountFormSuccess(writer, request) {
		return
	}
	http.Redirect(writer, request, "/account/security", http.StatusSeeOther)
}

func (h *Handler) securityPage(writer http.ResponseWriter, request *http.Request) {
	h.redirectToPortalAccountCenter(writer, request, "/account/login?next=%2Faccount%2Fsecurity")
}

func (h *Handler) securityRequestCode(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	_, _, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return
	}
	profile := h.browserCookies(request)
	coreCookie, err := request.Cookie(profile.core)
	if err != nil {
		writeError(writer, request, http.StatusUnauthorized, "CORE_SESSION_REQUIRED", "sign in again to change account security settings")
		return
	}
	session, err := h.verification.CoreSession(request.Context(), coreCookie.Value)
	if err != nil {
		writeError(writer, request, http.StatusUnauthorized, "CORE_SESSION_REQUIRED", "sign in again to change account security settings")
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
	if err != nil {
		writeError(writer, request, http.StatusServiceUnavailable, "VERIFICATION_UNAVAILABLE", "verification code delivery is unavailable")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *Handler) securityChangePassword(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	_, _, email, ok := h.parseLoginForm(writer, request)
	if !ok {
		return
	}
	profile := h.browserCookies(request)
	coreCookie, err := request.Cookie(profile.core)
	if err != nil {
		writeError(writer, request, http.StatusUnauthorized, "CORE_SESSION_REQUIRED", "sign in again to change account security settings")
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
		status, code, detail := http.StatusBadRequest, "PASSWORD_CHANGE_FAILED", "password change was rejected"
		switch {
		case errors.Is(err, verification.ErrChallengeRequired):
			status, code, detail = http.StatusTooManyRequests, "EMAIL_CODE_LOGIN_REQUIRED", "email-code login is required"
		case errors.Is(err, verification.ErrDependency):
			status, code, detail = http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "password change dependency is unavailable"
		}
		writeError(writer, request, status, code, detail)
		return
	}
	auditFrom(request.Context()).subjectUserID = maskSubject(changed.UserID)
	if writeExplicitAccountFormSuccess(writer, request) {
		return
	}
	http.Redirect(writer, request, "/account/security?password_changed=1", http.StatusSeeOther)
}

func (h *Handler) loginPage(writer http.ResponseWriter, request *http.Request) {
	h.redirectToPortalAccountCenter(writer, request, "/account/login")
}

func (h *Handler) loginRequestCode(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	_, _, email, ok := h.parseLoginForm(writer, request)
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
		writeError(writer, request, http.StatusServiceUnavailable, "VERIFICATION_UNAVAILABLE", "verification code delivery is unavailable")
		return
	}
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	writer.Header().Set("X-Verification-Expires", accepted.ExpiresAt.Format(time.RFC3339))
	writer.WriteHeader(http.StatusNoContent)
}

func (h *Handler) loginVerifyCode(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	_, returnTo, email, ok := h.parseLoginForm(writer, request)
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
		status, code, detail := http.StatusUnauthorized, "AUTHENTICATION_FAILED", "email code is invalid or expired"
		if errors.Is(err, verification.ErrDependency) {
			status, code, detail = http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "authentication dependency is unavailable"
		}
		writeError(writer, request, status, code, detail)
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.core, verified.SessionToken, max(1, int(time.Until(verified.SessionExpiresAt).Seconds())), verified.SessionExpiresAt, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(verified.UserID)
	if writeExplicitAccountFormSuccess(writer, request) {
		return
	}
	http.SetCookie(writer, h.expiredBrowserCookie(profile.csrf, profile.secure))
	http.Redirect(writer, request, returnTo, http.StatusSeeOther)
}

func (h *Handler) loginPassword(writer http.ResponseWriter, request *http.Request) {
	accountBrowserResponseHeaders(writer)
	_, returnTo, email, ok := h.parseLoginForm(writer, request)
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
		status, code, detail := http.StatusUnauthorized, "AUTHENTICATION_FAILED", "authentication failed"
		switch {
		case errors.Is(err, verification.ErrChallengeRequired):
			status, code, detail = http.StatusTooManyRequests, "EMAIL_CODE_LOGIN_REQUIRED", "email-code login is required"
		case errors.Is(err, verification.ErrDependency):
			status, code, detail = http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "authentication dependency is unavailable"
		}
		writeError(writer, request, status, code, detail)
		return
	}
	profile := h.browserCookies(request)
	http.SetCookie(writer, h.browserCookie(profile.core, loggedIn.SessionToken, max(1, int(time.Until(loggedIn.SessionExpiresAt).Seconds())), loggedIn.SessionExpiresAt, profile.secure))
	auditFrom(request.Context()).subjectUserID = maskSubject(loggedIn.UserID)
	if writeExplicitAccountFormSuccess(writer, request) {
		return
	}
	http.SetCookie(writer, h.expiredBrowserCookie(profile.csrf, profile.secure))
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

func (h *Handler) redirectToPortalAccountCenter(writer http.ResponseWriter, request *http.Request, target string) {
	accountBrowserResponseHeaders(writer)
	writer.Header().Set("Pragma", "no-cache")
	http.Redirect(writer, request, target, http.StatusFound)
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
	if _, supported := oauthContinuationProductName(query.ClientID); !supported {
		input := identity.AuthorizeInput{
			ClientID: query.ClientID, RedirectURI: query.RedirectURI, CodeChallenge: query.CodeChallenge,
		}
		if err := h.flow.ValidateAuthorizeRequest(request.Context(), input); err != nil {
			h.writeFlowError(writer, request, err)
			return
		}
		h.redirectUnsupportedOAuthContinuation(writer, request, query.ClientID)
		return
	}
	profile := h.browserCookies(request)
	cookie, err := request.Cookie(profile.core)
	if err != nil {
		h.startOAuthContinuation(writer, request, query)
		return
	}
	authorization, err := h.flow.Authorize(request.Context(), identity.AuthorizeInput{
		CoreSessionToken: cookie.Value,
		ClientID:         query.ClientID, RedirectURI: query.RedirectURI, CodeChallenge: query.CodeChallenge,
	})
	if err != nil {
		if errors.Is(err, identity.ErrUnauthorized) {
			h.startOAuthContinuation(writer, request, query)
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

func (h *Handler) startOAuthContinuation(writer http.ResponseWriter, request *http.Request, query contract.AuthorizeOAuthClientQuery) {
	input := identity.AuthorizeInput{
		ClientID: query.ClientID, RedirectURI: query.RedirectURI, CodeChallenge: query.CodeChallenge,
	}
	if err := h.flow.ValidateAuthorizeRequest(request.Context(), input); err != nil {
		h.writeFlowError(writer, request, err)
		return
	}
	auditFrom(request.Context()).serviceID = query.ClientID
	productName, supported := oauthContinuationProductName(query.ClientID)
	if !supported {
		h.redirectUnsupportedOAuthContinuation(writer, request, query.ClientID)
		return
	}
	browserID, deviceCookie, err := h.deviceID(request)
	if err != nil {
		h.redirectOAuthContinuationFailure(writer, request, "service")
		return
	}
	allowed, err := h.continuations.ConsumeCreationQuota(request.Context(), query.ClientID, browserID, h.clientIP(request))
	if err != nil {
		h.redirectOAuthContinuationFailure(writer, request, "service")
		return
	}
	if !allowed {
		h.redirectOAuthContinuationFailure(writer, request, "service")
		auditFrom(request.Context()).errorCode = "OAUTH_CONTINUATION_RATE_LIMITED"
		return
	}
	handle, err := h.continuations.Create(request.Context(), oauthcontinuation.CreateInput{
		ClientID: query.ClientID, ProductName: productName, RedirectURI: query.RedirectURI,
		State: query.State, CodeChallenge: query.CodeChallenge, BrowserID: browserID,
	})
	if err != nil {
		h.redirectOAuthContinuationFailure(writer, request, "service")
		return
	}
	if deviceCookie != nil {
		http.SetCookie(writer, deviceCookie)
	}
	accountBrowserResponseHeaders(writer)
	http.Redirect(writer, request, "/account/login?"+url.Values{"continuation": {handle}}.Encode(), http.StatusFound)
}

func (h *Handler) redirectUnsupportedOAuthContinuation(writer http.ResponseWriter, request *http.Request, clientID string) {
	auditFrom(request.Context()).serviceID = clientID
	auditFrom(request.Context()).errorCode = "OAUTH_CONTINUATION_CLIENT_UNSUPPORTED"
	h.redirectOAuthContinuationFailure(writer, request, "unsupported")
}

func oauthContinuationProductName(clientID string) (string, bool) {
	switch clientID {
	case "portal-gateway":
		return "HENU Kit", true
	case "console-gateway":
		return "HENUKit Console", true
	default:
		return "", false
	}
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
	var displayName *string
	if exchange.DisplayName != "" {
		displayName = &exchange.DisplayName
	}
	writeSuccess(writer, request, http.StatusOK, contract.ExchangeAuthorizationCodeResponse{
		User: contract.PlatformUser{
			UserID: exchange.UserID, DisplayName: displayName,
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
		writeError(writer, request, http.StatusBadRequest, "VERIFICATION_CODE_EXPIRED", "验证码已过期，请重新获取")
	case errors.Is(err, verification.ErrCodeAlreadyUsed):
		writeError(writer, request, http.StatusConflict, "VERIFICATION_CODE_ALREADY_USED", "验证码已使用过，请重新获取")
	case errors.Is(err, verification.ErrCodeInvalid):
		writeError(writer, request, http.StatusBadRequest, "VERIFICATION_CODE_INVALID", "验证码不正确，请重新输入")
	case errors.Is(err, verification.ErrRegistrationRequired):
		writeError(writer, request, http.StatusConflict, "REGISTRATION_REQUIRED", "该邮箱还没有注册，请先注册或换个邮箱")
	case errors.Is(err, verification.ErrAlreadyRegistered):
		writeError(writer, request, http.StatusConflict, "ACCOUNT_ALREADY_REGISTERED", "该邮箱已注册，请直接登录")
	case errors.Is(err, verification.ErrAuthentication):
		writeError(writer, request, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "邮箱或密码错误，或登录暂不可用")
	case errors.Is(err, verification.ErrChallengeRequired):
		writeError(writer, request, http.StatusTooManyRequests, "EMAIL_CODE_LOGIN_REQUIRED", "请改用邮箱验证码登录")
	case errors.Is(err, verification.ErrIdempotency):
		writeError(writer, request, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key conflicts with another request")
	case errors.Is(err, verification.ErrDependency):
		writeError(writer, request, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "service dependency is unavailable")
	case errors.Is(err, verification.ErrRateLimited):
		writeError(writer, request, http.StatusTooManyRequests, "RATE_LIMITED", "操作太频繁，请稍后再试")
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
		h.logger.Error("internal_error", "request_id", requestIDFrom(request.Context()), "error", err)
		writeError(writer, request, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用，请稍后再来")
	}
}

func (h *Handler) writeRandomSourceError(writer http.ResponseWriter, request *http.Request, operation string) {
	h.logger.Error("security_random_source_unavailable", "request_id", requestIDFrom(request.Context()), "operation", operation)
	writeError(writer, request, http.StatusServiceUnavailable, "RANDOM_SOURCE_UNAVAILABLE", "服务暂时不可用，请稍后再来")
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
	if identifier, ok := h.existingDeviceID(request); ok {
		return identifier, nil, nil
	}
	profile := h.browserCookies(request)
	identifier := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, identifier); err != nil {
		return "", nil, err
	}
	mac := hmac.New(sha256.New, h.deviceKey)
	_, _ = mac.Write(identifier)
	value := base64.RawURLEncoding.EncodeToString(identifier) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(identifier), h.browserCookie(profile.device, value, 365*24*60*60, time.Time{}, profile.secure), nil
}

func (h *Handler) existingDeviceID(request *http.Request) (string, bool) {
	cookie, err := request.Cookie(h.browserCookies(request).device)
	if err != nil {
		return "", false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return "", false
	}
	identifier, decodeIDErr := base64.RawURLEncoding.DecodeString(parts[0])
	signature, decodeSignatureErr := base64.RawURLEncoding.DecodeString(parts[1])
	mac := hmac.New(sha256.New, h.deviceKey)
	_, _ = mac.Write(identifier)
	if decodeIDErr != nil || decodeSignatureErr != nil || len(identifier) != 16 || !hmac.Equal(signature, mac.Sum(nil)) {
		return "", false
	}
	return parts[0], true
}
