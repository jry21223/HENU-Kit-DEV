package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	platformcore "henukit.dev/platform-core"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type membershipLogWriter struct{ entries chan []byte }

func (w membershipLogWriter) Write(value []byte) (int, error) {
	w.entries <- append([]byte(nil), value...)
	return len(value), nil
}

func TestQQBenefitsUsesCurrentOwnerAndNeverTrustsCallerMembership(t *testing.T) {
	ctx := context.Background()
	pool, redis := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redis)
	seedIdentity(t, ctx, pool)
	hash := sha256.Sum256([]byte(testClientSecret))
	for _, client := range []string{"henu-bot", "other-bot", "portal-gateway"} {
		pool.Exec(ctx, `INSERT INTO oauth_clients(id,redirect_uris) VALUES($1,ARRAY['https://example.test/callback'])`, client)
		pool.Exec(ctx, `INSERT INTO oauth_client_keys(client_id,key_id,secret_hash,status) VALUES($1,'primary',$2,'active')`, client, hash[:])
	}
	pool.Exec(ctx, `INSERT INTO qq_binding_apps(app_id,client_id) VALUES('app-one','henu-bot'),('app-two','other-bot')`)
	var user string
	pool.QueryRow(ctx, `SELECT id::text FROM users LIMIT 1`).Scan(&user)
	if _, err := pool.Exec(ctx, `INSERT INTO qq_bindings(app_id,subject,user_id) VALUES('app-one','qq-alice',$1)`, user); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var fixtureMutex sync.Mutex
	plan := "lifetime"
	mode := ""
	var redirectCalls atomic.Int32
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectCalls.Add(1); w.WriteHeader(200) }))
	defer redirect.Close()
	broken := false
	setPlan := func(value string) { fixtureMutex.Lock(); defer fixtureMutex.Unlock(); plan = value }
	setMode := func(value string) { fixtureMutex.Lock(); defer fixtureMutex.Unlock(); mode = value }
	setBroken := func(value bool) { fixtureMutex.Lock(); defer fixtureMutex.Unlock(); broken = value }
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fixtureMutex.Lock()
		currentPlan, currentMode, currentBroken := plan, mode, broken
		fixtureMutex.Unlock()
		client, secret, _ := r.BasicAuth()
		digest := sha256.Sum256(nil)
		canonical := strings.Join([]string{"GET", r.URL.RequestURI(), r.Header.Get("X-Timestamp"), r.Header.Get("X-Nonce"), hex.EncodeToString(digest[:]), user}, "\n")
		mac := hmac.New(sha256.New, []byte("independent-membership-reader-secret-32bytes"))
		mac.Write([]byte(canonical))
		if client != "membership-reader" || secret != "independent-membership-reader-secret-32bytes" || !hmac.Equal([]byte(r.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
			t.Error("owner credential/signature invalid")
		}
		switch currentMode {
		case "redirect":
			http.Redirect(w, r, redirect.URL, 302)
			return
		case "oversize":
			fmt.Fprint(w, strings.Repeat("x", 4097))
			return
		case "timeout":
			select {
			case <-time.After(3100 * time.Millisecond):
			case <-r.Context().Done():
			}
			return
		case "unlink":
			pool.Exec(ctx, `DELETE FROM qq_bindings WHERE app_id='app-one' AND subject='qq-alice'`)
		}
		if r.URL.Path != "/api/v1/account/membership" || r.Header.Get("X-Actor-User-Id") != user {
			t.Error("wrong owner actor/route")
		}
		if currentBroken {
			fmt.Fprint(w, `{"data":{"plan":"lifetime","lifetime":false}}`)
			return
		}
		fmt.Fprintf(w, `{"data":{"plan":%q,"lifetime":%t},"request_id":"req_owner"}`, currentPlan, currentPlan == "lifetime")
	}))
	defer owner.Close()
	logs := membershipLogWriter{entries: make(chan []byte, 256)}
	handler, err := platformcore.New(platformcore.Config{Logger: slog.New(slog.NewJSONHandler(logs, nil)), Database: pool, Redis: redis, IdempotencyEncryptionKey: testIdempotencyEncryptionKey, MembershipBaseURL: owner.URL, MembershipClientID: "membership-reader", MembershipKeyID: "reader-key", MembershipSecret: "independent-membership-reader-secret-32bytes"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	var lastHeaders http.Header
	call := func(client, raw string, want int) map[string]any {
		t.Helper()
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/qq-bindings/benefits", bytes.NewBufferString(raw))
		req.SetBasicAuth(client, testClientSecret)
		for k, v := range map[string]string{"X-Service-Id": client, "X-Key-Id": "primary", "X-Timestamp": fmt.Sprint(time.Now().Unix()), "X-Nonce": uuid.NewString()} {
			req.Header.Set(k, v)
		}
		signExchangeRequest(t, req)
		lastHeaders = req.Header.Clone()
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var env map[string]any
		json.NewDecoder(res.Body).Decode(&env)
		if res.StatusCode != want {
			t.Fatalf("%s: %d want%d %v", raw, res.StatusCode, want, env)
		}
		if res.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("cached benefits")
		}
		data, _ := env["data"].(map[string]any)
		return data
	}
	got := call("henu-bot", `{"subject":"qq-alice"}`, 200)
	if got["allowed"] != true || got["user_id"] != nil || fmt.Sprint(got["benefits"]) != "[yuketang ucampus chaoxing library_schedule]" {
		t.Fatal(got)
	}
	if _, err := time.Parse(time.RFC3339, got["checked_at"].(string)); err != nil {
		t.Fatal(err)
	}
	// The benefits route preserves the nonce replay guard and signature boundary.
	replay, _ := http.NewRequest("POST", server.URL+"/api/v1/qq-bindings/benefits", bytes.NewBufferString(`{"subject":"qq-alice"}`))
	replay.Header = lastHeaders.Clone()
	replayRes, err := http.DefaultClient.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	replayRes.Body.Close()
	if replayRes.StatusCode != 409 {
		t.Fatalf("replay status %d", replayRes.StatusCode)
	}
	replay.Header.Set("X-Nonce", uuid.NewString())
	replay.Header.Set("X-Signature", "invalid")
	replay.Body = io.NopCloser(strings.NewReader(`{"subject":"qq-alice"}`))
	invalidRes, err := http.DefaultClient.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	invalidRes.Body.Close()
	if invalidRes.StatusCode != 401 {
		t.Fatalf("signature status %d", invalidRes.StatusCode)
	}
	setPlan("free")
	got = call("henu-bot", `{"subject":"qq-alice"}`, 200)
	if got["allowed"] != false || len(got["benefits"].([]any)) != 0 {
		t.Fatal(got)
	}
	before := calls.Load()
	for _, input := range []string{`{"subject":"qq-alice","user_id":"` + user + `"}`, `{"subject":"qq-alice","lifetime":true}`, `{"subject":"qq-alice","token":"unexpected"}`} {
		call("henu-bot", input, 400)
	}
	call("other-bot", `{"subject":"qq-alice"}`, 404)
	call("henu-bot", `{"subject":"qq-bob"}`, 404)
	call("portal-gateway", `{"subject":"qq-alice"}`, 403)
	if calls.Load() != before {
		t.Fatal("invalid identity reached owner")
	}
	setBroken(true)
	call("henu-bot", `{"subject":"qq-alice"}`, 503)
	setBroken(false)
	for _, value := range []string{"redirect", "oversize", "timeout"} {
		setMode(value)
		call("henu-bot", `{"subject":"qq-alice"}`, 503)
	}
	if redirectCalls.Load() != 0 {
		t.Fatal("owner redirect forwarded credentials")
	}
	setMode("")
	setMode("unlink")
	call("henu-bot", `{"subject":"qq-alice"}`, 404)
	setMode("")
	pool.Exec(ctx, `INSERT INTO qq_bindings(app_id,subject,user_id) VALUES('app-one','qq-alice',$1)`, user)
	pool.Exec(ctx, `UPDATE users SET status='suspended'`)
	call("henu-bot", `{"subject":"qq-alice"}`, 404)
	// Absent owner configuration and accidental Bot credential reuse both fail closed.
	pool.Exec(ctx, `UPDATE users SET status='active'`)
	before = calls.Load()
	for _, config := range []platformcore.Config{
		{},
		{MembershipBaseURL: owner.URL, MembershipClientID: "membership-reader", MembershipKeyID: "reader-key", MembershipSecret: testClientSecret},
		{MembershipBaseURL: owner.URL, MembershipClientID: "henu-bot", MembershipKeyID: "reader-key", MembershipSecret: "independent-membership-reader-secret-32bytes"},
	} {
		config.Database = pool
		config.Redis = redis
		config.IdempotencyEncryptionKey = testIdempotencyEncryptionKey
		unavailable, err := platformcore.New(config)
		if err != nil {
			t.Fatal(err)
		}
		original := server
		server = httptest.NewServer(unavailable)
		call("henu-bot", `{"subject":"qq-alice"}`, 503)
		server.Close()
		server = original
	}
	if calls.Load() != before {
		t.Fatal("missing/reused credential reached owner")
	}
	categories := map[string]bool{}
	for len(logs.entries) > 0 {
		var entry map[string]any
		if err := json.Unmarshal(<-logs.entries, &entry); err != nil {
			t.Fatal(err)
		}
		if entry["msg"] != "membership_check_failed" {
			continue
		}
		category, _ := entry["category"].(string)
		categories[category] = true
		if !strings.HasPrefix(fmt.Sprint(entry["request_id"]), "req_") {
			t.Fatal("failure log lacks request correlation")
		}
		for field := range entry {
			if field != "time" && field != "level" && field != "msg" && field != "request_id" && field != "category" {
				t.Fatalf("unbounded failure log field %s", field)
			}
		}
	}
	for _, category := range []string{"owner_contract", "owner_status", "owner_transport"} {
		if !categories[category] {
			t.Fatalf("missing failure category %s", category)
		}
	}

}
