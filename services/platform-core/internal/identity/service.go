package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/coordination"
	"henukit.dev/platform-core/internal/store"
)

var (
	ErrUnauthorized    = errors.New("authentication failed")
	ErrForbidden       = errors.New("permission or scope is not granted")
	ErrInvalid         = errors.New("invalid authorization request")
	ErrCallback        = errors.New("callback is not registered")
	ErrCodeUsed        = errors.New("authorization code was already used")
	ErrCodeExpired     = errors.New("authorization code expired")
	ErrCodeBusy        = errors.New("authorization code exchange in progress")
	ErrDependency      = errors.New("coordination dependency unavailable")
	ErrNonceReplay     = errors.New("request nonce was already used")
	ErrSignature       = errors.New("request signature is invalid")
	ErrTimestamp       = errors.New("request timestamp is invalid")
	ErrIdempotency     = errors.New("idempotency key conflicts with another request")
	ErrIdempotencyBusy = errors.New("idempotent request is still in progress")
)

var (
	permissionCodePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9_-]*)+$`)
	productCodePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)
	resourceTypePattern   = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
)

type Coordinator interface {
	Acquire(context.Context, string, time.Duration) (func(context.Context) error, error)
	UseOnce(context.Context, string, time.Duration) error
}

type Service struct {
	queries            *store.Queries
	database           *pgxpool.Pool
	coordinator        Coordinator
	authorizationTTL   time.Duration
	exchangeSessionTTL time.Duration
	// exchangeSessionTTLByClientID holds per-client overrides for the exchange Session
	// TTL. The Portal OAuth client overrides the 8-hour default to 30 days so
	// the Portal Session cookie and its permission checks survive for the whole
	// Core Session window; clients without an override keep the short,
	// high-privilege default.
	exchangeSessionTTLByClientID map[string]time.Duration
	idempotencyTTL               time.Duration
}

type AuthorizeInput struct {
	CoreSessionToken string
	ClientID         string
	RedirectURI      string
	CodeChallenge    string
}

type Authorization struct {
	Code           string
	SessionExpires time.Time
	UserID         string
}

type ExchangeInput struct {
	ClientID       string
	ClientSecret   string
	Code           string
	RedirectURI    string
	CodeVerifier   string
	ServiceID      string
	KeyID          string
	Timestamp      string
	Nonce          string
	Signature      string
	BodyHash       []byte
	IdempotencyKey string
	PathAndQuery   string
}

type Exchange struct {
	SessionExchangeToken string
	ExpiresAt            time.Time
	UserID               string
	DisplayName          string
	EmailVerified        bool
	UserStatus           string
	UserCreatedAt        time.Time
}

type AuthorizationCheckInput struct {
	HTTPMethod     string
	SessionToken   string
	ClientSecret   string
	PermissionCode string
	ScopeKind      string
	ProductCode    string
	ResourceType   string
	ResourceID     string
	ServiceID      string
	KeyID          string
	Timestamp      string
	Nonce          string
	Signature      string
	BodyHash       []byte
	PathAndQuery   string
	RequestID      string
}

type AuthorizationDecision struct {
	ActorUserID           string
	GrantID               string
	AuthorizationRevision int64
	CheckedAt             time.Time
}

type serviceRequestCredentials struct {
	HTTPMethod     string
	ClientID       string
	ClientSecret   string
	KeyID          string
	Timestamp      string
	Nonce          string
	Signature      string
	BodyHash       []byte
	PathAndQuery   string
	NonceNamespace string
}

// ServiceRequestCredentials carries the five-line HMAC service credential for
// an internal read-only product boundary (e.g. display-name resolution) that
// authenticates the calling service without binding a browser session.
type ServiceRequestCredentials struct {
	HTTPMethod     string
	ClientID       string
	ClientSecret   string
	KeyID          string
	Timestamp      string
	Nonce          string
	Signature      string
	BodyHash       []byte
	PathAndQuery   string
	NonceNamespace string
}

// AuthenticateServiceRequest validates a caller-supplied five-line HMAC
// service credential. It is the exported entry point for handlers that need
// service authentication without an actor-bound session (ADR-0038).
func (s *Service) AuthenticateServiceRequest(ctx context.Context, credentials ServiceRequestCredentials) error {
	return s.authenticateServiceRequest(ctx, serviceRequestCredentials{
		HTTPMethod: credentials.HTTPMethod, ClientID: credentials.ClientID, ClientSecret: credentials.ClientSecret,
		KeyID: credentials.KeyID, Timestamp: credentials.Timestamp, Nonce: credentials.Nonce,
		Signature: credentials.Signature, BodyHash: credentials.BodyHash, PathAndQuery: credentials.PathAndQuery,
		NonceNamespace: credentials.NonceNamespace,
	})
}

func New(queries *store.Queries, database *pgxpool.Pool, coordinator Coordinator, authorizationTTL, exchangeSessionTTL time.Duration, exchangeSessionTTLByClientID map[string]time.Duration, idempotencyTTL time.Duration) *Service {
	return &Service{queries: queries, database: database, coordinator: coordinator, authorizationTTL: authorizationTTL, exchangeSessionTTL: exchangeSessionTTL, exchangeSessionTTLByClientID: exchangeSessionTTLByClientID, idempotencyTTL: idempotencyTTL}
}

func (s *Service) authenticateServiceRequest(ctx context.Context, credentials serviceRequestCredentials) error {
	if credentials.HTTPMethod == "" {
		credentials.HTTPMethod = "POST"
	}
	if credentials.ClientID == "" || credentials.ClientSecret == "" || credentials.KeyID == "" || credentials.Timestamp == "" || credentials.Nonce == "" || credentials.Signature == "" || len(credentials.BodyHash) != sha256.Size || !strings.HasPrefix(credentials.PathAndQuery, "/") || credentials.NonceNamespace == "" {
		return ErrInvalid
	}
	if credentials.HTTPMethod != "GET" && credentials.HTTPMethod != "POST" {
		return ErrInvalid
	}
	clientKey, err := s.queries.GetOAuthClientKey(ctx, store.GetOAuthClientKeyParams{ClientID: credentials.ClientID, KeyID: credentials.KeyID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthorized
		}
		return err
	}
	secretHash := sha256.Sum256([]byte(credentials.ClientSecret))
	if len(clientKey.SecretHash) != len(secretHash) || subtle.ConstantTimeCompare(clientKey.SecretHash, secretHash[:]) != 1 {
		return ErrUnauthorized
	}
	requestTimeUnix, err := strconv.ParseInt(credentials.Timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(requestTimeUnix, 0)).Abs() > 5*time.Minute {
		return ErrTimestamp
	}
	canonical := strings.Join([]string{credentials.HTTPMethod, credentials.PathAndQuery, credentials.Timestamp, credentials.Nonce, hex.EncodeToString(credentials.BodyHash)}, "\n")
	mac := hmac.New(sha256.New, []byte(credentials.ClientSecret))
	_, _ = mac.Write([]byte(canonical))
	expectedSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(credentials.Signature), []byte(expectedSignature)) != 1 {
		return ErrSignature
	}
	if err := s.coordinator.UseOnce(ctx, "platform-core:"+credentials.NonceNamespace+"-nonce:"+credentials.ClientID+":"+credentials.Nonce, 10*time.Minute); err != nil {
		if errors.Is(err, coordination.ErrBusy) {
			return ErrNonceReplay
		}
		return ErrDependency
	}
	return nil
}

func (s *Service) Authorize(ctx context.Context, input AuthorizeInput) (Authorization, error) {
	if input.CoreSessionToken == "" || input.ClientID == "" || input.RedirectURI == "" || !validCodeChallenge(input.CodeChallenge) {
		return Authorization{}, ErrInvalid
	}
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Authorization{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := s.queries.WithTx(tx)
	tokenHash := sha256.Sum256([]byte(input.CoreSessionToken))
	session, err := queries.GetActiveCoreSessionByTokenHash(ctx, tokenHash[:])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Authorization{}, ErrUnauthorized
		}
		return Authorization{}, err
	}
	if session.Status != "active" {
		return Authorization{}, ErrUnauthorized
	}
	client, err := queries.GetOAuthClient(ctx, input.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Authorization{}, ErrCallback
		}
		return Authorization{}, err
	}
	if !containsExact(client.RedirectUris, input.RedirectURI) {
		return Authorization{}, ErrCallback
	}
	code, codeHash, err := randomToken()
	if err != nil {
		return Authorization{}, err
	}
	expiresAt := time.Now().UTC().Add(s.authorizationTTL)
	_, err = queries.CreateAuthorizationCode(ctx, store.CreateAuthorizationCodeParams{
		CodeHash: codeHash, UserID: session.UserID, ClientID: input.ClientID, CoreSessionID: session.ID,
		RedirectUri: input.RedirectURI, CodeChallenge: input.CodeChallenge,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return Authorization{}, err
	}
	if err := queries.TouchCoreSession(ctx, session.ID); err != nil {
		return Authorization{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Authorization{}, err
	}
	return Authorization{Code: code, SessionExpires: session.ExpiresAt.Time, UserID: uuidString(session.UserID)}, nil
}

func (s *Service) Exchange(ctx context.Context, input ExchangeInput) (Exchange, error) {
	if input.ClientID == "" || input.Code == "" || input.RedirectURI == "" || input.CodeVerifier == "" || input.ServiceID == "" || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 {
		return Exchange{}, ErrInvalid
	}
	if err := s.authenticateServiceRequest(ctx, serviceRequestCredentials{
		ClientID: input.ClientID, ClientSecret: input.ClientSecret, KeyID: input.KeyID,
		Timestamp: input.Timestamp, Nonce: input.Nonce, Signature: input.Signature,
		BodyHash: input.BodyHash, PathAndQuery: input.PathAndQuery, NonceNamespace: "oauth",
	}); err != nil {
		return Exchange{}, err
	}
	if input.ServiceID != input.ClientID {
		return Exchange{}, ErrSignature
	}
	if cached, found, err := s.lookupIdempotency(ctx, s.queries, input); err != nil {
		return Exchange{}, err
	} else if found {
		return cached, nil
	}
	idempotencyHash := sha256.Sum256([]byte(input.ClientID + "\x00" + input.IdempotencyKey))
	releaseIdempotency, err := s.acquireWithWait(ctx, "platform-core:oauth-idempotency:"+hex.EncodeToString(idempotencyHash[:]), 30*time.Second, 5*time.Second)
	if err != nil {
		if errors.Is(err, coordination.ErrBusy) {
			return Exchange{}, ErrIdempotencyBusy
		}
		return Exchange{}, ErrDependency
	}
	defer releaseCoordination(releaseIdempotency)()
	if cached, found, err := s.lookupIdempotency(ctx, s.queries, input); err != nil {
		return Exchange{}, err
	} else if found {
		return cached, nil
	}
	codeHash := sha256.Sum256([]byte(input.Code))
	release, err := s.coordinator.Acquire(ctx, "platform-core:oauth-code:"+hex.EncodeToString(codeHash[:]), 5*time.Second)
	if err != nil {
		if errors.Is(err, coordination.ErrBusy) {
			return Exchange{}, ErrCodeBusy
		}
		return Exchange{}, ErrDependency
	}
	defer releaseCoordination(release)()

	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Exchange{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := s.queries.WithTx(tx)
	code, err := queries.GetAuthorizationCodeForUpdate(ctx, codeHash[:])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Exchange{}, ErrInvalid
		}
		return Exchange{}, err
	}
	if code.UsedAt.Valid {
		if cached, found, cacheErr := s.lookupIdempotency(ctx, queries, input); cacheErr != nil {
			return Exchange{}, cacheErr
		} else if found {
			return cached, nil
		}
		return Exchange{}, ErrCodeUsed
	}
	if !time.Now().UTC().Before(code.ExpiresAt.Time) {
		return Exchange{}, ErrCodeExpired
	}
	if code.ClientID != input.ClientID || code.RedirectUri != input.RedirectURI || !validPKCE(input.CodeVerifier, code.CodeChallenge) {
		return Exchange{}, ErrInvalid
	}
	if _, err := queries.GetActiveCoreSessionForExchange(ctx, code.CoreSessionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Exchange{}, ErrUnauthorized
		}
		return Exchange{}, err
	}
	rows, err := queries.MarkAuthorizationCodeUsed(ctx, code.ID)
	if err != nil {
		return Exchange{}, err
	}
	if rows != 1 {
		return Exchange{}, ErrCodeUsed
	}
	sessionToken, sessionHash, err := randomToken()
	if err != nil {
		return Exchange{}, err
	}
	exchangeTTL := s.exchangeSessionTTL
	if override, ok := s.exchangeSessionTTLByClientID[input.ClientID]; ok {
		exchangeTTL = override
	}
	expiresAt := time.Now().UTC().Add(exchangeTTL)
	_, err = queries.CreateExchangeSession(ctx, store.CreateExchangeSessionParams{
		UserID: code.UserID, TokenHash: sessionHash,
		ClientID: pgtype.Text{String: input.ClientID, Valid: true}, ParentSessionID: code.CoreSessionID,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return Exchange{}, err
	}
	user, err := queries.GetPlatformUser(ctx, code.UserID)
	if err != nil {
		return Exchange{}, err
	}
	displayName := ""
	if user.DisplayName.Valid {
		displayName = strings.TrimSpace(user.DisplayName.String)
	}
	result := Exchange{
		SessionExchangeToken: sessionToken, ExpiresAt: expiresAt, UserID: uuidString(user.ID),
		DisplayName: displayName, EmailVerified: user.EmailVerified, UserStatus: user.Status, UserCreatedAt: user.CreatedAt.Time,
	}
	if err := queries.CreateOAuthExchangeIdempotency(ctx, store.CreateOAuthExchangeIdempotencyParams{
		ClientID: input.ClientID, IdempotencyKey: input.IdempotencyKey, RequestHash: input.BodyHash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(s.idempotencyTTL), Valid: true},
	}); err != nil {
		return Exchange{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Exchange{}, err
	}
	return result, nil
}

func (s *Service) CheckAuthorization(ctx context.Context, input AuthorizationCheckInput) (AuthorizationDecision, error) {
	if input.SessionToken == "" || !validPermissionCode(input.PermissionCode) || input.ServiceID == "" || input.RequestID == "" || !validScope(input.ScopeKind, input.ProductCode, input.ResourceType, input.ResourceID) {
		return AuthorizationDecision{}, ErrInvalid
	}
	if err := s.authenticateServiceRequest(ctx, serviceRequestCredentials{
		HTTPMethod: input.HTTPMethod,
		ClientID:   input.ServiceID, ClientSecret: input.ClientSecret, KeyID: input.KeyID,
		Timestamp: input.Timestamp, Nonce: input.Nonce, Signature: input.Signature,
		BodyHash: input.BodyHash, PathAndQuery: input.PathAndQuery, NonceNamespace: "authorization",
	}); err != nil {
		return AuthorizationDecision{}, err
	}
	tokenHash := sha256.Sum256([]byte(input.SessionToken))
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return AuthorizationDecision{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := s.queries.WithTx(tx)
	session, err := queries.GetExchangeSessionAuthorizationContext(ctx, tokenHash[:])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthorizationDecision{}, ErrUnauthorized
		}
		return AuthorizationDecision{}, err
	}
	if !session.ClientID.Valid || session.ClientID.String != input.ServiceID {
		return AuthorizationDecision{}, ErrUnauthorized
	}
	deniedSession := func(reason string) (AuthorizationDecision, error) {
		if auditErr := queries.CreateAuthorizationAuditEvent(ctx, store.CreateAuthorizationAuditEventParams{
			ActorUserID: session.UserID, SessionID: session.ID, RequestID: input.RequestID, ServiceID: input.ServiceID,
			PermissionCode: input.PermissionCode, TargetKind: input.ScopeKind,
			TargetProductCode: nullableText(input.ProductCode), TargetResourceType: nullableText(input.ResourceType), TargetResourceID: nullableText(input.ResourceID),
			Decision: "denied", ReasonCode: reason,
		}); auditErr != nil {
			return AuthorizationDecision{}, auditErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return AuthorizationDecision{}, commitErr
		}
		return AuthorizationDecision{}, ErrUnauthorized
	}
	now := time.Now().UTC()
	switch {
	case session.SessionRevokedAt.Valid:
		return deniedSession("SESSION_REVOKED")
	case !now.Before(session.SessionExpiresAt.Time):
		return deniedSession("SESSION_EXPIRED")
	case session.ParentRevokedAt.Valid:
		return deniedSession("PARENT_SESSION_REVOKED")
	case !now.Before(session.ParentExpiresAt.Time):
		return deniedSession("PARENT_SESSION_EXPIRED")
	case session.UserStatus != "active":
		return deniedSession("ACCOUNT_NOT_ACTIVE")
	}
	grant, err := queries.GetAuthorizationGrant(ctx, store.GetAuthorizationGrantParams{
		TokenHash: tokenHash[:], ClientID: session.ClientID, PermissionCode: input.PermissionCode,
		ScopeKind: input.ScopeKind, ProductCode: input.ProductCode, ResourceType: input.ResourceType, ResourceID: input.ResourceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if auditErr := queries.CreateAuthorizationAuditEvent(ctx, store.CreateAuthorizationAuditEventParams{
				ActorUserID: session.UserID, SessionID: session.ID, RequestID: input.RequestID, ServiceID: input.ServiceID,
				PermissionCode: input.PermissionCode, TargetKind: input.ScopeKind,
				TargetProductCode: nullableText(input.ProductCode), TargetResourceType: nullableText(input.ResourceType), TargetResourceID: nullableText(input.ResourceID),
				Decision: "denied", ReasonCode: "PERMISSION_OR_SCOPE_MISSING",
			}); auditErr != nil {
				return AuthorizationDecision{}, auditErr
			}
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return AuthorizationDecision{}, commitErr
			}
			return AuthorizationDecision{}, ErrForbidden
		}
		return AuthorizationDecision{}, err
	}
	if err := queries.CreateAuthorizationAuditEvent(ctx, store.CreateAuthorizationAuditEventParams{
		ActorUserID: session.UserID, SessionID: session.ID, RequestID: input.RequestID, ServiceID: input.ServiceID,
		PermissionCode: input.PermissionCode, TargetKind: input.ScopeKind,
		TargetProductCode: nullableText(input.ProductCode), TargetResourceType: nullableText(input.ResourceType), TargetResourceID: nullableText(input.ResourceID),
		Decision: "allowed", ReasonCode: "GRANTED", GrantID: grant.GrantID,
		AuthorizationRevision: pgtype.Int8{Int64: grant.AuthorizationRevision, Valid: true},
	}); err != nil {
		return AuthorizationDecision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AuthorizationDecision{}, err
	}
	return AuthorizationDecision{
		ActorUserID: uuidString(grant.UserID), GrantID: uuidString(grant.GrantID),
		AuthorizationRevision: grant.AuthorizationRevision, CheckedAt: time.Now().UTC(),
	}, nil
}

func (s *Service) lookupIdempotency(ctx context.Context, queries *store.Queries, input ExchangeInput) (Exchange, bool, error) {
	cached, err := queries.GetOAuthExchangeIdempotency(ctx, store.GetOAuthExchangeIdempotencyParams{ClientID: input.ClientID, IdempotencyKey: input.IdempotencyKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return Exchange{}, false, nil
	}
	if err != nil {
		return Exchange{}, false, err
	}
	if subtle.ConstantTimeCompare(cached.RequestHash, input.BodyHash) != 1 {
		return Exchange{}, false, ErrIdempotency
	}
	return Exchange{}, true, ErrCodeUsed
}

func (s *Service) acquireWithWait(ctx context.Context, key string, lockTTL, wait time.Duration) (func(context.Context) error, error) {
	deadline := time.Now().Add(wait)
	for {
		release, err := s.coordinator.Acquire(ctx, key, lockTTL)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, coordination.ErrBusy) {
			return nil, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, coordination.ErrBusy
		}
		delay := min(25*time.Millisecond, remaining)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func releaseCoordination(release func(context.Context) error) func() {
	return func() {
		releaseContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = release(releaseContext)
	}
}

func randomToken() (string, []byte, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}

func validPKCE(verifier, expectedChallenge string) bool {
	if len(verifier) < 43 || len(verifier) > 128 {
		return false
	}
	hash := sha256.Sum256([]byte(verifier))
	actual := base64.RawURLEncoding.EncodeToString(hash[:])
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expectedChallenge)) == 1
}

func validCodeChallenge(challenge string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(challenge)
	return err == nil && len(decoded) == sha256.Size && len(challenge) == 43
}

func validScope(kind, productCode, resourceType, resourceID string) bool {
	switch kind {
	case "platform":
		return productCode == "" && resourceType == "" && resourceID == ""
	case "product":
		return productCodePattern.MatchString(productCode) && resourceType == "" && resourceID == ""
	case "resource":
		return productCodePattern.MatchString(productCode) && resourceTypePattern.MatchString(resourceType) && resourceID != "" && len(resourceID) <= 200
	default:
		return false
	}
}

func validPermissionCode(value string) bool {
	return len(value) >= 3 && len(value) <= 120 && permissionCodePattern.MatchString(value)
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func containsExact(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	bytes := value.Bytes
	return hex.EncodeToString(bytes[0:4]) + "-" + hex.EncodeToString(bytes[4:6]) + "-" + hex.EncodeToString(bytes[6:8]) + "-" + hex.EncodeToString(bytes[8:10]) + "-" + hex.EncodeToString(bytes[10:16])
}
