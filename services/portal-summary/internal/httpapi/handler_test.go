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

	"henukit.dev/portal-summary/internal/contract"
	"henukit.dev/portal-summary/internal/summary"
)

const testSecret = "portal-summary-secret-with-entropy"
const retiringSecret = "retiring-portal-secret-with-entropy"

func TestConsoleSummaryAuthenticatesConformsAndRejectsReplay(t *testing.T) {
	redisClient := integrationRedis(t)
	probe := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) }))
	defer probe.Close()
	service, _ := summary.New(summary.Config{
		Version: "2026.07.19", CommitSHA: "0123456789abcdef", DeployedAt: time.Now(), ReadinessURL: probe.URL,
		KeyProbes: []summary.Probe{{Name: "首页", URL: probe.URL}}, EntryProbes: []summary.Probe{{Name: "学习", URL: probe.URL}},
	}, probe.Client())
	handler, err := New(Config{ClientID: "console-gateway-portal", Keys: map[string]string{"portal-active-key": testSecret, "portal-retiring-key": retiringSecret}}, redisClient, service)
	if err != nil {
		t.Fatal(err)
	}
	if _, duplicateErr := New(Config{ClientID: "console-gateway-portal", Keys: map[string]string{"portal-active-key": testSecret, "portal-retiring-key": testSecret}}, redisClient, service); duplicateErr == nil {
		t.Fatal("duplicate active and retiring secrets must be rejected")
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(request, "req_portal_summary", "fixed-portal-nonce-value", "portal-active-key", testSecret)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		Data      contract.PortalSummary `json:"data"`
		RequestID string                 `json:"request_id"`
	}
	decodeErr := json.NewDecoder(response.Body).Decode(&body)
	contractErr := contract.ValidatePortalSummaryEnvelope(contract.PortalSummaryEnvelope(body))
	if decodeErr != nil || contractErr != nil || response.StatusCode != http.StatusOK || body.RequestID != "req_portal_summary" || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Request-Id") != body.RequestID {
		t.Fatalf("summary response = %d %+v (decode=%v contract=%v)", response.StatusCode, body, decodeErr, contractErr)
	}
	replay, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(replay, "req_portal_replay", "fixed-portal-nonce-value", "portal-active-key", testSecret)
	replayResponse, _ := server.Client().Do(replay)
	var replayError struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(replayResponse.Body).Decode(&replayError)
	replayResponse.Body.Close()
	if replayResponse.StatusCode != http.StatusConflict || replayError.Error.Code != "REPLAY_DETECTED" || replayResponse.Header.Get("X-Request-Id") != "req_portal_replay" {
		t.Fatalf("replay = %d/%s/%s, want 409/REPLAY_DETECTED/traced", replayResponse.StatusCode, replayError.Error.Code, replayResponse.Header.Get("X-Request-Id"))
	}

	invalid, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(invalid, "req_invalid value", "another-portal-nonce-val", "portal-active-key", testSecret)
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
	sign(badSignature, "req_bad_signature", "bad-signature-nonce-val", "portal-active-key", testSecret)
	badSignature.Header.Set("X-Signature", strings.Repeat("A", 43))
	badSignatureResponse, _ := server.Client().Do(badSignature)
	badSignatureResponse.Body.Close()
	if badSignatureResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d, want 401", badSignatureResponse.StatusCode)
	}

	retiring, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(retiring, "req_retiring_key", "retiring-key-nonce-value", "portal-retiring-key", retiringSecret)
	retiringResponse, _ := server.Client().Do(retiring)
	retiringResponse.Body.Close()
	if retiringResponse.StatusCode != http.StatusOK {
		t.Fatalf("retiring key = %d, want 200", retiringResponse.StatusCode)
	}

	revoked, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(revoked, "req_revoked_key", "revoked-key-nonce-value!", "portal-revoked-key", "revoked-portal-secret-with-entropy")
	revokedResponse, _ := server.Client().Do(revoked)
	revokedResponse.Body.Close()
	if revokedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked key = %d, want 401", revokedResponse.StatusCode)
	}

	if err := redisClient.Close(); err != nil {
		t.Fatal(err)
	}
	unavailable, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/console-summary", nil)
	sign(unavailable, "req_redis_unavailable", "redis-down-nonce-value!", "portal-active-key", testSecret)
	unavailableResponse, _ := server.Client().Do(unavailable)
	var unavailableError struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(unavailableResponse.Body).Decode(&unavailableError)
	unavailableResponse.Body.Close()
	if unavailableResponse.StatusCode != http.StatusServiceUnavailable || unavailableError.Error.Code != "DEPENDENCY_UNAVAILABLE" || unavailableResponse.Header.Get("X-Request-Id") != "req_redis_unavailable" {
		t.Fatalf("Redis failure = %d/%s/%s, want 503/DEPENDENCY_UNAVAILABLE/traced", unavailableResponse.StatusCode, unavailableError.Error.Code, unavailableResponse.Header.Get("X-Request-Id"))
	}
}

func sign(request *http.Request, requestID, nonceText, keyID, secret string) {
	nonce := make([]byte, 24)
	copy(nonce, []byte(nonceText))
	nonceValue := base64.RawURLEncoding.EncodeToString(nonce)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), timestamp, nonceValue, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth("console-gateway-portal", secret)
	request.Header.Set("X-Service-Id", "console-gateway-portal")
	request.Header.Set("X-Key-Id", keyID)
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
