package tests

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	accountportfolio "henukit.dev/account-portfolio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const membershipSecret = "independent-membership-reader-secret-32bytes"

func TestMembershipReaderCannotReadOtherAccountsOrMutate(t *testing.T) {
	initial, pool := newAccountPortfolioServer(t)
	initial.Close()
	defer pool.Close()
	handler, err := accountportfolio.New(accountportfolio.Config{Database: pool, ClientID: "portal-gateway", Keys: map[string]string{"account-key": serviceSecret}, ConsoleClientID: "console-gateway", ConsoleKeys: map[string]string{"console-key": consoleServiceSecret}, MembershipClientID: "membership-reader", MembershipKeys: map[string]string{"membership-key": membershipSecret}, PointCursorKey: pointCursorTestKey})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	const actor = "71111111-1111-4111-8111-111111111111"
	counter := 0
	call := func(method, path string, want int) string {
		t.Helper()
		counter++
		request, _ := http.NewRequest(method, server.URL+path, strings.NewReader("{}"))
		request.SetBasicAuth("membership-reader", membershipSecret)
		timestamp := fmt.Sprint(time.Now().Unix())
		nonceBytes := sha256.Sum256([]byte(fmt.Sprint(counter) + method + path))
		nonce := base64.RawURLEncoding.EncodeToString(nonceBytes[:24])
		digest := sha256.Sum256([]byte("{}"))
		canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:]), actor}, "\n")
		mac := hmac.New(sha256.New, []byte(membershipSecret))
		mac.Write([]byte(canonical))
		for k, v := range map[string]string{"X-Service-Id": "membership-reader", "X-Key-Id": "membership-key", "X-Timestamp": timestamp, "X-Nonce": nonce, "X-Actor-User-Id": actor, "X-Signature": base64.RawURLEncoding.EncodeToString(mac.Sum(nil))} {
			request.Header.Set(k, v)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body := string(readBody(t, response))
		if response.StatusCode != want {
			t.Fatalf("%s %s: %d %s", method, path, response.StatusCode, body)
		}
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("membership reader must not cache")
		}
		return body
	}
	body := call("GET", "/api/v1/account/membership", 200)
	if !strings.Contains(body, `"plan":"free"`) {
		t.Fatal(body)
	}
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM account_portfolio_accounts WHERE user_id=$1", actor).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("reader initialized an account")
	}
	for _, path := range []string{"/api/v1/account/summary", "/api/v1/account/points", "/api/v1/account/membership-orders", "/api/v1/console/memberships/" + actor} {
		call("GET", path, 403)
	}
	call("POST", "/api/v1/account/membership-orders", 403)
	for _, path := range []string{"/api/v1/console/memberships/" + actor + "/grants", "/api/v1/console/memberships/" + actor + "/revocations", "/api/v1/account/tickets"} {
		call("POST", path, 403)
	}
	seed := sendOwnerJSON(t, server.URL, "GET", actor, "/api/v1/account/summary", "reader-seed", "", "")
	if seed.StatusCode != 200 {
		t.Fatal(responseText(t, seed))
	}
	responseText(t, seed)
	grant := sendConsoleJSON(t, server.URL, "POST", actor, "/api/v1/console/memberships/"+actor+"/grants", "reader-grant", "reader-grant", `{"reason":"Synthetic entitlement fixture","expected_version":1}`)
	if grant.StatusCode != 200 {
		t.Fatal(responseText(t, grant))
	}
	responseText(t, grant)
	if body := call("GET", "/api/v1/account/membership", 200); !strings.Contains(body, `"lifetime":true`) {
		t.Fatal(body)
	}
	revoke := sendConsoleJSON(t, server.URL, "POST", actor, "/api/v1/console/memberships/"+actor+"/revocations", "reader-revoke", "reader-revoke", `{"reason":"Synthetic revocation fixture","expected_version":2}`)
	if revoke.StatusCode != 200 {
		t.Fatal(responseText(t, revoke))
	}
	responseText(t, revoke)
	if body := call("GET", "/api/v1/account/membership", 200); !strings.Contains(body, `"lifetime":false`) {
		t.Fatal(body)
	}
	for _, config := range []accountportfolio.Config{
		{MembershipClientID: "portal-gateway", MembershipKeys: map[string]string{"reader": membershipSecret}},
		{MembershipClientID: "console-gateway", MembershipKeys: map[string]string{"reader": membershipSecret}},
		{MembershipClientID: "membership-reader", MembershipKeys: map[string]string{"reader": serviceSecret}},
		{MembershipClientID: "membership-reader", MembershipKeys: map[string]string{"reader": consoleServiceSecret}},
	} {
		config.Database = pool
		config.ClientID = "portal-gateway"
		config.Keys = map[string]string{"account-key": serviceSecret}
		config.ConsoleClientID = "console-gateway"
		config.ConsoleKeys = map[string]string{"console-key": consoleServiceSecret}
		config.PointCursorKey = pointCursorTestKey
		if _, err := accountportfolio.New(config); err == nil {
			t.Fatal("reused credential accepted")
		}
	}

}
