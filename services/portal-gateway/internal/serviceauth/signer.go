package serviceauth

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Signer adds HMAC-SHA256 service-to-service authentication headers.
type Signer struct {
	clientID     string
	clientSecret string
	keyID        string
}

// NewSigner creates a Signer for a specific service pair.
func NewSigner(clientID, clientSecret, keyID string) *Signer {
	return &Signer{clientID: clientID, clientSecret: clientSecret, keyID: keyID}
}

// ClientID returns the service client identifier used for Basic auth and body.client_id.
func (s *Signer) ClientID() string {
	return s.clientID
}

// Sign adds authentication headers to an HTTP request.
// Canonical string matches platform-core / console-gateway:
//
//	METHOD\nRequestURI\ntimestamp\nnonce\nhex(SHA256(body))
//
// Nonce and signature use base64.RawURLEncoding.
func (s *Signer) Sign(req *http.Request) error {
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("rand.Read: %w", err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonceB64 := base64.RawURLEncoding.EncodeToString(nonce)

	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	hash := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(hash[:])

	canonical := fmt.Sprintf("%s\n%s\n%s\n%s\n%s",
		req.Method, req.URL.RequestURI(), timestamp, nonceB64, bodyHash)

	mac := hmac.New(sha256.New, []byte(s.clientSecret))
	mac.Write([]byte(canonical))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
		[]byte(s.clientID+":"+s.clientSecret)))
	req.Header.Set("X-Service-Id", s.clientID)
	req.Header.Set("X-Key-Id", s.keyID)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonceB64)
	req.Header.Set("X-Signature", signature)

	return nil
}
