package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestFoodFactsKeepAuthorizationAndRejectCachedFallback(t *testing.T) {
	server, pool := newFoodServer(t)
	t.Cleanup(server.Close)
	t.Cleanup(pool.Close)
	const actor = "99999999-9999-4999-8999-999999999999"
	for _, tc := range []struct {
		name, permission, path string
		headers                map[string]string
		want                   int
	}{
		{"wrong permission", "food.review", "/api/v1/workspace/facts", nil, 403},
		{"wrong product", "food.read", "/api/v1/workspace/facts", map[string]string{"X-Product-Code": "notice"}, 403},
		{"wrong scope kind", "food.read", "/api/v1/workspace/facts", map[string]string{"X-Scope-Kind": "platform"}, 403},
		{"unbound actor", "food.read", "/api/v1/workspace/facts", map[string]string{"X-Actor-User-Id": "invalid"}, 401},
		{"campus not global", "food.read", "/api/v1/workspace/facts?campus=minglun", nil, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := sendAs(t, server.URL, actor, tc.permission, http.MethodGet, tc.path, nil, "", tc.headers)
			body := readBody(t, response)
			if response.StatusCode != tc.want || bytes.Contains(body, []byte(`"counts"`)) {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
		})
	}
	// The old Workspace has a real 24h fallback, which facts must not relabel fresh.
	if response := send(t, server.URL, "food.read", http.MethodGet, "/api/v1/workspace", nil, ""); response.StatusCode != 200 {
		t.Fatal(string(readBody(t, response)))
	} else {
		_ = readBody(t, response)
	}
	if _, err := pool.Exec(t.Context(), `ALTER TABLE food_submissions RENAME TO facts_unavailable_submissions`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `ALTER TABLE facts_unavailable_submissions RENAME TO food_submissions`); err != nil {
			t.Error(err)
		}
	})
	response := send(t, server.URL, "food.read", http.MethodGet, "/api/v1/workspace/facts", nil, "")
	body := readBody(t, response)
	if response.StatusCode != 503 || bytes.Contains(body, []byte(`"counts"`)) {
		t.Fatalf("dependency failure fabricated facts: %d %s", response.StatusCode, body)
	}
}

