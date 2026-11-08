package accountowner_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	accountowner "henukit.dev/account-portfolio"
	accountclient "henukit.dev/console-gateway/internal/accountportfolio"
	"henukit.dev/console-gateway/internal/contract"
	"henukit.dev/console-gateway/internal/httpapi"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/serviceauth"
	"henukit.dev/console-gateway/internal/session"
)

type emptyOverview struct{}

func (emptyOverview) Fetch(context.Context, string) contract.ConsoleOverview {
	return contract.ConsoleOverview{}
}

// The Account Portfolio owner, Gateway handler/client and PostgreSQL are real.
// The Platform authorization/identity HTTP server is an isolated contract fixture;
// this test does not claim to exercise real Core permission persistence.
func TestGatewayRecoversOriginalCommittedAccountCommandFromRealOwner(t *testing.T) {
	ctx := t.Context()
	databaseURL := os.Getenv("ACCOUNT_GATEWAY_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		container, err := testcontainers.Run(ctx, "postgres:16-alpine",
			testcontainers.WithEnv(map[string]string{"POSTGRES_DB": "account_ticket_test", "POSTGRES_USER": "account_ticket", "POSTGRES_PASSWORD": "isolated_test_only"}),
			testcontainers.WithExposedPorts("5432/tcp"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute)))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })
		host, err := container.Host(ctx)
		if err != nil {
			t.Fatal(err)
		}
		port, err := container.MappedPort(ctx, "5432/tcp")
		if err != nil {
			t.Fatal(err)
		}
		databaseURL = fmt.Sprintf("postgres://account_ticket:isolated_test_only@%s:%s/account_ticket_test?sslmode=disable", host, port.Port())
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := accountowner.ApplyMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	const portalSecret = "isolated-portal-secret-at-least-32-bytes"
	const consoleSecret = "isolated-console-secret-at-least-32-bytes"
	ownerHandler, err := accountowner.New(accountowner.Config{Database: pool, ClientID: "portal-gateway", Keys: map[string]string{"portal-key": portalSecret}, ConsoleClientID: "console-gateway", ConsoleKeys: map[string]string{"console-key": consoleSecret}, PointCursorKey: []byte(strings.Repeat("p", 32))})
	if err != nil {
		t.Fatal(err)
	}
	owner := httptest.NewServer(ownerHandler)
	t.Cleanup(owner.Close)
	ownerClient, err := accountclient.New(owner.URL, "console-gateway", consoleSecret, "console-key", owner.Client())
	if err != nil {
		t.Fatal(err)
	}
	var authMu sync.Mutex
	permissionRevoked := false
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Token      string `json:"session_exchange_token"`
			Permission string `json:"permission_code"`
			Scope      struct {
				Kind    string `json:"kind"`
				Product string `json:"product_code"`
			} `json:"scope"`
			UserIDs []string `json:"user_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid fixture request", 400)
			return
		}
		authMu.Lock()
		revoked := permissionRevoked
		authMu.Unlock()
		if revoked && input.Permission == "account.tickets.reply" {
			http.Error(w, "denied", 403)
			return
		}
		switch r.URL.Path {
		case "/api/v1/authorization/check":
			if input.Token == "wrong-product-scope-exchange-token-at-least-32bytes" || input.Scope.Kind != "product" || input.Scope.Product != "account-portfolio" || (input.Permission != "account.tickets.reply" && input.Permission != "account.tickets.transition" && input.Permission != "account.tickets.read") {
				http.Error(w, "denied", 403)
				return
			}
			w.WriteHeader(200)
		case "/api/v1/console-user-identities/resolutions":
			identities := make([]map[string]string, 0, len(input.UserIDs))
			for _, id := range input.UserIDs {
				identities = append(identities, map[string]string{"id": id, "display_name": "隔离测试用户", "email": "fixture@example.edu"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"identities": identities}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(core.Close)
	coreClient, err := platformcore.New(core.URL, "console-gateway", "isolated-core-secret-at-least-32bytes", "core-key", core.Client())
	if err != nil {
		t.Fatal(err)
	}
	codec, err := session.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = redisClient.Close() })
	gatewayHandler, err := httpapi.New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", coreClient, nil, emptyOverview{}, redisClient, codec, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, ownerClient)
	if err != nil {
		t.Fatal(err)
	}
	var dropReplyResponse atomic.Bool
	dropReplyResponse.Store(true)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/replies") && r.Header.Get("Idempotency-Key") == "idem_gateway_lost_reply" && dropReplyResponse.CompareAndSwap(true, false) {
			result := httptest.NewRecorder()
			gatewayHandler.ServeHTTP(result, r)
			if result.Code != http.StatusOK {
				w.WriteHeader(result.Code)
				_, _ = w.Write(result.Body.Bytes())
				return
			}
			// The real owner and Gateway have completed, but no HTTP response is
			// delivered to the caller. Recovery must use the original read key.
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("drop committed fixture response: %v", err)
				return
			}
			_ = connection.Close()
			return
		}
		gatewayHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(gateway.Close)
	operator, user := uuid.NewString(), uuid.NewString()
	cookie := func(actor, token string) *http.Cookie {
		value, err := codec.Encode(session.Value{UserID: actor, ExchangeToken: token, ExpiresAt: time.Now().Add(time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: "__Host-henukit_console_session", Value: value}
	}
	operatorCookie := cookie(operator, "isolated-authorized-exchange-token-at-least-32bytes")
	portalSigner, _ := serviceauth.New("portal-gateway", portalSecret, "portal-key")
	portal := func(method, path, key, body string) *http.Response {
		r, _ := http.NewRequest(method, owner.URL+path, strings.NewReader(body))
		r.Header.Set("X-Request-Id", "req_account_owner_integration")
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		if err := portalSigner.SignWithActor(r, []byte(body), user); err != nil {
			t.Fatal(err)
		}
		result, err := owner.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	call := func(method, path, key, body string, actorCookie *http.Cookie) *http.Response {
		r, _ := http.NewRequest(method, gateway.URL+path, strings.NewReader(body))
		r.AddCookie(actorCookie)
		r.Header.Set("X-Request-Id", "req_account_gateway_integration")
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		result, err := gateway.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	read := func(response *http.Response, expected int) json.RawMessage {
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		if response.StatusCode != expected {
			t.Fatalf("HTTP %d, want %d: %s", response.StatusCode, expected, raw)
		}
		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return result.Data
	}
	create := func() string {
		raw := read(portal(http.MethodPost, "/api/v1/account/tickets", "idem_create_"+uuid.NewString(), `{"title":"Isolated recovery","category":"account","body":"Help please."}`), 201)
		var result struct {
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		}
		_ = json.Unmarshal(raw, &result)
		return result.Ticket.ID
	}
	ticketID := create()
	const key = "idem_gateway_lost_reply"
	replyPath := "/api/v1/account/tickets/" + ticketID + "/replies"
	replyBody := `{"body":"We are checking.","expected_version":1}`
	lostRequest, _ := http.NewRequest(http.MethodPost, gateway.URL+replyPath, strings.NewReader(replyBody))
	lostRequest.AddCookie(operatorCookie)
	lostRequest.Header.Set("Content-Type", "application/json")
	lostRequest.Header.Set("Idempotency-Key", key)
	lostResponse, lostError := gateway.Client().Do(lostRequest)
	if lostError == nil || lostResponse != nil {
		if lostResponse != nil {
			lostResponse.Body.Close()
		}
		t.Fatal("committed fixture response was not lost")
	}
	read(call(http.MethodPost, "/api/v1/account/tickets/"+ticketID+"/transitions", "idem_gateway_resolve", `{"status":"resolved","expected_version":2}`, operatorCookie), 200)
	lookupPath := "/api/v1/account/operations/ticket_reply/" + key
	recovered := read(call(http.MethodGet, lookupPath, "", "", operatorCookie), 200)
	var outcome struct {
		Status string `json:"status"`
		Result struct {
			Ticket struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			} `json:"ticket"`
		} `json:"result"`
	}
	_ = json.Unmarshal(recovered, &outcome)
	if outcome.Status != "succeeded" || outcome.Result.Ticket.ID != ticketID || outcome.Result.Ticket.Version != 2 || strings.Contains(string(recovered), `"user_id"`) {
		t.Fatalf("original result = %s", recovered)
	}
	read(call(http.MethodPost, replyPath, key, replyBody, operatorCookie), 200)
	for _, item := range []struct {
		path   string
		cookie *http.Cookie
	}{{lookupPath, cookie(uuid.NewString(), "isolated-other-actor-exchange-token-at-least-32bytes")}, {"/api/v1/account/operations/ticket_transition/" + key, operatorCookie}, {"/api/v1/account/operations/ticket_reply/idem_missing_result", operatorCookie}} {
		raw := read(call(http.MethodGet, item.path, "", "", item.cookie), 200)
		if strings.Contains(string(raw), `"result"`) || !strings.Contains(string(raw), `"unknown"`) {
			t.Fatalf("private/unknown lookup = %s", raw)
		}
	}
	authMu.Lock()
	permissionRevoked = true
	authMu.Unlock()
	read(call(http.MethodGet, lookupPath, "", "", operatorCookie), 403)
	authMu.Lock()
	permissionRevoked = false
	authMu.Unlock()
	read(call(http.MethodGet, lookupPath, "", "", cookie(operator, "wrong-product-scope-exchange-token-at-least-32bytes")), 403)
	conversation := read(portal(http.MethodGet, "/api/v1/account/tickets/"+ticketID, "", ""), 200)
	var details struct {
		Messages []json.RawMessage `json:"messages"`
		Events   []json.RawMessage `json:"events"`
		Ticket   struct {
			Version int `json:"version"`
		} `json:"ticket"`
	}
	_ = json.Unmarshal(conversation, &details)
	if len(details.Messages) != 2 || len(details.Events) != 2 || details.Ticket.Version != 3 {
		t.Fatalf("durable conversation = %s", conversation)
	}
	notifications := read(portal(http.MethodGet, "/api/v1/account/notifications", "", ""), 200)
	var inbox struct {
		Notifications []json.RawMessage `json:"notifications"`
	}
	_ = json.Unmarshal(notifications, &inbox)
	if len(inbox.Notifications) != 2 {
		t.Fatalf("notifications = %s", notifications)
	}
	second := create()
	start := make(chan struct{})
	statuses := make(chan int, 2)
	for i := range 2 {
		go func(i int) {
			<-start
			result := call(http.MethodPost, "/api/v1/account/tickets/"+second+"/transitions", fmt.Sprintf("idem_gateway_concurrent_%d", i), `{"status":"resolved","expected_version":1}`, operatorCookie)
			result.Body.Close()
			statuses <- result.StatusCode
		}(i)
	}
	close(start)
	firstStatus, secondStatus := <-statuses, <-statuses
	if (firstStatus != 200 || secondStatus != 409) && (firstStatus != 409 || secondStatus != 200) {
		t.Fatalf("concurrent revisions = %d/%d", firstStatus, secondStatus)
	}
}
