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
	consoleClientID  = "console-gateway"
	consoleSecret    = "account-portfolio-console-secret-at-least-32-bytes"
	consoleKeyID     = "account-console-key"
	operatorID       = "11111111-1111-4111-8111-111111111111"
	ticketID         = "22222222-2222-4222-8222-222222222222"
	membershipUserID = "33333333-3333-4333-8333-333333333333"
)

func TestConsoleTicketCallsUseActorBoundSignaturesAndExactOwnerRoutes(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		response map[string]any
		invoke   func(*Client) (json.RawMessage, error)
		wantKey  string
	}{
		{
			name:     "queue",
			method:   http.MethodGet,
			path:     TicketsPath,
			response: map[string]any{"tickets": []any{}},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.Tickets(WithRequestID(context.Background(), "req_account_queue"), operatorID)
			},
		},
		{
			name:     "detail",
			method:   http.MethodGet,
			path:     TicketPath(ticketID),
			response: map[string]any{"ticket": testTicket(), "messages": []any{}, "events": []any{}},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.Ticket(WithRequestID(context.Background(), "req_account_detail"), operatorID, ticketID)
			},
		},
		{
			name:     "operator reply",
			method:   http.MethodPost,
			path:     TicketRepliesPath(ticketID),
			body:     "{\n  \"body\": \"We are investigating.\", \"expected_version\": 1\n}",
			response: map[string]any{"ticket": testTicket()},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.Reply(WithRequestID(context.Background(), "req_account_reply"), operatorID, ticketID, "idem_account_reply", []byte("{\n  \"body\": \"We are investigating.\", \"expected_version\": 1\n}"))
			},
			wantKey: "idem_account_reply",
		},
		{
			name:     "transition",
			method:   http.MethodPost,
			path:     TicketTransitionsPath(ticketID),
			body:     `{"status":"resolved","expected_version":1}`,
			response: map[string]any{"ticket": testTicket()},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.Transition(WithRequestID(context.Background(), "req_account_transition"), operatorID, ticketID, "idem_account_transition", []byte(`{"status":"resolved","expected_version":1}`))
			},
			wantKey: "idem_account_transition",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != test.method || request.URL.Path != test.path {
					t.Fatalf("owner request = %s %s, want %s %s", request.Method, request.URL.Path, test.method, test.path)
				}
				if request.Header.Get("X-Actor-User-Id") != operatorID || request.Header.Get("X-Request-Id") == "" {
					t.Fatalf("owner actor/request id = %q/%q", request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Request-Id"))
				}
				if request.Header.Get("Idempotency-Key") != test.wantKey {
					t.Fatalf("owner idempotency key = %q, want %q", request.Header.Get("Idempotency-Key"), test.wantKey)
				}
				assertActorBoundSignature(t, request)
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != test.body {
					t.Fatalf("owner body = %q, want exact %q", body, test.body)
				}
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(map[string]any{"data": test.response, "request_id": "req_account_owner"})
			}))
			defer server.Close()

			client, err := New(server.URL, consoleClientID, consoleSecret, consoleKeyID, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			data, err := test.invoke(client)
			if err != nil || len(data) == 0 {
				t.Fatalf("account console call data=%s err=%v", data, err)
			}
		})
	}
}

func TestConsoleMembershipCallsUseActorBoundSignaturesAndExactOwnerRoutes(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		response map[string]any
		invoke   func(*Client) (json.RawMessage, error)
		wantKey  string
	}{
		{
			name:     "membership lookup",
			method:   http.MethodGet,
			path:     MembershipPath(membershipUserID),
			response: map[string]any{"membership": testMembership("free", false, 1)},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.Membership(WithRequestID(context.Background(), "req_membership_lookup"), operatorID, membershipUserID)
			},
		},
		{
			name:     "lifetime grant",
			method:   http.MethodPost,
			path:     MembershipGrantsPath(membershipUserID),
			body:     "{\n  \"reason\": \"Verified support entitlement.\", \"expected_version\": 1\n}",
			response: map[string]any{"membership": testMembership("lifetime", true, 2)},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.Grant(WithRequestID(context.Background(), "req_membership_grant"), operatorID, membershipUserID, "idem_membership_grant", []byte("{\n  \"reason\": \"Verified support entitlement.\", \"expected_version\": 1\n}"))
			},
			wantKey: "idem_membership_grant",
		},
		{
			name:     "lifetime revocation",
			method:   http.MethodPost,
			path:     MembershipRevocationsPath(membershipUserID),
			body:     `{"reason":"Membership correction","expected_version":2}`,
			response: map[string]any{"membership": testMembership("free", false, 3)},
			invoke: func(client *Client) (json.RawMessage, error) {
				return client.Revoke(WithRequestID(context.Background(), "req_membership_revocation"), operatorID, membershipUserID, "idem_membership_revoke", []byte(`{"reason":"Membership correction","expected_version":2}`))
			},
			wantKey: "idem_membership_revoke",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != test.method || request.URL.Path != test.path {
					t.Fatalf("owner request = %s %s, want %s %s", request.Method, request.URL.Path, test.method, test.path)
				}
				if request.Header.Get("X-Actor-User-Id") != operatorID || request.Header.Get("X-Request-Id") == "" {
					t.Fatalf("owner actor/request id = %q/%q", request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Request-Id"))
				}
				if request.Header.Get("Idempotency-Key") != test.wantKey {
					t.Fatalf("owner idempotency key = %q, want %q", request.Header.Get("Idempotency-Key"), test.wantKey)
				}
				assertActorBoundSignature(t, request)
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != test.body {
					t.Fatalf("owner body = %q, want exact %q", body, test.body)
				}
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(map[string]any{"data": test.response, "request_id": "req_account_owner"})
			}))
			defer server.Close()

			client, err := New(server.URL, consoleClientID, consoleSecret, consoleKeyID, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			data, err := test.invoke(client)
			if err != nil || len(data) == 0 {
				t.Fatalf("account membership call data=%s err=%v", data, err)
			}
		})
	}
}

