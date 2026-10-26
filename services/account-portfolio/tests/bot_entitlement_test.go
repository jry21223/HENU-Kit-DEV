package tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	accountportfolio "henukit.dev/account-portfolio"
)

const botSecret = "bot-entitlement-reader-independent-secret-32-bytes"

func TestBotEntitlementReadTracksGrantAndRevocationWithoutOwnerWrites(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	handler, err := accountportfolio.New(accountportfolio.Config{
		Database: pool, ClientID: "portal-gateway", Keys: map[string]string{"account-key": serviceSecret},
		ConsoleClientID: "console-gateway", ConsoleKeys: map[string]string{"console-key": consoleServiceSecret},
		BotClientID: "henu-bot-membership", BotKeys: map[string]string{"bot-key": botSecret}, PointCursorKey: pointCursorTestKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	uid := "dededede-dede-4ede-8ede-dededededede"
	call := func(path, nonce string) *http.Response {
		request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Actor-User-Id", uid)
		sign(t, request, nonce)
		request.SetBasicAuth("henu-bot-membership", botSecret)
		request.Header.Set("X-Service-Id", "henu-bot-membership")
		request.Header.Set("X-Key-Id", "bot-key")
		// Resign with the dedicated reader key.
		digest := sha256.Sum256(nil)
		canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:]), uid}, "\n")
		mac := hmac.New(sha256.New, []byte(botSecret))
		mac.Write([]byte(canonical))
		request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	portal := send(t, server.URL, uid, "/api/v1/bot/membership-entitlement", "portal-bot-denied")
	if portal.StatusCode != http.StatusForbidden {
		t.Fatalf("Portal entitlement status %d", portal.StatusCode)
	}
	_ = readBody(t, portal)
	console := sendConsoleJSON(t, server.URL, http.MethodGet, uid, "/api/v1/bot/membership-entitlement", "console-bot-denied", "", "")
	if console.StatusCode != http.StatusForbidden {
		t.Fatalf("Console entitlement status %d", console.StatusCode)
	}
	_ = readBody(t, console)
	for i, plan := range []string{"", "lifetime", "free"} {
		if plan != "" {
			_, err = pool.Exec(t.Context(), `INSERT INTO account_portfolio_accounts(user_id) VALUES($1) ON CONFLICT DO NOTHING`, uid)
			if err != nil {
				t.Fatal(err)
			}
			_, err = pool.Exec(t.Context(), `INSERT INTO account_portfolio_memberships(user_id,plan,source) VALUES($1,$2,'operator') ON CONFLICT(user_id) DO UPDATE SET plan=$2`, uid, plan)
			if err != nil {
				t.Fatal(err)
			}
		}
		response := call("/api/v1/bot/membership-entitlement", "entitlement-"+string(rune('a'+i)))
		body := string(readBody(t, response))
		expected := `"lifetime":false`
		if plan == "lifetime" {
			expected = `"lifetime":true`
		}
		if plan == "" {
			var count int
			if err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM account_portfolio_accounts WHERE user_id=$1)+(SELECT count(*) FROM account_portfolio_memberships WHERE user_id=$1)`, uid).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("entitlement lookup initialized durable owner rows")
			}
		}
		if response.StatusCode != 200 || !strings.Contains(body, expected) {
			t.Fatalf("status %d body %s", response.StatusCode, body)
		}
	}
	for _, path := range []string{"/api/v1/account/summary", "/api/v1/account/membership", "/api/v1/console/tickets"} {
		response := call(path, "denied-"+path)
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("%s status %d", path, response.StatusCode)
		}
		_ = readBody(t, response)
	}
	pool.Close()
	response := call("/api/v1/bot/membership-entitlement", "dependency-unavailable")
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("dependency status %d", response.StatusCode)
	}
	_ = readBody(t, response)
}

func TestBotReaderRejectsCredentialReuse(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, change := range []func(*accountportfolio.Config){
		func(c *accountportfolio.Config) { c.BotClientID = c.ClientID },
		func(c *accountportfolio.Config) { c.BotClientID = c.ConsoleClientID },
		func(c *accountportfolio.Config) { c.BotKeys = c.Keys },
		func(c *accountportfolio.Config) { c.BotKeys = c.ConsoleKeys },
		func(c *accountportfolio.Config) { c.BotClientID = "" },
		func(c *accountportfolio.Config) { c.BotKeys = nil },
		func(c *accountportfolio.Config) {
			c.PointCursorKey = []byte(botSecret[:32])
			c.BotKeys = map[string]string{"bot-key": botSecret[:32]}
		},
	} {
		config := accountportfolio.Config{Database: pool, ClientID: "portal", Keys: map[string]string{"key": serviceSecret}, ConsoleClientID: "console", ConsoleKeys: map[string]string{"key": consoleServiceSecret}, BotClientID: "bot", BotKeys: map[string]string{"key": botSecret}, PointCursorKey: pointCursorTestKey}
		change(&config)
		if _, err := accountportfolio.New(config); err == nil {
			t.Fatal("invalid Bot reader configuration accepted")
		}
	}
}
