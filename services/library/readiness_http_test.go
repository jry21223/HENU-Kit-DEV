package library

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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

type unusedReadinessStore struct{ DownloadObjectStore }

// Readiness has no database facts. A deliberately unconnected pool proves the
// real HTTP/auth boundary does not need a catalog or touch product records.
func TestConsoleReadinessHTTP(t *testing.T) {
	address := os.Getenv("LIBRARY_TEST_REDIS_ADDR")
	if address == "" {
		address = "127.0.0.1:6379"
	}
	redisClient := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = redisClient.Close() })
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	const secret = "library-readiness-test-secret-at-least-32-bytes"
	handler, err := New(Config{Database: &pgxpool.Pool{}, Redis: redisClient, ClientID: "console-gateway", Keys: map[string]string{"test": secret}, DownloadClientID: "portal-gateway", DownloadKeys: map[string]string{"test": "independent-download-test-secret-at-least-32-bytes"}, DownloadStore: unusedReadinessStore{}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	get := func(path, permission, product, actorID string, authenticated bool) *http.Response {
		t.Helper()
		request, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
		request.Header.Set("X-Actor-User-Id", actorID)
		request.Header.Set("X-Permission-Code", permission)
		request.Header.Set("X-Scope-Kind", "product")
		request.Header.Set("X-Product-Code", product)
		if authenticated {
			request.SetBasicAuth("console-gateway", secret)
			request.Header.Set("X-Service-Id", "console-gateway")
			request.Header.Set("X-Key-Id", "test")
			request.Header.Set("X-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
			request.Header.Set("X-Nonce", base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24])))
			digest := sha256.Sum256(nil)
			canonical := strings.Join([]string{http.MethodGet, path, request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:])}, "\n")
			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write([]byte(canonical))
			request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = response.Body.Close() })
		return response
	}
	const actorID = "99999999-9999-4999-8999-999999999999"
	response := get("/api/v1/console-readiness", "library.read", "library", actorID, true)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("readiness HTTP status = %d, want 200", response.StatusCode)
	}
	var envelope struct {
		Data struct {
			Management   string          `json:"management"`
			Reason       string          `json:"reason"`
			Capabilities map[string]bool `json:"capabilities"`
			CheckedAt    time.Time       `json:"checked_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Management != "not_ready" || envelope.Data.Reason != "management_migration_pending" || envelope.Data.CheckedAt.IsZero() {
		t.Fatalf("readiness = %+v", envelope.Data)
	}
	for _, capability := range []string{"workspace", "commands", "operation_results"} {
		available, present := envelope.Data.Capabilities[capability]
		if !present || available {
			t.Fatalf("%s capability = %v/%v", capability, available, present)
		}
	}
	for _, tc := range []struct {
		name, permission, product, actor string
		authenticated                    bool
		status                           int
	}{
		{"no service credential", "library.read", "library", actorID, false, 401},
		{"foreign permission", "notice.read", "library", actorID, true, 403},
		{"foreign scope", "library.read", "notice", actorID, true, 403},
		{"invalid actor", "library.read", "library", "invalid", true, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if response := get("/api/v1/console-readiness", tc.permission, tc.product, tc.actor, tc.authenticated); response.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.status)
			}
		})
	}
	legacy := get("/api/v1/workspace", "library.read", "library", actorID, true)
	var oldEnvelope struct {
		Data Workspace `json:"data"`
	}
	// The data object remains compatible with the old strict Workspace decoder.
	var raw struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(legacy.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw.Data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&oldEnvelope.Data); err != nil || !oldEnvelope.Data.Degraded {
		t.Fatalf("legacy Workspace changed: %v / %+v", err, oldEnvelope.Data)
	}
}
