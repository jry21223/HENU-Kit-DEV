package accountowner_test

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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	accountowner "henukit.dev/account-portfolio"
	accountclient "henukit.dev/console-gateway/internal/accountportfolio"
	"henukit.dev/console-gateway/internal/httpapi"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

// Actual signed Gateway -> Account owner -> PostgreSQL. Core authorization is
// an isolated HTTP contract fixture, not a claim about live roles or sessions.
func TestGatewayTicketFactsWithRealOwnerAndBoundedGlobalQueue(t *testing.T) {
	ctx := t.Context()
	pg, err := testcontainers.Run(ctx, "postgres:16-alpine", testcontainers.WithEnv(map[string]string{"POSTGRES_DB": "facts", "POSTGRES_USER": "facts", "POSTGRES_PASSWORD": "isolated"}), testcontainers.WithExposedPorts("5432/tcp"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(pg); err != nil {
			t.Error(err)
		}
	})
	host, err := pg.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := pg.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, fmt.Sprintf("postgres://facts:isolated@%s:%s/facts?sslmode=disable", host, port.Port()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := accountowner.ApplyMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO account_portfolio_accounts(user_id) VALUES('11111111-1111-4111-8111-111111111111'); INSERT INTO account_portfolio_tickets(id,user_id,title,category,status) SELECT md5('integration-facts-'||n)::uuid,'11111111-1111-4111-8111-111111111111','受控运营工单','account',CASE WHEN n<=105 THEN 'open' WHEN n<=108 THEN 'in_progress' ELSE 'resolved' END FROM generate_series(1,115) n`); err != nil {
		t.Fatal(err)
	}
	const secret = "isolated-console-facts-secret-at-least-32bytes"
	ownerHandler, err := accountowner.New(accountowner.Config{Database: pool, ClientID: "portal-gateway", Keys: map[string]string{"portal-key": "isolated-portal-facts-secret-at-least-32bytes"}, ConsoleClientID: "console-gateway", ConsoleKeys: map[string]string{"console-key": secret}, PointCursorKey: []byte(strings.Repeat("p", 32))})
	if err != nil {
		t.Fatal(err)
	}
	var ownerCalls atomic.Int32
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ownerCalls.Add(1); ownerHandler.ServeHTTP(w, r) }))
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
		if input.Permission != "account.tickets.read" || input.Scope.Kind != "product" || input.Scope.Product != "account-portfolio" {
			t.Error("wrong facts permission/scope")
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
	cache := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = cache.Close() })
	gateway := func(clientSecret string) *httptest.Server {
		client, err := accountclient.New(owner.URL, "console-gateway", clientSecret, "console-key", owner.Client())
		if err != nil {
			t.Fatal(err)
		}
		handler, err := httpapi.New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", coreClient, nil, emptyOverview{}, cache, codec, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, client)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		return server
	}
	server := gateway(secret)
	badOwnerServer := gateway("wrong-isolated-owner-secret-at-least-32bytes")
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
	const route = "/api/v1/account/tickets/facts?status=unresolved"
	const token = "isolated-authorized-exchange-token-at-least-32bytes"
	response, body := request(server, route, token)
	if response.StatusCode != 200 {
		t.Fatalf("facts=%d %s", response.StatusCode, body)
	}
	var envelope struct {
		Data struct {
			Counts map[string]int64
			Window struct {
				Filter       string
				MatchedTotal int `json:"matched_total"`
				Returned     int
				Items        []struct{ ID, Reference, Title, Status string }
			}
		}
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Counts["unresolved"] != 108 || envelope.Data.Counts["resolved"] != 7 || envelope.Data.Window.MatchedTotal != 108 || envelope.Data.Window.Returned != 100 || len(envelope.Data.Window.Items) != 100 {
		t.Fatalf("truncated total=%s", body)
	}
	for _, item := range envelope.Data.Window.Items {
		if item.Status == "resolved" || item.Reference != "HKT-"+item.ID || item.Title != "受控运营工单" {
			t.Fatalf("wrong queue row=%+v", item)
		}
	}
	for _, tc := range []struct {
		name                   string
		mode                   int32
		server                 *httptest.Server
		path, token            string
		status                 int
		clearCookie, ownerCall bool
	}{
		{"no session", 0, server, route, "", 401, false, false},
		{"denied", 1, server, route, token, 403, false, false},
		{"wrong product scope", 0, server, route, "wrong-product-scope-exchange-token-at-least-32bytes", 403, false, false},
		{"verified expired session", 2, server, route, token, 401, true, false},
		{"Core service401", 3, server, route, token, 503, false, false},
		{"owner service401", 0, badOwnerServer, route, token, 503, false, true},
		{"invalid filter", 0, server, "/api/v1/account/tickets/facts?status=invalid", token, 400, false, false},
		{"duplicate filter", 0, server, "/api/v1/account/tickets/facts?status=open&status=resolved", token, 400, false, false},
		{"authorization deadline", 4, server, route, token, 503, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode.Store(tc.mode)
			before := ownerCalls.Load()
			started := time.Now()
			response, body := request(tc.server, tc.path, tc.token)
			if response.StatusCode != tc.status || strings.Contains(string(body), `"counts"`) {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
			if (ownerCalls.Load() > before) != tc.ownerCall {
				t.Fatal("owner contacted across denied/invalid boundary")
			}
			if (len(response.Cookies()) > 0) != tc.clearCookie {
				t.Fatal("owner/service rejection changed browser session")
			}
			if tc.mode == 4 && time.Since(started) > 2800*time.Millisecond {
				t.Fatal("Gateway authorization escaped2s budget")
			}
		})
	}
}
