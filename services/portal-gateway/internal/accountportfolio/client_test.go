package accountportfolio

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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
					"data":       validOwnerData(test.path),
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
			if len(data) == 0 {
				t.Fatal("owner returned no validated data")
			}
		})
	}
}

func TestPointsPageSignsItsQueryButValidatesAgainstTheStaticOwnerRoute(t *testing.T) {
	const actorID = "11111111-1111-4111-8111-111111111111"
	const cursor = "plc1.b9Nl4wX2vJm9_0DK-cW1H3s9pQm8aXoZr2LtE5yYv7g"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != PointsPath || request.URL.RawQuery != "cursor="+cursor+"&limit=7" {
			t.Fatalf("owner point page = %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		if request.Header.Get("X-Actor-User-Id") != actorID {
			t.Fatalf("owner actor = %q", request.Header.Get("X-Actor-User-Id"))
		}
		assertSignedOwnerRequest(t, request)
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"balance": 12,
				"entries": []any{map[string]any{
					"id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "amount": 12, "reason": "adjusted", "created_at": "2026-07-28T00:00:00Z",
				}},
				"next_cursor": nil,
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
	data, err := client.PointsPage(context.Background(), actorID, "req_points_page", cursor, 7)
	if err != nil || !strings.Contains(string(data), `"next_cursor":null`) {
		t.Fatalf("PointsPage() data=%s err=%v", data, err)
	}
}

func TestReadMethodsRejectIncompleteOwnerPayloads(t *testing.T) {
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
				assertSignedOwnerRequest(t, request)
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"data":{},"request_id":"req_owner"}`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, testClientID, testSecret, testKeyID)
			if err != nil {
				t.Fatal(err)
			}
			client.httpClient = server.Client()
			data, err := test.read(client)
			if !errors.Is(err, ErrInvalid) || len(data) != 0 {
				t.Fatalf("%s incomplete owner result data=%s err=%v, want ErrInvalid with no data", test.path, data, err)
			}
		})
	}
}

func TestTicketCommandsPreserveRawBodyAndUseActorBoundOwnerAuthentication(t *testing.T) {
	const actorID = "11111111-1111-4111-8111-111111111111"
	const ticketID = "22222222-2222-4222-8222-222222222222"
	const notificationID = "33333333-3333-4333-8333-333333333333"
	tests := []struct {
		name     string
		path     string
		status   int
		body     []byte
		response map[string]any
		invoke   func(*Client) (json.RawMessage, error)
	}{
		{
			name:     "create",
			path:     TicketsPath,
			status:   http.StatusCreated,
			body:     []byte("{\n  \"title\": \"Need help\", \"category\": \"account\", \"body\": \"Please help.\"\n}"),
			response: map[string]any{"ticket": testTicketData()},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.CreateTicket(context.Background(), actorID, "req_ticket_create", "idem_ticket_create", []byte("{\n  \"title\": \"Need help\", \"category\": \"account\", \"body\": \"Please help.\"\n}"))
			},
		},
		{
			name:     "follow-up",
			path:     TicketFollowUpsPath(ticketID),
			status:   http.StatusOK,
			body:     []byte(`{"body":"Please update me.","expected_version":1}`),
			response: map[string]any{"ticket": testTicketData()},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.FollowUp(context.Background(), actorID, "req_ticket_follow_up", ticketID, "idem_ticket_follow_up", []byte(`{"body":"Please update me.","expected_version":1}`))
			},
		},
		{
			name:     "notification-read",
			path:     NotificationReadPath(notificationID),
			status:   http.StatusOK,
			response: map[string]any{"notification": testNotificationData()},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.MarkNotificationRead(context.Background(), actorID, "req_notification_read", notificationID, "idem_notification_read")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost || request.URL.Path != test.path {
					t.Fatalf("owner command = %s %s, want POST %s", request.Method, request.URL.Path, test.path)
				}
				if request.Header.Get("X-Actor-User-Id") != actorID || request.Header.Get("Idempotency-Key") == "" {
					t.Fatalf("owner command actor/key = %q/%q", request.Header.Get("X-Actor-User-Id"), request.Header.Get("Idempotency-Key"))
				}
				assertSignedOwnerRequest(t, request)
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != string(test.body) {
					t.Fatalf("owner command body = %q, want exact %q", body, test.body)
				}
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(test.status)
				_ = json.NewEncoder(writer).Encode(map[string]any{"data": test.response, "request_id": "req_owner"})
			}))
			defer server.Close()

			client, err := NewClient(server.URL, testClientID, testSecret, testKeyID)
			if err != nil {
				t.Fatal(err)
			}
			client.httpClient = server.Client()
			data, err := test.invoke(client)
			if err != nil || len(data) == 0 {
				t.Fatalf("ticket command data=%s err=%v", data, err)
			}
		})
	}
}

func TestTicketCommandMapsOwnerConflictWithoutAFalseSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertSignedOwnerRequest(t, request)
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"error":{"code":"VERSION_CONFLICT"},"request_id":"req_owner"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, testClientID, testSecret, testKeyID)
	if err != nil {
		t.Fatal(err)
	}
	client.httpClient = server.Client()
	data, err := client.CreateTicket(context.Background(), "11111111-1111-4111-8111-111111111111", "req_ticket_conflict", "idem_ticket_conflict", []byte(`{"title":"Help","category":"account","body":"Please help."}`))
	if !errors.Is(err, ErrConflict) || len(data) != 0 {
		t.Fatalf("conflicting command data=%s err=%v, want ErrConflict and no data", data, err)
	}
}

func validOwnerData(path string) map[string]any {
	switch path {
	case SummaryPath:
		return map[string]any{
			"points_balance":            0,
			"plan":                      "free",
			"lifetime":                  false,
			"unread_notification_count": 0,
			"open_ticket_count":         0,
		}
	case PointsPath:
		return map[string]any{"balance": 0, "entries": []any{}, "next_cursor": nil}
	case MembershipPath:
		return map[string]any{"plan": "free", "lifetime": false}
	case NotificationsPath:
		return map[string]any{"notifications": []any{}}
	case TicketsPath:
		return map[string]any{"tickets": []any{}}
	case MembershipOrdersPath:
		return map[string]any{"orders": []any{}}
	default:
		panic("unknown Account Portfolio route " + path)
	}
}

func testTicketData() map[string]any {
	return map[string]any{
		"id":         "22222222-2222-4222-8222-222222222222",
		"reference":  "HKT-22222222-2222-4222-8222-222222222222",
		"title":      "Need help",
		"category":   "account",
		"status":     "open",
		"version":    1,
		"created_at": "2026-07-28T00:00:00Z",
		"updated_at": "2026-07-28T00:00:00Z",
	}
}

func testNotificationData() map[string]any {
	return map[string]any{
		"id":         "33333333-3333-4333-8333-333333333333",
		"title":      "客服工单状态已更新",
		"body":       "工单状态已更新。",
		"kind":       "ticket_status",
		"created_at": "2026-07-28T00:00:00Z",
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
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	request.Body = io.NopCloser(strings.NewReader(string(body)))
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:]), request.Header.Get("X-Actor-User-Id")}, "\n")
	mac := hmac.New(sha256.New, []byte(testSecret))
	_, _ = mac.Write([]byte(canonical))
	if timestamp == "" || nonce == "" || request.Header.Get("X-Signature") != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("owner request signature is invalid")
	}
}
