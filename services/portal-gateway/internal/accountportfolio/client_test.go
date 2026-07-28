package accountportfolio

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testClientID = "portal-gateway"
	testSecret   = "account-portfolio-gateway-secret-at-least-32-bytes"
	testKeyID    = "account-key"
)

func TestSummaryForwardsOnlyTheAuthenticatedActorThroughSignedOwnerRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != SummaryPath {
			t.Fatalf("owner request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("X-Actor-User-Id"); got != "11111111-1111-4111-8111-111111111111" {
			t.Fatalf("actor header = %q", got)
		}
		if got := request.Header.Get("X-Request-Id"); got != "req_account_summary" {
			t.Fatalf("request ID = %q", got)
		}
		assertSignedOwnerRequest(t, request)
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"points_balance":            0,
				"plan":                      "free",
				"lifetime":                  false,
				"unread_notification_count": 0,
				"open_ticket_count":         0,
			},
			"request_id": "req_owner",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, testClientID, testSecret, testKeyID)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = server.Client()
	data, err := client.Summary(context.Background(), "11111111-1111-4111-8111-111111111111", "req_account_summary")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"points_balance":0`) || strings.Contains(strings.ToLower(string(data)), "mock") {
		t.Fatalf("summary data = %s", data)
	}
}

func TestSummaryRejectsUpstreamFailureInsteadOfReturningFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertSignedOwnerRequest(t, request)
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":{"code":"DEPENDENCY_UNAVAILABLE"},"request_id":"req_owner"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, testClientID, testSecret, testKeyID)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = server.Client()
	data, err := client.Summary(context.Background(), "11111111-1111-4111-8111-111111111111", "req_account_failure")
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Summary() error = %v, want unavailable", err)
	}
	if len(data) != 0 {
		t.Fatalf("Summary() returned fallback data on owner failure: %s", data)
	}
}

func TestReadMethodsUseTheDeclaredOwnerRoutes(t *testing.T) {
	tests := []struct {
		name string
		path string
		read func(*Client) (json.RawMessage, error)
	}{
		{name: "summary", path: SummaryPath, read: func(client *Client) (json.RawMessage, error) {
			return client.Summary(context.Background(), "11111111-1111-4111-8111-111111111111", "req_summary")
		}},
		{name: "points", path: PointsPath, read: func(client *Client) (json.RawMessage, error) {
			return client.Points(context.Background(), "11111111-1111-4111-8111-111111111111", "req_points")
		}},
		{name: "membership", path: MembershipPath, read: func(client *Client) (json.RawMessage, error) {
			return client.Membership(context.Background(), "11111111-1111-4111-8111-111111111111", "req_membership")
		}},
		{name: "notifications", path: NotificationsPath, read: func(client *Client) (json.RawMessage, error) {
			return client.Notifications(context.Background(), "11111111-1111-4111-8111-111111111111", "req_notifications")
		}},
		{name: "tickets", path: TicketsPath, read: func(client *Client) (json.RawMessage, error) {
			return client.Tickets(context.Background(), "11111111-1111-4111-8111-111111111111", "req_tickets")
		}},
		{name: "membership orders", path: MembershipOrdersPath, read: func(client *Client) (json.RawMessage, error) {
			return client.MembershipOrders(context.Background(), "11111111-1111-4111-8111-111111111111", "req_orders")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != test.path {
					t.Fatalf("owner path = %q, want %q", request.URL.Path, test.path)
				}
				assertSignedOwnerRequest(t, request)
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"data":       map[string]string{"route": test.path},
					"request_id": "req_owner",
				})
			}))
			defer server.Close()

			client, err := NewClient(server.URL, testClientID, testSecret, testKeyID)
			if err != nil {
				t.Fatal(err)
			}
			client.httpClient = server.Client()
			data, err := test.read(client)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), test.path) {
				t.Fatalf("owner data = %s", data)
			}
		})
	}
}

func assertSignedOwnerRequest(t *testing.T, request *http.Request) {
	t.Helper()
	user, password, ok := request.BasicAuth()
	if !ok || user != testClientID || password != testSecret || request.Header.Get("X-Service-Id") != testClientID || request.Header.Get("X-Key-Id") != testKeyID {
		t.Fatal("owner request omitted service credentials")
	}
	timestamp := request.Header.Get("X-Timestamp")
	nonce := request.Header.Get("X-Nonce")
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(testSecret))
	_, _ = mac.Write([]byte(canonical))
	if timestamp == "" || nonce == "" || request.Header.Get("X-Signature") != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("owner request signature is invalid")
	}
}
