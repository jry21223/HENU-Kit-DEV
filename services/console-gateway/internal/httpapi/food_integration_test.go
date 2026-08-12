package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	foodapi "henukit.dev/console-gateway/internal/food"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

const (
	integrationFoodApproveID = "11111111-1111-4111-8111-111111111111"
	integrationFoodRejectID  = "22222222-2222-4222-8222-222222222222"
)

func TestFoodSubmissionReviewPersistsAcrossGatewayAndOwner(t *testing.T) {
	ownerURL := os.Getenv("CONSOLE_GATEWAY_TEST_FOOD_URL")
	clientID := os.Getenv("CONSOLE_GATEWAY_TEST_FOOD_CLIENT_ID")
	clientSecret := os.Getenv("CONSOLE_GATEWAY_TEST_FOOD_CLIENT_SECRET")
	keyID := os.Getenv("CONSOLE_GATEWAY_TEST_FOOD_KEY_ID")
	if ownerURL == "" || clientID == "" || clientSecret == "" || keyID == "" {
		t.Skip("real Food owner configuration is required")
	}

	owner, err := foodapi.New(ownerURL, clientID, clientSecret, keyID, &http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("create real Food HTTP client: %v", err)
	}
	redisClient := testRedis(t)
	codec, err := session.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	userID := "171f1c6f-7b10-4c92-91a2-b39bf5af5302"
	token := "food_integration_exchange_token_32_bytes"
	platform := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: token}}
	handler, err := New(
		"https://account.henukit.test",
		"console-gateway",
		"https://console.henukit.test/api/v1/auth/callback",
		platform,
		nil,
		fakeOverview{},
		redisClient,
		codec,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		nil,
		owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	cookieValue, err := codec.Encode(session.Value{UserID: userID, ExchangeToken: token, ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookie, Value: cookieValue}

	before := readFoodWorkspaceFromGateway(t, server, cookie)
	assertFoodSubmissionState(t, before, integrationFoodApproveID, "pending", 1)
	assertFoodSubmissionState(t, before, integrationFoodRejectID, "pending", 1)

	sendFoodReviewCommand(t, server, cookie, "submission_approve", integrationFoodApproveID, 1, "idem_food_gateway_owner_approve", http.StatusOK)
	sendFoodReviewCommand(t, server, cookie, "submission_approve", integrationFoodApproveID, 1, "idem_food_gateway_owner_approve", http.StatusOK)
	sendFoodReviewCommand(t, server, cookie, "submission_reject", integrationFoodRejectID, 1, "idem_food_gateway_owner_reject", http.StatusOK)
	sendFoodReviewCommand(t, server, cookie, "submission_reject", integrationFoodApproveID, 1, "idem_food_gateway_owner_stale", http.StatusConflict)

	after := readFoodWorkspaceFromGateway(t, server, cookie)
	assertFoodSubmissionAbsent(t, after, integrationFoodApproveID)
	assertFoodSubmissionAbsent(t, after, integrationFoodRejectID)
}

func sendFoodReviewCommand(t *testing.T, server *httptest.Server, cookie *http.Cookie, kind, resourceID string, expectedVersion int, key string, expectedStatus int) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"kind": kind, "resource_id": resourceID, "expected_version": expectedVersion,
		"payload": map[string]string{"note": "Gateway to Food Owner integration"},
	})
	if err != nil {
		t.Fatal(err)
	}
	command, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/food/commands", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	command.AddCookie(cookie)
	command.Header.Set("Content-Type", "application/json")
	command.Header.Set("Idempotency-Key", key)
	response, err := server.Client().Do(command)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != expectedStatus {
		t.Fatalf("%s through Gateway status=%d body=%s, want %d", kind, response.StatusCode, responseBody, expectedStatus)
	}
}

type foodWorkspaceEnvelope struct {
	Data struct {
		Submissions []struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Version int    `json:"version"`
		} `json:"submissions"`
	} `json:"data"`
}

func readFoodWorkspaceFromGateway(t *testing.T, server *httptest.Server, cookie *http.Cookie) foodWorkspaceEnvelope {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/food", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope foodWorkspaceEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode Food workspace: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read Food workspace status=%d", response.StatusCode)
	}
	return envelope
}

func assertFoodSubmissionState(t *testing.T, workspace foodWorkspaceEnvelope, id, status string, version int) {
	t.Helper()
	for _, submission := range workspace.Data.Submissions {
		if submission.ID == id {
			if submission.Status != status || submission.Version != version {
				t.Fatalf("submission state=%s/v%d, want %s/v%d", submission.Status, submission.Version, status, version)
			}
			return
		}
	}
	t.Fatalf("submission %s not found", id)
}

func assertFoodSubmissionAbsent(t *testing.T, workspace foodWorkspaceEnvelope, id string) {
	t.Helper()
	for _, submission := range workspace.Data.Submissions {
		if submission.ID == id {
			t.Fatalf("reviewed submission %s remained pending", id)
		}
	}
}
