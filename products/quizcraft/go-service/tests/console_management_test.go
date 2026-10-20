package tests

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const managementSecret = "management-separate-secret-at-least-32-bytes"

func managementRequest(t *testing.T, method, target, actor, permission, bank, key string, payload any) *http.Request {
	t.Helper()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	r, _ := http.NewRequest(method, target, bytes.NewReader(body))
	n := make([]byte, 24)
	_, _ = rand.Read(n)
	nonce := base64.RawURLEncoding.EncodeToString(n)
	ts := fmt.Sprint(time.Now().Unix())
	d := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(managementSecret))
	_, _ = mac.Write([]byte(strings.Join([]string{method, r.URL.RequestURI(), ts, nonce, hex.EncodeToString(d[:]), actor, permission, bank, key}, "\n")))
	r.SetBasicAuth("console-management", managementSecret)
	for k, v := range map[string]string{"X-Service-Id": "console-management", "X-Key-Id": "management-1", "X-Timestamp": ts, "X-Nonce": nonce, "X-Signature": base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), "X-Actor-User-Id": actor, "X-Permission-Code": permission, "X-Resource-Id": bank, "Idempotency-Key": key} {
		r.Header.Set(k, v)
	}
	return r
}
func TestConsoleManagementLifecycleAndSignedBoundary(t *testing.T) {
	pool := practicePool(t)
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), ConsoleManagementEnabled: true, ConsoleManagementClientID: "console-management", ConsoleManagementKeys: map[string]string{"management-1": managementSecret}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	actor := uuid.NewString()
	base := server.URL + "/api/v1/console/workshop"
	call := func(method, path, permission, bank, key string, payload any, want int) []byte {
		t.Helper()
		r := managementRequest(t, method, base+path, actor, permission, bank, key, payload)
		res, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s=%d %s, want %d", method, path, res.StatusCode, b, want)
		}
		return b
	}
	payload := map[string]any{"bank_key": "console-management-" + uuid.NewString(), "name": "管理边界验证"}
	created := call("POST", "/banks", "quizcraft.workshop.write", "", "management-create", payload, 201)
	bank := operationResourceID(t, created)
	replay := call("POST", "/banks", "quizcraft.workshop.write", "", "management-create", payload, 201)
	if !bytes.Equal(created, replay) {
		t.Fatal("idempotent create changed")
	}
	questions := []map[string]any{{"source_question_id": "source-5", "type": "single", "chapter_id": "ch1", "chapter": "网络基础", "content": "1+1=?", "options": []string{"1", "2"}, "answer": 1, "analysis": "1+1=2"}}
	call("POST", "/banks/"+bank+"/imports", "quizcraft.workshop.write", bank, "management-missing-version", map[string]any{"source_sha256": strings.Repeat("a", 64), "questions": questions}, 400)
	imported := call("POST", "/banks/"+bank+"/imports", "quizcraft.workshop.write", bank, "management-import", map[string]any{"expected_version": 1, "source_sha256": strings.Repeat("a", 64), "questions": questions}, 201)
	var env struct {
		Data struct {
			BankVersionID string `json:"bank_version_id"`
		}
	}
	_ = json.Unmarshal(imported, &env)
	version := env.Data.BankVersionID
	path := "/banks/" + bank + "/versions/" + version
	detail := call("GET", path, "quizcraft.workshop.read", bank, "", nil, 200)
	if !bytes.Contains(detail, []byte(`"source_question_id":"source-5"`)) || !bytes.Contains(detail, []byte(`"analysis":"1+1=2"`)) {
		t.Fatalf("incomplete version: %s", detail)
	}
	call("POST", path+"/publish", "quizcraft.workshop.publish", bank, "management-premature", map[string]any{"expected_version": 2}, 409)
	call("POST", path+"/validate", "quizcraft.workshop.write", bank, "management-validate", map[string]any{"expected_version": 2, "note": "完整版本逐题复核"}, 200)
	call("POST", path+"/publish", "quizcraft.workshop.publish", bank, "management-stale", map[string]any{"expected_version": 2}, 409)
	call("POST", path+"/publish", "quizcraft.workshop.publish", bank, "management-publish", map[string]any{"expected_version": 3}, 200)
	// A corrected analysis creates a new immutable version while preserving learning identity.
	var first struct {
		Data struct {
			Questions []struct {
				QuestionID        string `json:"question_id"`
				QuestionVersionID string `json:"question_version_id"`
			} `json:"questions"`
		}
	}
	if err := json.Unmarshal(detail, &first); err != nil {
		t.Fatal(err)
	}
	questions[0]["analysis"] = "两个一相加得到二"
	changed := call("POST", "/banks/"+bank+"/imports", "quizcraft.workshop.write", bank, "management-corrected", map[string]any{"expected_version": 4, "source_sha256": strings.Repeat("b", 64), "questions": questions}, 201)
	_ = json.Unmarshal(changed, &env)
	nextVersion := env.Data.BankVersionID
	nextPath := "/banks/" + bank + "/versions/" + nextVersion
	var second struct {
		Data struct {
			Questions []struct {
				QuestionID        string `json:"question_id"`
				QuestionVersionID string `json:"question_version_id"`
			} `json:"questions"`
		}
	}
	changedDetail := call("GET", nextPath, "quizcraft.workshop.read", bank, "", nil, 200)
	_ = json.Unmarshal(changedDetail, &second)
	if first.Data.Questions[0].QuestionID != second.Data.Questions[0].QuestionID || first.Data.Questions[0].QuestionVersionID == second.Data.Questions[0].QuestionVersionID {
		t.Fatal("correction changed identity or overwrote content")
	}
	call("POST", nextPath+"/validate", "quizcraft.workshop.write", bank, "management-corrected-validate", map[string]any{"expected_version": 5}, 200)
	call("POST", nextPath+"/publish", "quizcraft.workshop.publish", bank, "management-corrected-publish", map[string]any{"expected_version": 6}, 200)
	call("POST", "/banks/"+bank+"/rollback", "quizcraft.workshop.publish", bank, "management-rollback", map[string]any{"expected_version": 7, "target_bank_version_id": version}, 200)
	r := managementRequest(t, "GET", base+"/catalog", actor, "quizcraft.workshop.read", "", "", nil)
	r.Header.Set("X-Actor-User-Id", uuid.NewString())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 401 {
		t.Fatalf("actor tampering=%d", rec.Code)
	}
	r = managementRequest(t, "GET", base+"/catalog", actor, "quizcraft.workshop.read", "", "", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatalf("signed read=%d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 409 {
		t.Fatalf("nonce replay=%d", rec.Code)
	}
	call("GET", "/catalog", "quizcraft.workshop.publish", "", "", nil, 403)
	disabled, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	rec = httptest.NewRecorder()
	disabled.ServeHTTP(rec, managementRequest(t, "GET", base+"/catalog", actor, "quizcraft.workshop.read", "", "", nil))
	if rec.Code != 404 {
		t.Fatalf("default-off=%d", rec.Code)
	}
}

