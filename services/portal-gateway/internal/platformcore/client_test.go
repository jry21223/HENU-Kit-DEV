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

func TestExchangeCodeSendsClientIDAndDecodesEnvelope(t *testing.T) {
	const secret = "portal-client-secret-with-enough-entropy"
	const clientID = "portal-gateway"
	const keyID = "active-key"
	expiresAt := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/oauth/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)

		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("body not json: %v", err)
		}
		if payload["client_id"] != clientID {
			t.Errorf("client_id = %q, want %q", payload["client_id"], clientID)
		}
		if payload["grant_type"] != "authorization_code" || payload["code"] == "" || payload["code_verifier"] == "" {
			t.Errorf("incomplete token body: %#v", payload)
		}

		digest := sha256.Sum256(body)
		canonical := strings.Join([]string{
			r.Method,
			r.URL.RequestURI(),
			r.Header.Get("X-Timestamp"),
			r.Header.Get("X-Nonce"),
			hex.EncodeToString(digest[:]),
		}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

		user, pass, ok := r.BasicAuth()
		if !ok || user != clientID || pass != secret ||
			r.Header.Get("X-Service-Id") != clientID ||
			r.Header.Get("X-Key-Id") != keyID ||
			r.Header.Get("X-Signature") != wantSig ||
			r.Header.Get("Idempotency-Key") == "" {
			t.Errorf("invalid authenticated exchange request")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"user": map[string]string{
					"user_id":      "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
					"display_name": "小河同学",
				},
				"session_exchange_token": "exchange_token_with_at_least_32_characters",
				"expires_at":             expiresAt,
			},
			"request_id": "req_test",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "https://portal.example/callback", clientID, secret, keyID)
	client.httpClient = server.Client()

	result, err := client.ExchangeCode(t.Context(), "authorization-code", strings.Repeat("v", 43), "idem_test_exchange")
	if err != nil {
		t.Fatalf("ExchangeCode error: %v", err)
	}
	if result.UserID != "171f1c6f-7b10-4c92-91a2-b39bf5af5302" {
		t.Fatalf("UserID = %q", result.UserID)
	}
	if result.DisplayName != "小河同学" {
		t.Fatalf("DisplayName = %q, want exchange display name", result.DisplayName)
	}
	if result.SessionExchangeToken != "exchange_token_with_at_least_32_characters" {
		t.Fatalf("SessionExchangeToken = %q", result.SessionExchangeToken)
	}
	if !result.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("ExpiresAt = %v want %v", result.ExpiresAt, expiresAt)
	}
}

func TestExchangeCodeRejectsFlatJSONAndShortToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Flat JSON (pre-fix shape) must not be accepted as a valid envelope.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id":                "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
			"session_exchange_token": "short",
			"expires_at":             time.Now().Add(time.Minute),
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "https://portal.example/callback", "portal-gateway", "portal-client-secret-with-enough-entropy", "active-key")
	client.httpClient = server.Client()
	if _, err := client.ExchangeCode(t.Context(), "code", strings.Repeat("v", 43), "idem_bad"); err == nil {
		t.Fatal("expected invalid exchange response error")
	}
}

func TestCheckPermissionSendsProductScopeDerivedFromPermissionCode(t *testing.T) {
	var got struct {
		PermissionCode       string `json:"permission_code"`
		SessionExchangeToken string `json:"session_exchange_token"`
		Scope                struct {
			Kind        string `json:"kind"`
			ProductCode string `json:"product_code"`
		} `json:"scope"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode check body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "https://portal.example/callback", "portal-gateway", "portal-client-secret-with-enough-entropy", "active-key")
	client.httpClient = server.Client()
	if err := client.CheckPermission(t.Context(), "exchange_token_with_at_least_32_characters", "portal.notice.read"); err != nil {
		t.Fatalf("CheckPermission: %v", err)
	}
	if got.Scope.Kind != "product" || got.Scope.ProductCode != "portal" {
		t.Fatalf("scope = %#v, want product scope for portal", got.Scope)
	}
	if got.PermissionCode != "portal.notice.read" || got.SessionExchangeToken != "exchange_token_with_at_least_32_characters" {
		t.Fatalf("check body = %#v", got)
	}
}

func TestCheckPermissionPlatformScopeForPlatformCodes(t *testing.T) {
	var scope struct {
		Kind        string `json:"kind"`
		ProductCode string `json:"product_code"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Scope json.RawMessage `json:"scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode check body: %v", err)
		}
		if err := json.Unmarshal(body.Scope, &scope); err != nil {
			t.Fatalf("decode scope: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "https://portal.example/callback", "portal-gateway", "portal-client-secret-with-enough-entropy", "active-key")
	client.httpClient = server.Client()
	if err := client.CheckPermission(t.Context(), "exchange_token_with_at_least_32_characters", "platform.operations.read"); err != nil {
		t.Fatalf("CheckPermission: %v", err)
	}
	if scope.Kind != "platform" || scope.ProductCode != "" {
		t.Fatalf("scope = %#v, want platform scope", scope)
	}
}

func TestCheckPermissionStatusMapping(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusUnauthorized: ErrUnauthorized,
		http.StatusForbidden:    ErrForbidden,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		client := NewClient(server.URL, "https://portal.example/callback", "portal-gateway", "portal-client-secret-with-enough-entropy", "active-key")
		client.httpClient = server.Client()
		err := client.CheckPermission(t.Context(), "exchange_token_with_at_least_32_characters", "portal.read")
		server.Close()
		if err != want {
			t.Errorf("status %d = %v, want %v", status, err, want)
		}
	}
}
