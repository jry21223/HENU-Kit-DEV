package verification

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/store"
	"henukit.dev/platform-core/internal/verificationmail"
)

var (
	ErrInvalid         = errors.New("verification request is invalid")
	ErrDependency      = errors.New("verification dependency unavailable")
	ErrIdempotency     = errors.New("idempotency key conflicts with another request")
	ErrCodeInvalid     = errors.New("verification code is invalid")
	ErrCodeExpired     = errors.New("verification code expired")
	ErrCodeAlreadyUsed = errors.New("verification code was already used")
	ErrRateLimited     = errors.New("verification attempts are rate limited")
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
	allowedDomains map[string]struct{}
	codeTTL        time.Duration
	resendDelay    time.Duration
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
	VerificationID string
}

func New(queries *store.Queries, database *pgxpool.Pool, coordinator Coordinator, masterKey []byte, allowedDomains []string, codeTTL, resendDelay time.Duration) (*Service, error) {
	if queries == nil || database == nil || coordinator == nil || len(masterKey) != 32 {
		return nil, errors.New("verification dependencies and a 32-byte key are required")
	}
	if codeTTL < 5*time.Minute || codeTTL > 10*time.Minute || resendDelay < 60*time.Second {
		return nil, errors.New("verification TTL must be 5-10m and resend delay at least 60s")
	}
	domains := make(map[string]struct{}, len(allowedDomains))
	for _, domain := range allowedDomains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain != "" {
			domains[domain] = struct{}{}
		}
	}
	if len(domains) == 0 {
		return nil, errors.New("at least one student email domain is required")
	}
	mailCodec, err := verificationmail.NewCodec(masterKey)
	if err != nil {
		return nil, err
	}
	return &Service{
		queries: queries, database: database, coordinator: coordinator, secretKey: append([]byte(nil), masterKey...),
		mailCodec: mailCodec, allowedDomains: domains,
		codeTTL: codeTTL, resendDelay: resendDelay,
	}, nil
}

func (s *Service) Request(ctx context.Context, input RequestInput) (Accepted, error) {
	email, err := s.normalizeEmail(input.Email)
	if err != nil || !validPurpose(input.Purpose) || len(input.DeviceID) < 8 || len(input.DeviceID) > 200 || input.ClientIP == "" || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 || input.RequestID == "" {
		return Accepted{}, ErrInvalid
	}
	emailHash := s.digest("email", []byte(email))
	fingerprint := s.digest("request", emailHash, []byte(input.Purpose), []byte(input.ClientID))
	if existing, err := s.queries.GetVerificationRequestByKey(ctx, input.IdempotencyKey); err == nil {
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
		return Accepted{}, err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return Accepted{}, err
	}
	codeHash := s.digest("code", nonce, []byte(code))
	now := time.Now().UTC()
	expiresAt := now.Add(s.codeTTL)
	recipientCiphertext, payloadCiphertext, err := s.mailCodec.Encode(email, verificationmail.Payload{Code: code, Purpose: input.Purpose, ExpiresAt: expiresAt})
	if err != nil {
		return Accepted{}, err
	}
	tx, err := s.database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Accepted{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := s.queries.WithTx(tx)
	created, err := queries.CreateVerificationCode(ctx, store.CreateVerificationCodeParams{
		EmailLookupHash: emailHash, Purpose: input.Purpose, RequestKey: input.IdempotencyKey,
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
	if err != nil || !validPurpose(input.Purpose) || len(input.Code) != 6 || len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 200 || input.DeviceID == "" || input.ClientIP == "" {
		return Verified{}, ErrInvalid
	}
	emailHash := s.digest("email", []byte(email))
	requestFingerprint := s.digest("consume", emailHash, []byte(input.Purpose), []byte(input.Code))
	if replay, replayErr := s.queries.GetConsumedVerificationReplay(ctx, pgtype.Text{String: input.IdempotencyKey, Valid: true}); replayErr == nil {
		if subtle.ConstantTimeCompare(replay.ConsumedRequestFingerprint, requestFingerprint) == 1 {
			return Verified{VerificationID: uuidString(replay.ID)}, nil
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
			return Verified{VerificationID: uuidString(verification.ID)}, nil
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
	if err := tx.Commit(ctx); err != nil {
		return Verified{}, err
	}
	return Verified{VerificationID: uuidString(verification.ID)}, nil
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
	return value == "login" || value == "bind_email" || value == "security"
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