func TestConsolePointAdjustmentUsesActorBoundSignatureAndRejectsIdentityLeaks(t *testing.T) {
	raw := "{\n  \"user_id\": \"33333333-3333-4333-8333-333333333333\", \"amount\": 120, \"reason\": \"Verified support correction.\"\n}"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != PointAdjustmentsPath {
			t.Fatalf("owner point adjustment request = %s %s, want POST %s", request.Method, request.URL.Path, PointAdjustmentsPath)
		}
		if request.Header.Get("X-Actor-User-Id") != operatorID || request.Header.Get("Idempotency-Key") != "idem_account_points_adjust" || request.Header.Get("X-Request-Id") != "req_account_points_adjust" {
			t.Fatalf("owner point adjustment headers actor/key/request = %q/%q/%q", request.Header.Get("X-Actor-User-Id"), request.Header.Get("Idempotency-Key"), request.Header.Get("X-Request-Id"))
		}
		assertActorBoundSignature(t, request)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != raw {
			t.Fatalf("owner point adjustment body = %q, want exact %q", body, raw)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"balance": 120, "entry": testPointLedgerEntry()}, "request_id": "req_account_owner"})
	}))
	defer server.Close()
	client, err := New(server.URL, consoleClientID, consoleSecret, consoleKeyID, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	data, err := client.Adjust(WithRequestID(context.Background(), "req_account_points_adjust"), operatorID, "idem_account_points_adjust", []byte(raw))
	if err != nil || len(data) == 0 {
		t.Fatalf("point adjustment data=%s err=%v", data, err)
	}

	leakingServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
			"balance": 120,
			"entry":   map[string]any{"id": "55555555-5555-4555-8555-555555555555", "amount": 120, "reason": "Verified support correction.", "created_at": "2026-07-28T00:00:00Z", "operator_user_id": operatorID},
		}, "request_id": "req_account_owner"})
	}))
	defer leakingServer.Close()
	leakingClient, err := New(leakingServer.URL, consoleClientID, consoleSecret, consoleKeyID, leakingServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	data, err = leakingClient.Adjust(WithRequestID(context.Background(), "req_account_points_leak"), operatorID, "idem_account_points_leak", []byte(raw))
	if !errors.Is(err, ErrInvalid) || len(data) != 0 {
		t.Fatalf("identity-leaking point owner success data=%s err=%v, want ErrInvalid and no data", data, err)
	}
}

func TestConsolePointAdjustmentClientRejectsUnsafePublicPointValues(t *testing.T) {
	const unsafe = int64(9_007_199_254_740_992)
	raw := []byte(`{"user_id":"33333333-3333-4333-8333-333333333333","amount":1,"reason":"Validated command fixture."}`)
	for _, rejected := range []struct {
		name string
		data map[string]any
	}{
		{
			name: "balance beyond JavaScript-safe range",
			data: map[string]any{"balance": unsafe, "entry": testPointLedgerEntry()},
		},
		{
			name: "positive ledger entry beyond JavaScript-safe range",
			data: map[string]any{"balance": 0, "entry": func() map[string]any {
				entry := testPointLedgerEntry()
				entry["amount"] = unsafe
				return entry
			}()},
		},
		{
			name: "negative ledger entry beyond JavaScript-safe range",
			data: map[string]any{"balance": 0, "entry": func() map[string]any {
				entry := testPointLedgerEntry()
				entry["amount"] = -unsafe
				return entry
			}()},
		},
	} {
		t.Run(rejected.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assertActorBoundSignature(t, request)
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(map[string]any{"data": rejected.data, "request_id": "req_account_owner"})
			}))
			defer server.Close()
			client, err := New(server.URL, consoleClientID, consoleSecret, consoleKeyID, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			data, err := client.Adjust(WithRequestID(context.Background(), "req_account_points_unsafe"), operatorID, "idem_account_points_unsafe", raw)
			if !errors.Is(err, ErrInvalid) || len(data) != 0 {
				t.Fatalf("unsafe point owner success data=%s err=%v, want ErrInvalid and no data", data, err)
			}
		})
	}
}

