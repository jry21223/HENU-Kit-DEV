package smtpprovider

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9:._-]{8,200}$`)
var auditIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type Mail struct {
	Recipient string
	Code      string
	Purpose   string
	ExpiresAt time.Time
	RequestID string
	MessageID string
}

type Mailer interface {
	Send(context.Context, Mail) error
}

type Config struct {
	Token           string
	LedgerDirectory string
	Mailer          Mailer
	Logger          *slog.Logger
	ProviderID      string
	KeyID           string
}

type Provider struct {
	token           string
	ledger          string
	mailer          Mailer
	logger          *slog.Logger
	providerID      string
	keyID           string
	readAccepted    func(string) ([]byte, error)
	persistAccepted func(string, string, []byte) error
	mu              sync.Mutex
}

func New(config Config) (*Provider, error) {
	if len(config.Token) < 32 || config.LedgerDirectory == "" || config.Mailer == nil || !auditIdentifierPattern.MatchString(config.ProviderID) || !auditIdentifierPattern.MatchString(config.KeyID) {
		return nil, errors.New("provider token, ledger directory, mailer, provider ID, and key ID are required")
	}
	if config.Logger == nil {
		config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if err := os.MkdirAll(config.LedgerDirectory, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(config.LedgerDirectory, 0o700); err != nil {
		return nil, err
	}
	return &Provider{
		token: config.Token, ledger: config.LedgerDirectory, mailer: config.Mailer,
		logger: config.Logger, providerID: config.ProviderID, keyID: config.KeyID,
		readAccepted:    os.ReadFile,
		persistAccepted: persistAcceptedMarker,
	}, nil
}

func (provider *Provider) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.Path != "/internal/send" {
		http.NotFound(writer, request)
		return
	}
	startedAt := time.Now()
	requestID := safeAuditIdentifier(request.Header.Get("X-Request-ID"))
	attempt, attemptValid := auditAttempt(request.Header.Get("X-Mail-Attempt"))
	provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	if len(provided) != len(provider.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(provider.token)) != 1 {
		provider.audit(requestID, "rejected", "AUTHENTICATION_FAILED", attempt, startedAt)
		writeProviderError(writer, http.StatusUnauthorized, "authentication_failed")
		return
	}
	if !attemptValid {
		provider.audit(requestID, "rejected", "INVALID_REQUEST", attempt, startedAt)
		writeProviderError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	var payload struct {
		Recipient string `json:"recipient"`
		Template  string `json:"template"`
		Variables struct {
			Code      string `json:"code"`
			Purpose   string `json:"purpose"`
			ExpiresAt string `json:"expires_at"`
		} `json:"variables"`
		RequestID      string `json:"request_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		provider.audit(requestID, "rejected", "INVALID_REQUEST", attempt, startedAt)
		writeProviderError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if requestID == "" {
		requestID = safeAuditIdentifier(payload.RequestID)
	}
	expiresAt, parseErr := time.Parse(time.RFC3339, payload.Variables.ExpiresAt)
	address, addressErr := mail.ParseAddress(payload.Recipient)
	headerKey := request.Header.Get("Idempotency-Key")
	validPurpose := payload.Variables.Purpose == "login" || payload.Variables.Purpose == "bind_email" || payload.Variables.Purpose == "security"
	if parseErr != nil || addressErr != nil || address.Address != payload.Recipient || payload.Template != "henukit_verification_code" || len(payload.Variables.Code) != 6 || !validPurpose || !auditIdentifierPattern.MatchString(payload.RequestID) || (requestID != "" && payload.RequestID != requestID) || payload.IdempotencyKey != headerKey || !idempotencyPattern.MatchString(headerKey) {
		provider.audit(requestID, "rejected", "INVALID_REQUEST", attempt, startedAt)
		writeProviderError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if !time.Now().UTC().Before(expiresAt) {
		provider.audit(requestID, "rejected", "VERIFICATION_EXPIRED", attempt, startedAt)
		writeProviderError(writer, http.StatusGone, "verification_expired")
		return
	}
	digest := sha256.Sum256([]byte(headerKey))
	ledgerKey := hex.EncodeToString(digest[:])
	acceptedPath := filepath.Join(provider.ledger, ledgerKey+".accepted.json")
	pendingPath := filepath.Join(provider.ledger, ledgerKey+".pending")
	messageID := "henukit-" + ledgerKey[:32] + "@superhuazai.me"

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if accepted, err := provider.readAccepted(acceptedPath); err == nil {
		provider.audit(requestID, "replayed", "NONE", attempt, startedAt)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(accepted)
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		provider.audit(requestID, "retry", "LEDGER_UNAVAILABLE", attempt, startedAt)
		writeProviderError(writer, http.StatusServiceUnavailable, "ledger_unavailable")
		return
	}
	if pending, err := os.ReadFile(pendingPath); err == nil {
		if acceptedMarker(pending, messageID) {
			_ = os.Rename(pendingPath, acceptedPath)
			provider.audit(requestID, "replayed", "NONE", attempt, startedAt)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write(pending)
			return
		}
		provider.audit(requestID, "retry", "DELIVERY_IN_PROGRESS", attempt, startedAt)
		writeProviderError(writer, http.StatusConflict, "delivery_in_progress")
		return
	}
	claim, err := os.OpenFile(pendingPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		provider.audit(requestID, "retry", "DELIVERY_IN_PROGRESS", attempt, startedAt)
		writeProviderError(writer, http.StatusConflict, "delivery_in_progress")
		return
	}
	if err := claim.Close(); err != nil {
		_ = os.Remove(pendingPath)
		provider.audit(requestID, "retry", "LEDGER_UNAVAILABLE", attempt, startedAt)
		writeProviderError(writer, http.StatusServiceUnavailable, "ledger_unavailable")
		return
	}
	if err := syncDirectory(provider.ledger); err != nil {
		_ = os.Remove(pendingPath)
		provider.audit(requestID, "retry", "LEDGER_UNAVAILABLE", attempt, startedAt)
		writeProviderError(writer, http.StatusServiceUnavailable, "ledger_unavailable")
		return
	}
	removePending := true
	defer func() {
		if removePending {
			_ = os.Remove(pendingPath)
		}
	}()
	if err := provider.mailer.Send(request.Context(), Mail{Recipient: payload.Recipient, Code: payload.Variables.Code, Purpose: payload.Variables.Purpose, ExpiresAt: expiresAt, RequestID: payload.RequestID, MessageID: messageID}); err != nil {
		status, result, errorCode, responseCode := classifySMTPFailure(err)
		provider.audit(requestID, result, errorCode, attempt, startedAt)
		writeProviderError(writer, status, responseCode)
		return
	}
	// From this point forward, deleting the claim could cause a duplicate send.
	// An unreadable/empty claim is therefore a durable fail-closed unknown state.
	removePending = false
	accepted, _ := json.Marshal(map[string]string{"message_id": messageID})
	accepted = append(accepted, '\n')
	if err := provider.persistAccepted(pendingPath, acceptedPath, accepted); err != nil {
		provider.audit(requestID, "retry", "LEDGER_UNAVAILABLE", attempt, startedAt)
		writeProviderError(writer, http.StatusServiceUnavailable, "ledger_unavailable")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	provider.audit(requestID, "succeeded", "NONE", attempt, startedAt)
	_, _ = writer.Write(accepted)
}

func persistAcceptedMarker(pendingPath, acceptedPath string, accepted []byte) error {
	marker, err := os.OpenFile(pendingPath, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := marker.Write(accepted); err != nil {
		_ = marker.Close()
		return err
	}
	if err := marker.Sync(); err != nil {
		_ = marker.Close()
		return err
	}
	if err := marker.Close(); err != nil {
		return err
	}
	if err := os.Rename(pendingPath, acceptedPath); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(acceptedPath))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func acceptedMarker(marker []byte, messageID string) bool {
	var accepted struct {
		MessageID string `json:"message_id"`
	}
	return json.Unmarshal(marker, &accepted) == nil && accepted.MessageID == messageID
}

func (provider *Provider) audit(requestID, result, errorCode string, attempt int, startedAt time.Time) {
	provider.logger.Info("smtp_provider_delivery",
		"request_id", requestID,
		"result", result,
		"error_code", errorCode,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"attempt_count", attempt,
		"retry_count", max(attempt-1, 0),
		"provider_id", provider.providerID,
		"key_id", provider.keyID,
	)
}

func auditAttempt(raw string) (int, bool) {
	if raw == "" {
		return 1, true
	}
	attempt, err := strconv.Atoi(raw)
	if err != nil || attempt < 1 || attempt > 100 {
		return 1, false
	}
	return attempt, true
}

func safeAuditIdentifier(value string) string {
	if auditIdentifierPattern.MatchString(value) {
		return value
	}
	return ""
}

func classifySMTPFailure(err error) (status int, result, errorCode, responseCode string) {
	var smtpError *textproto.Error
	if errors.As(err, &smtpError) && smtpError.Code >= 500 && smtpError.Code <= 599 {
		return http.StatusUnprocessableEntity, "failed", "SMTP_PERMANENT_FAILURE", "smtp_permanent_failure"
	}
	return http.StatusServiceUnavailable, "retry", "SMTP_TEMPORARY_FAILURE", "smtp_temporary_failure"
}

func writeProviderError(writer http.ResponseWriter, status int, code string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"error": code})
}