func TestConsoleManagementRejectsTamperingAndSharedKeys(t *testing.T) {
	pool := practicePool(t)
	cfg := quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), ConsoleManagementEnabled: true, ConsoleManagementClientID: "console-management", ConsoleManagementKeys: map[string]string{"management-1": managementSecret}}
	h, err := quizcraft.NewPracticeHTTP(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"X-Permission-Code", "X-Resource-Id", "Idempotency-Key", "X-Key-Id"} {
		t.Run(header, func(t *testing.T) {
			r := managementRequest(t, "GET", "https://owner.test/api/v1/console/workshop/catalog", uuid.NewString(), "quizcraft.workshop.read", "", "", nil)
			r.Header.Set(header, "tampered")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatalf("tampering %s=%d", header, w.Code)
			}
		})
	}
	r := managementRequest(t, "POST", "https://owner.test/api/v1/console/workshop/banks", uuid.NewString(), "quizcraft.workshop.write", "", "management-body", map[string]any{"bank_key": "original", "name": "原文"})
	r.Body = io.NopCloser(strings.NewReader(`{"bank_key":"changed","name":"篡改"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("body tampering=%d", w.Code)
	}
	cfg.SummaryKeys = map[string]string{"summary": managementSecret}
	cfg.SummaryClientID = "summary"
	if _, err = quizcraft.NewPracticeHTTP(cfg); err == nil {
		t.Fatal("shared management/summary secret accepted")
	}
}

func TestConsoleManagementRejectsPlatformAndSessionSecrets(t *testing.T) {
	pool := practicePool(t)
	for _, name := range []string{"platform", "session"} {
		t.Run(name, func(t *testing.T) {
			cfg := quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), ConsoleManagementEnabled: true, ConsoleManagementClientID: "console-management", ConsoleManagementKeys: map[string]string{"management-1": managementSecret}}
			if name == "platform" {
				cfg.PlatformCoreURL = "https://platform.test"
				cfg.PlatformClientID = "quizcraft"
				cfg.PlatformClientSecret = managementSecret
				cfg.PlatformKeyID = "platform-1"
				cfg.PublicURL = "https://quiz.test"
				cfg.SessionEncryptionKey = []byte("0123456789abcdef0123456789abcdef")
			} else {
				cfg.SessionEncryptionKey = []byte("0123456789abcdef0123456789abcdef")
				cfg.PlatformCoreURL = "https://platform.test"
				cfg.PlatformClientID = "quizcraft"
				cfg.PlatformClientSecret = "separate-platform-credential-at-least-32"
				cfg.PlatformKeyID = "platform-1"
				cfg.PublicURL = "https://quiz.test"
				cfg.ConsoleManagementKeys = map[string]string{"management-1": string(cfg.SessionEncryptionKey)}
			}
			if _, err := quizcraft.NewPracticeHTTP(cfg); err == nil {
				t.Fatalf("shared %s credential accepted", name)
			}
		})
	}
}