func TestConsoleMembershipClientRejectsMalformedOrInternalOwnerFields(t *testing.T) {
	tests := []struct {
		name string
		data map[string]any
	}{
		{
			name: "internal operator identity",
			data: map[string]any{"membership": map[string]any{
				"plan": "lifetime", "lifetime": true, "version": 2, "actor_user_id": operatorID,
			}},
		},
		{
			name: "internal target identity",
			data: map[string]any{"membership": testMembership("free", false, 1), "user_id": membershipUserID},
		},
		{
			name: "inconsistent entitlement",
			data: map[string]any{"membership": testMembership("free", true, 1)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(map[string]any{"data": test.data, "request_id": "req_account_owner"})
			}))
			defer server.Close()
			client, err := New(server.URL, consoleClientID, consoleSecret, consoleKeyID, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			data, err := client.Membership(WithRequestID(context.Background(), "req_membership_invalid"), operatorID, membershipUserID)
			if !errors.Is(err, ErrInvalid) || len(data) != 0 {
				t.Fatalf("invalid membership owner success data=%s err=%v, want ErrInvalid and no data", data, err)
			}
		})
	}
}

func TestConsoleTicketClientRejectsOwnerConflictWithoutAFalseSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertActorBoundSignature(t, request)
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"error":{"code":"VERSION_CONFLICT"},"request_id":"req_account_owner"}`))
	}))
	defer server.Close()

	client, err := New(server.URL, consoleClientID, consoleSecret, consoleKeyID, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	data, err := client.Transition(WithRequestID(context.Background(), "req_account_conflict"), operatorID, ticketID, "idem_account_conflict", []byte(`{"status":"resolved","expected_version":1}`))
	if !errors.Is(err, ErrConflict) || len(data) != 0 {
		t.Fatalf("conflicting transition data=%s err=%v, want ErrConflict and no data", data, err)
	}
}

func TestConsoleTicketClientRejectsMalformedOwnerSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"data":{"tickets":[{"id":"not-a-uuid"}]},"request_id":"req_account_owner"}`))
	}))
	defer server.Close()
	client, err := New(server.URL, consoleClientID, consoleSecret, consoleKeyID, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	data, err := client.Tickets(WithRequestID(context.Background(), "req_account_malformed"), operatorID)
	if !errors.Is(err, ErrInvalid) || len(data) != 0 {
		t.Fatalf("malformed owner success data=%s err=%v, want ErrInvalid and no data", data, err)
	}
}

func TestConsoleTicketClientRejectsOwnerIdentityLeakInTicket(t *testing.T) {
	response := testTicket()
	response["user_id"] = "33333333-3333-4333-8333-333333333333"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data":       map[string]any{"tickets": []any{response}},
			"request_id": "req_account_owner",
		})
	}))
	defer server.Close()

	client, err := New(server.URL, consoleClientID, consoleSecret, consoleKeyID, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	data, err := client.Tickets(WithRequestID(context.Background(), "req_account_identity_leak"), operatorID)
	if !errors.Is(err, ErrInvalid) || len(data) != 0 {
		t.Fatalf("identity-leaking owner success data=%s err=%v, want ErrInvalid and no data", data, err)
	}
}

func testTicket() map[string]any {
	return map[string]any{
		"id":         ticketID,
		"reference":  "HKT-" + ticketID,
		"title":      "Need help",
		"category":   "account",
		"status":     "open",
		"version":    1,
		"created_at": "2026-07-28T00:00:00Z",
		"updated_at": "2026-07-28T00:00:00Z",
	}
}

func testMembership(plan string, lifetime bool, version int) map[string]any {
	return map[string]any{"plan": plan, "lifetime": lifetime, "version": version}
}

func testPointLedgerEntry() map[string]any {
	return map[string]any{
		"id":         "55555555-5555-4555-8555-555555555555",
		"amount":     120,
		"reason":     "Verified support correction.",
		"created_at": "2026-07-28T00:00:00Z",
	}
}

func assertActorBoundSignature(t *testing.T, request *http.Request) {
	t.Helper()
	user, password, ok := request.BasicAuth()
	if !ok || user != consoleClientID || password != consoleSecret || request.Header.Get("X-Service-Id") != consoleClientID || request.Header.Get("X-Key-Id") != consoleKeyID {
		t.Fatal("owner request omitted Console service credentials")
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	request.Body = io.NopCloser(strings.NewReader(string(body)))
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:]), request.Header.Get("X-Actor-User-Id")}, "\n")
	mac := hmac.New(sha256.New, []byte(consoleSecret))
	_, _ = mac.Write([]byte(canonical))
	if request.Header.Get("X-Timestamp") == "" || request.Header.Get("X-Nonce") == "" || request.Header.Get("X-Signature") != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("owner request did not use a valid actor-bound signature")
	}
}
