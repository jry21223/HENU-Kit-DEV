package serviceauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Signer struct {
	clientID, secret, keyID string
	now                     func() time.Time
}

func New(clientID, secret, keyID string) (*Signer, error) {
	if clientID == "" || len(secret) < 32 || keyID == "" {
		return nil, errors.New("invalid service signing credentials")
	}
	return &Signer{clientID: clientID, secret: secret, keyID: keyID, now: time.Now}, nil
}

func (s *Signer) Sign(request *http.Request, body []byte) error {
	return s.sign(request, body, "")
}

// SignWithActor signs an Account Portfolio-style owner request whose actor is
// part of the authorization subject. The actor header is set before the
// signature is calculated so a downstream owner can reject header swapping.
// Existing Console owner contracts retain Sign's five-line canonical form.
func (s *Signer) SignWithActor(request *http.Request, body []byte, actorUserID string) error {
	actorUserID = strings.TrimSpace(actorUserID)
	if actorUserID == "" {
		return errors.New("actor is required for actor-bound service authentication")
	}
	request.Header.Set("X-Actor-User-Id", actorUserID)
	return s.sign(request, body, actorUserID)
}

func (s *Signer) sign(request *http.Request, body []byte, actorUserID string) error {
	nonceBytes := make([]byte, 24)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	timestamp := fmt.Sprintf("%d", s.now().Unix())
	digest := sha256.Sum256(body)
	canonicalParts := []string{request.Method, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:])}
	if actorUserID != "" {
		canonicalParts = append(canonicalParts, actorUserID)
	}
	canonical := strings.Join(canonicalParts, "\n")
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth(s.clientID, s.secret)
	request.Header.Set("X-Service-Id", s.clientID)
	request.Header.Set("X-Key-Id", s.keyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	return nil
}
