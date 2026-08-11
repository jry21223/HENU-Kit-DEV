package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"henukit.dev/deploy-webhook/internal/state"
)

type fakeQueue struct {
	events   []state.Event
	result   state.EnqueueResult
	err      error
	snapshot state.Snapshot
}

func (f *fakeQueue) Enqueue(event state.Event) (state.EnqueueResult, error) {
	f.events = append(f.events, event)
	if f.result == (state.EnqueueResult{}) {
		f.result.Queued = true
	}
	return f.result, f.err
}

func (f *fakeQueue) Snapshot() (state.Snapshot, error) { return f.snapshot, f.err }

func TestVerifySignatureOfficialVector(t *testing.T) {
	secret := []byte("It's a Secret to Everybody")
	body := []byte("Hello, World!")
	signature := "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	if !verifySignature(secret, body, signature) {
		t.Fatal("official GitHub signature vector did not verify")
	}
	if verifySignature(secret, body, signature[:len(signature)-1]+"0") {
		t.Fatal("tampered signature verified")
	}
}

func TestHandlerQueuesValidMainPush(t *testing.T) {
	queue := &fakeQueue{}
	handler := newTestHandler(t, queue, 1024*1024)
	payload := map[string]any{
		"ref":    "refs/heads/main",
		"before": "1111111111111111111111111111111111111111",
		"after":  "2222222222222222222222222222222222222222",
		"repository": map[string]any{
			"full_name": "jry21223/HENU-Kit-DEV",
		},
	}
	response := deliver(t, handler, "push", "delivery-1", payload, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(queue.events) != 1 {
		t.Fatalf("queued events = %d, want 1", len(queue.events))
	}
	if queue.events[0].After != "2222222222222222222222222222222222222222" {
		t.Fatalf("queued SHA = %q", queue.events[0].After)
	}
}

func TestHandlerReportsWhenAQueuedPushWasCoalesced(t *testing.T) {
	queue := &fakeQueue{result: state.EnqueueResult{Queued: true, Coalesced: true}}
	handler := newTestHandler(t, queue, 1024*1024)
	payload := map[string]any{
		"ref":    "refs/heads/main",
		"before": "1111111111111111111111111111111111111111",
		"after":  "2222222222222222222222222222222222222222",
		"repository": map[string]any{
			"full_name": "jry21223/HENU-Kit-DEV",
		},
	}
	response := deliver(t, handler, "push", "delivery-coalesced", payload, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"coalesced":true`)) {
		t.Fatalf("response does not expose coalescing: %s", response.Body.String())
	}
}

func TestHandlerRejectsInvalidSignature(t *testing.T) {
	queue := &fakeQueue{}
	handler := newTestHandler(t, queue, 1024)
	response := deliver(t, handler, "push", "delivery-2", map[string]any{
		"ref": "refs/heads/main", "after": "2222222222222222222222222222222222222222",
		"repository": map[string]any{"full_name": "jry21223/HENU-Kit-DEV"},
	}, false)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if len(queue.events) != 0 {
		t.Fatal("invalid signature was queued")
	}
}

func TestHandlerIgnoresWrongRepositoryOrBranch(t *testing.T) {
	for name, payload := range map[string]map[string]any{
		"repository": {
			"ref": "refs/heads/main", "after": "2222222222222222222222222222222222222222",
			"repository": map[string]any{"full_name": "someone/else"},
		},
		"branch": {
			"ref": "refs/heads/feature", "after": "2222222222222222222222222222222222222222",
			"repository": map[string]any{"full_name": "jry21223/HENU-Kit-DEV"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			queue := &fakeQueue{}
			handler := newTestHandler(t, queue, 1024)
			response := deliver(t, handler, "push", "delivery-"+name, payload, true)
			if response.Code != http.StatusAccepted {
				t.Fatalf("status = %d", response.Code)
			}
			if len(queue.events) != 0 {
				t.Fatal("mismatched push was queued")
			}
		})
	}
}

func TestHandlerAcceptsPingAndIgnoresOtherEvents(t *testing.T) {
	queue := &fakeQueue{}
	handler := newTestHandler(t, queue, 1024)
	if response := deliver(t, handler, "ping", "delivery-ping", map[string]any{"zen": "ok"}, true); response.Code != http.StatusOK {
		t.Fatalf("ping status = %d", response.Code)
	}
	if response := deliver(t, handler, "issues", "delivery-issues", map[string]any{"action": "opened"}, true); response.Code != http.StatusAccepted {
		t.Fatalf("issues status = %d", response.Code)
	}
	if len(queue.events) != 0 {
		t.Fatal("non-push event was queued")
	}
}

func TestHandlerRejectsOversizedBody(t *testing.T) {
	queue := &fakeQueue{}
	handler := newTestHandler(t, queue, 32)
	body := bytes.Repeat([]byte("x"), 64)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Delivery", "delivery-large")
	request.Header.Set("X-GitHub-Event", "push")
	request.Header.Set("X-Hub-Signature-256", sign(testSecret, body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.Code)
	}
}

func TestStatusEndpointDoesNotRequireWebhookSignature(t *testing.T) {
	queue := &fakeQueue{snapshot: state.Snapshot{QueueDepth: 2}}
	handler := newTestHandler(t, queue, 1024)
	request := httptest.NewRequest(http.MethodGet, "/statusz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	data, _ := io.ReadAll(response.Body)
	if !bytes.Contains(data, []byte(`"queue_depth":2`)) {
		t.Fatalf("unexpected status body: %s", data)
	}
}

var testSecret = []byte("0123456789abcdef0123456789abcdef")

func newTestHandler(t *testing.T, queue Queue, maxBody int64) *Handler {
	t.Helper()
	handler, err := New(Config{
		Path: "/webhooks/github", Repository: "jry21223/HENU-Kit-DEV", Branch: "main",
		Secret: testSecret, MaxBodyBytes: maxBody,
	}, queue, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func deliver(t *testing.T, handler http.Handler, event, delivery string, payload any, validSignature bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", event)
	request.Header.Set("X-GitHub-Delivery", delivery)
	signature := sign(testSecret, body)
	if !validSignature {
		signature = "sha256=" + hex.EncodeToString(bytes.Repeat([]byte{0xff}, sha256.Size))
	}
	request.Header.Set("X-Hub-Signature-256", signature)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
