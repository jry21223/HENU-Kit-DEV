package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/coordination"
	"henukit.dev/platform-core/internal/store"
)

var (
	ErrUnauthorized = errors.New("authentication failed")
	ErrInvalid      = errors.New("invalid authorization request")
	ErrCallback     = errors.New("callback is not registered")
	ErrCodeUsed     = errors.New("authorization code was already used")
	ErrCodeExpired  = errors.New("authorization code expired")
	ErrCodeBusy     = errors.New("authorization code exchange in progress")
	ErrDependency   = errors.New("coordination dependency unavailable")
)

type Coordinator interface {
	Acquire(context.Context, string, time.Duration) (func(context.Context) error, error)
}

type Service struct {
	queries            *store.Queries
	database           *pgxpool.Pool
	coordinator        Coordinator
	authorizationTTL   time.Duration
	exchangeSessionTTL time.Duration
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
}

type ExchangeInput struct {
	ClientID     string
	ClientSecret string
	Code         string
	RedirectURI  string
	CodeVerifier string
}

type Exchange struct {
	SessionExchangeToken string
	ExpiresAt            time.Time
	UserID               string
	EmailVerified        bool
	UserStatus           string
	UserCreatedAt        time.Time
}

func New(queries *store.Queries, database *pgxpool.Pool, coordinator Coordinator, authorizationTTL, exchangeSessionTTL time.Duration) *Service {
	return &Service{queries: queries, database: database, coordinator: coordinator, authorizationTTL: authorizationTTL, exchangeSessionTTL: exchangeSessionTTL}
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
	return Authorization{Code: code, SessionExpires: session.ExpiresAt.Time}, nil
}

func (s *Service) Exchange(ctx context.Context, input ExchangeInput) (Exchange, error) {
	if input.ClientID == "" || input.ClientSecret == "" || input.Code == "" || input.RedirectURI == "" || input.CodeVerifier == "" {
		return Exchange{}, ErrInvalid
	}
	client, err := s.queries.GetOAuthClient(ctx, input.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Exchange{}, ErrUnauthorized
		}
		return Exchange{}, err
	}
	secretHash := sha256.Sum256([]byte(input.ClientSecret))
	if len(client.SecretHash) != len(secretHash) || subtle.ConstantTimeCompare(client.SecretHash, secretHash[:]) != 1 {
		return Exchange{}, ErrUnauthorized
	}
	codeHash := sha256.Sum256([]byte(input.Code))
	release, err := s.coordinator.Acquire(ctx, "platform-core:oauth-code:"+hex.EncodeToString(codeHash[:]), 5*time.Second)
	if err != nil {
		if errors.Is(err, coordination.ErrBusy) {
			return Exchange{}, ErrCodeBusy
		}
		return Exchange{}, ErrDependency
	}
	defer func() {
		releaseContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = release(releaseContext)
	}()

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
	expiresAt := time.Now().UTC().Add(s.exchangeSessionTTL)
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
	if err := tx.Commit(ctx); err != nil {
		return Exchange{}, err
	}
	return Exchange{
		SessionExchangeToken: sessionToken, ExpiresAt: expiresAt, UserID: uuidString(user.ID),
		EmailVerified: user.EmailVerified, UserStatus: user.Status, UserCreatedAt: user.CreatedAt.Time,
	}, nil
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
