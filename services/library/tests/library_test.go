package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
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

	library "henukit.dev/library"
)

const serviceSecret = "library-gateway-secret-at-least-32-bytes"

func TestBoundedWorkspaceDegradesWithoutLeakingLegacyDomains(t *testing.T) {
	server, pool := newLibraryServer(t)
	defer server.Close()
	defer pool.Close()

	response := send(t, server.URL, "library.read", http.MethodGet, "/api/v1/workspace", nil, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("workspace status = %d", response.StatusCode)
	}
	body := readBody(t, response)
	for _, expected := range []string{`"courses"`, `"materials"`, `"downloads"`, `"submissions"`, `"corrections"`, `"status":"partial"`, `"degraded":true`} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("workspace omitted %s: %s", expected, body)
		}
	}
	for _, forbidden := range []string{"payment", "membership", "member_only", "paid", "points", "forum", "quiz", "userAgent", "127.0.0.1"} {
		if bytes.Contains(bytes.ToLower(body), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("workspace leaked %q: %s", forbidden, body)
		}
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE library_adapter_audit_events"); err == nil {
		t.Fatal("append-only audit accepted TRUNCATE")
	}
}

func TestCommandsUnavailableAfterAdapterRemoval(t *testing.T) {
	server, pool := newLibraryServer(t)
	defer server.Close()
	defer pool.Close()

	body := []byte(`{"kind":"submission_approve","resource_id":"22222222-2222-4222-8222-222222222222","expected_version":"2026-07-19T00:00:00.123Z","payload":{"reviewReason":"checked"}}`)
	response := send(t, server.URL, "library.review", http.MethodPost, "/api/v1/commands", body, "idem_library_unavailable")
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("command status = %d", response.StatusCode)
	}
	payload, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !bytes.Contains(payload, []byte("LIBRARY_COMMANDS_UNAVAILABLE")) {
		t.Fatalf("command error code missing: %s", payload)
	}

	operation := send(t, server.URL, "library.review", http.MethodGet, "/api/v1/operations/submission_approve", nil, "idem_library_unavailable")
	if operation.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("operation status = %d", operation.StatusCode)
	}

	// No legacy requests and no operation ledger rows are created anymore.
	var operations int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_adapter_operations`).Scan(&operations); err != nil || operations != 0 {
		t.Fatalf("operation ledger rows = %d, %v", operations, err)
	}
}

func TestScopeAndCommandAllowlistDefaultDeny(t *testing.T) {
	server, pool := newLibraryServer(t)
	defer server.Close()
	defer pool.Close()

	denied := send(t, server.URL, "notice.read", http.MethodGet, "/api/v1/workspace", nil, "")
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign scope status = %d", denied.StatusCode)
	}
	command := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"payment_refund","resource_id":"x","expected_version":"v1","payload":{}}`), "idem_forbidden_command")
	if command.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("forbidden command status = %d", command.StatusCode)
	}
	commercial := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"material_update","resource_id":"22222222-2222-4222-8222-222222222222","expected_version":"2026-07-19T00:00:00Z","payload":{"accessLevel":"member_only"}}`), "idem_forbidden_access")
	if commercial.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("commercial access level status = %d", commercial.StatusCode)
	}
	missingCreate := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"course_create","payload":{}}`), "idem_invalid_create_missing")
	if missingCreate.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("missing create fields status = %d", missingCreate.StatusCode)
	}
	// A foreign permission still cannot reach the command surface.
	deniedCommand := send(t, server.URL, "notice.read", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"course_create","payload":{}}`), "idem_denied_command")
	if deniedCommand.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign-scope command status = %d", deniedCommand.StatusCode)
	}
}

func newLibraryServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("LIBRARY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("LIBRARY_TEST_REDIS_ADDR")})
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	handler, err := library.New(library.Config{Database: pool, Redis: redisClient, ClientID: "console-gateway", Keys: map[string]string{"active": serviceSecret}, DownloadClientID: "portal-gateway", DownloadKeys: map[string]string{"active": downloadServiceSecret}, DownloadStore: &fakeDownloadStore{}, HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(func() { _ = redisClient.Close() })
	return server, pool
}

func send(t *testing.T, baseURL, permission, method, path string, body []byte, key string) *http.Response {
	return sendAs(t, baseURL, "99999999-9999-4999-8999-999999999999", permission, method, path, body, key)
}

func sendAs(t *testing.T, baseURL, actorID, permission, method, path string, body []byte, key string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Id", "console-gateway")
	request.Header.Set("X-Key-Id", "active")
	request.Header.Set("X-Actor-User-Id", actorID)
	request.Header.Set("X-Permission-Code", permission)
	request.Header.Set("X-Scope-Kind", "product")
	request.Header.Set("X-Product-Code", "library")
	request.Header.Set("X-Request-Id", "req_library_test_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(serviceSecret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.SetBasicAuth("console-gateway", serviceSecret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readBody(t *testing.T, response *http.Response) []byte {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode >= 500 {
		t.Fatalf("server error %d: %s", response.StatusCode, body)
	}
	return body
}
