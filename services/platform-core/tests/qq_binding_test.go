package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	platformcore "henukit.dev/platform-core"
)

func TestQQBindingRequiresBothChannelsAndUnlinkRevokes(t *testing.T) {
	ctx := context.Background()
	pool, redis := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redis)
	seedIdentity(t, ctx, pool)
	hash := sha256.Sum256([]byte(testClientSecret))
	for _, client := range []string{"henu-bot", "portal-gateway"} {
		if _, err := pool.Exec(ctx, `INSERT INTO oauth_clients(id,redirect_uris) VALUES($1,ARRAY['https://example.test/callback']);`, client); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO oauth_client_keys(client_id,key_id,secret_hash,status) VALUES($1,'primary',$2,'active')`, client, hash[:]); err != nil {
			t.Fatal(err)
		}
	}
	// Provisioning is outside the public API: only this registered Bot owns this app.
	if _, err := pool.Exec(ctx, `INSERT INTO qq_binding_apps(app_id,client_id) VALUES('qq-test-app','henu-bot')`); err != nil {
		t.Fatal(err)
	}
	exchange := "qq-binding-portal-session-token-at-least-32"
	exchangeHash := sha256.Sum256([]byte(exchange))
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,kind,token_hash,client_id,parent_session_id,expires_at) SELECT user_id,'client_exchange',$1,'portal-gateway',id,now()+interval '1 hour' FROM sessions WHERE kind='core'`, exchangeHash[:]); err != nil {
		t.Fatal(err)
	}
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redis, IdempotencyEncryptionKey: testIdempotencyEncryptionKey})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	call := func(client, action string, body map[string]string, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/qq-bindings/"+action, bytes.NewReader(raw))
		req.SetBasicAuth(client, testClientSecret)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Service-Id", client)
		req.Header.Set("X-Key-Id", "primary")
		req.Header.Set("X-Timestamp", fmt.Sprint(time.Now().Unix()))
		req.Header.Set("X-Nonce", uuid.NewString())
		signExchangeRequest(t, req)
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var envelope map[string]any
		_ = json.NewDecoder(res.Body).Decode(&envelope)
		if res.StatusCode != want {
			t.Fatalf("%s %s: status=%d want=%d code=%v", client, action, res.StatusCode, want, envelope["error"])
		}
		data, _ := envelope["data"].(map[string]any)
		return data
	}
	start := map[string]string{"subject": "qq-alice", "request_id": uuid.NewString()}
	pending := call("henu-bot", "start", start, 200)
	token := pending["token"].(string)
	if replay := call("henu-bot", "start", start, 200); replay["token"] != token {
		t.Fatal("start retry must retain the same link")
	}
	call("henu-bot", "confirm", map[string]string{"subject": "qq-alice", "token": token}, 409)
	call("portal-gateway", "authorize", map[string]string{"session_token": exchange, "token": token}, 200)
	call("henu-bot", "confirm", map[string]string{"subject": "qq-bob", "token": token}, 404)
	call("henu-bot", "confirm", map[string]string{"subject": "qq-alice", "token": token}, 200)
	call("henu-bot", "confirm", map[string]string{"subject": "qq-alice", "token": token}, 200)
	if got := call("henu-bot", "resolve", map[string]string{"subject": "qq-alice"}, 200); got["user_id"] == nil {
		t.Fatal("bound user missing")
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET status='suspended'`); err != nil {
		t.Fatal(err)
	}
	call("henu-bot", "unlink", map[string]string{"subject": "qq-alice"}, 200)
	if _, err := pool.Exec(ctx, `UPDATE users SET status='active'`); err != nil {
		t.Fatal(err)
	}
	call("henu-bot", "resolve", map[string]string{"subject": "qq-alice"}, 404)
	call("portal-gateway", "unlink", map[string]string{"session_token": exchange}, 200)
	call("henu-bot", "resolve", map[string]string{"subject": "qq-alice"}, 404)
	call("henu-bot", "confirm", map[string]string{"subject": "qq-alice", "token": token}, 410)
	// A registered but unprovisioned service cannot claim a QQ identity.
	call(testClientID, "start", map[string]string{"subject": "qq-alice", "request_id": uuid.NewString()}, 403)
	second := call("henu-bot", "start", map[string]string{"subject": "qq-alice", "request_id": uuid.NewString()}, 200)
	token = second["token"].(string)
	call("portal-gateway", "authorize", map[string]string{"session_token": exchange, "token": token}, 200)
	// Revoking the parent login after consent must prevent the final bind.
	if _, err := pool.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE kind='core'`); err != nil {
		t.Fatal(err)
	}
	call("henu-bot", "confirm", map[string]string{"subject": "qq-alice", "token": token}, 410)
	call("portal-gateway", "status", map[string]string{"session_token": exchange}, 403)
}
