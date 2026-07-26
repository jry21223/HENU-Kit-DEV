package verification

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/password"
	"henukit.dev/platform-core/internal/securebox"
	"henukit.dev/platform-core/internal/store"
	"henukit.dev/platform-core/internal/verificationmail"
)

var (
	ErrInvalid              = errors.New("verification request is invalid")
	ErrDependency           = errors.New("verification dependency unavailable")
	ErrIdempotency          = errors.New("idempotency key conflicts with another request")
	ErrCodeInvalid          = errors.New("verification code is invalid")
	ErrCodeExpired          = errors.New("verification code expired")
	ErrCodeAlreadyUsed      = errors.New("verification code was already used")
	ErrRateLimited          = errors.New("verification attempts are rate limited")
	ErrRandomSource         = errors.New("verification random source unavailable")
	ErrAlreadyRegistered    = errors.New("email identity is already registered")
	ErrRegistrationRequired = errors.New("registration is required")
	ErrAuthentication       = errors.New("authentication failed")
	ErrChallengeRequired    = errors.New("email-code login is required")
)

type Coordinator interface {
	Allow(context.Context, string, int64, time.Duration) (bool, error)
	FailureCount(context.Context, string) (int64, error)
	RecordFailure(context.Context, string, time.Duration) (int64, error)
	Clear(context.Context, ...string) error
}

type Service struct {
	queries        *store.Queries
	database       *pgxpool.Pool
	coordinator    Coordinator
	secretKey      []byte
	mailCodec      *verificationmail.Codec
	emailCodec     *securebox.Codec
	allowedDomains map[string]struct{}
	codeTTL        time.Duration
	resendDelay    time.Duration
	coreSessionTTL time.Duration
	passwords      *password.Manager
	dummyVerifier  string
}

type RequestInput struct {
	Email          string
	Purpose        string
	ClientID       string
	DeviceID       string
	ClientIP       string
	IdempotencyKey string
	RequestID      string
}

type Accepted struct {
	ExpiresAt   time.Time
	ResendAfter time.Time
}

type VerifyInput struct {
	Email          string
	Code           string
	Purpose        string
	IdempotencyKey string
	DeviceID       string
	ClientIP       string
}

type Verified struct {
	VerificationID   string
	UserID           string
	EmailVerified    bool
	UserStatus       string
	UserCreatedAt    time.Time
	SessionToken     string
	SessionExpiresAt time.Time
}

type RegisterInput struct {
	Email, Code, DisplayName, Password string
	IdempotencyKey, DeviceID, ClientIP string
}

type PasswordLoginInput struct {
	Email, Password, DeviceID, ClientIP string
}

type PasswordRecoveryInput struct {
	Email, Code, Password              string
	IdempotencyKey, DeviceID, ClientIP string
}

type PasswordChangeInput struct {
	Email, Code, CurrentPassword, NewPassword string
	CoreSessionToken, IdempotencyKey          string
	DeviceID, ClientIP                        string
}

type CoreSession struct {
	SessionID, UserID string
	ExpiresAt         time.Time
}

func New(queries *store.Queries, database *pgxpool.Pool, coordinator Coordinator, passwordManager *password.Manager, masterKey []byte, allowedDomains []string, codeTTL, resendDelay, coreSessionTTL time.Duration) (*Service, error) {
	if queries == nil || database == nil || coordinator == nil || passwordManager == nil || len(masterKey) != 32 {
		return nil, errors.New("verification dependencies and a 32-byte key are required")
	}
	if codeTTL < 5*time.Minute || codeTTL > 10*time.Minute || resendDelay < 60*time.Second {
		return nil, errors.New("verification TTL must be 5-10m and resend delay at least 60s")
	}
	if coreSessionTTL != 15*24*time.Hour {
		return nil, errors.New("core Session TTL must be 15 days")
	}
	if len(allowedDomains) != 1 || strings.ToLower(strings.TrimSpace(allowedDomains[0])) != "henu.edu.cn" {
		return nil, errors.New("student email domain must be exactly henu.edu.cn")
	}
	mailCodec, err := verificationmail.NewCodec(masterKey)
	if err != nil {
		return nil, err
	}
	emailCodec, err := securebox.New(masterKey, "email-identity")
	if err != nil {
		return nil, err
	}
	dummyVerifier, err := passwordManager.Hash(context.Background(), "dummy password credential")
	if err != nil {
		return nil, err
	}
	return &Service{
		queries: queries, database: database, coordinator: coordinator, secretKey: append([]byte(nil), masterKey...),
		mailCodec: mailCodec, emailCodec: emailCodec, allowedDomains: map[string]struct{}{"henu.edu.cn": {}},
		codeTTL: codeTTL, resendDelay: resendDelay, coreSessionTTL: coreSessionTTL,
		passwords: passwordManager, dummyVerifier: dummyVerifier,
	}, nil
}

