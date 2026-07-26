package verification

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
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
)

type Coordinator interface {
	Allow(context.Context, string, int64, time.Duration) (bool, error)
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
	if err != nil || !password.AcceptableInput(input.Password) || input.DeviceID == "" || input.ClientIP == "" {
		return Verified{}, ErrAuthentication
	}
	emailHash := s.digest("email", []byte(email))
	for _, dimension := range []string{
		"email:" + hex.EncodeToString(emailHash),
		"device:" + hex.EncodeToString(s.digest("device", []byte(input.DeviceID))),
		"ip:" + hex.EncodeToString(s.digest("ip", []byte(input.ClientIP))),
	} {
		allowed, allowErr := s.coordinator.Allow(ctx, "platform-core:password-login:"+dimension, 100, time.Hour)
		if allowErr != nil {
			return Verified{}, ErrDependency
		}
		if !allowed {
			return Verified{}, ErrRateLimited
		}
	}
	credential, err := s.queries.GetPasswordLoginCredential(ctx, emailHash)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _, _ = s.passwords.Verify(ctx, input.Password, s.dummyVerifier)
		return Verified{}, ErrAuthentication
	}
	if err != nil {
		return Verified{}, err
	}
	valid, needsRehash, err := s.passwords.Verify(ctx, input.Password, credential.Verifier)
	if err != nil {
		return Verified{}, ErrDependency
	}
	if !valid || credential.Status != "active" {
		return Verified{}, ErrAuthentication
	}
	var upgradedVerifier string
	if needsRehash || credential.PolicyVersion != password.PolicyVersion {
		upgradedVerifier, err = s.passwords.Hash(ctx, input.Password)
		if err != nil {
			return Verified{}, ErrDependency
		}
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
	queries := s.queries.WithTx(tx)
	if _, err := queries.CreateCoreSession(ctx, store.CreateCoreSessionParams{
		UserID: credential.UserID, TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: sessionExpiresAt, Valid: true},
	}); err != nil {
		return Verified{}, err
	}
	if upgradedVerifier != "" {
		if _, err := queries.UpgradePasswordCredential(ctx, store.UpgradePasswordCredentialParams{
			UserID: credential.UserID, NewVerifier: upgradedVerifier,
			PolicyVersion: password.PolicyVersion, OldVerifier: credential.Verifier,
		}); err != nil {
			return Verified{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Verified{}, err
	}
	return Verified{
		UserID: uuidString(credential.UserID), EmailVerified: credential.EmailVerified,
		UserStatus: credential.Status, UserCreatedAt: credential.CreatedAt.Time,
		SessionToken: sessionToken, SessionExpiresAt: sessionExpiresAt,
	}, nil
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
