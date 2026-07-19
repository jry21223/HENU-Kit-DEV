package platformcore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientSignsExchangeAndNeverPlacesTokenInURL(t *testing.T) {
	const secret = "test-console-client-secret-with-entropy"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		digest := sha256.Sum256(body)
		canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		wantSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		clientID, password, basic := request.BasicAuth()
		if !basic || clientID != "console-gateway" || password != secret || request.Header.Get("X-Service-Id") != clientID || request.Header.Get("X-Key-Id") != "active-key" || request.Header.Get("X-Signature") != wantSignature || request.Header.Get("Idempotency-Key") == "" {
			t.Errorf("invalid authenticated exchange request")
		}
		if strings.Contains(request.URL.String(), "authorization-code") || strings.Contains(request.URL.String(), "exchange_token") {
			t.Errorf("credential leaked into URL: %s", request.URL)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"user":                   map[string]string{"id": "171f1c6f-7b10-4c92-91a2-b39bf5af5302"},
			"session_exchange_token": "exchange_token_with_at_least_32_characters", "expires_at": time.Now().Add(5 * time.Minute),
		}})
	}))
	defer server.Close()
	client, err := New(server.URL, "console-gateway", secret, "active-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := client.ExchangeCode(t.Context(), "authorization-code", "https://console.example/callback", strings.Repeat("v", 43), "idem_test_exchange")
	if err != nil || exchange.UserID == "" || exchange.ExchangeToken == "" {
		t.Fatalf("exchange = %+v, err=%v", exchange, err)
	}
}

func TestAuthorizationStatusMapping(t *testing.T) {
	for status, want := range map[int]error{http.StatusUnauthorized: ErrUnauthorized, http.StatusForbidden: ErrForbidden, http.StatusConflict: ErrConflict} {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(status) }))
		client, _ := New(server.URL, "console-gateway", "secret-with-enough-entropy", "active-key", server.Client())
		err := client.CheckOverview(t.Context(), "exchange_token_with_at_least_32_characters")
		server.Close()
		if err != want {
			t.Errorf("status %d = %v, want %v", status, err, want)
		}
	}
}

func TestClientRejectsPlaintextRemotePlatformCore(t *testing.T) {
	if _, err := New("http://platform-core.example", "console-gateway", "secret-with-enough-entropy", "active-key", nil); err == nil {
		t.Fatal("expected plaintext remote Platform Core URL to be rejected")
	}
}
