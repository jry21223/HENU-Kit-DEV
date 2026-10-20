package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"henukit.dev/console-gateway/internal/platformcore"
	quizcraftapi "henukit.dev/console-gateway/internal/quizcraft"
	"henukit.dev/console-gateway/internal/session"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type managementPlatform struct {
	fakePlatform
	permission, bank, token string
	denial                  error
}

func (p *managementPlatform) CheckQuizcraft(_ context.Context, token, permission, bank string) error {
	p.permission, p.bank, p.token = permission, bank, token
	return p.denial
}
func TestQuizcraftManagementRequiresConsoleSessionPermissionAndSameOrigin(t *testing.T) {
	calls := 0
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/console/workshop/banks" || r.Header.Get("X-Actor-User-Id") != "171f1c6f-7b10-4c92-91a2-b39bf5af5302" || r.Header.Get("X-Permission-Code") != "quizcraft.workshop.write" {
			t.Errorf("invalid owner request")
		}
		if r.Header.Get("Cookie") != "" || strings.Contains(r.Header.Get("Authorization"), "exchange_token") {
			t.Error("browser credential leaked")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"request_id":"req_owner","data":{"resource_id":"bank"}}`)
	}))
	defer owner.Close()
	client, err := quizcraftapi.New(owner.URL, "console-management", "management-dedicated-secret-at-least-32-bytes", "management-1", owner.Client())
	if err != nil {
		t.Fatal(err)
	}
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	platform := &managementPlatform{}
	h, err := New("https://account.test", "console-gateway", "https://console.test/api/v1/auth/callback", platform, nil, fakeOverview{}, redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), codec, nil, nil, nil, nil, client)
	if err != nil {
		t.Fatal(err)
	}
	cookie, _ := codec.Encode(session.Value{UserID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", ExchangeToken: "exchange_token_with_at_least_32_characters", ExpiresAt: time.Now().Add(time.Minute)})
	requestKey := "console-management-create"
	call := func(auth bool, origin string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", "https://console.test/api/v1/quizcraft/workshop/banks", strings.NewReader(`{"bank_key":"review","name":"复核"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("Idempotency-Key", requestKey)
		if auth {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("status=%d body=%s want=%d", w.Code, w.Body.String(), want)
		}
		if !json.Valid(w.Body.Bytes()) {
			t.Fatal("invalid envelope")
		}
	}
	call(false, "https://console.test", 401)
	call(true, "https://evil.test", 403)
	platform.denial = platformcore.ErrForbidden
	call(true, "https://console.test", 403)
	if calls != 0 {
		t.Fatal("denied call reached owner")
	}
	platform.denial = nil
	call(true, "https://console.test", 201)
	if calls != 1 || platform.permission != "quizcraft.workshop.write" || platform.bank != "" || platform.token == "" {
		t.Fatal("missing exact authorization")
	}
	for _, size := range []int{8, 15, 16, 160, 161, 200} {
		requestKey = strings.Repeat("k", size)
		want := 400
		if size >= 16 && size <= 160 {
			want = 201
		}
		call(true, "https://console.test", want)
	}
}

func TestQuizcraftManagementRealOwnerLifecycle(t *testing.T) {
	ownerURL := os.Getenv("CONSOLE_GATEWAY_TEST_QUIZCRAFT_URL")
	if ownerURL == "" {
		t.Skip("real QuizCraft owner URL is required")
	}
	owner, err := quizcraftapi.New(ownerURL, "console-management", "management-separate-secret-at-least-32-bytes", "management-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	platform := &managementPlatform{}
	h, err := New("https://account.test", "console-gateway", "https://console.test/api/v1/auth/callback", platform, nil, fakeOverview{}, redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}), codec, nil, nil, nil, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	cookie, _ := codec.Encode(session.Value{UserID: uuid.NewString(), ExchangeToken: "console_real_session_test_exchange_token", ExpiresAt: time.Now().Add(time.Minute)})
	call := func(method, path, key string, body any, want int) []byte {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, "https://console.test/api/v1/quizcraft/workshop"+path, strings.NewReader(string(raw)))
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		r.Header.Set("Origin", "https://console.test")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s=%d %s, want %d", method, path, w.Code, w.Body.String(), want)
		}
		return w.Body.Bytes()
	}
	created := call("POST", "/banks", "integration-create-bank", map[string]any{"bank_key": "gateway-" + uuid.NewString(), "name": "真实管理旅程验证"}, 201)
	var operation struct {
		Data struct {
			ResourceID string `json:"resource_id"`
		}
	}
	if err = json.Unmarshal(created, &operation); err != nil {
		t.Fatal(err)
	}
	bank := operation.Data.ResourceID
	payload := map[string]any{"expected_version": 1, "source_sha256": strings.Repeat("a", 64), "questions": []map[string]any{{"source_question_id": "q1", "type": "single", "chapter_id": "ch1", "content": "1+1=?", "options": []string{"1", "2"}, "answer": 1, "analysis": "1+1=2"}}}
	imported := call("POST", "/banks/"+bank+"/imports", "integration-import-bank", payload, 201)
	var report struct {
		Data struct {
			BankVersionID string `json:"bank_version_id"`
		}
	}
	_ = json.Unmarshal(imported, &report)
	if report.Data.BankVersionID == "" {
		t.Fatal("no imported version")
	}
	path := fmt.Sprintf("/banks/%s/versions/%s", bank, report.Data.BankVersionID)
	detail := call("GET", path, "", nil, 200)
	if !strings.Contains(string(detail), `"analysis":"1+1=2"`) {
		t.Fatal("full review omitted analysis")
	}
	call("POST", path+"/publish", "integration-premature", map[string]any{"expected_version": 2}, 409)
	call("POST", path+"/validate", "integration-validate", map[string]any{"expected_version": 2, "note": "完整题目复核"}, 200)
	call("POST", path+"/publish", "integration-publish", map[string]any{"expected_version": 3}, 200)
	if platform.permission != "quizcraft.workshop.publish" || platform.bank != bank {
		t.Fatal("resource permission not checked")
	}
}
