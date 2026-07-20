package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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

func TestWorkshopUsesPlatformCoreOAuthAndLiveAuthorization(t *testing.T) {
	pool := practicePool(t)
	var mu sync.Mutex
	var checked map[string]any
	var exchangeKeys []string
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Signature") == "" || request.Header.Get("X-Service-Id") != "quizcraft" {
			t.Errorf("missing signed service identity")
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/api/v1/oauth/token":
			mu.Lock()
			exchangeKeys = append(exchangeKeys, request.Header.Get("Idempotency-Key"))
			mu.Unlock()
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"user": map[string]string{"id": "11111111-1111-4111-8111-111111111111"}, "session_exchange_token": strings.Repeat("x", 40), "expires_at": time.Now().Add(time.Hour).UTC()}})
		case "/api/v1/authorization/check":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			mu.Lock()
			checked = body
			mu.Unlock()
			_ = json.NewEncoder(writer).Encode(map[string]any{"data": map[string]any{"allowed": true}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer platform.Close()
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), PlatformCoreURL: platform.URL, PlatformClientID: "quizcraft", PlatformClientSecret: strings.Repeat("s", 40), PlatformKeyID: "key-1", PublicURL: "https://quizcraft.henukit.test", SessionEncryptionKey: []byte("0123456789abcdef0123456789abcdef")})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	login, err := client.Get(server.URL + "/auth/login?return_to=%2Fextract")
	if err != nil {
		t.Fatal(err)
	}
	if login.StatusCode != http.StatusFound {
		t.Fatalf("login = %d", login.StatusCode)
	}
	oauthCookie := findCookie(t, login.Cookies(), "__Host-quizcraft_oauth")
	redirect, _ := url.Parse(login.Header.Get("Location"))
	state := redirect.Query().Get("state")
	_ = login.Body.Close()
	callback, _ := http.NewRequest(http.MethodGet, server.URL+"/auth/callback?code=single-use-code&state="+url.QueryEscape(state), nil)
	callback.AddCookie(oauthCookie)
	callbackResponse, err := client.Do(callback)
	if err != nil {
		t.Fatal(err)
	}
	if callbackResponse.StatusCode != http.StatusFound || callbackResponse.Header.Get("Location") != "/extract" {
		t.Fatalf("callback = %d %s", callbackResponse.StatusCode, callbackResponse.Header.Get("Location"))
	}
	sessionCookie := findCookie(t, callbackResponse.Cookies(), "__Host-quizcraft_session")
	_ = callbackResponse.Body.Close()
	retryCallback, _ := http.NewRequest(http.MethodGet, server.URL+"/auth/callback?code=single-use-code&state="+url.QueryEscape(state), nil)
	retryCallback.AddCookie(oauthCookie)
	retried, err := client.Do(retryCallback)
	if err != nil {
		t.Fatal(err)
	}
	_ = retried.Body.Close()
	mu.Lock()
	keys := append([]string(nil), exchangeKeys...)
	mu.Unlock()
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("OAuth exchange idempotency keys = %#v", keys)
	}
	create, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/workshop/banks", strings.NewReader(`{"bank_key":"oauth-bank","name":"OAuth Bank"}`))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Idempotency-Key", "oauth-bank-create")
	create.AddCookie(sessionCookie)
	created, err := client.Do(create)
	if err != nil {
		t.Fatal(err)
	}
	defer created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("Platform-authorized create = %d", created.StatusCode)
	}
	mu.Lock()
	decision := checked
	mu.Unlock()
	scope, _ := decision["scope"].(map[string]any)
	if decision["permission_code"] != "quizcraft.workshop.write" || scope["kind"] != "product" || scope["product_code"] != "quizcraft" || decision["session_exchange_token"] != strings.Repeat("x", 40) {
		t.Fatalf("authorization check = %#v", decision)
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
