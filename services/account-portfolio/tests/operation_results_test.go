package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestConsoleOperationResultRecoversCommittedReplyWithoutRepeatingIt(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()
	const owner = "a8311111-1111-4111-8111-111111111111"
	const operator = "a8322222-2222-4222-8222-222222222222"
	const key = "idem_operation_reply_recovery"
	created := sendOwnerJSON(t, server.URL, http.MethodPost, owner, "/api/v1/account/tickets", "operation-create", "idem_operation_create", `{"title":"Recovery","category":"account","body":"Help please."}`)
	var creation struct {
		Data struct {
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		} `json:"data"`
	}
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d: %s", created.StatusCode, responseText(t, created))
	}
	decodeResponse(t, created, &creation)
	id := creation.Data.Ticket.ID
	reply := sendConsoleJSON(t, server.URL, http.MethodPost, operator, "/api/v1/console/tickets/"+id+"/replies", "operation-reply", key, `{"body":"We are checking.","expected_version":1}`)
	if reply.StatusCode != http.StatusOK {
		t.Fatalf("reply = %d: %s", reply.StatusCode, responseText(t, reply))
	}
	// The caller loses the write response; only the original operation/key is known.
	reply.Body.Close()
	route := "/api/v1/console/operations/ticket_reply/" + key
	for index, actor := range []string{operator, "a8333333-3333-4333-8333-333333333333", operator} {
		response := sendConsoleJSON(t, server.URL, http.MethodGet, actor, route, "operation-lookup-"+fmtInt(int64(index)), "", "")
		if response.StatusCode != http.StatusOK {
			t.Fatalf("lookup = %d: %s", response.StatusCode, responseText(t, response))
		}
		var result struct {
			Data struct {
				Operation string          `json:"operation"`
				Status    string          `json:"status"`
				Result    json.RawMessage `json:"result"`
			} `json:"data"`
		}
		decodeResponse(t, response, &result)
		if actor != operator {
			if result.Data.Status != "unknown" || len(result.Data.Result) != 0 {
				t.Fatalf("other actor received private result: %+v", result.Data)
			}
			continue
		}
		var recovered struct {
			Ticket struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			} `json:"ticket"`
		}
		if json.Unmarshal(result.Data.Result, &recovered) != nil || result.Data.Operation != "ticket_reply" || result.Data.Status != "succeeded" || recovered.Ticket.ID != id || recovered.Ticket.Version != 2 {
			t.Fatalf("recovered = %+v / %s", result.Data, result.Data.Result)
		}
	}
	for index, route := range []string{"/api/v1/console/operations/ticket_transition/" + key, "/api/v1/console/operations/ticket_reply/idem_no_known_operation"} {
		response := sendConsoleJSON(t, server.URL, http.MethodGet, operator, route, "operation-unknown-"+fmtInt(int64(index)), "", "")
		if response.StatusCode != http.StatusOK {
			t.Fatalf("unknown lookup = %d: %s", response.StatusCode, responseText(t, response))
		}
		var result struct {
			Data struct {
				Status string          `json:"status"`
				Result json.RawMessage `json:"result"`
			} `json:"data"`
		}
		decodeResponse(t, response, &result)
		if result.Data.Status != "unknown" || len(result.Data.Result) != 0 {
			t.Fatalf("unknown = %+v", result.Data)
		}
	}
	detail := sendOwnerJSON(t, server.URL, http.MethodGet, owner, "/api/v1/account/tickets/"+id, "operation-final-detail", "", "")
	var conversation struct {
		Data struct {
			Messages []json.RawMessage `json:"messages"`
			Events   []json.RawMessage `json:"events"`
		} `json:"data"`
	}
	decodeResponse(t, detail, &conversation)
	if len(conversation.Data.Messages) != 2 || len(conversation.Data.Events) != 1 {
		t.Fatalf("messages/events = %d/%d, want 2/1", len(conversation.Data.Messages), len(conversation.Data.Events))
	}
	notifications := sendOwnerJSON(t, server.URL, http.MethodGet, owner, "/api/v1/account/notifications", "operation-final-notifications", "", "")
	var inbox struct {
		Data struct {
			Notifications []json.RawMessage `json:"notifications"`
		} `json:"data"`
	}
	decodeResponse(t, notifications, &inbox)
	if len(inbox.Data.Notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(inbox.Data.Notifications))
	}
	portal := sendOwnerJSON(t, server.URL, http.MethodGet, owner, route, "operation-portal-denied", "", "")
	defer portal.Body.Close()
	if portal.StatusCode != http.StatusForbidden {
		t.Fatalf("Portal lookup = %d, want 403", portal.StatusCode)
	}
}

func TestConsoleOperationResultIncludesMembershipAndPointsButDoesNotInventFailures(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()
	const user = "b8311111-1111-4111-8111-111111111111"
	const operator = "b8322222-2222-4222-8222-222222222222"
	initialized := sendOwnerJSON(t, server.URL, http.MethodGet, user, "/api/v1/account/summary", "operation-account-initialize", "", "")
	if initialized.StatusCode != 200 {
		t.Fatalf("initialize = %d", initialized.StatusCode)
	}
	initialized.Body.Close()
	for _, command := range []struct {
		operation, path, body, want string
		status                      int
	}{
		{"membership_grant", "/api/v1/console/memberships/" + user + "/grants", `{"reason":"Isolated grant","expected_version":1}`, `"version":2`, 200},
		{"membership_revoke", "/api/v1/console/memberships/" + user + "/revocations", `{"reason":"Isolated revocation","expected_version":2}`, `"version":3`, 200},
		{"point_adjustment", "/api/v1/console/points/adjustments", `{"user_id":"` + user + `","amount":7,"reason":"Isolated adjustment"}`, `"balance":7`, 200},
		{"point_adjustment", "/api/v1/console/points/adjustments", `{"user_id":"` + user + `","amount":-8,"reason":"Rejected debit"}`, `"status":"unknown"`, 409},
	} {
		key := "idem_lookup_" + command.operation + "_" + fmtInt(int64(command.status))
		response := sendConsoleJSON(t, server.URL, http.MethodPost, operator, command.path, key+"-post", key, command.body)
		if response.StatusCode != command.status {
			t.Fatalf("%s = %d: %s", command.operation, response.StatusCode, responseText(t, response))
		}
		response.Body.Close()
		lookup := sendConsoleJSON(t, server.URL, http.MethodGet, operator, "/api/v1/console/operations/"+command.operation+"/"+key, key+"-get", "", "")
		if lookup.StatusCode != 200 {
			t.Fatalf("lookup = %d: %s", lookup.StatusCode, responseText(t, lookup))
		}
		raw := responseText(t, lookup)
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal([]byte(raw), &envelope) != nil {
			t.Fatalf("invalid result: %s", raw)
		}
		var value map[string]json.RawMessage
		_ = json.Unmarshal(envelope.Data, &value)
		// Re-encoding normalizes PostgreSQL jsonb whitespace; expected facts are literal examples.
		compact, _ := json.Marshal(value)
		if !strings.Contains(string(compact), command.want) {
			t.Fatalf("%s lookup = %s, want %s", command.operation, compact, command.want)
		}
		if command.status != 200 && value["result"] != nil {
			t.Fatalf("rolled-back failure has a made-up result: %s", compact)
		}
	}
}
