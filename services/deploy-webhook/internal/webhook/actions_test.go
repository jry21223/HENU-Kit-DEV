package webhook

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func actionsPayload() map[string]any {
	return map[string]any{
		"action":     "completed",
		"repository": map[string]any{"full_name": "jry21223/HENU-Kit-DEV"},
		"workflow":   map[string]any{"path": ".github/workflows/deploy-henukit.yml"},
		"workflow_run": map[string]any{
			"id": 41040034007, "run_attempt": 1, "status": "completed", "conclusion": "success",
			"event": "push", "head_branch": "main", "head_sha": strings.Repeat("a", 40),
			"repository": map[string]any{"full_name": "jry21223/HENU-Kit-DEV"},
		},
	}
}

func TestActionsHandlerAcceptsOnlySuccessfulMainReleaseWorkflow(t *testing.T) {
	queue := &fakeQueue{}
	handler, err := NewActions(Config{Path: "/webhooks/github", Repository: "jry21223/HENU-Kit-DEV", Branch: "main", Secret: testSecret, MaxBodyBytes: 1024 * 1024}, queue, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := deliver(t, handler, "workflow_run", "actions-success", actionsPayload(), true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["notified"] != true || body["deployed"] != nil || body["queued"] != nil {
		t.Fatalf("response claims incorrect operation: %v", body)
	}
	if len(queue.events) != 1 || queue.events[0].After != strings.Repeat("a", 40) || queue.events[0].Ref != "refs/heads/main" {
		t.Fatalf("notification=%v", queue.events)
	}
}

func TestActionsHandlerRejectsUnrelatedOrIncompleteRuns(t *testing.T) {
	for _, tc := range []struct {
		name, section, key string
		value              any
		status             int
	}{
		{"requested", "", "action", "requested", 202},
		{"running", "workflow_run", "status", "in_progress", 202},
		{"failure", "workflow_run", "conclusion", "failure", 202},
		{"cancelled", "workflow_run", "conclusion", "cancelled", 202},
		{"pull request", "workflow_run", "event", "pull_request", 202},
		{"manual dispatch", "workflow_run", "event", "workflow_dispatch", 202},
		{"branch", "workflow_run", "head_branch", "feature", 202},
		{"repository", "repository", "full_name", "someone/else", 202},
		{"nested repository", "workflow_run", "repository", map[string]any{"full_name": "someone/else"}, 202},
		{"workflow", "workflow", "path", ".github/workflows/food.yml", 202},
		{"invalid sha", "workflow_run", "head_sha", "main", 400},
		{"zero sha", "workflow_run", "head_sha", strings.Repeat("0", 40), 400},
		{"sha256", "workflow_run", "head_sha", strings.Repeat("a", 64), 400},
		{"missing attempt", "workflow_run", "run_attempt", 0, 400},
		{"missing id", "workflow_run", "id", 0, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQueue{}
			h, err := NewActions(Config{Path: "/webhooks/github", Repository: "jry21223/HENU-Kit-DEV", Branch: "main", Secret: testSecret, MaxBodyBytes: 1024 * 1024}, q, nil)
			if err != nil {
				t.Fatal(err)
			}
			p := actionsPayload()
			if tc.section == "" {
				p[tc.key] = tc.value
			} else {
				p[tc.section].(map[string]any)[tc.key] = tc.value
			}
			r := deliver(t, h, "workflow_run", "actions-rejected", p, true)
			if r.Code != tc.status || len(q.events) != 0 {
				t.Fatalf("status=%d events=%v body=%s", r.Code, q.events, r.Body)
			}
		})
	}
}

func TestActionsHandlerRetainsSignatureAndPingBoundary(t *testing.T) {
	q := &fakeQueue{}
	h, err := NewActions(Config{Path: "/webhooks/github", Repository: "jry21223/HENU-Kit-DEV", Branch: "main", Secret: testSecret, MaxBodyBytes: 1024 * 1024}, q, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r := deliver(t, h, "workflow_run", "actions-signature", actionsPayload(), false); r.Code != 401 {
		t.Fatalf("bad signature status=%d", r.Code)
	}
	if r := deliver(t, h, "ping", "actions-ping", map[string]any{}, true); r.Code != 200 {
		t.Fatalf("ping status=%d", r.Code)
	}
	if r := deliver(t, h, "push", "actions-push", actionsPayload(), true); r.Code != 202 {
		t.Fatalf("push status=%d", r.Code)
	}
	if len(q.events) != 0 {
		t.Fatal("non-workflow event or invalid signature notified watcher")
	}
}
