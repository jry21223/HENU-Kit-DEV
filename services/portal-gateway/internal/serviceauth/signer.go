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
	"strings"
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

// Sign adds the standard authentication headers to an HTTP request.
// Canonical string matches platform-core / console-gateway:
//
//	METHOD\nRequestURI\ntimestamp\nnonce\nhex(SHA256(body))
//
// Nonce and signature use base64.RawURLEncoding.
func (s *Signer) Sign(req *http.Request) error {
	return s.sign(req, "")
}

// SignWithActor binds a trusted actor value to a request for an owner that
// treats that header as its authorization subject. It intentionally uses a
// separate canonical form so existing product service contracts keep their
// established five-line signature format.
func (s *Signer) SignWithActor(req *http.Request, actorUserID string) error {
	actorUserID = strings.TrimSpace(actorUserID)
	if actorUserID == "" {
		return fmt.Errorf("actor is required for bound service authentication")
	}
	req.Header.Set("X-Actor-User-Id", actorUserID)
	return s.sign(req, actorUserID)
}

func (s *Signer) sign(req *http.Request, actorUserID string) error {
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

	canonicalParts := []string{req.Method, req.URL.RequestURI(), timestamp, nonceB64, bodyHash}
	if actorUserID != "" {
		canonicalParts = append(canonicalParts, actorUserID)
	}
	canonical := strings.Join(canonicalParts, "\n")

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
