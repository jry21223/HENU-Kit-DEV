package signing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// TestSignMatchesFiveLineCanonical pins the exact canonical string contract
// shared with services/food and services/portal-gateway: any drift in the
// header set or canonical composition breaks the Food service's verification.
func TestSignMatchesFiveLineCanonical(t *testing.T) {
	signer := NewSigner("food-post-create", "food-post-create-secret-at-least-32-bytes", "active")
	fixed := time.Unix(1755230400, 0)
	signer.now = func() time.Time { return fixed }

	body := []byte(`{"venue_name":"仁和食堂"}`)
	request, err := http.NewRequest(http.MethodPost, "http://food/api/v1/food/posts", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Sign(request); err != nil {
		t.Fatal(err)
	}

	clientID, secret, ok := request.BasicAuth()
	if !ok || clientID != "food-post-create" || secret != "food-post-create-secret-at-least-32-bytes" {
		t.Fatalf("basic auth = %q/%q ok=%v", clientID, secret, ok)
	}
	timestamp := request.Header.Get("X-Timestamp")
	if timestamp != strconv.FormatInt(fixed.Unix(), 10) {
		t.Fatalf("timestamp = %q", timestamp)
	}
	nonce := request.Header.Get("X-Nonce")
	decoded, err := base64.RawURLEncoding.DecodeString(nonce)
	if err != nil || len(decoded) != 24 {
		t.Fatalf("nonce = %q (%v)", nonce, err)
	}
	digest := sha256.Sum256(body)
	canonical := http.MethodPost + "\n" + "/api/v1/food/posts" + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(digest[:])
	mac := hmac.New(sha256.New, []byte("food-post-create-secret-at-least-32-bytes"))
	_, _ = mac.Write([]byte(canonical))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if got := request.Header.Get("X-Signature"); got != expected {
		t.Fatalf("signature = %q, want %q", got, expected)
	}
	if got := request.Header.Get("X-Service-Id"); got != "food-post-create" {
		t.Fatalf("X-Service-Id = %q", got)
	}
	if got := request.Header.Get("X-Key-Id"); got != "active" {
		t.Fatalf("X-Key-Id = %q", got)
	}
}

// TestSignReSignsReusableBody verifies the body is restored after signing so
// the caller can send it.
func TestSignReSignsReusableBody(t *testing.T) {
	signer := NewSigner("a", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "k")
	body := []byte(`{"x":1}`)
	request, err := http.NewRequest(http.MethodPost, "http://food/api/v1/food/posts", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Sign(request); err != nil {
		t.Fatal(err)
	}
	replayed := make([]byte, len(body))
	if _, err := request.Body.Read(replayed); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(replayed, body) {
		t.Fatalf("body not restored: %q", replayed)
	}
}