func TestFoodFactsKeepCountsAndReferencesInOneConcurrentReadSnapshot(t *testing.T) {
	server, pool := newFoodServer(t)
	t.Cleanup(server.Close)
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(t.Context(), `INSERT INTO food_submissions(id,venue_name,item_name,description,status,version) VALUES('f1111111-1111-4111-8111-111111111111','受控地点','受控菜品','facts-read-barrier','pending',1)`); err != nil {
		t.Fatal(err)
	}
	// A test-only view blocks the read after PostgreSQL has acquired its MVCC
	// snapshot. It does not inspect or depend on the implementation's query text.
	if _, err := pool.Exec(t.Context(), `ALTER TABLE food_submissions RENAME TO facts_snapshot_submissions;
		CREATE FUNCTION facts_read_barrier() RETURNS boolean LANGUAGE plpgsql VOLATILE AS $$ BEGIN PERFORM pg_advisory_xact_lock(81234017); RETURN true; END $$;
		CREATE VIEW food_submissions AS SELECT * FROM facts_snapshot_submissions WHERE facts_read_barrier()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP VIEW food_submissions; ALTER TABLE facts_snapshot_submissions RENAME TO food_submissions; DROP FUNCTION facts_read_barrier(); DELETE FROM food_submissions WHERE description='facts-read-barrier'`); err != nil {
			t.Error(err)
		}
	})
	holder, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(t.Context(), `SELECT pg_advisory_lock(81234017)`); err != nil {
		holder.Release()
		t.Fatal(err)
	}
	defer func() {
		_, _ = holder.Exec(context.Background(), `SELECT pg_advisory_unlock(81234017)`)
		holder.Release()
	}()
	done := make(chan []byte, 1)
	go func() {
		response := send(t, server.URL, "food.read", http.MethodGet, "/api/v1/workspace/facts", nil, "")
		done <- readBody(t, response)
	}()
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := pool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=81234017 AND NOT granted)`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case <-deadline:
			t.Fatal("facts read never reached the snapshot barrier")
		case <-ticker.C:
		}
	}
	if _, err := pool.Exec(t.Context(), `UPDATE facts_snapshot_submissions SET status='approved',version=2,updated_at=clock_timestamp() WHERE description='facts-read-barrier'`); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(t.Context(), `SELECT pg_advisory_unlock(81234017)`); err != nil {
		t.Fatal(err)
	}
	var body []byte
	select {
	case body = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("facts read did not finish after releasing barrier")
	}
	data := decodeData(t, body)
	counts := data["counts"].(map[string]any)
	window := data["windows"].(map[string]any)["submissions"].(map[string]any)
	items := window["items"].([]any)
	if counts["pending_submissions"] != float64(1) || len(items) != 1 || items[0].(map[string]any)["status"] != "pending" || items[0].(map[string]any)["version"] != float64(1) {
		t.Fatalf("concurrent update mixed snapshots: %s", body)
	}
}

func TestFoodFactsCountsWholeQueuesAndBoundsMatchingReferences(t *testing.T) {
	server, pool := newFoodServer(t)
	t.Cleanup(server.Close)
	t.Cleanup(pool.Close)
	for _, statement := range []string{
		`INSERT INTO food_submissions(id,venue_name,item_name,description,status,version) SELECT md5('facts-sub-'||n)::uuid,'受控地点','受控菜品','facts-contract-fixture',CASE WHEN n<=205 THEN 'pending' ELSE 'approved' END,1 FROM generate_series(1,207) n`,
		`INSERT INTO food_anomaly_tickets(id,venue_name,kind,details,severity,status,version) SELECT md5('facts-anomaly-'||n)::uuid,'受控地点','quality','facts-contract-fixture','low',CASE WHEN n<=203 THEN 'open' ELSE 'resolved' END,1 FROM generate_series(1,205) n`,
		`INSERT INTO food_tier_adjustments(id,venue_name,current_tier,proposed_tier,reason,status,version) SELECT md5('facts-tier-'||n)::uuid,'受控地点','standard','recommended','facts-contract-fixture',CASE WHEN n<=202 THEN 'pending' ELSE 'confirmed' END,1 FROM generate_series(1,204) n`,
	} {
		if _, err := pool.Exec(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, query := range []string{`DELETE FROM food_submissions WHERE description='facts-contract-fixture'`, `DELETE FROM food_anomaly_tickets WHERE details='facts-contract-fixture'`, `DELETE FROM food_tier_adjustments WHERE reason='facts-contract-fixture'`} {
			if _, err := pool.Exec(context.Background(), query); err != nil {
				t.Error(err)
			}
		}
	})
	before := time.Now().UTC()
	response := send(t, server.URL, "food.read", http.MethodGet, "/api/v1/workspace/facts", nil, "")
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("facts status=%d body=%s", response.StatusCode, body)
	}
	var envelope struct {
		Data struct {
			Scope   string           `json:"scope"`
			AsOf    time.Time        `json:"as_of"`
			Stale   *bool            `json:"stale"`
			Counts  map[string]int64 `json:"counts"`
			Windows map[string]struct {
				Filter   string `json:"filter"`
				Limit    int    `json:"limit"`
				Returned int    `json:"returned"`
				HasMore  bool   `json:"has_more"`
				Items    []struct {
					ID, Status string
					Version    int
					UpdatedAt  time.Time `json:"updated_at"`
				} `json:"items"`
			} `json:"windows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope.Data
	if data.Scope != "global" || data.Stale == nil || *data.Stale || data.AsOf.Before(before.Add(-time.Second)) || data.AsOf.After(time.Now().Add(time.Second)) {
		t.Fatalf("invalid fresh owner metadata: %s", body)
	}
	for field, expected := range map[string]int64{"pending_submissions": 205, "open_anomalies": 203, "pending_tier_adjustments": 202} {
		if data.Counts[field] != expected {
			t.Fatalf("count %s=%d want %d", field, data.Counts[field], expected)
		}
	}
	for name, status := range map[string]string{"submissions": "pending", "anomalies": "open", "tier_adjustments": "pending"} {
		window := data.Windows[name]
		if window.Filter != status || window.Limit != 200 || window.Returned != 200 || len(window.Items) != 200 || !window.HasMore {
			t.Fatalf("window %s omitted bounded metadata: %+v", name, window)
		}
		for _, item := range window.Items {
			if item.ID == "" || item.Status != status || item.Version != 1 || item.UpdatedAt.IsZero() {
				t.Fatalf("window %s contains invalid reference: %+v", name, item)
			}
		}
	}
}
