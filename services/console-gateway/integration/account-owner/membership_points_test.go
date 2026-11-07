package accountowner_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	accountowner "henukit.dev/account-portfolio"
	accountclient "henukit.dev/console-gateway/internal/accountportfolio"
	"henukit.dev/console-gateway/internal/httpapi"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/serviceauth"
	"henukit.dev/console-gateway/internal/session"
)

// Real Gateway, signed owner HTTP and PostgreSQL. Core authorization is an
// explicit HTTP contract fixture, not real permission-persistence evidence.
func TestMembershipAndPointsRecoverOriginalFactsThroughGateway(t *testing.T) {
	ctx := t.Context()
	databaseURL := os.Getenv("ACCOUNT_GATEWAY_INTEGRATION_DATABASE_URL")
	if databaseURL == "" {
		container, err := testcontainers.Run(ctx, "postgres:16-alpine",
			testcontainers.WithEnv(map[string]string{"POSTGRES_DB": "account_recovery", "POSTGRES_USER": "account_recovery", "POSTGRES_PASSWORD": "isolated_test_only"}),
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
		databaseURL = fmt.Sprintf("postgres://account_recovery:isolated_test_only@%s:%s/account_recovery?sslmode=disable", host, port.Port())
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
	var mutex sync.Mutex
	denied := false
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Token      string `json:"session_exchange_token"`
			Permission string `json:"permission_code"`
			Scope      struct {
				Kind    string `json:"kind"`
				Product string `json:"product_code"`
			} `json:"scope"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid fixture", 400)
			return
		}
		mutex.Lock()
		forbidden := denied
		mutex.Unlock()
		if r.URL.Path != "/api/v1/authorization/check" || forbidden || input.Token == "wrong-product-exchange-token-at-least-32bytes" || input.Scope.Kind != "product" || input.Scope.Product != "account-portfolio" || (input.Permission != "account.membership.write" && input.Permission != "account.points.adjust") {
			http.Error(w, "denied", 403)
			return
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(core.Close)
	coreClient, err := platformcore.New(core.URL, "console-gateway", "isolated-core-secret-at-least-32bytes", "core-key", core.Client())
	if err != nil {
		t.Fatal(err)
	}
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = redisClient.Close() })
	handler, err := httpapi.New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", coreClient, nil, emptyOverview{}, redisClient, codec, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, ownerClient)
	if err != nil {
		t.Fatal(err)
	}
	drop := map[string]bool{"idem_member_lost": true, "idem_points_lost": true}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		lost := drop[r.Header.Get("Idempotency-Key")]
		delete(drop, r.Header.Get("Idempotency-Key"))
		mutex.Unlock()
		if lost && r.Method == http.MethodPost {
			result := httptest.NewRecorder()
			handler.ServeHTTP(result, r)
			if result.Code != 200 {
				w.WriteHeader(result.Code)
				_, _ = w.Write(result.Body.Bytes())
				return
			}
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("drop completed response: %v", err)
				return
			}
			_ = connection.Close()
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(gateway.Close)
	// Each request uses a fresh connection so net/http cannot automatically
	// retry a replayable POST after a reused-connection failure.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	operator, user, absent := uuid.NewString(), uuid.NewString(), uuid.NewString()
	cookie := func(actor, token string) *http.Cookie {
		value, err := codec.Encode(session.Value{UserID: actor, ExchangeToken: token, ExpiresAt: time.Now().Add(time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: "__Host-henukit_console_session", Value: value}
	}
	authorized := cookie(operator, "isolated-authorized-exchange-token-at-least-32bytes")
	request := func(method, path, key, body string, actor *http.Cookie) (*http.Response, error) {
		r, _ := http.NewRequest(method, gateway.URL+path, strings.NewReader(body))
		r.AddCookie(actor)
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		return client.Do(r)
	}
	read := func(result *http.Response, expected int) json.RawMessage {
		defer result.Body.Close()
		raw, _ := io.ReadAll(result.Body)
		if result.StatusCode != expected {
			t.Fatalf("HTTP %d want %d: %s", result.StatusCode, expected, raw)
		}
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(raw, &envelope) != nil {
			t.Fatalf("invalid JSON: %s", raw)
		}
		for _, key := range []string{`"user_id"`, `"actor_user_id"`, `"operator_user_id"`} {
			if strings.Contains(string(envelope.Data), key) {
				t.Fatalf("private identity in browser result: %s", raw)
			}
		}
		return envelope.Data
	}
	call := func(method, path, key, body string, actor *http.Cookie, status int) json.RawMessage {
		result, err := request(method, path, key, body, actor)
		if err != nil {
			t.Fatal(err)
		}
		return read(result, status)
	}
	portalSigner, _ := serviceauth.New("portal-gateway", portalSecret, "portal-key")
	portal := func(path string) json.RawMessage {
		r, _ := http.NewRequest(http.MethodGet, owner.URL+path, nil)
		if err := portalSigner.SignWithActor(r, nil, user); err != nil {
			t.Fatal(err)
		}
		result, err := owner.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return read(result, 200)
	}
	lookup := func(operation, key string, actor *http.Cookie) json.RawMessage {
		return call(http.MethodGet, "/api/v1/account/operations/"+operation+"/"+key, "", "", actor, 200)
	}
	memberPath := "/api/v1/account/memberships/" + user
	call(http.MethodGet, "/api/v1/account/memberships/"+absent, "", "", authorized, 404)
	portal("/api/v1/account/summary")
	grant := `{"reason":"Isolated grant","expected_version":1}`
	result, err := request(http.MethodPost, memberPath+"/grants", "idem_member_lost", grant, authorized)
	if err == nil || result != nil {
		if result != nil {
			result.Body.Close()
		}
		t.Fatal("member response was not lost")
	}
	firstGrant := lookup("membership_grant", "idem_member_lost", authorized)
	if !strings.Contains(string(firstGrant), `"succeeded"`) || !strings.Contains(string(firstGrant), `"version":2`) {
		t.Fatalf("original grant = %s", firstGrant)
	}
	call(http.MethodPost, memberPath+"/grants", "idem_member_lost", grant, authorized, 200)
	call(http.MethodPost, memberPath+"/grants", "idem_member_lost", `{"reason":"Different payload","expected_version":1}`, authorized, 409)
	call(http.MethodPost, "/api/v1/account/memberships/"+absent+"/grants", "idem_member_lost", grant, authorized, 409)
	call(http.MethodGet, "/api/v1/account/memberships/"+absent, "", "", authorized, 404)
	call(http.MethodPost, memberPath+"/grants", "idem_member_stale", grant, authorized, 409)
	call(http.MethodPost, memberPath+"/revocations", "idem_member_revoke", `{"reason":"Isolated revoke","expected_version":2}`, authorized, 200)
	if !strings.Contains(string(lookup("membership_revoke", "idem_member_revoke", authorized)), `"version":3`) {
		t.Fatal("missing revoke fact")
	}
	if string(lookup("membership_grant", "idem_member_lost", authorized)) != string(firstGrant) {
		t.Fatal("later entitlement replaced original grant result")
	}
	pointPath := "/api/v1/account/points/adjustments"
	credit := `{"user_id":"` + user + `","amount":7,"reason":"Isolated credit"}`
	result, err = request(http.MethodPost, pointPath, "idem_points_lost", credit, authorized)
	if err == nil || result != nil {
		if result != nil {
			result.Body.Close()
		}
		t.Fatal("point response was not lost")
	}
	firstCredit := lookup("point_adjustment", "idem_points_lost", authorized)
	if !strings.Contains(string(firstCredit), `"balance":7`) || !strings.Contains(string(firstCredit), `"amount":7`) {
		t.Fatalf("original credit = %s", firstCredit)
	}
	call(http.MethodPost, pointPath, "idem_points_lost", credit, authorized, 200)
	call(http.MethodPost, pointPath, "idem_points_lost", strings.Replace(credit, `"amount":7`, `"amount":8`, 1), authorized, 409)
	call(http.MethodPost, pointPath, "idem_points_lost", strings.Replace(credit, user, absent, 1), authorized, 409)
	call(http.MethodPost, pointPath, "idem_points_next", `{"user_id":"`+user+`","amount":3,"reason":"Later credit"}`, authorized, 200)
	if string(lookup("point_adjustment", "idem_points_lost", authorized)) != string(firstCredit) {
		t.Fatal("later balance replaced original credit fact")
	}
	for _, operation := range []string{"membership_grant", "point_adjustment"} {
		key := "idem_member_lost"
		if operation == "point_adjustment" {
			key = "idem_points_lost"
		}
		for _, actor := range []*http.Cookie{cookie(uuid.NewString(), "other-actor-exchange-token-at-least-32bytes")} {
			if raw := lookup(operation, key, actor); strings.Contains(string(raw), `"result"`) || !strings.Contains(string(raw), `"unknown"`) {
				t.Fatalf("other actor result = %s", raw)
			}
		}
		if raw := lookup(operation, "idem_missing_result", authorized); !strings.Contains(string(raw), `"unknown"`) {
			t.Fatalf("missing = %s", raw)
		}
		mutex.Lock()
		denied = true
		mutex.Unlock()
		call(http.MethodGet, "/api/v1/account/operations/"+operation+"/"+key, "", "", authorized, 403)
		mutex.Lock()
		denied = false
		mutex.Unlock()
		call(http.MethodGet, "/api/v1/account/operations/"+operation+"/"+key, "", "", cookie(operator, "wrong-product-exchange-token-at-least-32bytes"), 403)
	}
	concurrent := func(path, keyPrefix, body string) {
		start := make(chan struct{})
		statuses := make(chan int, 2)
		for index := range 2 {
			go func(index int) {
				<-start
				result, err := request(http.MethodPost, path, fmt.Sprintf("%s%d", keyPrefix, index), body, authorized)
				if err != nil {
					statuses <- 0
					return
				}
				result.Body.Close()
				statuses <- result.StatusCode
			}(index)
		}
		close(start)
		first, second := <-statuses, <-statuses
		if (first != 200 || second != 409) && (first != 409 || second != 200) {
			t.Fatalf("concurrent statuses = %d/%d", first, second)
		}
		var success, unknown int
		operation := "point_adjustment"
		if strings.HasSuffix(path, "/grants") {
			operation = "membership_grant"
		}
		for index := range 2 {
			raw := lookup(operation, fmt.Sprintf("%s%d", keyPrefix, index), authorized)
			if strings.Contains(string(raw), `"succeeded"`) {
				success++
			} else if strings.Contains(string(raw), `"unknown"`) {
				unknown++
			}
		}
		if success != 1 || unknown != 1 {
			t.Fatalf("concurrent committed/unknown = %d/%d", success, unknown)
		}
	}
	concurrent(memberPath+"/grants", "idem_member_concurrent_", `{"reason":"Concurrent grant","expected_version":3}`)
	concurrent(pointPath, "idem_points_concurrent_", `{"user_id":"`+user+`","amount":-7,"reason":"Concurrent debit"}`)
	var points struct {
		Balance int `json:"balance"`
		Entries []struct {
			Amount int `json:"amount"`
		} `json:"entries"`
	}
	if json.Unmarshal(portal("/api/v1/account/points?limit=10"), &points) != nil || points.Balance != 3 || len(points.Entries) != 3 {
		t.Fatalf("final public ledger = %+v", points)
	}
	var inbox struct {
		Notifications []json.RawMessage `json:"notifications"`
	}
	if json.Unmarshal(portal("/api/v1/account/notifications"), &inbox) != nil || len(inbox.Notifications) != 6 {
		t.Fatalf("notifications=%d, want 6 exact successful facts", len(inbox.Notifications))
	}
}
