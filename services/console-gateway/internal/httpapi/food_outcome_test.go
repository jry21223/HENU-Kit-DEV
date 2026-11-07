package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	foodapi "henukit.dev/console-gateway/internal/food"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

func TestFoodOutcomeRepresentationIsExplicitAndKeepsQueryErrorsUnconfirmed(t *testing.T) {
	for _, tc := range []struct {
		name, query   string
		queryConflict bool
		accessErr     error
		want          int
	}{
		{name: "legacy", want: 409},
		{name: "stored failure", query: "?outcome=v1", want: 200},
		{name: "query conflict", query: "?outcome=v1", queryConflict: true, want: 409},
		{name: "expired", query: "?outcome=v1", accessErr: platformcore.ErrUnauthorized, want: 401},
		{name: "denied", query: "?outcome=v1", accessErr: platformcore.ErrForbidden, want: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := miniredis.RunT(t)
			redisClient := redis.NewClient(&redis.Options{Addr: cache.Addr()})
			t.Cleanup(func() { _ = redisClient.Close() })
			type observed struct{ uri, actor, permission, key string }
			requests := make(chan observed, 1)
			ownerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- observed{r.URL.RequestURI(), r.Header.Get("X-Actor-User-Id"), r.Header.Get("X-Permission-Code"), r.Header.Get("Idempotency-Key")}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("outcome") != "v1" || tc.queryConflict {
					w.WriteHeader(http.StatusConflict)
					_, _ = io.WriteString(w, `{"error":{"code":"FOOD_CONFLICT","message":"conflict"}}`)
					return
				}
				_, _ = io.WriteString(w, `{"data":{"operation":"post_edit","resource_id":"11111111-1111-4111-8111-111111111111","state":"failed","error_code":"FOOD_CONFLICT"}}`)
			}))
			t.Cleanup(ownerServer.Close)
			owner, err := foodapi.New(ownerServer.URL, "food-outcome-test", "isolated-food-outcome-test-secret-32bytes", "test-key", ownerServer.Client())
			if err != nil {
				t.Fatal(err)
			}
			codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
			const userID, token = "171f1c6f-7b10-4c92-91a2-b39bf5af5302", "food_outcome_exchange_token_32bytes"
			platform := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: token}, checkErr: tc.accessErr}
			handler, err := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", platform, nil, fakeOverview{}, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)), nil, owner)
			if err != nil {
				t.Fatal(err)
			}
			gateway := httptest.NewTLSServer(handler)
			t.Cleanup(gateway.Close)
			cookie, _ := codec.Encode(session.Value{UserID: userID, ExchangeToken: token, ExpiresAt: time.Now().Add(time.Minute)})
			request, _ := http.NewRequest(http.MethodGet, gateway.URL+"/api/v1/food/operations/post_edit"+tc.query, nil)
			request.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
			request.Header.Set("Idempotency-Key", "idem_food_outcome_query")
			response, err := gateway.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if response.StatusCode != tc.want {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
			if tc.want == 200 && !strings.Contains(string(body), `"state":"failed"`) {
				t.Fatalf("stored outcome lost: %s", body)
			}
			if tc.want != 200 && strings.Contains(string(body), `"state":"failed"`) {
				t.Fatalf("query error became a stored failure: %s", body)
			}
			select {
			case call := <-requests:
				if tc.accessErr != nil || call.uri != "/api/v1/operations/post_edit"+tc.query || call.actor != userID || call.permission != "food.review" || call.key != "idem_food_outcome_query" {
					t.Fatalf("wrong owner request: %+v", call)
				}
			default:
				if tc.accessErr == nil {
					t.Fatal("authorized request never reached owner")
				}
			}
		})
	}
}
