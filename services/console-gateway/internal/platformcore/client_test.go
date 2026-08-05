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
		// Mirror contract.ExchangeAuthorizationCodeResponse exactly: Platform
		// Core serialises the subject as `user_id`. Asserting a hand-made `id`
		// here is what let the real decoder ship reading the wrong field.
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"user":                   map[string]string{"user_id": "171f1c6f-7b10-4c92-91a2-b39bf5af5302"},
			"session_exchange_token": "exchange_token_with_at_least_32_characters", "expires_at": time.Now().Add(5 * time.Minute),
		}})
	}))
	defer server.Close()
	client, err := New(server.URL, "console-gateway", secret, "active-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := client.ExchangeCode(t.Context(), "authorization-code", "https://console.example/callback", strings.Repeat("v", 43), "idem_test_exchange")
	if err != nil || exchange.ExchangeToken == "" {
		t.Fatalf("exchange = %+v, err=%v", exchange, err)
	}
	if exchange.UserID != "171f1c6f-7b10-4c92-91a2-b39bf5af5302" {
		t.Fatalf("exchange.UserID = %q, want the subject Platform Core returned", exchange.UserID)
	}
}

func TestAuthorizationStatusMapping(t *testing.T) {
	for status, want := range map[int]error{http.StatusUnauthorized: ErrUnauthorized, http.StatusForbidden: ErrForbidden, http.StatusConflict: ErrConflict} {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(status) }))
		client, err := New(server.URL, "console-gateway", "secret-with-enough-entropy-at-least-32", "active-key", server.Client())
		if err != nil {
			t.Fatal(err)
		}
		err = client.CheckOverview(t.Context(), "exchange_token_with_at_least_32_characters")
		server.Close()
		if err != want {
			t.Errorf("status %d = %v, want %v", status, err, want)
		}
	}
}

func TestCheckAccountUsesOnlyAllowlistedPermissionAndAccountPortfolioProductScope(t *testing.T) {
	const secret = "test-console-client-secret-with-entropy"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			PermissionCode string `json:"permission_code"`
			Scope          struct {
				Kind        string `json:"kind"`
				ProductCode string `json:"product_code"`
			} `json:"scope"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if (body.PermissionCode != "account.tickets.reply" && body.PermissionCode != "account.membership.write" && body.PermissionCode != "account.points.adjust") || body.Scope.Kind != "product" || body.Scope.ProductCode != "account-portfolio" {
			t.Fatalf("Account authorization body = %+v", body)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := New(server.URL, "console-gateway", secret, "active-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range []string{"account.tickets.reply", "account.membership.write", "account.points.adjust"} {
		if err := client.CheckAccount(t.Context(), "exchange_token_with_at_least_32_characters", permission); err != nil {
			t.Fatalf("CheckAccount(%q) error = %v", permission, err)
		}
	}
	if err := client.CheckAccount(t.Context(), "exchange_token_with_at_least_32_characters", "account.points.write"); err != ErrInvalid {
		t.Fatalf("CheckAccount() unsupported permission = %v, want ErrInvalid", err)
	}
}

func TestPlatformOperationsForwardSessionOnlyInHeaderAndSignExactPath(t *testing.T) {
	const secret = "test-console-client-secret-with-entropy"
	const sessionToken = "exchange_token_with_at_least_32_characters"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		digest := sha256.Sum256(body)
		canonical := strings.Join([]string{request.Method, request.URL.Path, request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		if request.Header.Get("X-Session-Exchange-Token") != sessionToken || request.Header.Get("Idempotency-Key") != "idem_platform_write" || request.Header.Get("X-Signature") != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
			t.Errorf("operation headers/signature are invalid")
		}
		if strings.Contains(request.URL.String(), sessionToken) {
			t.Errorf("session token leaked into URL")
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"operation": "session_revoke", "status": "succeeded"}})
	}))
	defer server.Close()
	client, _ := New(server.URL, "console-gateway", secret, "active-key", server.Client())
	result, err := client.RevokeSession(t.Context(), sessionToken, "171f1c6f-7b10-4c92-91a2-b39bf5af5302", "idem_platform_write", []byte(`{"expected_active":true}`))
	if err != nil || !strings.Contains(string(result), `"succeeded"`) {
		t.Fatalf("result=%s err=%v", result, err)
	}
}

func TestClientRejectsPlaintextRemotePlatformCore(t *testing.T) {
	if _, err := New("http://platform-core.example", "console-gateway", "secret-with-enough-entropy", "active-key", nil); err == nil {
		t.Fatal("expected plaintext remote Platform Core URL to be rejected")
	}
}

func TestAccountLookupForwardsEmailOnlyInBodyAndNeverInURL(t *testing.T) {
	const secret = "test-console-client-secret-with-entropy"
	const sessionToken = "exchange_token_with_at_least_32_characters"
	const email = "student@stu.henu.edu.cn"
	var forwarded string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/platform-operations/account-lookups" {
			t.Errorf("account lookup request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-Session-Exchange-Token") != sessionToken {
			t.Errorf("account lookup session token header is missing")
		}
		body, _ := io.ReadAll(request.Body)
		forwarded = string(body)
		digest := sha256.Sum256(body)
		canonical := strings.Join([]string{request.Method, request.URL.Path, request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		if request.Header.Get("X-Signature") != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
			t.Errorf("account lookup signature is invalid")
		}
		if strings.Contains(request.URL.String(), email) {
			t.Errorf("email leaked into URL: %s", request.URL)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"account": map[string]any{"id": "171f1c6f-7b10-4c92-91a2-b39bf5af5302", "display_name": "张同学", "status": "active"},
		}})
	}))
	defer server.Close()
	client, err := New(server.URL, "console-gateway", secret, "active-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	data, err := client.AccountLookup(t.Context(), sessionToken, []byte(`{"email":"`+email+`"}`))
	if err != nil || !strings.Contains(string(data), "张同学") {
		t.Fatalf("lookup data=%s err=%v", data, err)
	}
	if strings.Contains(string(data), email) {
		t.Errorf("Platform Core echoed the email back in the lookup response: %s", data)
	}
	var sent struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal([]byte(forwarded), &sent); err != nil || sent.Email != email {
		t.Errorf("forwarded body=%q, want email %q", forwarded, email)
	}
}

func TestAccountLookupMapsPlatformCoreErrorsWithoutEchoingTheEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client, err := New(server.URL, "console-gateway", "secret-with-enough-entropy-at-least-32", "active-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.AccountLookup(t.Context(), "exchange_token_with_at_least_32_characters", []byte(`{"email":"absent@stu.henu.edu.cn"}`)); err == nil || strings.Contains(err.Error(), "absent@") {
		t.Fatalf("lookup error=%v, want a non-echoing transport error", err)
	}
}