func (s *Service) Request(ctx context.Context, input RequestInput) (Accepted, error) {
	email, err := s.normalizeEmail(input.Email)
	if err != nil || !validPurpose(input.Purpose) || len(input.DeviceID) < 8 || len(input.DeviceID) > 200 || input.ClientIP == "" || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 || input.RequestID == "" {
		return Accepted{}, ErrInvalid
	}
	emailHash := s.digest("email", []byte(email))
	fingerprint := s.digest("request", emailHash, []byte(input.Purpose), []byte(input.ClientID))
	if existing, err := s.queries.GetVerificationRequestByKey(ctx, pgtype.Text{String: input.IdempotencyKey, Valid: true}); err == nil {
		if subtle.ConstantTimeCompare(existing.RequestFingerprint, fingerprint) != 1 {
			return Accepted{}, ErrIdempotency
		}
		return Accepted{ExpiresAt: existing.ExpiresAt.Time, ResendAfter: existing.CreatedAt.Time.Add(s.resendDelay)}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Accepted{}, err
	}
	limited, err := s.rateLimited(ctx, emailHash, input.DeviceID, input.ClientIP, input.Purpose)
	if err != nil {
		return Accepted{}, err
	}
	if limited {
		now := time.Now().UTC()
		return Accepted{ExpiresAt: now.Add(s.codeTTL), ResendAfter: now.Add(s.resendDelay)}, nil
	}
	code, err := randomCode()
	if err != nil {
		return Accepted{}, ErrRandomSource
	}
	nonce := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Accepted{}, ErrRandomSource
	}
	codeHash := s.digest("code", nonce, []byte(code))
	now := time.Now().UTC()
	expiresAt := now.Add(s.codeTTL)
	recipientCiphertext, payloadCiphertext, err := s.mailCodec.Encode(email, verificationmail.Payload{Code: code, Purpose: input.Purpose, ExpiresAt: expiresAt})
	if err != nil {
		return Accepted{}, ErrRandomSource
	}
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Accepted{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := s.queries.WithTx(tx)
	created, err := queries.CreateVerificationCode(ctx, store.CreateVerificationCodeParams{
		EmailLookupHash: emailHash, Purpose: input.Purpose, RequestKey: pgtype.Text{String: input.IdempotencyKey, Valid: true},
		RequestFingerprint: fingerprint, CodeNonce: nonce, CodeHash: codeHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return Accepted{}, err
	}
	if _, err := queries.CreateVerificationMailOutbox(ctx, store.CreateVerificationMailOutboxParams{
		VerificationCodeID: created.ID, DedupeKey: "verification:" + uuidString(created.ID), RequestID: input.RequestID,
		RecipientCiphertext: recipientCiphertext, PayloadCiphertext: payloadCiphertext,
	}); err != nil {
		return Accepted{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Accepted{}, err
	}
	return Accepted{ExpiresAt: created.ExpiresAt.Time, ResendAfter: created.CreatedAt.Time.Add(s.resendDelay)}, nil
}

func (s *Service) rateLimited(ctx context.Context, emailHash []byte, deviceID, clientIP, purpose string) (bool, error) {
	dimensions := []struct {
		key    string
		limit  int64
		window time.Duration
	}{
		{key: "email-resend:" + hex.EncodeToString(emailHash) + ":" + purpose, limit: 1, window: s.resendDelay},
		{key: "email-hour:" + hex.EncodeToString(emailHash), limit: 5, window: time.Hour},
		{key: "email-day:" + hex.EncodeToString(emailHash), limit: 20, window: 24 * time.Hour},
		{key: "ip-hour:" + hex.EncodeToString(s.digest("ip", []byte(clientIP))), limit: 30, window: time.Hour},
		{key: "ip-day:" + hex.EncodeToString(s.digest("ip", []byte(clientIP))), limit: 100, window: 24 * time.Hour},
		{key: "device-hour:" + hex.EncodeToString(s.digest("device", []byte(deviceID))), limit: 10, window: time.Hour},
		{key: "device-day:" + hex.EncodeToString(s.digest("device", []byte(deviceID))), limit: 40, window: 24 * time.Hour},
	}
	limited := false
	for _, dimension := range dimensions {
		allowed, err := s.coordinator.Allow(ctx, "platform-core:verification:"+dimension.key, dimension.limit, dimension.window)
		if err != nil {
			return false, ErrDependency
		}
		limited = limited || !allowed
	}
	return limited, nil
}

func (s *Service) Verify(ctx context.Context, input VerifyInput) (Verified, error) {
	email, err := s.normalizeEmail(input.Email)
	if err != nil || !validPurpose(input.Purpose) || input.Purpose == "register" ||
		len(input.Code) != 6 || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 ||
		input.DeviceID == "" || input.ClientIP == "" {
		return Verified{}, ErrInvalid
	}
	emailHash := s.digest("email", []byte(email))
	requestFingerprint := s.digest("consume", emailHash, []byte(input.Purpose), []byte(input.Code))
	if replay, replayErr := s.queries.GetConsumedVerificationReplay(ctx, pgtype.Text{String: input.IdempotencyKey, Valid: true}); replayErr == nil {
		if subtle.ConstantTimeCompare(replay.ConsumedRequestFingerprint, requestFingerprint) == 1 {
			return verifiedReplay(input.Purpose, replay)
		}
		return Verified{}, ErrIdempotency
	} else if !errors.Is(replayErr, pgx.ErrNoRows) {
		return Verified{}, replayErr
	}
	limited, err := s.verifyRateLimited(ctx, emailHash, input.DeviceID, input.ClientIP)
	if err != nil {
		return Verified{}, err
	}
	if limited {
		return Verified{}, ErrRateLimited
	}
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Verified{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := s.queries.WithTx(tx)
	verification, err := queries.GetVerificationCodeForUpdate(ctx, store.GetVerificationCodeForUpdateParams{EmailLookupHash: emailHash, Purpose: input.Purpose})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Verified{}, ErrCodeInvalid
		}
		return Verified{}, err
	}
	if verification.UsedAt.Valid {
		if verification.ConsumedRequestKey.Valid && verification.ConsumedRequestKey.String == input.IdempotencyKey && verification.ConsumedRequestFingerprint != nil && subtle.ConstantTimeCompare(verification.ConsumedRequestFingerprint, requestFingerprint) == 1 {
			replay, err := queries.GetConsumedVerificationReplay(ctx, pgtype.Text{String: input.IdempotencyKey, Valid: true})
			if err != nil {
				return Verified{}, ErrDependency
			}
			return verifiedReplay(input.Purpose, replay)
		}
		return Verified{}, ErrCodeAlreadyUsed
	}
	if verification.RevokedAt.Valid {
		return Verified{}, ErrCodeInvalid
	}
	if !time.Now().UTC().Before(verification.ExpiresAt.Time) {
		return Verified{}, ErrCodeExpired
	}
	expectedHash := s.digest("code", verification.CodeNonce, []byte(input.Code))
	if subtle.ConstantTimeCompare(expectedHash, verification.CodeHash) != 1 {
		if _, err := queries.RegisterFailedVerificationAttempt(ctx, verification.ID); err != nil {
			return Verified{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Verified{}, err
		}
		return Verified{}, ErrCodeInvalid
	}
	result := Verified{VerificationID: uuidString(verification.ID)}
	var loginIdentity store.GetEmailIdentityForUpdateRow
	if input.Purpose == "login" {
		loginIdentity, err = queries.GetEmailIdentityForUpdate(ctx, emailHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return Verified{}, ErrRegistrationRequired
		}
		if err != nil {
			return Verified{}, err
		}
		if loginIdentity.Status != "active" {
			return Verified{}, ErrInvalid
		}
	}
	rows, err := queries.ConsumeVerificationCode(ctx, store.ConsumeVerificationCodeParams{
		ID: verification.ID, ConsumedRequestKey: pgtype.Text{String: input.IdempotencyKey, Valid: true},
		ConsumedRequestFingerprint: requestFingerprint,
	})
	if err != nil {
		return Verified{}, err
	}
	if rows != 1 {
		return Verified{}, ErrCodeAlreadyUsed
	}
	if input.Purpose == "login" {
		var sessionTokenHash []byte
		result.SessionToken, sessionTokenHash, result.SessionExpiresAt, err = s.newCoreSession()
		if err != nil {
			return Verified{}, err
		}
		session, err := queries.CreateCoreSession(ctx, store.CreateCoreSessionParams{
			UserID: loginIdentity.UserID, TokenHash: sessionTokenHash,
			ExpiresAt: pgtype.Timestamptz{Time: result.SessionExpiresAt, Valid: true},
		})
		if err != nil {
			return Verified{}, err
		}
		attached, err := queries.AttachLoginSessionToVerification(ctx, store.AttachLoginSessionToVerificationParams{
			ID: verification.ID, LoginSessionID: session.ID,
		})
		if err != nil || attached != 1 {
			if err != nil {
				return Verified{}, err
			}
			return Verified{}, ErrCodeAlreadyUsed
		}
		result.UserID = uuidString(loginIdentity.UserID)
		result.EmailVerified, result.UserStatus, result.UserCreatedAt =
			loginIdentity.EmailVerified, loginIdentity.Status, loginIdentity.CreatedAt.Time
		if err := s.coordinator.Clear(ctx, passwordFailureKeyNames(s.passwordFailureKeys(emailHash, input.DeviceID, input.ClientIP))...); err != nil {
			return Verified{}, ErrDependency
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Verified{}, err
	}
	return result, nil
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (Verified, error) {
	email, err := s.normalizeEmail(input.Email)
	displayName := strings.TrimSpace(input.DisplayName)
	if err != nil || input.DeviceID == "" || input.ClientIP == "" ||
		len(input.Code) != 6 || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 ||
		displayName == "" || len([]rune(displayName)) > 80 || password.Validate(email, input.Password) != nil {
		return Verified{}, ErrInvalid
	}
	emailHash := s.digest("email", []byte(email))
	limited, err := s.verifyRateLimited(ctx, emailHash, input.DeviceID, input.ClientIP)
	if err != nil {
		return Verified{}, err
	}
	if limited {
		return Verified{}, ErrRateLimited
	}
	verifier, err := s.passwords.Hash(ctx, input.Password)
	if err != nil {
		return Verified{}, ErrDependency
	}
	emailCiphertext, err := s.emailCodec.Seal([]byte(email))
	if err != nil {
		return Verified{}, ErrRandomSource
	}
	sessionToken, tokenHash, sessionExpiresAt, err := s.newCoreSession()
	if err != nil {
		return Verified{}, err
	}
	requestFingerprint := s.digest("register", emailHash, []byte(input.Code), []byte(displayName))
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Verified{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := s.queries.WithTx(tx)
	verification, err := queries.GetVerificationCodeForUpdate(ctx, store.GetVerificationCodeForUpdateParams{
		EmailLookupHash: emailHash, Purpose: "register",
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Verified{}, ErrCodeInvalid
		}
		return Verified{}, err
	}
	if verification.UsedAt.Valid {
		return Verified{}, ErrCodeAlreadyUsed
	}
	if verification.RevokedAt.Valid {
		return Verified{}, ErrCodeInvalid
	}
	if !time.Now().UTC().Before(verification.ExpiresAt.Time) {
		return Verified{}, ErrCodeExpired
	}
	expectedHash := s.digest("code", verification.CodeNonce, []byte(input.Code))
	if subtle.ConstantTimeCompare(expectedHash, verification.CodeHash) != 1 {
		if _, err := queries.RegisterFailedVerificationAttempt(ctx, verification.ID); err != nil {
			return Verified{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Verified{}, err
		}
		return Verified{}, ErrCodeInvalid
	}
	if _, err := queries.GetEmailIdentityForUpdate(ctx, emailHash); err == nil {
		return Verified{}, ErrAlreadyRegistered
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Verified{}, err
	}
	created, err := queries.CreateRegisteredUser(ctx, pgtype.Text{String: displayName, Valid: true})
	if err != nil {
		return Verified{}, err
	}
	if err := queries.CreateEmailIdentity(ctx, store.CreateEmailIdentityParams{
		UserID: created.ID, EmailLookupHash: emailHash, EmailCiphertext: emailCiphertext,
	}); err != nil {
		if isUniqueViolation(err) {
			return Verified{}, ErrAlreadyRegistered
		}
		return Verified{}, err
	}
	if err := queries.CreatePasswordCredential(ctx, store.CreatePasswordCredentialParams{
		UserID: created.ID, Verifier: verifier, PolicyVersion: password.PolicyVersion,
	}); err != nil {
		return Verified{}, err
	}
	session, err := queries.CreateCoreSession(ctx, store.CreateCoreSessionParams{
		UserID: created.ID, TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: sessionExpiresAt, Valid: true},
	})
	if err != nil {
		return Verified{}, err
	}
	rows, err := queries.ConsumeVerificationCode(ctx, store.ConsumeVerificationCodeParams{
		ID: verification.ID, ConsumedRequestKey: pgtype.Text{String: input.IdempotencyKey, Valid: true},
		ConsumedRequestFingerprint: requestFingerprint,
	})
	if err != nil || rows != 1 {
		if err != nil {
			return Verified{}, err
		}
		return Verified{}, ErrCodeAlreadyUsed
	}
	attached, err := queries.AttachLoginSessionToVerification(ctx, store.AttachLoginSessionToVerificationParams{
		ID: verification.ID, LoginSessionID: session.ID,
	})
	if err != nil || attached != 1 {
		if err != nil {
			return Verified{}, err
		}
		return Verified{}, ErrCodeAlreadyUsed
	}
	if err := tx.Commit(ctx); err != nil {
		return Verified{}, err
	}
	return Verified{
		VerificationID: uuidString(verification.ID), UserID: uuidString(created.ID),
		EmailVerified: created.EmailVerified, UserStatus: created.Status, UserCreatedAt: created.CreatedAt.Time,
		SessionToken: sessionToken, SessionExpiresAt: sessionExpiresAt,
	}, nil
}

func (s *Service) PasswordLogin(ctx context.Context, input PasswordLoginInput) (Verified, error) {
	email, err := s.normalizeEmail(input.Email)
	if err != nil || input.DeviceID == "" || input.ClientIP == "" {
		return Verified{}, ErrAuthentication
	}
	emailHash := s.digest("email", []byte(email))
	failureKeys := s.passwordFailureKeys(emailHash, input.DeviceID, input.ClientIP)
	challenged, err := s.passwordChallengeRequired(ctx, failureKeys)
	if err != nil {
		return Verified{}, ErrDependency
	}
	if challenged {
		return Verified{}, ErrChallengeRequired
	}
	if !password.AcceptableInput(input.Password) {
		return Verified{}, s.recordPasswordFailure(ctx, failureKeys)
	}
	reservation, err := s.passwords.TryReserve()
	if err != nil {
		return Verified{}, ErrDependency
	}
	defer reservation.Release()
	// Recheck after reserving capacity so retries observe failures recorded by
	// earlier requests before they are allowed to touch PostgreSQL.
	challenged, err = s.passwordChallengeRequired(ctx, failureKeys)
	if err != nil {
		return Verified{}, ErrDependency
	}
	if challenged {
		return Verified{}, ErrChallengeRequired
	}
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Verified{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockCredentialScope(ctx, tx, emailHash); err != nil {
		return Verified{}, err
	}
	queries := s.queries.WithTx(tx)
	identity, err := queries.GetEmailIdentityForUpdate(ctx, emailHash)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _, _ = reservation.Verify(input.Password, s.dummyVerifier)
		_ = tx.Rollback(ctx)
		return Verified{}, s.recordPasswordFailure(ctx, failureKeys)
	}
	if err != nil {
		return Verified{}, err
	}
	credential, err := queries.GetPasswordCredentialForUpdate(ctx, identity.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _, _ = reservation.Verify(input.Password, s.dummyVerifier)
		_ = tx.Rollback(ctx)
		return Verified{}, s.recordPasswordFailure(ctx, failureKeys)
	}
	if err != nil {
		return Verified{}, err
	}
	valid, needsRehash, err := reservation.Verify(input.Password, credential.Verifier)
	if err != nil {
		return Verified{}, ErrDependency
	}
	if !valid || identity.Status != "active" {
		_ = tx.Rollback(ctx)
		return Verified{}, s.recordPasswordFailure(ctx, failureKeys)
	}
	var upgradedVerifier string
	if needsRehash || credential.PolicyVersion != password.PolicyVersion {
		upgradedVerifier, err = reservation.Hash(input.Password)
		if err != nil {
			return Verified{}, ErrDependency
		}
	}
	if err := s.coordinator.Clear(ctx, passwordFailureKeyNames(failureKeys)...); err != nil {
		return Verified{}, ErrDependency
	}
	sessionToken, tokenHash, sessionExpiresAt, err := s.newCoreSession()
	if err != nil {
		return Verified{}, err
	}
	if _, err := queries.CreateCoreSession(ctx, store.CreateCoreSessionParams{
		UserID: identity.UserID, TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: sessionExpiresAt, Valid: true},
	}); err != nil {
		return Verified{}, err
	}
	if upgradedVerifier != "" {
		if rows, err := queries.ReplacePasswordCredential(ctx, store.ReplacePasswordCredentialParams{
			UserID: identity.UserID, Verifier: upgradedVerifier, PolicyVersion: password.PolicyVersion,
		}); err != nil || rows != 1 {
			if err == nil {
				return Verified{}, ErrDependency
			}
			return Verified{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Verified{}, err
	}
	return Verified{
		UserID: uuidString(identity.UserID), EmailVerified: identity.EmailVerified,
		UserStatus: identity.Status, UserCreatedAt: identity.CreatedAt.Time,
		SessionToken: sessionToken, SessionExpiresAt: sessionExpiresAt,
	}, nil
}

func (s *Service) CoreSession(ctx context.Context, token string) (CoreSession, error) {
	if len(token) < 32 {
		return CoreSession{}, ErrAuthentication
	}
	tokenHash := sha256.Sum256([]byte(token))
	session, err := s.queries.GetCoreSessionByTokenHash(ctx, tokenHash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return CoreSession{}, ErrAuthentication
	}
	if err != nil {
		return CoreSession{}, err
	}
	if session.Status != "active" || !time.Now().UTC().Before(session.ExpiresAt.Time) {
		return CoreSession{}, ErrAuthentication
	}
	return CoreSession{SessionID: uuidString(session.ID), UserID: uuidString(session.UserID), ExpiresAt: session.ExpiresAt.Time}, nil
}

func (s *Service) RecoverPassword(ctx context.Context, input PasswordRecoveryInput) (Verified, error) {
	email, err := s.normalizeEmail(input.Email)
	if err != nil || input.DeviceID == "" || input.ClientIP == "" || len(input.Code) != 6 ||
		len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 ||
		password.Validate(email, input.Password) != nil {
		return Verified{}, ErrInvalid
	}
	emailHash := s.digest("email", []byte(email))
	limited, err := s.verifyRateLimited(ctx, emailHash, input.DeviceID, input.ClientIP)
	if err != nil {
		return Verified{}, err
	}
	if limited {
		return Verified{}, ErrRateLimited
	}
	verifier, err := s.passwords.Hash(ctx, input.Password)
	if err != nil {
		return Verified{}, ErrDependency
	}
	sessionToken, tokenHash, sessionExpiresAt, err := s.newCoreSession()
	if err != nil {
		return Verified{}, err
	}
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Verified{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockCredentialScope(ctx, tx, emailHash); err != nil {
		return Verified{}, err
	}
	queries := s.queries.WithTx(tx)
	identity, err := queries.GetEmailIdentityForUpdate(ctx, emailHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Verified{}, ErrAuthentication
	}
	if err != nil {
		return Verified{}, err
	}
	if identity.Status != "active" {
		return Verified{}, ErrAuthentication
	}
	if _, err := queries.GetPasswordCredentialForUpdate(ctx, identity.UserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Verified{}, ErrAuthentication
		}
		return Verified{}, err
	}
	verification, err := s.validateVerificationCodeForUpdate(ctx, queries, emailHash, "security", input.Code)
	if err != nil {
		if errors.Is(err, ErrCodeInvalid) {
			_ = tx.Commit(ctx)
		}
		return Verified{}, err
	}
	if rows, err := queries.ReplacePasswordCredential(ctx, store.ReplacePasswordCredentialParams{
		UserID: identity.UserID, Verifier: verifier, PolicyVersion: password.PolicyVersion,
	}); err != nil || rows != 1 {
		if err != nil {
			return Verified{}, err
		}
		return Verified{}, ErrDependency
	}
	if _, err := queries.RevokeAllUserSessions(ctx, identity.UserID); err != nil {
		return Verified{}, err
	}
	if _, err := queries.CreateCoreSession(ctx, store.CreateCoreSessionParams{
		UserID: identity.UserID, TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: sessionExpiresAt, Valid: true},
	}); err != nil {
		return Verified{}, err
	}
	requestFingerprint := s.digest("recover", emailHash, []byte(input.Code))
	rows, err := queries.ConsumeVerificationCode(ctx, store.ConsumeVerificationCodeParams{
		ID: verification.ID, ConsumedRequestKey: pgtype.Text{String: input.IdempotencyKey, Valid: true},
		ConsumedRequestFingerprint: requestFingerprint,
	})
	if err != nil || rows != 1 {
		if err != nil {
			return Verified{}, err
		}
		return Verified{}, ErrCodeAlreadyUsed
	}
	if err := s.coordinator.Clear(ctx, passwordFailureKeyNames(s.passwordFailureKeys(emailHash, input.DeviceID, input.ClientIP))...); err != nil {
		return Verified{}, ErrDependency
	}
	if err := tx.Commit(ctx); err != nil {
		return Verified{}, err
	}
	return Verified{
		VerificationID: uuidString(verification.ID), UserID: uuidString(identity.UserID),
		EmailVerified: identity.EmailVerified, UserStatus: identity.Status, UserCreatedAt: identity.CreatedAt.Time,
		SessionToken: sessionToken, SessionExpiresAt: sessionExpiresAt,
	}, nil
}

func (s *Service) ChangePassword(ctx context.Context, input PasswordChangeInput) (Verified, error) {
	email, err := s.normalizeEmail(input.Email)
	if err != nil || input.DeviceID == "" || input.ClientIP == "" || len(input.Code) != 6 ||
		len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 ||
		!password.AcceptableInput(input.CurrentPassword) || password.Validate(email, input.NewPassword) != nil ||
		len(input.CoreSessionToken) < 32 {
		return Verified{}, ErrInvalid
	}
	emailHash := s.digest("email", []byte(email))
	failureKeys := s.passwordFailureKeys(emailHash, input.DeviceID, input.ClientIP)
	challenged, err := s.passwordChallengeRequired(ctx, failureKeys)
	if err != nil {
		return Verified{}, ErrDependency
	}
	if challenged {
		return Verified{}, ErrChallengeRequired
	}
	limited, err := s.verifyRateLimited(ctx, emailHash, input.DeviceID, input.ClientIP)
	if err != nil {
		return Verified{}, err
	}
	if limited {
		return Verified{}, ErrRateLimited
	}
	newVerifier, err := s.passwords.Hash(ctx, input.NewPassword)
	if err != nil {
		return Verified{}, ErrDependency
	}
	sessionHash := sha256.Sum256([]byte(input.CoreSessionToken))
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Verified{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockCredentialScope(ctx, tx, emailHash); err != nil {
		return Verified{}, err
	}
	queries := s.queries.WithTx(tx)
	identity, err := queries.GetEmailIdentityForUpdate(ctx, emailHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Verified{}, ErrAuthentication
	}
	if err != nil {
		return Verified{}, err
	}
	credential, err := queries.GetPasswordCredentialForUpdate(ctx, identity.UserID)
	if err != nil {
		return Verified{}, err
	}
	session, err := queries.GetCoreSessionForUpdate(ctx, sessionHash[:])
	if errors.Is(err, pgx.ErrNoRows) || err == nil && identity.UserID != session.UserID {
		return Verified{}, ErrAuthentication
	}
	if err != nil {
		return Verified{}, err
	}
	if session.Status != "active" || !time.Now().UTC().Before(session.ExpiresAt.Time) {
		return Verified{}, ErrAuthentication
	}
	valid, _, err := s.passwords.Verify(ctx, input.CurrentPassword, credential.Verifier)
	if err != nil {
		return Verified{}, ErrDependency
	}
	if !valid {
		return Verified{}, s.recordPasswordFailure(ctx, failureKeys)
	}
	verification, err := s.validateVerificationCodeForUpdate(ctx, queries, emailHash, "security", input.Code)
	if err != nil {
		if errors.Is(err, ErrCodeInvalid) {
			_ = tx.Commit(ctx)
		}
		return Verified{}, err
	}
	if rows, err := queries.ReplacePasswordCredential(ctx, store.ReplacePasswordCredentialParams{
		UserID: session.UserID, Verifier: newVerifier, PolicyVersion: password.PolicyVersion,
	}); err != nil || rows != 1 {
		if err != nil {
			return Verified{}, err
		}
		return Verified{}, ErrDependency
	}
	if _, err := queries.RevokeOtherUserSessions(ctx, store.RevokeOtherUserSessionsParams{
		UserID: session.UserID, ID: session.ID,
	}); err != nil {
		return Verified{}, err
	}
	requestFingerprint := s.digest("change-password", emailHash, []byte(input.Code))
	rows, err := queries.ConsumeVerificationCode(ctx, store.ConsumeVerificationCodeParams{
		ID: verification.ID, ConsumedRequestKey: pgtype.Text{String: input.IdempotencyKey, Valid: true},
		ConsumedRequestFingerprint: requestFingerprint,
	})
	if err != nil || rows != 1 {
		if err != nil {
			return Verified{}, err
		}
		return Verified{}, ErrCodeAlreadyUsed
	}
	if err := s.coordinator.Clear(ctx, passwordFailureKeyNames(failureKeys)...); err != nil {
		return Verified{}, ErrDependency
	}
	if err := tx.Commit(ctx); err != nil {
		return Verified{}, err
	}
	return Verified{
		VerificationID: uuidString(verification.ID), UserID: uuidString(session.UserID),
		EmailVerified: identity.EmailVerified, UserStatus: identity.Status, UserCreatedAt: identity.CreatedAt.Time,
		SessionExpiresAt: session.ExpiresAt.Time,
	}, nil
}

// lockCredentialScope serializes every password authentication or mutation for
// one Email Identity before any durable identity, credential, verification, or
// Session row is locked. This prevents a verified old password from issuing a
// Session after a reset and gives recovery/change one consistent lock root.
func lockCredentialScope(ctx context.Context, tx pgx.Tx, emailHash []byte) error {
	if len(emailHash) < 8 {
		return ErrDependency
	}
	lockID := int64(binary.BigEndian.Uint64(emailHash[:8]))
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID)
	return err
}

func (s *Service) validateVerificationCodeForUpdate(ctx context.Context, queries *store.Queries, emailHash []byte, purpose, code string) (store.GetVerificationCodeForUpdateRow, error) {
	verification, err := queries.GetVerificationCodeForUpdate(ctx, store.GetVerificationCodeForUpdateParams{
		EmailLookupHash: emailHash, Purpose: purpose,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.GetVerificationCodeForUpdateRow{}, ErrCodeInvalid
	}
	if err != nil {
		return store.GetVerificationCodeForUpdateRow{}, err
	}
	if verification.UsedAt.Valid {
		return store.GetVerificationCodeForUpdateRow{}, ErrCodeAlreadyUsed
	}
	if verification.RevokedAt.Valid {
		return store.GetVerificationCodeForUpdateRow{}, ErrCodeInvalid
	}
	if !time.Now().UTC().Before(verification.ExpiresAt.Time) {
		return store.GetVerificationCodeForUpdateRow{}, ErrCodeExpired
	}
	expectedHash := s.digest("code", verification.CodeNonce, []byte(code))
	if subtle.ConstantTimeCompare(expectedHash, verification.CodeHash) != 1 {
		if _, err := queries.RegisterFailedVerificationAttempt(ctx, verification.ID); err != nil {
			return store.GetVerificationCodeForUpdateRow{}, err
		}
		return store.GetVerificationCodeForUpdateRow{}, ErrCodeInvalid
	}
	return verification, nil
}

func (s *Service) newCoreSession() (string, []byte, time.Time, error) {
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		return "", nil, time.Time{}, ErrRandomSource
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := sha256.Sum256([]byte(token))
	return token, tokenHash[:], time.Now().UTC().Add(s.coreSessionTTL), nil
}

func isUniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

type passwordFailureKey struct {
	name   string
	limit  int64
	window time.Duration
}

func (s *Service) passwordFailureKeys(emailHash []byte, deviceID, clientIP string) []passwordFailureKey {
	axes := []struct {
		name, value string
	}{
		{"email", hex.EncodeToString(emailHash)},
		{"device", hex.EncodeToString(s.digest("device", []byte(deviceID)))},
		{"ip", hex.EncodeToString(s.digest("ip", []byte(clientIP)))},
	}
	tiers := []struct {
		name   string
		limit  int64
		window time.Duration
	}{
		{"15m", 5, 15 * time.Minute},
		{"hour", 10, time.Hour},
		{"day", 20, 24 * time.Hour},
	}
	keys := make([]passwordFailureKey, 0, len(axes)*len(tiers))
	for _, axis := range axes {
		for _, tier := range tiers {
			keys = append(keys, passwordFailureKey{
				name:  "platform-core:password-failure:" + axis.name + ":" + axis.value + ":" + tier.name,
				limit: tier.limit, window: tier.window,
			})
		}
	}
	return keys
}

func (s *Service) passwordChallengeRequired(ctx context.Context, keys []passwordFailureKey) (bool, error) {
	challenged := false
	for _, key := range keys {
		count, err := s.coordinator.FailureCount(ctx, key.name)
		if err != nil {
			return false, err
		}
		challenged = challenged || count >= key.limit
	}
	return challenged, nil
}

func (s *Service) recordPasswordFailure(ctx context.Context, keys []passwordFailureKey) error {
	challenged := false
	for _, key := range keys {
		count, err := s.coordinator.RecordFailure(ctx, key.name, key.window)
		if err != nil {
			return ErrDependency
		}
		challenged = challenged || count >= key.limit
	}
	if challenged {
		return ErrChallengeRequired
	}
	return ErrAuthentication
}

func passwordFailureKeyNames(keys []passwordFailureKey) []string {
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, key.name)
	}
	return names
}

func verifiedReplay(purpose string, replay store.GetConsumedVerificationReplayRow) (Verified, error) {
	result := Verified{VerificationID: uuidString(replay.ID)}
	if purpose != "login" {
		return result, nil
	}
	if !replay.LoginUserID.Valid || !replay.LoginSessionExpiresAt.Valid {
		return Verified{}, ErrDependency
	}
	result.UserID = uuidString(replay.LoginUserID)
	result.EmailVerified = replay.LoginUserEmailVerified.Bool
	result.UserStatus = replay.LoginUserStatus.String
	result.UserCreatedAt = replay.LoginUserCreatedAt.Time
	result.SessionExpiresAt = replay.LoginSessionExpiresAt.Time
	return result, nil
}

func (s *Service) verifyRateLimited(ctx context.Context, emailHash []byte, deviceID, clientIP string) (bool, error) {
	dimensions := []struct {
		key    string
		limit  int64
		window time.Duration
	}{
		{"email-verify-hour:" + hex.EncodeToString(emailHash), 50, time.Hour},
		{"email-verify-day:" + hex.EncodeToString(emailHash), 200, 24 * time.Hour},
		{"ip-verify-hour:" + hex.EncodeToString(s.digest("ip", []byte(clientIP))), 60, time.Hour},
		{"ip-verify-day:" + hex.EncodeToString(s.digest("ip", []byte(clientIP))), 500, 24 * time.Hour},
		{"device-verify-hour:" + hex.EncodeToString(s.digest("device", []byte(deviceID))), 30, time.Hour},
		{"device-verify-day:" + hex.EncodeToString(s.digest("device", []byte(deviceID))), 200, 24 * time.Hour},
	}
	limited := false
	for _, dimension := range dimensions {
		allowed, err := s.coordinator.Allow(ctx, "platform-core:verification:"+dimension.key, dimension.limit, dimension.window)
		if err != nil {
			return false, ErrDependency
		}
		limited = limited || !allowed
	}
	return limited, nil
}

func (s *Service) normalizeEmail(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(normalized)
	if err != nil || parsed.Address != normalized || strings.Count(normalized, "@") != 1 {
		return "", ErrInvalid
	}
	domain := normalized[strings.LastIndexByte(normalized, '@')+1:]
	if _, allowed := s.allowedDomains[domain]; !allowed {
		return "", ErrInvalid
	}
	return normalized, nil
}

func (s *Service) digest(label string, parts ...[]byte) []byte {
	mac := hmac.New(sha256.New, s.secretKey)
	_, _ = mac.Write([]byte("henukit-verification:" + label))
	for _, part := range parts {
		_, _ = mac.Write([]byte{0})
		_, _ = mac.Write(part)
	}
	return mac.Sum(nil)
}

func randomCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	digits := value.String()
	return strings.Repeat("0", 6-len(digits)) + digits, nil
}

func validPurpose(value string) bool {
	return value == "register" || value == "login" || value == "bind_email" || value == "security"
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	id, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return ""
	}
	return id.String()
}
