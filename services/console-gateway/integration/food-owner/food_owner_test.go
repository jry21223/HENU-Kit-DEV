package foodowner_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	foodclient "henukit.dev/console-gateway/internal/food"
	"henukit.dev/console-gateway/internal/overview"
	"henukit.dev/console-gateway/internal/serviceauth"
	foodowner "henukit.dev/food"
)

var databaseURL, redisAddress string

func TestMain(m *testing.M) { os.Exit(runTests(m)) }

func runTests(m *testing.M) (code int) {
	ctx := context.Background()
	postgres, err := testcontainers.Run(ctx, "postgres:16-alpine",
		testcontainers.WithEnv(map[string]string{"POSTGRES_DB": "food_contract_test", "POSTGRES_USER": "food", "POSTGRES_PASSWORD": "food"}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(postgres); err != nil {
			fmt.Fprintln(os.Stderr, "Food contract PostgreSQL cleanup:", err)
			code = 1
		} else {
			fmt.Println("Food contract isolated PostgreSQL terminated")
		}
	}()
	cache, err := testcontainers.Run(ctx, "redis:7-alpine", testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("Ready to accept connections").WithStartupTimeout(60*time.Second)))
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(cache); err != nil {
			fmt.Fprintln(os.Stderr, "Food contract Redis cleanup:", err)
			code = 1
		} else {
			fmt.Println("Food contract isolated Redis terminated")
		}
	}()
	host, err := postgres.Host(ctx)
	if err != nil {
		panic(err)
	}
	port, err := postgres.MappedPort(ctx, "5432/tcp")
	if err != nil {
		panic(err)
	}
	databaseURL = fmt.Sprintf("postgres://food:food@%s:%s/food_contract_test?sslmode=disable", host, port.Port())
	redisHost, err := cache.Host(ctx)
	if err != nil {
		panic(err)
	}
	redisPort, err := cache.MappedPort(ctx, "6379/tcp")
	if err != nil {
		panic(err)
	}
	redisAddress = redisHost + ":" + redisPort.Port()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	migrations, err := filepath.Glob("../../../food/db/migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		panic("Food migrations unavailable")
	}
	sort.Strings(migrations)
	for _, path := range migrations {
		sql, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			panic(fmt.Errorf("apply %s: %w", path, err))
		}
	}
	return m.Run()
}

