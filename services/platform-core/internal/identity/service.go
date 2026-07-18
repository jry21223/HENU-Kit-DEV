package identity

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	idempotencyKey     []byte
	idempotencyTTL     time.Duration
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
	EmailVerified        bool
	UserStatus           string
	UserCreatedAt        time.Time
}

func New(queries *store.Queries, database *pgxpool.Pool, coordinator Coordinator, authorizationTTL, exchangeSessionTTL, idempotencyTTL time.Duration, idempotencyKey []byte) *Service {
	return &Service{queries: queries, database: database, coordinator: coordinator, authorizationTTL: authorizationTTL, exchangeSessionTTL: exchangeSessionTTL, idempotencyTTL: idempotencyTTL, idempotencyKey: idempotencyKey}
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
	if input.ClientID == "" || input.ClientSecret == "" || input.Code == "" || input.RedirectURI == "" || input.CodeVerifier == "" || input.ServiceID == "" || input.KeyID == "" || input.Timestamp == "" || input.Nonce == "" || input.Signature == "" || len(input.BodyHash) != sha256.Size || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 || !strings.HasPrefix(input.PathAndQuery, "/") {
		return Exchange{}, ErrInvalid
	}
	clientKey, err := s.queries.GetOAuthClientKey(ctx, store.GetOAuthClientKeyParams{ClientID: input.ClientID, KeyID: input.KeyID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Exchange{}, ErrUnauthorized
		}
		return Exchange{}, err
	}
	secretHash := sha256.Sum256([]byte(input.ClientSecret))
	if len(clientKey.SecretHash) != len(secretHash) || subtle.ConstantTimeCompare(clientKey.SecretHash, secretHash[:]) != 1 {
		return Exchange{}, ErrUnauthorized
	}
	if input.ServiceID != input.ClientID {
		return Exchange{}, ErrSignature
	}
	requestTimeUnix, err := strconv.ParseInt(input.Timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(requestTimeUnix, 0)).Abs() > 5*time.Minute {
		return Exchange{}, ErrTimestamp
	}
	canonical := strings.Join([]string{"POST", input.PathAndQuery, input.Timestamp, input.Nonce, hex.EncodeToString(input.BodyHash)}, "\n")
	mac := hmac.New(sha256.New, []byte(input.ClientSecret))
	_, _ = mac.Write([]byte(canonical))
	expectedSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(input.Signature), []byte(expectedSignature)) != 1 {
		return Exchange{}, ErrSignature
	}
	if err := s.coordinator.UseOnce(ctx, "platform-core:oauth-nonce:"+input.ClientID+":"+input.Nonce, 10*time.Minute); err != nil {
		if errors.Is(err, coordination.ErrBusy) {
			return Exchange{}, ErrNonceReplay
		}
		return Exchange{}, ErrDependency
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
	cached, err := s.queries.GetOAuthExchangeIdempotency(ctx, store.GetOAuthExchangeIdempotencyParams{ClientID: input.ClientID, IdempotencyKey: input.IdempotencyKey})
	if err == nil {
		if subtle.ConstantTimeCompare(cached.RequestHash, input.BodyHash) != 1 {
			return Exchange{}, ErrIdempotency
		}
		return s.decryptExchange(cached.ResponseCiphertext)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Exchange{}, err
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
		cached, cacheErr := queries.GetOAuthExchangeIdempotency(ctx, store.GetOAuthExchangeIdempotencyParams{ClientID: input.ClientID, IdempotencyKey: input.IdempotencyKey})
		if cacheErr == nil {
			if subtle.ConstantTimeCompare(cached.RequestHash, input.BodyHash) != 1 {
				return Exchange{}, ErrIdempotency
			}
			return s.decryptExchange(cached.ResponseCiphertext)
		}
		if !errors.Is(cacheErr, pgx.ErrNoRows) {
			return Exchange{}, cacheErr
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
	result := Exchange{
		SessionExchangeToken: sessionToken, ExpiresAt: expiresAt, UserID: uuidString(user.ID),
		EmailVerified: user.EmailVerified, UserStatus: user.Status, UserCreatedAt: user.CreatedAt.Time,
	}
	ciphertext, err := s.encryptExchange(result)
	if err != nil {
		return Exchange{}, err
	}
	if err := queries.CreateOAuthExchangeIdempotency(ctx, store.CreateOAuthExchangeIdempotencyParams{
		ClientID: input.ClientID, IdempotencyKey: input.IdempotencyKey, RequestHash: input.BodyHash,
		ResponseCiphertext: ciphertext, ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(s.idempotencyTTL), Valid: true},
	}); err != nil {
		return Exchange{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Exchange{}, err
	}
	return result, nil
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

func (s *Service) encryptExchange(value Exchange) ([]byte, error) {
	plaintext, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(s.idempotencyKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (s *Service) decryptExchange(ciphertext []byte) (Exchange, error) {
	block, err := aes.NewCipher(s.idempotencyKey)
	if err != nil {
		return Exchange{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Exchange{}, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return Exchange{}, errors.New("invalid cached exchange")
	}
	plaintext, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
	if err != nil {
		return Exchange{}, err
	}
	var value Exchange
	if err := json.Unmarshal(plaintext, &value); err != nil {
		return Exchange{}, err
	}
	return value, nil
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
