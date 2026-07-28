package serviceauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSignCanonicalUsesHexBodyHashAndRawURLEncoding(t *testing.T) {
	const secret = "portal-client-secret-with-enough-entropy"
	signer := NewSigner("portal-gateway", secret, "active-key")

	body := []byte(`{"grant_type":"authorization_code","client_id":"portal-gateway"}`)
	req, err := http.NewRequest(http.MethodPost, "https://platform-core.example/api/v1/oauth/token", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Sign(req); err != nil {
		t.Fatal(err)
	}

	// Body must remain readable after signing.
	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBody) != string(body) {
		t.Fatalf("body rewritten incorrectly: %s", gotBody)
	}

	timestamp := req.Header.Get("X-Timestamp")
	nonce := req.Header.Get("X-Nonce")
	signature := req.Header.Get("X-Signature")
	if timestamp == "" || nonce == "" || signature == "" {
		t.Fatalf("missing signing headers: ts=%q nonce=%q sig=%q", timestamp, nonce, signature)
	}

	// Nonce and signature must be RawURLEncoding (no padding '=' for 24-byte nonce / 32-byte MAC).
	if strings.ContainsAny(nonce, "+/=") || strings.ContainsAny(signature, "+/=") {
		t.Fatalf("nonce/signature must be base64.RawURLEncoding, got nonce=%q sig=%q", nonce, signature)
	}
	if _, err := base64.RawURLEncoding.DecodeString(nonce); err != nil {
		t.Fatalf("nonce not RawURLEncoding: %v", err)
	}
	if _, err := base64.RawURLEncoding.DecodeString(signature); err != nil {
		t.Fatalf("signature not RawURLEncoding: %v", err)
	}

	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{
		http.MethodPost,
		req.URL.RequestURI(),
		timestamp,
		nonce,
		hex.EncodeToString(digest[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if signature != want {
		t.Fatalf("signature mismatch\n got %s\nwant %s\ncanonical:\n%s", signature, want, canonical)
	}

	// Basic auth must remain clientID:secret (StdEncoding).
	user, pass, ok := req.BasicAuth()
	if !ok || user != "portal-gateway" || pass != secret {
		t.Fatalf("BasicAuth = (%q,%q,%v)", user, pass, ok)
	}
	if req.Header.Get("X-Service-Id") != "portal-gateway" || req.Header.Get("X-Key-Id") != "active-key" {
		t.Fatalf("service headers mismatch: id=%q key=%q", req.Header.Get("X-Service-Id"), req.Header.Get("X-Key-Id"))
	}
}

func TestSignEmptyBodyHashesEmptySlice(t *testing.T) {
	const secret = "portal-client-secret-with-enough-entropy"
	signer := NewSigner("portal-gateway", secret, "active-key")
	req, err := http.NewRequest(http.MethodGet, "https://platform-core.example/api/v1/ready", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Sign(req); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{
		http.MethodGet,
		req.URL.RequestURI(),
		req.Header.Get("X-Timestamp"),
		req.Header.Get("X-Nonce"),
		hex.EncodeToString(digest[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if got := req.Header.Get("X-Signature"); got != want {
		t.Fatalf("empty-body signature mismatch got=%s want=%s", got, want)
	}
}

func TestSignWithActorBindsActorToCanonicalRequest(t *testing.T) {
	const secret = "portal-client-secret-with-enough-entropy"
	const actor = "11111111-1111-4111-8111-111111111111"
	signer := NewSigner("portal-gateway", secret, "active-key")
	req, err := http.NewRequest(http.MethodGet, "https://account.example/api/v1/account/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.SignWithActor(req, actor); err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("X-Actor-User-Id"); got != actor {
		t.Fatalf("actor header = %q, want %q", got, actor)
	}
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{
		http.MethodGet,
		req.URL.RequestURI(),
		req.Header.Get("X-Timestamp"),
		req.Header.Get("X-Nonce"),
		hex.EncodeToString(digest[:]),
		actor,
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if got := req.Header.Get("X-Signature"); got != want {
		t.Fatalf("actor-bound signature mismatch got=%s want=%s", got, want)
	}

	tamperedActor := "22222222-2222-4222-8222-222222222222"
	req.Header.Set("X-Actor-User-Id", tamperedActor)
	tamperedCanonical := strings.TrimSuffix(canonical, actor) + tamperedActor
	tamperedMAC := hmac.New(sha256.New, []byte(secret))
	_, _ = tamperedMAC.Write([]byte(tamperedCanonical))
	if req.Header.Get("X-Signature") == base64.RawURLEncoding.EncodeToString(tamperedMAC.Sum(nil)) {
		t.Fatal("signature remained valid after actor mutation")
	}
}
