package signing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"testing"
)

const (
	contractSecret  = "career-contract-test-secret-at-least-32-bytes"
	contractActorID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

// TestSignWithActorProducesCareerContractSignature verifies the signer matches
// Career's six-line canonical form exactly (actor is the sixth line), so a
// request signed here passes services/career-opportunities' authenticate.
func TestSignWithActorProducesCareerContractSignature(t *testing.T) {
	body := []byte(`{"profile":{"target_roles":"后端"}}`)
	request, err := http.NewRequest(http.MethodPost, "http://career.test/api/v1/career/profile/extractions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	signer := NewSigner("career-mcp", contractSecret, "active")
	if err := signer.SignWithActor(request, contractActorID); err != nil {
		t.Fatal(err)
	}

	if request.Header.Get("X-Service-Id") != "career-mcp" || request.Header.Get("X-Key-Id") != "active" {
		t.Fatalf("service headers = %q / %q", request.Header.Get("X-Service-Id"), request.Header.Get("X-Key-Id"))
	}
	clientID, secret, ok := request.BasicAuth()
	if !ok || clientID != "career-mcp" || secret != contractSecret {
		t.Fatalf("basic auth = %q / %q", clientID, secret)
	}
	if request.Header.Get("X-Actor-User-Id") != contractActorID {
		t.Fatalf("actor header = %q", request.Header.Get("X-Actor-User-Id"))
	}
	timestamp := request.Header.Get("X-Timestamp")
	nonce := request.Header.Get("X-Nonce")
	if timestamp == "" || nonce == "" {
		t.Fatal("timestamp or nonce missing")
	}
	digest := sha256.Sum256(body)
	canonical := request.Method + "\n" + request.URL.RequestURI() + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(digest[:]) + "\n" + contractActorID
	mac := hmac.New(sha256.New, []byte(contractSecret))
	_, _ = mac.Write([]byte(canonical))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if got := request.Header.Get("X-Signature"); got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}
