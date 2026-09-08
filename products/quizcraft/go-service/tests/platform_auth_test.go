package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
)

func TestFeedbackOutboxDeliversReferenceOnlyToPlatformCore(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "inbox-delivery-"+uuid.NewString())
	const feedbackCount = 25
	delivered := make(chan []byte, feedbackCount)
	deliveryStarted := make(chan struct{})
	releaseDelivery := make(chan struct{})
	var deliveryOnce sync.Once
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/operations-inbox/items" {
			http.NotFound(writer, request)
			return
		}
		body, _ := io.ReadAll(request.Body)
		if request.Header.Get("X-Session-Exchange-Token") != strings.Repeat("i", 40) || request.Header.Get("Idempotency-Key") == "" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		deliveryOnce.Do(func() { close(deliveryStarted) })
		<-releaseDelivery
		delivered <- body
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]string{"id": "22222222-2222-4222-8222-222222222222"}})
	}))
	defer platform.Close()
	workerContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), PlatformCoreURL: platform.URL, PlatformClientID: "quizcraft", PlatformClientSecret: strings.Repeat("s", 40), PlatformKeyID: "key-1", PublicURL: "https://quizcraft.henukit.test", SessionEncryptionKey: []byte("0123456789abcdef0123456789abcdef"), InboxExchangeToken: strings.Repeat("i", 40), WorkerContext: workerContext})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	payload := map[string]any{"bank_id": report.BankID, "question_id": report.Questions[0].QuestionID, "question_version_id": report.Questions[0].QuestionVersionID, "category": "typo", "detail": "secret full feedback body"}
	postFeedback := func(index int) {
		status, responseBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/feedback", map[string]string{"Cookie": "quizcraft_session=" + practiceToken(t, uuid.NewString()), "Idempotency-Key": fmt.Sprintf("inbox-delivery-feedback-%d", index)}, payload)
		if status != http.StatusAccepted {
			t.Fatalf("feedback %d = %d %s", index, status, responseBody)
		}
	}
	postFeedback(0)
	<-deliveryStarted
	for index := 1; index < feedbackCount; index++ {
		postFeedback(index)
	}
	close(releaseDelivery)
	deadline := time.After(5 * time.Second)
	for index := 0; index < feedbackCount; index++ {
		select {
		case body := <-delivered:
			if bytes.Contains(body, []byte("secret full feedback body")) || !bytes.Contains(body, []byte(`"source_product_code":"quizcraft"`)) || !bytes.Contains(body, []byte(`"source_resource_type":"feedback"`)) {
				t.Fatalf("Inbox payload boundary = %s", body)
			}
		case <-deadline:
			t.Fatalf("only %d/%d feedback references were delivered to Operations Inbox", index, feedbackCount)
		}
	}
}

