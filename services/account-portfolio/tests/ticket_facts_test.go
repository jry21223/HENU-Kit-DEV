package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type ticketFactsEnvelope struct {
	Data struct {
		Scope  string           `json:"scope"`
		AsOf   time.Time        `json:"as_of"`
		Stale  *bool            `json:"stale"`
		Counts map[string]int64 `json:"counts"`
		Window struct {
			Filter          string `json:"filter"`
			MatchedTotal    int64  `json:"matched_total"`
			Limit, Returned int
			HasMore         bool `json:"has_more"`
			Items           []struct {
				ID, Reference, Title, Status string
				Version                      int
				UpdatedAt                    time.Time `json:"updated_at"`
			} `json:"items"`
		} `json:"window"`
	} `json:"data"`
}

func TestConsoleTicketFactsCountAllAndReturnMatchingBoundedQueue(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()
	_, err := pool.Exec(t.Context(), `INSERT INTO account_portfolio_accounts(user_id) VALUES('11111111-1111-4111-8111-111111111111');
 INSERT INTO account_portfolio_tickets(id,user_id,title,category,status) SELECT md5('facts-ticket-'||n)::uuid,'11111111-1111-4111-8111-111111111111','受控工单','account',CASE WHEN n<=105 THEN 'open' WHEN n<=108 THEN 'in_progress' ELSE 'resolved' END FROM generate_series(1,115) n`)
	if err != nil {
		t.Fatal(err)
	}
	for _, filter := range []string{"", "unresolved", "all", "open", "in_progress", "resolved"} {
		route := "/api/v1/console/tickets/facts"
		if filter != "" {
			route += "?status=" + filter
		}
		response := sendConsoleJSON(t, server.URL, http.MethodGet, "99999999-9999-4999-8999-999999999999", route, "facts-"+filter, "", "")
		raw := responseText(t, response)
		if response.StatusCode != 200 {
			t.Fatalf("facts status=%d: %s", response.StatusCode, raw)
		}
		var envelope ticketFactsEnvelope
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			t.Fatal(err)
		}
		data := envelope.Data
		if data.Scope != "global" || data.Stale == nil || *data.Stale || data.AsOf.IsZero() {
			t.Fatalf("metadata: %s", raw)
		}
		for key, want := range map[string]int64{"open": 105, "in_progress": 3, "resolved": 7, "unresolved": 108} {
			if data.Counts[key] != want {
				t.Fatalf("%s=%d want%d", key, data.Counts[key], want)
			}
		}
		selected := filter
		if selected == "" {
			selected = "unresolved"
		}
		want := map[string]int64{"unresolved": 108, "all": 115, "open": 105, "in_progress": 3, "resolved": 7}[selected]
		returned := min(int(want), 100)
		w := data.Window
		if w.Filter != selected || w.MatchedTotal != want || w.Limit != 100 || w.Returned != returned || len(w.Items) != returned || w.HasMore != (want > 100) {
			t.Fatalf("window mismatch: %s", raw)
		}
		for _, item := range w.Items {
			if item.Reference != "HKT-"+item.ID || item.Title != "受控工单" || item.Version != 1 || item.UpdatedAt.IsZero() || (selected == "unresolved" && item.Status == "resolved") || (selected != "all" && selected != "unresolved" && item.Status != selected) {
				t.Fatalf("bad row: %+v", item)
			}
		}
		if bytes.Contains([]byte(raw), []byte(`"user_id"`)) || bytes.Contains([]byte(raw), []byte(`"body"`)) {
			t.Fatalf("private fields in facts: %s", raw)
		}
	}
}

func TestConsoleTicketFactsRejectInvalidIdentityQueryAndDependencyFailure(t *testing.T) {
	server, pool := newAccountPortfolioServerWithConsole(t)
	defer server.Close()
	defer pool.Close()
	for n, query := range []string{"?status=invalid", "?status=open&status=resolved", "?campus=minglun"} {
		response := sendConsoleJSON(t, server.URL, "GET", "99999999-9999-4999-8999-999999999999", "/api/v1/console/tickets/facts"+query, fmt.Sprint("facts-invalid-", n), "", "")
		raw := responseText(t, response)
		if response.StatusCode != 400 || bytes.Contains([]byte(raw), []byte(`"counts"`)) {
			t.Fatalf("invalid query=%d %s", response.StatusCode, raw)
		}
	}
	response := sendOwnerJSON(t, server.URL, "GET", "99999999-9999-4999-8999-999999999999", "/api/v1/console/tickets/facts", "facts-portal", "", "")
	if response.StatusCode != 403 {
		t.Fatalf("portal status=%d %s", response.StatusCode, responseText(t, response))
	}
	response.Body.Close()
	response = sendConsoleJSON(t, server.URL, "GET", "invalid", "/api/v1/console/tickets/facts", "facts-bad-actor", "", "")
	if response.StatusCode != 401 {
		t.Fatalf("actor status=%d %s", response.StatusCode, responseText(t, response))
	}
	response.Body.Close()
	if _, err := pool.Exec(t.Context(), `ALTER TABLE account_portfolio_tickets RENAME TO facts_unavailable_tickets`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, err := pool.Exec(context.Background(), `ALTER TABLE facts_unavailable_tickets RENAME TO account_portfolio_tickets`)
		if err != nil {
			t.Error(err)
		}
	}()
	response = sendConsoleJSON(t, server.URL, "GET", "99999999-9999-4999-8999-999999999999", "/api/v1/console/tickets/facts", "facts-db-down", "", "")
	raw := responseText(t, response)
	if response.StatusCode != 503 || bytes.Contains([]byte(raw), []byte(`"counts"`)) {
		t.Fatalf("fake empty success=%d %s", response.StatusCode, raw)
	}
}

