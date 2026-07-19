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
	nonceBytes := make([]byte, 24)
	if _, err := rand.Read(nonceBytes); err != nil {
		return err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	timestamp := fmt.Sprintf("%d", s.now().Unix())
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
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
