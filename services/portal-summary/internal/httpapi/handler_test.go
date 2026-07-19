package httpapi

import (
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

	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-summary/internal/summary"
)

const testSecret = "portal-summary-secret-with-entropy"

func TestConsoleSummaryAuthenticatesConformsAndRejectsReplay(t *testing.T) {
	redisClient := integrationRedis(t)
	probe := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) }))
	defer probe.Close()
	service, _ := summary.New(summary.Config{
		Version: "2026.07.19", CommitSHA: "0123456789abcdef", DeployedAt: time.Now(), ReadinessURL: probe.URL,
		KeyProbes: []summary.Probe{{Name: "首页", URL: probe.URL}}, EntryProbes: []summary.Probe{{Name: "学习", URL: probe.URL}},
	}, probe.Client())
	handler, err := New(Config{ClientID: "console-gateway-portal", ClientSecret: testSecret, KeyID: "portal-active-key"}, redisClient, service)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(request, "req_portal_summary", "fixed-portal-nonce-value")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		Data      summary.Result `json:"data"`
		RequestID string         `json:"request_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || response.StatusCode != http.StatusOK || body.RequestID != "req_portal_summary" || body.Data.RequestID != body.RequestID || body.Data.ID != "portal" || len(body.Data.Metrics) != 8 || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("summary response = %d %+v (%v)", response.StatusCode, body, err)
	}
	replay, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(replay, "req_portal_replay", "fixed-portal-nonce-value")
	replayResponse, _ := server.Client().Do(replay)
	replayResponse.Body.Close()
	if replayResponse.StatusCode != http.StatusConflict {
		t.Fatalf("replay = %d, want 409", replayResponse.StatusCode)
	}

	invalid, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(invalid, "req_invalid value", "another-portal-nonce-val")
	invalidResponse, _ := server.Client().Do(invalid)
	invalidResponse.Body.Close()
	if invalidResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid request ID = %d, want 400", invalidResponse.StatusCode)
	}

	unsigned, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	unsigned.Header.Set("X-Request-Id", "req_unsigned")
	unsignedResponse, _ := server.Client().Do(unsigned)
	unsignedResponse.Body.Close()
	if unsignedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned request = %d, want 401", unsignedResponse.StatusCode)
	}

	badSignature, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(badSignature, "req_bad_signature", "bad-signature-nonce-val")
	badSignature.Header.Set("X-Signature", strings.Repeat("A", 43))
	badSignatureResponse, _ := server.Client().Do(badSignature)
	badSignatureResponse.Body.Close()
	if badSignatureResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d, want 401", badSignatureResponse.StatusCode)
	}
}

func sign(request *http.Request, requestID, nonceText string) {
	nonce := make([]byte, 24)
	copy(nonce, []byte(nonceText))
	nonceValue := base64.RawURLEncoding.EncodeToString(nonce)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), timestamp, nonceValue, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(testSecret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth("console-gateway-portal", testSecret)
	request.Header.Set("X-Service-Id", "console-gateway-portal")
	request.Header.Set("X-Key-Id", "portal-active-key")
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonceValue)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-Request-Id", requestID)
}

func integrationRedis(t *testing.T) *redis.Client {
	t.Helper()
	address := os.Getenv("PORTAL_SUMMARY_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("PORTAL_SUMMARY_TEST_REDIS_ADDR is required")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	if err := client.FlushDB(t.Context()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
