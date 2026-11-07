package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	accountapi "henukit.dev/console-gateway/internal/accountportfolio"
	foodapi "henukit.dev/console-gateway/internal/food"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

// The handler and signed clients use actual HTTP. Core authorization and owner
// rejection bodies are isolated contract fixtures; the real Account owner role
// rejection is covered by the retained v1 PG/HTTP evidence, without another PG.
func TestFactsOwner403IsDependencyFailureAfterCoreAllows(t *testing.T) {
	for _, tc := range []struct {
		product, code, clientID string
		coreDeny                bool
	}{
		{"food", "", "console-gateway-food", false},
		{"food", "SCOPE_DENIED", "console-gateway-food", false},
		{"account", "", "console-gateway", false},
		{"account", "SCOPE_DENIED", "console-gateway", false},
		{"account", "ACCESS_DENIED", "portal-gateway", false},
		{"food", "", "console-gateway-food", true},
		{"account", "", "console-gateway", true},
	} {
		t.Run(tc.product+"/"+tc.clientID+"/"+tc.code+"/deny="+fmt.Sprint(tc.coreDeny), func(t *testing.T) {
			var calls atomic.Int32
			owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				client, _, ok := r.BasicAuth()
				if !ok || client != tc.clientID || r.Header.Get("X-Signature") == "" || r.Header.Get("X-Actor-User-Id") != "11111111-1111-4111-8111-111111111111" {
					t.Error("wrong signed owner identity")
				}
				w.WriteHeader(403)
				if tc.code == "" {
					_, _ = io.WriteString(w, "forbidden")
				} else {
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": tc.code}})
				}
			}))
			defer owner.Close()
			product, permission := "food", "food.read"
			route := "/api/v1/food/facts"
			if tc.product == "account" {
				product = "account-portfolio"
				permission = "account.tickets.read"
				route = "/api/v1/account/tickets/facts?status=unresolved"
			}
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Permission string `json:"permission_code"`
					Scope      struct {
						Kind    string `json:"kind"`
						Product string `json:"product_code"`
					} `json:"scope"`
				}
				if json.NewDecoder(r.Body).Decode(&input) != nil || input.Permission != permission || input.Scope.Kind != "product" || input.Scope.Product != product {
					t.Error("wrong Core authorization boundary")
					http.Error(w, "invalid", 400)
					return
				}
				if tc.coreDeny {
					http.Error(w, "denied", 403)
					return
				}
				w.WriteHeader(200)
			}))
			defer core.Close()
			platform, err := platformcore.New(core.URL, "console-gateway", "isolated-core-facts-secret-at-least-32bytes", "core-key", core.Client())
			if err != nil {
				t.Fatal(err)
			}
			food, err := foodapi.New(owner.URL, tc.clientID, "isolated-owner-facts-secret-at-least-32bytes", "owner-key", owner.Client())
			if err != nil {
				t.Fatal(err)
			}
			account, err := accountapi.New(owner.URL, tc.clientID, "isolated-owner-facts-secret-at-least-32bytes", "owner-key", owner.Client())
			if err != nil {
				t.Fatal(err)
			}
			cache := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
			defer cache.Close()
			codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
			handler, err := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", platform, nil, fakeOverview{}, cache, codec, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, food, account)
			if err != nil {
				t.Fatal(err)
			}
			gateway := httptest.NewServer(handler)
			defer gateway.Close()
			encoded, _ := codec.Encode(session.Value{UserID: "11111111-1111-4111-8111-111111111111", ExchangeToken: "isolated-exchange-token-at-least-32bytes", ExpiresAt: time.Now().Add(time.Minute)})
			request, _ := http.NewRequest("GET", gateway.URL+route, nil)
			request.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
			response, err := gateway.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			expected := 503
			expectedCalls := int32(1)
			if tc.coreDeny {
				expected = 403
				expectedCalls = 0
			}
			if response.StatusCode != expected || calls.Load() != expectedCalls || len(response.Cookies()) > 0 || strings.Contains(string(body), `"data"`) {
				t.Fatalf("status=%d calls=%d cookies=%v body=%s", response.StatusCode, calls.Load(), response.Cookies(), body)
			}
		})
	}
}
