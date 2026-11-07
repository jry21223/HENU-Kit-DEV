package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	accountportfolioapi "henukit.dev/console-gateway/internal/accountportfolio"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

func TestAccountOperationLookupRechecksOriginalPermissionAndHidesDeniedResults(t *testing.T) {
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	const actor = "171f1c6f-7b10-4c92-91a2-b39bf5af5302"
	platform := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: "exchange_token_with_at_least_32_characters"}}
	owner := &fakeAccountPortfolio{result: json.RawMessage(`{"operation":"ticket_reply","status":"succeeded","result":{"ticket":{"id":"22222222-2222-4222-8222-222222222222","reference":"HKT-22222222-2222-4222-8222-222222222222","user_id":"33333333-3333-4333-8333-333333333333","title":"Help","category":"account","status":"in_progress","version":2,"created_at":"2026-07-28T00:00:00Z","updated_at":"2026-07-28T00:01:00Z"}}}`)}
	// Read-only account routes do not contact Redis; no shared test database is used.
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer redisClient.Close()
	handler, err := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", platform, nil, fakeOverview{}, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)), nil, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := codec.Encode(session.Value{UserID: actor, ExchangeToken: platform.exchange.ExchangeToken, ExpiresAt: time.Now().Add(time.Minute)})
	lookup := func(operation string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/account/operations/"+operation+"/idem_original_operation", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
		r.Header.Set("X-Actor-User-Id", "44444444-4444-4444-8444-444444444444")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	response := lookup("ticket_reply")
	if response.Code != http.StatusOK {
		t.Fatalf("lookup = %d: %s", response.Code, response.Body)
	}
	if owner.actor != actor || owner.key != "idem_original_operation" || platform.identityPermission != "account.tickets.reply" || strings.Contains(response.Body.String(), `"user_id"`) || !strings.Contains(response.Body.String(), `"display_name":"测试用户"`) {
		t.Fatalf("incorrect actor or identity result: actor=%s key=%s permission=%s result=%s", owner.actor, owner.key, platform.identityPermission, response.Body)
	}
	platform.accountErrors = map[string]error{"account.tickets.reply": platformcore.ErrForbidden}
	response = lookup("ticket_reply")
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), `"succeeded"`) {
		t.Fatalf("revoked permission = %d: %s", response.Code, response.Body)
	}
	platform.accountErrors = nil
	for operation, permission := range map[string]string{"membership_grant": "account.membership.write", "membership_revoke": "account.membership.write", "point_adjustment": "account.points.adjust", "ticket_transition": "account.tickets.transition"} {
		owner.result = json.RawMessage(`{"operation":"` + operation + `","status":"unknown"}`)
		response = lookup(operation)
		if response.Code != http.StatusOK || platform.accountPermissions[len(platform.accountPermissions)-1] != permission {
			t.Fatalf("%s = %d permission=%v", operation, response.Code, platform.accountPermissions)
		}
	}
}

func TestAccountTicketHTTPRejectsMalformedOwnerDetailWithoutIdentityLeak(t *testing.T) {
	const ticketID = "22222222-2222-4222-8222-222222222222"
	const privateActor = "33333333-3333-4333-8333-333333333333"
	for _, test := range []struct {
		name, section, field string
		value                any
	}{
		{"message actor identity", "messages", "actor_user_id", privateActor},
		{"message nested identity", "messages", "metadata", map[string]any{"user_id": privateActor}},
		{"event actor identity", "events", "actor_user_id", privateActor},
		{"event nested identity", "events", "metadata", map[string]any{"operator_user_id": privateActor}},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := map[string]any{"id": ticketID, "author_kind": "operator", "body": "Reply", "created_at": "2026-07-28T00:00:00Z"}
			event := map[string]any{"id": ticketID, "kind": "operator_reply", "from_status": "open", "to_status": "in_progress", "created_at": "2026-07-28T00:00:00Z"}
			if test.section == "messages" {
				message[test.field] = test.value
			} else {
				event[test.field] = test.value
			}
			// This controlled malformed-owner fixture is not a claim about data
			// emitted by the real Account owner. The private HTTP client is real.
			ownerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
					"ticket":   map[string]any{"id": ticketID, "reference": "HKT-" + ticketID, "user_id": privateActor, "title": "Help", "category": "account", "status": "open", "version": 1, "created_at": "2026-07-28T00:00:00Z", "updated_at": "2026-07-28T00:00:00Z"},
					"messages": []any{message}, "events": []any{event},
				}, "request_id": "req_malformed_owner_detail"})
			}))
			defer ownerServer.Close()
			owner, err := accountportfolioapi.New(ownerServer.URL, "console-gateway", strings.Repeat("s", 32), "test-key", ownerServer.Client())
			if err != nil {
				t.Fatal(err)
			}
			codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
			platform := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: "exchange_token_with_at_least_32_characters"}}
			redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
			defer redisClient.Close()
			handler, err := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", platform, nil, fakeOverview{}, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)), nil, nil, owner)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := codec.Encode(session.Value{UserID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", ExchangeToken: platform.exchange.ExchangeToken, ExpiresAt: time.Now().Add(time.Minute)})
			request := httptest.NewRequest(http.MethodGet, "/api/v1/account/tickets/"+ticketID, nil)
			request.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var body map[string]json.RawMessage
			if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &body) != nil || body["error"] == nil || body["data"] != nil {
				t.Fatalf("malformed owner HTTP result = %d: %s", response.Code, response.Body)
			}
			for _, private := range []string{privateActor, "actor_user_id", "operator_user_id", "metadata"} {
				if strings.Contains(response.Body.String(), private) {
					t.Fatalf("browser JSON leaked %q: %s", private, response.Body)
				}
			}
		})
	}
}
