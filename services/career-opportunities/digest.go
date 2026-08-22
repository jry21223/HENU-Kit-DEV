package career

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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DigestJob is the browser-safe subset of one top Career match sent in the
// digest. It mirrors the Platform Core career-digest mail contract; the raw
// profile, crawler internals, and full job descriptions never cross this seam.
type DigestJob struct {
	Company      string   `json:"company"`
	Title        string   `json:"title"`
	Location     string   `json:"location"`
	URL          string   `json:"url"`
	MatchScore   int      `json:"match_score"`
	MatchReasons []string `json:"match_reasons,omitempty"`
}

// DigestRequest is the enqueue body for one completed search. The recipient is
// never part of it: Platform Core resolves the verified email from UserID.
// RequestID is correlation-only and never serialized.
type DigestRequest struct {
	UserID       string      `json:"user_id"`
	SearchID     string      `json:"search_id"`
	CompletedAt  string      `json:"completed_at"`
	SourceCount  int         `json:"source_count"`
	JobCount     int         `json:"job_count"`
	MatchedCount int         `json:"matched_count"`
	Summary      string      `json:"summary"`
	CareerURL    string      `json:"career_url,omitempty"`
	TopJobs      []DigestJob `json:"top_jobs,omitempty"`

	RequestID string `json:"-"`
}

// DigestSender is the #397 enqueue seam: the worker posts one digest per
// completed search through this boundary. The production implementation is an
// HTTP client signing the Platform Core endpoint; tests inject a fake. The
// browser never reaches this seam and never chooses the recipient.
type DigestSender interface {
	SendDigest(ctx context.Context, request DigestRequest) error
}

// ErrDigestUnconfigured is returned when the production sender was never wired
// with an endpoint or credentials.
var ErrDigestUnconfigured = errors.New("career digest sender is not configured")

const digestEnqueuePath = "/api/v1/career-digest-mails"

// HTTPDigestSender posts signed enqueue requests to Platform Core using the
// same five-line HMAC canonical form every service pair in this repo uses.
type HTTPDigestSender struct {
	endpoint string
	clientID string
	secret   string
	keyID    string
	client   *http.Client
}

// NewHTTPDigestSender creates the Platform Core digest client. It accepts HTTP
// only for local compose and test origins, exactly like the Gateway's Career
// client; public deployments reach Platform Core over an isolated network.
func NewHTTPDigestSender(endpoint, clientID, secret, keyID string, client *http.Client) (*HTTPDigestSender, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		clientID == "" || keyID == "" || len(secret) < 32 || credentialPlaceholder(secret) || client == nil {
		return nil, errors.New("career digest endpoint and service credentials are required")
	}
	return &HTTPDigestSender{
		endpoint: strings.TrimRight(parsed.String(), "/") + digestEnqueuePath,
		clientID: clientID, secret: secret, keyID: keyID, client: client,
	}, nil
}

func credentialPlaceholder(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "local-career-digest-secret-32bytes-only!" {
		return true
	}
	for _, marker := range []string{"replace", "example", "change-me", "changeme", "test-secret", "for-test", "test-only"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

// SendDigest signs and posts one digest enqueue. The Idempotency-Key is the
// search-scoped dedupe key so Platform Core treats replays as no-ops.
func (s *HTTPDigestSender) SendDigest(ctx context.Context, request DigestRequest) error {
	if s == nil || s.client == nil {
		return ErrDigestUnconfigured
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Request-Id", request.RequestID)
	httpRequest.Header.Set("Idempotency-Key", "career_search_completed:"+request.SearchID)
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonceValue := base64.RawURLEncoding.EncodeToString(nonce)
	digest := sha256.Sum256(raw)
	canonical := strings.Join([]string{http.MethodPost, digestEnqueuePath, timestamp, nonceValue, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write([]byte(canonical))
	httpRequest.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(s.clientID+":"+s.secret)))
	httpRequest.Header.Set("X-Service-Id", s.clientID)
	httpRequest.Header.Set("X-Key-Id", s.keyID)
	httpRequest.Header.Set("X-Timestamp", timestamp)
	httpRequest.Header.Set("X-Nonce", nonceValue)
	httpRequest.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	response, err := s.client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("enqueue career digest: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("career digest enqueue rejected with status %d", response.StatusCode)
	}
	return nil
}
