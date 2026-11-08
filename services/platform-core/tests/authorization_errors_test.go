package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"henukit.dev/platform-core/internal/contract"
)

func TestAuthorizationErrorsSeparateServiceAuthenticationFromInvalidSession(t *testing.T) {
	fixture := newAuthorizationFixture(t, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, test := range []struct{ name, token, secret, wantCode string }{
		{"wrong service secret", fixture.token, "isolated-wrong-service-secret", "CLIENT_AUTH_FAILED"},
		{"unknown exchange Session", "missing_exchange_token_at_least_32_characters", testClientSecret, "SESSION_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"session_exchange_token":%q,"permission_code":"quizcraft.bank.manage","scope":{"kind":"product","product_code":"quizcraft"}}`, test.token)
			request, _ := http.NewRequest(http.MethodPost, fixture.server.URL+"/api/v1/authorization/check", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.SetBasicAuth(testClientID, test.secret)
			request.Header.Set(contract.ServiceIDHeader, testClientID)
			request.Header.Set(contract.KeyIDHeader, testKeyID)
			request.Header.Set(contract.TimestampHeader, fmt.Sprintf("%d", time.Now().Unix()))
			request.Header.Set(contract.NonceHeader, "nonce_auth_error_"+test.wantCode)
			signExchangeRequestWithSecret(t, request, test.secret)
			response, err := fixture.server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var envelope struct {
				Error struct{ Code, Message string }
			}
			if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusUnauthorized || envelope.Error.Code != test.wantCode {
				t.Fatalf("%s = %d/%s; want 401/%s", test.name, response.StatusCode, envelope.Error.Code, test.wantCode)
			}
			if strings.Contains(envelope.Error.Message, test.secret) || strings.Contains(envelope.Error.Message, testKeyID) {
				t.Fatal("public error leaked service credential details")
			}
		})
	}
}
