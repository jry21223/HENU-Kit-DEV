package career

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func TestConsoleCredentialIndependence(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	t.Cleanup(func() { _ = redisClient.Close() })
	const shared = "shared-secret-value-at-least-32-bytes-long"
	if _, err := New(Config{
		Database:        &pgxpool.Pool{},
		Redis:           redisClient,
		ClientID:        "portal-gateway",
		Keys:            map[string]string{"k1": shared},
		ConsoleClientID: "console-gateway",
		ConsoleKeys:     map[string]string{"ck1": shared},
	}); err == nil {
		t.Fatal("expected overlapping portal/console secret to be rejected")
	}
	if _, err := New(Config{
		Database:        &pgxpool.Pool{},
		Redis:           redisClient,
		ClientID:        "portal-gateway",
		Keys:            map[string]string{"k1": shared},
		ConsoleClientID: "portal-gateway",
		ConsoleKeys:     map[string]string{"ck1": "distinct-console-secret-at-least-32-bytes"},
	}); err == nil {
		t.Fatal("expected identical portal/console client id to be rejected")
	}
}

func TestConsoleWorkspaceAuthAndScopeGuards(t *testing.T) {
	addr := os.Getenv("CAREER_TEST_REDIS_ADDR")
	if addr == "" {
		addr = "redis:6379"
	}
	redisClient := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = redisClient.Close() })
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis unavailable at %s: %v", addr, err)
	}
	const consoleSecret = "career-console-secret-at-least-32-bytes-long"
	svc, err := New(Config{
		Database:        &pgxpool.Pool{},
		Redis:           redisClient,
		ClientID:        "portal-gateway",
		Keys:            map[string]string{"k1": "career-portal-secret-at-least-32-bytes-long"},
		ConsoleClientID: "console-gateway-career",
		ConsoleKeys:     map[string]string{"ck1": consoleSecret},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv := httptest.NewServer(svc)
	defer srv.Close()

	signReq := func(method, path, actorID, scopeKind, product, perm string) *http.Response {
		req, _ := http.NewRequest(method, srv.URL+path, nil)
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
		sum := sha256.Sum256(nil)
		canonical := strings.Join([]string{method, path, ts, nonce, hex.EncodeToString(sum[:]), actorID}, "\n")
		mac := hmac.New(sha256.New, []byte(consoleSecret))
		_, _ = mac.Write([]byte(canonical))
		req.SetBasicAuth("console-gateway-career", consoleSecret)
		req.Header.Set("X-Service-Id", "console-gateway-career")
		req.Header.Set("X-Key-Id", "ck1")
		req.Header.Set("X-Actor-User-Id", actorID)
		req.Header.Set("X-Timestamp", ts)
		req.Header.Set("X-Nonce", nonce)
		req.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
		req.Header.Set("X-Scope-Kind", scopeKind)
		req.Header.Set("X-Product-Code", product)
		req.Header.Set("X-Permission-Code", perm)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	const actorID = "11111111-1111-4111-8111-111111111111"
	// Unauthenticated -> 401
	unauth, err := srv.Client().Get(srv.URL + "/api/v1/console/workspace")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauth.Body.Close()
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d, want 401", unauth.StatusCode)
	}
	// Wrong product scope -> 403
	if resp := signReq(http.MethodGet, "/api/v1/console/workspace", actorID, "product", "food", "career.read"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong scope status = %d, want 403", resp.StatusCode)
	}
	// Wrong permission -> 403
	if resp := signReq(http.MethodGet, "/api/v1/console/workspace", actorID, "product", "career", "food.read"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong perm status = %d, want 403", resp.StatusCode)
	}
	// Retry digest missing Idempotency-Key -> 400
	if resp := signReq(http.MethodPost, "/api/v1/console/searches/"+actorID+"/retry-digest", actorID, "product", "career", "career.manage"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing idempotency status = %d, want 400", resp.StatusCode)
	}
}
