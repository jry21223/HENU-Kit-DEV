// Package signing implements the Career six-line HMAC-SHA256 service
// signature. Unlike Food (five lines), Career binds the actor into the
// canonical string as the sixth line (see services/career-opportunities and
// services/portal-gateway/internal/serviceauth for the canonical contract).
// career-mcp is an independent module and cannot import the gateway's internal
// packages, so this tiny re-implementation exists; contract tests keep it
// honest.
package signing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Signer adds the Career service authentication headers to a request, binding
// the actor user ID into the signature so a signed request cannot be replayed
// with a different actor.
type Signer struct {
	clientID string
	secret   string
	keyID    string
	now      func() time.Time
}

// NewSigner creates a Signer for one Career credential ring.
func NewSigner(clientID, secret, keyID string) *Signer {
	return &Signer{clientID: clientID, secret: secret, keyID: keyID, now: time.Now}
}

// SignWithActor adds Basic auth plus the canonical headers and signs the
// six-line canonical string (method, RequestURI, timestamp, nonce,
// hex(SHA256(body)), actorUserID).
func (s *Signer) SignWithActor(request *http.Request, actorUserID string) error {
	body, err := readBody(request)
	if err != nil {
		return fmt.Errorf("read request body for signature: %w", err)
	}
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.New().String()[:24]))
	digest := sha256.Sum256(body)
	timestamp := strconv.FormatInt(s.now().Unix(), 10)
	canonical := request.Method + "\n" + request.URL.RequestURI() + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(digest[:]) + "\n" + actorUserID
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write([]byte(canonical))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	request.Header.Set("X-Service-Id", s.clientID)
	request.Header.Set("X-Key-Id", s.keyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", signature)
	request.Header.Set("X-Actor-User-Id", actorUserID)
	request.SetBasicAuth(s.clientID, s.secret)
	return nil
}

func readBody(request *http.Request) ([]byte, error) {
	if request.Body == nil || request.Body == http.NoBody {
		return []byte{}, nil
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}