func TestConsoleTicketFactsConcurrentSnapshotAndBoundedDeadline(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(fmt.Sprint("expire=", expire), func(t *testing.T) {
			server, pool := newAccountPortfolioServerWithConsole(t)
			defer server.Close()
			defer pool.Close()
			if _, err := pool.Exec(t.Context(), `INSERT INTO account_portfolio_accounts(user_id) VALUES('11111111-1111-4111-8111-111111111111'); INSERT INTO account_portfolio_tickets(id,user_id,title,category,status,version) VALUES('22222222-2222-4222-8222-222222222222','11111111-1111-4111-8111-111111111111','受控并发工单','account','open',1);
   ALTER TABLE account_portfolio_tickets RENAME TO facts_snapshot_tickets;
   CREATE FUNCTION facts_ticket_barrier() RETURNS boolean LANGUAGE plpgsql VOLATILE AS $$ BEGIN PERFORM pg_advisory_xact_lock(81234018); RETURN true; END $$;
   CREATE VIEW account_portfolio_tickets AS SELECT * FROM facts_snapshot_tickets WHERE facts_ticket_barrier()`); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := pool.Exec(context.Background(), `DROP VIEW account_portfolio_tickets; ALTER TABLE facts_snapshot_tickets RENAME TO account_portfolio_tickets; DROP FUNCTION facts_ticket_barrier()`); err != nil {
					t.Error(err)
				}
			}()
			holder, err := pool.Acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := holder.Exec(t.Context(), `SELECT pg_advisory_lock(81234018)`); err != nil {
				holder.Release()
				t.Fatal(err)
			}
			defer func() {
				_, _ = holder.Exec(context.Background(), `SELECT pg_advisory_unlock(81234018)`)
				holder.Release()
			}()
			type outcome struct {
				status int
				body   string
			}
			done := make(chan outcome, 1)
			started := time.Now()
			go func() {
				response := sendConsoleJSON(t, server.URL, "GET", "99999999-9999-4999-8999-999999999999", "/api/v1/console/tickets/facts", fmt.Sprint("facts-concurrent-", expire), "", "")
				done <- outcome{response.StatusCode, responseText(t, response)}
			}()
			deadline := time.After(3 * time.Second)
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				var blocked bool
				if err := pool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=81234018 AND NOT granted)`).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				select {
				case <-deadline:
					t.Fatal("facts never reached database snapshot barrier")
				case <-ticker.C:
				}
			}
			if !expire {
				if _, err := pool.Exec(t.Context(), `UPDATE facts_snapshot_tickets SET status='resolved',version=2,updated_at=clock_timestamp()`); err != nil {
					t.Fatal(err)
				}
				if _, err := holder.Exec(t.Context(), `SELECT pg_advisory_unlock(81234018)`); err != nil {
					t.Fatal(err)
				}
			}
			var result outcome
			select {
			case result = <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("facts deadline did not finish")
			}
			if expire {
				if result.status != 503 || bytes.Contains([]byte(result.body), []byte(`"counts"`)) || time.Since(started) > 2800*time.Millisecond {
					t.Fatalf("deadline response=%+v elapsed=%s", result, time.Since(started))
				}
				return
			}
			var envelope ticketFactsEnvelope
			if err := json.Unmarshal([]byte(result.body), &envelope); err != nil {
				t.Fatal(err)
			}
			data := envelope.Data
			if result.status != 200 || data.Counts["unresolved"] != 1 || data.Counts["resolved"] != 0 || data.Window.Returned != 1 || len(data.Window.Items) != 1 || data.Window.Items[0].Status != "open" || data.Window.Items[0].Version != 1 {
				t.Fatalf("mixed snapshot=%s", result.body)
			}
		})
	}
}
