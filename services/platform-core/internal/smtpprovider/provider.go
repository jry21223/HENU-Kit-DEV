package smtpprovider

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9:._-]{8,200}$`)

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
}

type Provider struct {
	token  string
	ledger string
	mailer Mailer
	mu     sync.Mutex
}

func New(config Config) (*Provider, error) {
	if len(config.Token) < 32 || config.LedgerDirectory == "" || config.Mailer == nil {
		return nil, errors.New("provider token, ledger directory, and mailer are required")
	}
	if err := os.MkdirAll(config.LedgerDirectory, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(config.LedgerDirectory, 0o700); err != nil {
		return nil, err
	}
	return &Provider{token: config.Token, ledger: config.LedgerDirectory, mailer: config.Mailer}, nil
}

func (provider *Provider) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.Path != "/internal/send" {
		http.NotFound(writer, request)
		return
	}
	provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	if len(provided) != len(provider.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(provider.token)) != 1 {
		writeProviderError(writer, http.StatusUnauthorized, "authentication_failed")
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
		writeProviderError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	expiresAt, parseErr := time.Parse(time.RFC3339, payload.Variables.ExpiresAt)
	address, addressErr := mail.ParseAddress(payload.Recipient)
	headerKey := request.Header.Get("Idempotency-Key")
	validPurpose := payload.Variables.Purpose == "login" || payload.Variables.Purpose == "bind_email" || payload.Variables.Purpose == "security"
	if parseErr != nil || addressErr != nil || address.Address != payload.Recipient || payload.Template != "henukit_verification_code" || len(payload.Variables.Code) != 6 || !validPurpose || payload.RequestID == "" || payload.IdempotencyKey != headerKey || !idempotencyPattern.MatchString(headerKey) {
		writeProviderError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if !time.Now().UTC().Before(expiresAt) {
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
	if accepted, err := os.ReadFile(acceptedPath); err == nil {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(accepted)
		return
	}
	if info, err := os.Stat(pendingPath); err == nil && time.Since(info.ModTime()) < 2*time.Minute {
		writeProviderError(writer, http.StatusConflict, "delivery_in_progress")
		return
	}
	_ = os.Remove(pendingPath)
	claim, err := os.OpenFile(pendingPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		writeProviderError(writer, http.StatusConflict, "delivery_in_progress")
		return
	}
	_ = claim.Close()
	defer func() { _ = os.Remove(pendingPath) }()
	if err := provider.mailer.Send(request.Context(), Mail{Recipient: payload.Recipient, Code: payload.Variables.Code, Purpose: payload.Variables.Purpose, ExpiresAt: expiresAt, RequestID: payload.RequestID, MessageID: messageID}); err != nil {
		writeProviderError(writer, http.StatusServiceUnavailable, "smtp_unavailable")
		return
	}
	accepted, _ := json.Marshal(map[string]string{"message_id": messageID})
	accepted = append(accepted, '\n')
	temporaryPath := acceptedPath + ".next"
	if err := os.WriteFile(temporaryPath, accepted, 0o600); err != nil {
		writeProviderError(writer, http.StatusServiceUnavailable, "ledger_unavailable")
		return
	}
	if err := os.Rename(temporaryPath, acceptedPath); err != nil {
		writeProviderError(writer, http.StatusServiceUnavailable, "ledger_unavailable")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(accepted)
}

func writeProviderError(writer http.ResponseWriter, status int, code string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"error": code})
}
