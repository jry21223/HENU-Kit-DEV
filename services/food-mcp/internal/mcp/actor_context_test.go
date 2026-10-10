package mcp_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"henukit.dev/food-mcp/internal/foodclient"
	foodmcp "henukit.dev/food-mcp/internal/mcp"
)

const actorSecret = "test-food-actor-secret-at-least-32-bytes"

func actorHarness(t *testing.T) *harness {
	t.Helper()
	f := &fakeFood{nextCreate: func(input map[string]any) (int, map[string]any) { return 200, map[string]any{"id": "post"} }}
	owner := httptest.NewServer(f.handler())
	t.Cleanup(owner.Close)
	c, err := foodclient.NewClient(owner.URL, testCreateClientID, testCreateSecret, "active", testReadClientID, testReadSecret, "active")
	if err != nil {
		t.Fatal(err)
	}
	h, err := foodmcp.NewHandler(foodmcp.Options{Client: c, AccessToken: "test-token", ActorSecret: actorSecret, RequireActorContext: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h.Handler())
	t.Cleanup(server.Close)
	return &harness{mcpServer: server, food: f}
}

func proof(tool string, args map[string]any, scope, uid string, stamp int64) mcpsdk.Meta {
	raw, _ := json.Marshal(args)
	digest := sha256.Sum256(raw)
	kind := "guest"
	name := ""
	if uid != "" {
		kind = "bound"
		name = "绑定同学"
	}
	claims := map[string]any{"tool": tool, "arguments_sha256": hex.EncodeToString(digest[:]), "scope": scope, "kind": kind, "issued_at": stamp, "nonce": "test_nonce_01234567890123456789012", "user_id": uid, "display_name": name}
	payload, _ := json.Marshal(claims)
	mac := hmac.New(sha256.New, []byte(actorSecret))
	mac.Write(append([]byte("henukit-food-actor-v1\n"), payload...))
	return mcpsdk.Meta{"henukit.dev/food-actor": map[string]any{"payload": base64.RawURLEncoding.EncodeToString(payload), "signature": base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}}
}

func proofCall(t *testing.T, s *mcpsdk.ClientSession, name string, args map[string]any, meta mcpsdk.Meta) bool {
	t.Helper()
	r, err := s.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args, Meta: meta})
	if err != nil {
		t.Fatal(err)
	}
	return r.IsError
}

func TestSignedUsersOnSharedSessionAndAcrossReconnects(t *testing.T) {
	h := actorHarness(t)
	s := h.connect(t)
	args := map[string]any{}
	scope := strings.Repeat("a", 64)
	if proofCall(t, s, "list_my_food_posts", args, proof("list_my_food_posts", args, scope, "", time.Now().Unix())) {
		t.Fatal("guest rejected")
	}
	first := h.food.readRequests[0].Header.Get("X-Actor-User-Id")
	proofCall(t, s, "list_my_food_posts", args, proof("list_my_food_posts", args, strings.Repeat("b", 64), "", time.Now().Unix()))
	other := h.food.readRequests[1].Header.Get("X-Actor-User-Id")
	proofCall(t, h.connect(t), "list_my_food_posts", args, proof("list_my_food_posts", args, scope, "", time.Now().Unix()))
	again := h.food.readRequests[2].Header.Get("X-Actor-User-Id")
	if first == "" || first == other || first != again {
		t.Fatalf("guest isolation failed: %q %q %q", first, other, again)
	}
	proofCall(t, s, "list_my_food_posts", args, proof("list_my_food_posts", args, scope, testActorID, time.Now().Unix()))
	if h.food.readRequests[3].Header.Get("X-Actor-User-Id") != testActorID {
		t.Fatal("bound account not preferred")
	}
	proofCall(t, s, "list_my_food_posts", args, proof("list_my_food_posts", args, scope, "", time.Now().Unix()))
	if h.food.readRequests[4].Header.Get("X-Actor-User-Id") != first {
		t.Fatal("unbinding retained old account")
	}
}

func TestProofFailsClosedAndCreateReplayKeepsIdempotency(t *testing.T) {
	h := actorHarness(t)
	s := h.connect(t)
	args := map[string]any{"venue_name": "店<&>\u2028", "campus": "minglun", "tier": "top", "review_text": "很好吃"}
	scope := strings.Repeat("a", 64)
	meta := proof("create_food_post", args, scope, "", time.Now().Unix())
	for range 2 {
		if proofCall(t, s, "create_food_post", args, meta) {
			t.Fatal("signed create failed")
		}
	}
	keys := []string{h.food.createRequests[0].Header.Get("Idempotency-Key"), h.food.createRequests[1].Header.Get("Idempotency-Key")}
	if keys[0] == "" || keys[0] != keys[1] {
		t.Fatalf("replay duplicated key: %v", keys)
	}
	bad := proof("create_food_post", args, scope, "", time.Now().Unix()-120)
	if !proofCall(t, s, "create_food_post", args, bad) {
		t.Fatal("expired proof accepted")
	}
	if !proofCall(t, s, "create_food_post", args, nil) {
		t.Fatal("missing proof accepted")
	}
	changed := map[string]any{"venue_name": "冒名店", "campus": "minglun", "tier": "top", "review_text": "篡改了"}
	if !proofCall(t, s, "create_food_post", changed, meta) {
		t.Fatal("mismatched payload accepted")
	}
	forged := proof("create_food_post", args, scope, testActorID, time.Now().Unix())
	forged["henukit.dev/food-actor"].(map[string]any)["signature"] = strings.Repeat("A", 43)
	if !proofCall(t, s, "create_food_post", args, forged) {
		t.Fatal("forged proof accepted")
	}
	if len(h.food.createRequests) != 2 {
		t.Fatal("rejected proof reached owner")
	}
	if proofCall(t, s, "list_food_posts", map[string]any{}, nil) {
		t.Fatal("public reads require identity")
	}
}
