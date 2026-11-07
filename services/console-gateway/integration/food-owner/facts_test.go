package foodowner_test

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"henukit.dev/console-gateway/internal/contract"
	foodclient "henukit.dev/console-gateway/internal/food"
	"henukit.dev/console-gateway/internal/httpapi"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/serviceauth"
	"henukit.dev/console-gateway/internal/session"
	foodowner "henukit.dev/food"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type factsOverview struct{}

func (factsOverview) Fetch(context.Context, string) contract.ConsoleOverview {
	return contract.ConsoleOverview{}
}

// Core authorization is an explicit HTTP fixture; Food/Gateway/PG/Redis are
// actual isolated implementations. No summary URL or paused flag is configured.
func TestGatewayFoodFactsWithRealOwnerCountsBeyondWindow(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, sql := range []string{
		`INSERT INTO food_submissions(id,venue_name,item_name,description,status,version) SELECT md5('gateway-facts-sub-'||n)::uuid,'受控地点','受控菜品','gateway-facts-fixture',CASE WHEN n<=205 THEN 'pending' ELSE 'approved' END,1 FROM generate_series(1,207) n`,
		`INSERT INTO food_anomaly_tickets(id,venue_name,kind,details,severity,status,version) SELECT md5('gateway-facts-anomaly-'||n)::uuid,'受控地点','quality','gateway-facts-fixture','low',CASE WHEN n<=203 THEN 'open' ELSE 'resolved' END,1 FROM generate_series(1,205) n`,
		`INSERT INTO food_tier_adjustments(id,venue_name,current_tier,proposed_tier,reason,status,version) SELECT md5('gateway-facts-tier-'||n)::uuid,'受控地点','standard','recommended','gateway-facts-fixture',CASE WHEN n<=202 THEN 'pending' ELSE 'confirmed' END,1 FROM generate_series(1,204) n`} {
		if _, err := pool.Exec(t.Context(), sql); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, sql := range []string{`DELETE FROM food_submissions WHERE description='gateway-facts-fixture'`, `DELETE FROM food_anomaly_tickets WHERE details='gateway-facts-fixture'`, `DELETE FROM food_tier_adjustments WHERE reason='gateway-facts-fixture'`} {
			if _, err := pool.Exec(context.Background(), sql); err != nil {
				t.Error(err)
			}
		}
	})
	cache := redis.NewClient(&redis.Options{Addr: redisAddress, DB: 3})
	t.Cleanup(func() { _ = cache.Close() })
	const secret = "isolated-food-facts-secret-at-least-32bytes"
	ownerHandler, err := foodowner.New(foodowner.Config{Database: pool, Redis: cache, ClientID: "console-gateway-food", Keys: map[string]string{"food-key": secret}})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); ownerHandler.ServeHTTP(w, r) }))
	t.Cleanup(owner.Close)
	var mode atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Token      string `json:"session_exchange_token"`
			Permission string `json:"permission_code"`
			Scope      struct {
				Kind    string `json:"kind"`
				Product string `json:"product_code"`
			} `json:"scope"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || r.URL.Path != "/api/v1/authorization/check" {
			http.Error(w, "invalid", 400)
			return
		}
		if input.Permission != "food.read" || input.Scope.Kind != "product" || input.Scope.Product != "food" {
			t.Error("wrong food permission/scope")
			http.Error(w, "denied", 403)
			return
		}
		switch mode.Load() {
		case 1:
			http.Error(w, "denied", 403)
		case 2:
			w.WriteHeader(401)
			_, _ = io.WriteString(w, `{"error":{"code":"SESSION_INVALID"}}`)
		case 3:
			w.WriteHeader(401)
			_, _ = io.WriteString(w, `{"error":{"code":"SERVICE_AUTHENTICATION_FAILED"}}`)
		case 4:
			<-r.Context().Done()
		default:
			if input.Token == "wrong-product-scope-exchange-token-at-least-32bytes" {
				http.Error(w, "denied", 403)
				return
			}
			w.WriteHeader(200)
		}
	}))
	t.Cleanup(core.Close)
	coreClient, err := platformcore.New(core.URL, "console-gateway", "isolated-core-facts-secret-at-least-32bytes", "core-key", core.Client())
	if err != nil {
		t.Fatal(err)
	}
	codec, err := session.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	gateway := func(secret string) *httptest.Server {
		client, err := foodclient.New(owner.URL, "console-gateway-food", secret, "food-key", owner.Client())
		if err != nil {
			t.Fatal(err)
		}
		handler, err := httpapi.New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", coreClient, nil, factsOverview{}, cache, codec, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, client)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		return server
	}
	server := gateway(secret)
	badOwner := gateway("wrong-isolated-food-secret-at-least-32bytes")
	request := func(server *httptest.Server, path, token string) (*http.Response, []byte) {
		r, _ := http.NewRequest("GET", server.URL+path, nil)
		if token != "" {
			encoded, err := codec.Encode(session.Value{UserID: "99999999-9999-4999-8999-999999999999", ExchangeToken: token, ExpiresAt: time.Now().Add(time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			r.AddCookie(&http.Cookie{Name: "__Host-henukit_console_session", Value: encoded})
		}
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return response, body
	}
	const route = "/api/v1/food/facts"
	const token = "isolated-authorized-exchange-token-at-least-32bytes"
	response, body := request(server, route, token)
	if response.StatusCode != 200 {
		t.Fatalf("facts=%d %s", response.StatusCode, body)
	}
	var envelope struct {
		Data struct {
			Counts  map[string]int64
			Windows map[string]struct {
				Returned int
				HasMore  bool `json:"has_more"`
				Items    []struct{ ID, Status string }
			}
		}
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int64{"pending_submissions": 205, "open_anomalies": 203, "pending_tier_adjustments": 202} {
		if envelope.Data.Counts[name] != want {
			t.Fatalf("count %s=%d want%d", name, envelope.Data.Counts[name], want)
		}
	}
	for name, status := range map[string]string{"submissions": "pending", "anomalies": "open", "tier_adjustments": "pending"} {
		w := envelope.Data.Windows[name]
		if w.Returned != 200 || len(w.Items) != 200 || !w.HasMore {
			t.Fatalf("wrong window %s=%+v", name, w)
		}
		for _, item := range w.Items {
			if item.Status != status {
				t.Fatalf("wrong status %s", item.Status)
			}
		}
	}
	for _, tc := range []struct {
		name             string
		mode             int32
		server           *httptest.Server
		path, token      string
		status           int
		clear, ownerCall bool
	}{
		{"no session", 0, server, route, "", 401, false, false}, {"denied", 1, server, route, token, 403, false, false}, {"wrong product scope", 0, server, route, "wrong-product-scope-exchange-token-at-least-32bytes", 403, false, false}, {"session expired", 2, server, route, token, 401, true, false}, {"core service401", 3, server, route, token, 503, false, false}, {"owner service401", 0, badOwner, route, token, 503, false, true}, {"no campus on global facts", 0, server, route + "?campus=minglun", token, 400, false, false}, {"auth deadline", 4, server, route, token, 503, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode.Store(tc.mode)
			before := calls.Load()
			started := time.Now()
			response, body := request(tc.server, tc.path, tc.token)
			if response.StatusCode != tc.status || strings.Contains(string(body), `"counts"`) {
				t.Fatalf("facts status=%d %s", response.StatusCode, body)
			}
			if (calls.Load() > before) != tc.ownerCall {
				t.Fatal("owner contacted across denied/invalid boundary")
			}
			if (len(response.Cookies()) > 0) != tc.clear {
				t.Fatal("service failure changed browser session")
			}
			if tc.mode == 4 && time.Since(started) > 2800*time.Millisecond {
				t.Fatal("Gateway escaped2s budget")
			}
		})
	}
	// Direct owner HTTP must also terminate an otherwise blocked database read;
	// this call does not rely on the Gateway/client deadline to cancel it.
	mode.Store(0)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(), `LOCK TABLE food_submissions IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	direct, _ := http.NewRequest("GET", owner.URL+"/api/v1/workspace/facts", nil)
	signer, err := serviceauth.New("console-gateway-food", secret, "food-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Sign(direct, nil); err != nil {
		t.Fatal(err)
	}
	direct.Header.Set("X-Actor-User-Id", "99999999-9999-4999-8999-999999999999")
	direct.Header.Set("X-Permission-Code", "food.read")
	direct.Header.Set("X-Scope-Kind", "product")
	direct.Header.Set("X-Product-Code", "food")
	started := time.Now()
	directResponse, err := owner.Client().Do(direct)
	if err != nil {
		t.Fatal(err)
	}
	directBody, err := io.ReadAll(directResponse.Body)
	directResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if directResponse.StatusCode != 503 || strings.Contains(string(directBody), `"counts"`) || time.Since(started) > 2800*time.Millisecond {
		t.Fatalf("owner deadline=%d %s elapsed=%s", directResponse.StatusCode, directBody, time.Since(started))
	}

}
