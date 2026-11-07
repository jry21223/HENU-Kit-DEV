package noticeowner_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	gateway "henukit.dev/console-gateway"
	noticeclient "henukit.dev/console-gateway/internal/notice"
	"henukit.dev/console-gateway/internal/overview"
	"henukit.dev/console-gateway/internal/session"
	noticeowner "henukit.dev/notice"
)

// Core is an explicit authorization HTTP fixture here, not a replacement for
// Core's PostgreSQL/Redis authorization suite. Notice is its real SQL owner.
func TestConsoleGatewayHTTPRecoversCommittedNoticeReviewThroughOriginalKey(t *testing.T) {
	if os.Getenv("NOTICE_GATEWAY_INTEGRATION_DATABASE_URL") == "" || os.Getenv("NOTICE_GATEWAY_INTEGRATION_REDIS_ADDR") == "" {
		t.Skip("isolated Notice database and Redis are required")
	}
	pool, err := pgxpool.New(t.Context(), os.Getenv("NOTICE_GATEWAY_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("NOTICE_GATEWAY_INTEGRATION_REDIS_ADDR"), DB: 14})
	defer redisClient.Close()
	ownerHandler, err := noticeowner.New(noticeowner.Config{Database: pool, Redis: redisClient, ClientID: clientID, Keys: map[string]string{keyID: secret}})
	if err != nil {
		t.Fatal(err)
	}
	var dropReview atomic.Bool
	dropReview.Store(true)
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews") && dropReview.CompareAndSwap(true, false) {
			recorder := httptest.NewRecorder()
			ownerHandler.ServeHTTP(recorder, r)
			if recorder.Code == http.StatusOK {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			for key, values := range recorder.Header() {
				w.Header()[key] = values
			}
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(recorder.Body.Bytes())
			return
		}
		ownerHandler.ServeHTTP(w, r)
	}))
	defer owner.Close()
	client, _ := noticeclient.New(owner.URL, clientID, secret, keyID, owner.Client())
	actorID, runID := uuid.NewString(), uuid.NewString()
	sourcePayload, err := client.CreateSource(t.Context(), actorID, "idem_http_source_"+runID, []byte(`{"code":"http-`+runID+`","name":"隔离测试来源","canonical_url":"https://example.edu/notices"}`))
	source := call(t, sourcePayload, err)
	versionPayload, err := client.CreateVersion(t.Context(), actorID, stringValue(t, source, "id"), "idem_http_version_"+runID, []byte(`{"title":"隔离通知","body":"不可变的隔离正文","source_url":"https://example.edu/notices/fixture"}`))
	versionID := stringValue(t, call(t, versionPayload, err), "id")
	var allowReview atomic.Bool
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var check struct {
			Permission string            `json:"permission_code"`
			Scope      map[string]string `json:"scope"`
		}
		_ = json.NewDecoder(r.Body).Decode(&check)
		if check.Scope["kind"] != "product" || check.Scope["product_code"] != "notice" || r.Header.Get("X-Signature") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if check.Permission == "notice.read" || allowReview.Load() && (check.Permission == "notice.review" || check.Permission == "notice.distribute") {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer core.Close()
	sessionKey := []byte("0123456789abcdef0123456789abcdef")
	handler, err := gateway.New(gateway.Config{PlatformCoreURL: core.URL, PlatformAccountOrigin: "https://account.henukit.test", ClientID: "console-gateway", ClientSecret: "isolated-core-service-secret-32bytes", KeyID: "isolated-core-key", RedirectURI: "https://console.henukit.test/api/v1/auth/callback", SessionKey: sessionKey, Redis: redisClient, HTTPClient: owner.Client(), NoticeAPIURL: owner.URL, NoticeCredentials: overview.Credentials{ClientID: clientID, ClientSecret: secret, KeyID: keyID}})
	if err != nil {
		t.Fatal(err)
	}
	console := httptest.NewServer(handler)
	defer console.Close()
	codec, _ := session.New(sessionKey)
	cookieFor := func(actor string) *http.Cookie {
		encoded, encodeErr := codec.Encode(session.Value{UserID: actor, ExchangeToken: "isolated_exchange_token_at_least_32_characters", ExpiresAt: time.Now().Add(time.Hour)})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		return &http.Cookie{Name: "__Host-henukit_console_session", Value: encoded}
	}
	request := func(method, path, body, key, actor string) (int, []byte) {
		req, _ := http.NewRequest(method, console.URL+path, strings.NewReader(body))
		req.AddCookie(cookieFor(actor))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		req.Header.Set("X-Actor-User-Id", actorID) // Browser-supplied actor must never override its Session.
		res, requestErr := console.Client().Do(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		return res.StatusCode, data
	}
	reviewPath, reviewBody, key := "/api/v1/notices/versions/"+versionID+"/reviews", `{"decision":"approved","expected_revision":1}`, "idem_http_review_"+runID
	if status, data := request(http.MethodPost, reviewPath, reviewBody, key, actorID); status != http.StatusForbidden {
		t.Fatalf("ungranted review = %d %s", status, data)
	}
	if status, data := request(http.MethodGet, "/api/v1/notices", "", "", actorID); status != http.StatusOK || !strings.Contains(string(data), `"state":"pending_review"`) {
		t.Fatalf("denied write changed owner snapshot: %d %s", status, data)
	}
	allowReview.Store(true)
	if status, data := request(http.MethodPost, reviewPath, reviewBody, key, actorID); status != http.StatusServiceUnavailable {
		t.Fatalf("lost committed response = %d %s", status, data)
	}
	if status, data := request(http.MethodGet, "/api/v1/notices/operations/review", "", key, actorID); status != http.StatusOK || !strings.Contains(string(data), `"state":"approved"`) || !strings.Contains(string(data), `"revision":2`) {
		t.Fatalf("original-key recovery = %d %s", status, data)
	}
	if status, data := request(http.MethodGet, "/api/v1/notices/operations/review", "", key, uuid.NewString()); status != http.StatusOK || !strings.Contains(string(data), `"status":"unknown"`) {
		t.Fatalf("another Session actor discovered the result: %d %s", status, data)
	}
	if status, data := request(http.MethodPost, reviewPath, reviewBody, key, actorID); status != http.StatusOK || !strings.Contains(string(data), `"revision":2`) {
		t.Fatalf("confirmed replay changed revision: %d %s", status, data)
	}
	if status, data := request(http.MethodGet, "/api/v1/notices", "", "", actorID); status != http.StatusOK || !strings.Contains(string(data), "不可变的隔离正文") || !strings.Contains(string(data), `"state":"approved"`) {
		t.Fatalf("owner-confirmed immutable version missing: %d %s", status, data)
	}
}