func TestFoodOutcomeOptInKeepsRawReadsAndSeparatesStoredFailureFromQueryConflicts(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	cache := redis.NewClient(&redis.Options{Addr: redisAddress, DB: 2})
	t.Cleanup(func() { _ = cache.Close() })
	const clientID, keyID, secret = "food-outcome-contract", "test-key", "isolated-food-outcome-contract-secret-32bytes"
	handler, err := foodowner.New(foodowner.Config{Database: pool, Redis: cache, ClientID: clientID, Keys: map[string]string{keyID: secret}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := foodclient.New(server.URL, clientID, secret, keyID, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	const actorID = "171f1c6f-7b10-4c92-91a2-b39bf5af5302"
	const resourceID = "e1111111-1111-4111-8111-111111111111"
	const key = "idem_food_failed_contract_11111111-1111-4111-8111-111111111111"
	if _, err := pool.Exec(t.Context(), `INSERT INTO food_submissions(id,venue_name,item_name,description,status,version) VALUES($1,'受控餐厅','受控菜品','isolated-outcome-fixture','pending',1)`, resourceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM food_submissions WHERE id=$1`, resourceID); err != nil {
			t.Error(err)
		}
	})
	command := []byte(`{"kind":"submission_approve","resource_id":"` + resourceID + `","expected_version":99,"payload":{"note":"隔离过期版本验证"}}`)
	if _, err := client.Command(t.Context(), actorID, key, command); !errors.Is(err, foodclient.ErrConflict) {
		t.Fatalf("stale command error=%v, want conflict", err)
	}
	const path = "/api/v1/operations/submission_approve"
	read := func(actor, permission, route, key string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+route, nil)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := serviceauth.New(clientID, secret, keyID)
		if err != nil {
			t.Fatal(err)
		}
		if err := signer.Sign(request, nil); err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Actor-User-Id", actor)
		request.Header.Set("X-Permission-Code", permission)
		request.Header.Set("X-Scope-Kind", "product")
		request.Header.Set("X-Product-Code", "food")
		request.Header.Set("X-Request-Id", "req_food_outcome_contract")
		request.Header.Set("Idempotency-Key", key)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, body
	}
	legacyStatus, legacyBody := read(actorID, "food.review", path, key)
	const legacyExpected = "{\"error\":{\"code\":\"FOOD_CONFLICT\",\"message\":\"stored Food operation failed\"},\"request_id\":\"req_food_outcome_contract\"}\n"
	if legacyStatus != http.StatusConflict || string(legacyBody) != legacyExpected {
		t.Fatalf("legacy raw result changed: %d %s", legacyStatus, legacyBody)
	}
	status, body := read(actorID, "food.review", path+"?outcome=v1", key)
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || envelope.Data["state"] != "failed" || envelope.Data["operation"] != "submission_approve" || envelope.Data["resource_id"] != resourceID || envelope.Data["error_code"] != "FOOD_CONFLICT" {
		t.Fatalf("persisted failed outcome unavailable: %d %s", status, body)
	}
	viaGateway, err := client.Operation(foodclient.WithOperationOutcome(t.Context()), actorID, "submission_approve", key)
	if err != nil {
		t.Fatal(err)
	}
	var gatewayOutcome map[string]any
	if err := json.Unmarshal(viaGateway, &gatewayOutcome); err != nil {
		t.Fatal(err)
	}
	if gatewayOutcome["state"] != "failed" || gatewayOutcome["resource_id"] != resourceID {
		t.Fatalf("Gateway opt-in did not preserve stored owner facts: %s", viaGateway)
	}
	status, body = read(actorID, "food.review", "/api/v1/operations/post_edit?outcome=v1", key)
	if status != http.StatusConflict {
		t.Fatalf("query operation conflict became a failed outcome: %d %s", status, body)
	}
	status, body = read(actorID, "food.anomaly", path+"?outcome=v1", key)
	if status != http.StatusForbidden {
		t.Fatalf("query permission denied became a failed outcome: %d %s", status, body)
	}
	status, body = read("271f1c6f-7b10-4c92-91a2-b39bf5af5302", "food.review", path+"?outcome=v1", key)
	envelope.Data = nil
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || envelope.Data["state"] != "unknown" || envelope.Data["resource_id"] != nil {
		t.Fatalf("another actor read a stored outcome: %d %s", status, body)
	}
	afterStatus, afterBody := read(actorID, "food.review", path, key)
	if afterStatus != legacyStatus || string(afterBody) != string(legacyBody) {
		t.Fatalf("opt-in changed legacy raw result: %d %s", afterStatus, afterBody)
	}
}

func TestRealFoodSummaryIsAcceptedByGatewayWithGlobalCountsAndHonestStaleFallback(t *testing.T) {
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ownerCache := redis.NewClient(&redis.Options{Addr: redisAddress})
	gatewayCache := redis.NewClient(&redis.Options{Addr: redisAddress, DB: 1})
	t.Cleanup(func() { _ = ownerCache.Close(); _ = gatewayCache.Close() })
	const clientID, keyID, secret = "food-summary-contract", "test-key", "isolated-food-summary-contract-secret-32bytes"
	handler, err := foodowner.New(foodowner.Config{Database: pool, Redis: ownerCache, ClientID: clientID, Keys: map[string]string{keyID: secret}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	aggregator, err := overview.New(map[string]string{"food": server.URL + "/api/v1/console-summary"}, server.Client(), gatewayCache,
		map[string]overview.Credentials{"food": {ClientID: clientID, ClientSecret: secret, KeyID: keyID}}, overview.Options{RetryDelay: func(int) time.Duration { return 0 }})
	if err != nil {
		t.Fatal(err)
	}
	empty := aggregator.Fetch(ctx, "req_food_owner_empty").Modules[5]
	if empty.ID != "food" || empty.Status != "empty" || empty.AsOf == nil {
		t.Fatalf("real empty Food summary rejected by Gateway: %+v", empty)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO food_submissions(id,venue_name,item_name,description,campus,status,version,submitted_at)
		SELECT md5('summary-'||n)::uuid,'受控餐厅','测试菜品','isolated-contract-fixture','minglun','pending',1,now() FROM generate_series(1,205) n`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM food_submissions WHERE description='isolated-contract-fixture'`); err != nil {
			t.Error(err)
		}
	})
	live := aggregator.Fetch(ctx, "req_food_owner_live").Modules[5]
	if live.Status != "ok" || live.AsOf == nil || len(live.Metrics) != 3 || live.Metrics[0].Label != "待审核投稿" || live.Metrics[0].Value != "205" {
		t.Fatalf("owner global pending count was not accepted intact: %+v", live)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE food_submissions RENAME TO unavailable_food_submissions`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `ALTER TABLE unavailable_food_submissions RENAME TO food_submissions`)
	})
	stale := aggregator.Fetch(ctx, "req_food_owner_stale").Modules[5]
	if stale.Status != "stale" || stale.AsOf == nil || !stale.AsOf.Equal(*live.AsOf) || stale.RequestID != "req_food_owner_stale_food" {
		t.Fatalf("Gateway did not retain source time and current trace on cached fallback: %+v", stale)
	}
	if ttl, err := gatewayCache.TTL(ctx, "console:overview:food").Result(); err != nil || ttl <= 0 || ttl > 5*time.Minute {
		t.Fatalf("Gateway cache TTL=%v error=%v", ttl, err)
	}
	if ttl, err := ownerCache.TTL(ctx, "food:workspace:last-success").Result(); err != nil || ttl <= 0 || ttl > 24*time.Hour {
		t.Fatalf("Food workspace cache TTL=%v error=%v", ttl, err)
	}
	if err := gatewayCache.Del(ctx, "console:overview:food").Err(); err != nil {
		t.Fatal(err)
	}
	withoutGatewayCache := aggregator.Fetch(ctx, "req_food_owner_no_gateway_cache").Modules[5]
	if withoutGatewayCache.Status != "unavailable" || withoutGatewayCache.AsOf != nil {
		t.Fatalf("old owner workspace was accepted as fresh summary success: %+v", withoutGatewayCache)
	}
}