func TestFeedbackStatusProjectsOperationsInboxStateAndListsOwnedFeedback(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "feedback-status-projection-"+uuid.NewString())
	const platformItemID = "33333333-3333-4333-8333-333333333333"
	projected := make(chan struct{}, 1)
	var inboxStatusReads atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Session-Exchange-Token") != strings.Repeat("i", 40) || request.Header.Get("X-Signature") == "" || request.Header.Get("X-Service-Id") != "quizcraft" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/v1/operations-inbox/items":
			if request.Method != http.MethodPost {
				writer.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]string{"id": platformItemID}})
		case "/api/v1/operations-inbox/items/" + platformItemID:
			if request.Method != http.MethodGet || request.URL.Query().Get("source_product_code") != "quizcraft" || request.URL.Query().Get("source_resource_type") != "feedback" || request.URL.Query().Get("source_resource_id") == "" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			select {
			case projected <- struct{}{}:
			default:
			}
			status := "resolved"
			version := int64(3)
			updatedAt := "2026-07-28T00:05:00Z"
			if inboxStatusReads.Add(1) > 1 {
				// Simulate a stale in-flight read arriving after the newer state.
				status = "blocked"
				version = 2
				updatedAt = "2026-07-28T00:04:00Z"
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{
				"id": platformItemID, "source_product_code": "quizcraft", "source_resource_type": "feedback", "source_resource_id": request.URL.Query().Get("source_resource_id"),
				"priority": "high", "status": status, "version": version, "created_at": "2026-07-28T00:00:00Z", "updated_at": updatedAt,
			}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer platform.Close()
	workerContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), PlatformCoreURL: platform.URL,
		PlatformClientID: "quizcraft", PlatformClientSecret: strings.Repeat("s", 40), PlatformKeyID: "key-1",
		PublicURL: "https://quizcraft.henukit.test", SessionEncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		InboxExchangeToken: strings.Repeat("i", 40), WorkerContext: workerContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	payload := map[string]any{"bank_id": report.BankID, "question_id": report.Questions[0].QuestionID, "question_version_id": report.Questions[0].QuestionVersionID, "category": "wrong_answer", "detail": "状态应来自 Operations Inbox"}
	createStatus, createBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/feedback", map[string]string{"Cookie": auth, "Idempotency-Key": "feedback-status-projection-001"}, payload)
	if createStatus != http.StatusAccepted {
		t.Fatalf("create feedback = %d %s", createStatus, createBody)
	}
	feedbackID := operationResourceID(t, createBody)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var deliveredCount int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_feedback_inbox_outbox o JOIN quizcraft_feedback_inbox_deliveries d ON d.outbox_id=o.id WHERE o.feedback_id=$1 AND d.platform_item_id=$2`, feedbackID, platformItemID).Scan(&deliveredCount); err != nil {
			t.Fatal(err)
		}
		if deliveredCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("current feedback's Operations Inbox delivery was not recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, statusBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/feedback/"+feedbackID+"/status", map[string]string{"Cookie": auth}, nil)
	if status != http.StatusOK || !bytes.Contains(statusBody, []byte(`"status":"resolved"`)) {
		t.Fatalf("projected feedback status = %d %s", status, statusBody)
	}
	select {
	case <-projected:
	case <-time.After(5 * time.Second):
		t.Fatal("feedback status did not read Operations Inbox")
	}
	staleStatus, staleBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/feedback/"+feedbackID+"/status", map[string]string{"Cookie": auth}, nil)
	if staleStatus != http.StatusOK || !bytes.Contains(staleBody, []byte(`"status":"resolved"`)) {
		t.Fatalf("a stale Operations Inbox read regressed feedback status = %d %s", staleStatus, staleBody)
	}
	listStatus, listBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/feedback", map[string]string{"Cookie": auth}, nil)
	if listStatus != http.StatusOK || !bytes.Contains(listBody, []byte(`"feedback_id":"`+feedbackID+`"`)) || !bytes.Contains(listBody, []byte(`"status":"resolved"`)) {
		t.Fatalf("recoverable owned feedback list = %d %s", listStatus, listBody)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_feedback_status_facts SET status='pending' WHERE feedback_id=$1`, feedbackID); err == nil {
		t.Fatal("feedback status fact mutation succeeded")
	}
}

func TestIndependentOAuthRoutesAreRetired(t *testing.T) {
	pool := practicePool(t)
	var platformRequests atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		platformRequests.Add(1)
		http.NotFound(writer, request)
	}))
	defer platform.Close()
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), PlatformCoreURL: platform.URL, PlatformClientID: "quizcraft", PlatformClientSecret: strings.Repeat("s", 40), PlatformKeyID: "key-1", PublicURL: "https://quizcraft.henukit.test", SessionEncryptionKey: []byte("0123456789abcdef0123456789abcdef")})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	for _, path := range []string{"/auth/login?return_to=%2Fpractice", "/auth/callback?code=retired&state=retired"} {
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound || response.Header.Get("Location") != "" || len(response.Cookies()) != 0 {
			t.Fatalf("retired OAuth route %s = %d location=%q cookies=%+v", path, response.StatusCode, response.Header.Get("Location"), response.Cookies())
		}
	}
	if platformRequests.Load() != 0 {
		t.Fatalf("retired OAuth routes made %d Platform Core requests", platformRequests.Load())
	}
}

func TestWorkshopFailsClosedWithoutPlatformCore(t *testing.T) {
	pool := practicePool(t)
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	localClaims := "quizcraft_session=" + workshopToken(t, uuid.NewString(), []string{"quizcraft.workshop.read"}, []map[string]string{{"kind": "product", "product_code": "quizcraft"}})
	status, _ := requestJSON(t, http.MethodGet, server.URL+"/api/v1/workshop/catalog", map[string]string{"Cookie": localClaims}, nil)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("Workshop without Platform Core = %d, want 503", status)
	}
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %s missing", name)
	return nil
}
