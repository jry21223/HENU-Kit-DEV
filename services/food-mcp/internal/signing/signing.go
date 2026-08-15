// Package signing implements the Food Post five-line HMAC-SHA256 service
// signature (see services/food and services/portal-gateway for the canonical
// contract). food-mcp is an independent module and cannot import the gateway's
// internal packages, so this tiny re-implementation exists; contract tests
// keep it honest.
package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Signer adds the five-line service authentication headers to a request.
type Signer struct {
	clientID string
	secret   string
	keyID    string
	now      func() time.Time
}

// NewSigner creates a Signer for one Food Post credential ring.
func NewSigner(clientID, secret, keyID string) *Signer {
	return &Signer{clientID: clientID, secret: secret, keyID: keyID, now: time.Now}
}

// Sign adds Basic auth plus the five canonical headers (method, RequestURI,
// timestamp, nonce, hex(SHA256(body))). Actor headers are caller-managed and
// intentionally not part of the canonical string.
func (s *Signer) Sign(request *http.Request) error {
	body, err := readBody(request)
	if err != nil {
		return fmt.Errorf("read request body for signature: %w", err)
	}
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.New().String()[:24]))
	digest := sha256.Sum256(body)
	timestamp := strconv.FormatInt(s.now().Unix(), 10)
	canonical := request.Method + "\n" + request.URL.RequestURI() + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(digest[:])
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write([]byte(canonical))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	request.Header.Set("X-Service-Id", s.clientID)
	request.Header.Set("X-Key-Id", s.keyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", signature)
	request.SetBasicAuth(s.clientID, s.secret)
	return nil
}
