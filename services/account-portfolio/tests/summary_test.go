package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	accountportfolio "henukit.dev/account-portfolio"
)

const serviceSecret = "account-portfolio-gateway-secret-at-least-32-bytes"

func TestNewUserSummaryStartsWithPersistedZeroAndFreeMembership(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	userID := "11111111-1111-4111-8111-111111111111"
	response := send(t, server.URL, userID, "/api/v1/account/summary", "nonce-summary")
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("summary status = %d: %s", response.StatusCode, body)
	}
	for _, want := range []string{
		`"points_balance":0`,
		`"plan":"free"`,
		`"lifetime":false`,
		`"unread_notification_count":0`,
		`"open_ticket_count":0`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("summary omitted %s: %s", want, body)
		}
	}

	var accounts, points, memberships int
	err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_accounts WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_points WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_memberships WHERE user_id=$1)
	`, userID).Scan(&accounts, &points, &memberships)
	if err != nil {
		t.Fatal(err)
	}
	if accounts != 1 || points != 1 || memberships != 1 {
		t.Fatalf("new-user durable state = accounts=%d points=%d memberships=%d, want 1 each", accounts, points, memberships)
	}
}

func TestAccountReadEndpointsReturnHonestEmptyCollections(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	userID := "22222222-2222-4222-8222-222222222222"
	for index, route := range []string{
		"/api/v1/account/points",
		"/api/v1/account/membership",
		"/api/v1/account/notifications",
		"/api/v1/account/tickets",
		"/api/v1/account/membership-orders",
	} {
		response := send(t, server.URL, userID, route, "nonce-empty-"+strconv.Itoa(index))
		body := readBody(t, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d: %s", route, response.StatusCode, body)
		}
		if bytes.Contains(bytes.ToLower(body), []byte("mock")) {
			t.Fatalf("%s returned a mock payload: %s", route, body)
		}
		if !bytes.Contains(body, []byte(`"data"`)) {
			t.Fatalf("%s omitted response data: %s", route, body)
		}
	}
}

func TestConcurrentFirstReadsCreateOnlyOneDurableDefaultState(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	const userID = "33333333-3333-4333-8333-333333333333"
	var group sync.WaitGroup
	errs := make(chan error, 12)
	for index := 0; index < cap(errs); index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			response := send(t, server.URL, userID, "/api/v1/account/summary", "nonce-race-"+strconv.Itoa(index))
			if response.StatusCode != http.StatusOK {
				errs <- &statusError{status: response.StatusCode, body: string(readBody(t, response))}
			}
		}(index)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	var accounts, points, memberships int
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_accounts WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_points WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_memberships WHERE user_id=$1)
	`, userID).Scan(&accounts, &points, &memberships); err != nil {
		t.Fatal(err)
	}
	if accounts != 1 || points != 1 || memberships != 1 {
		t.Fatalf("concurrent first reads created accounts=%d points=%d memberships=%d, want one each", accounts, points, memberships)
	}
}

func TestAccountEndpointsRejectBrowserSuppliedOrReplayedIdentity(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/account/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Actor-User-Id", uuid.NewString())
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned browser identity status = %d, want 401", response.StatusCode)
	}
	_ = response.Body.Close()

	first := send(t, server.URL, "44444444-4444-4444-8444-444444444444", "/api/v1/account/summary", "nonce-replay")
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first signed request status = %d: %s", first.StatusCode, readBody(t, first))
	}
	_ = first.Body.Close()
	second := send(t, server.URL, "44444444-4444-4444-8444-444444444444", "/api/v1/account/summary", "nonce-replay")
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("replayed request status = %d, want 409: %s", second.StatusCode, readBody(t, second))
	}
}

func TestHealthFailsClosedWhenDatabaseIsUnavailable(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://invalid:invalid@127.0.0.1:1/unavailable?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	handler, err := accountportfolio.New(accountportfolio.Config{
		Database: pool,
		ClientID: "portal-gateway",
		Keys:     map[string]string{"account-key": serviceSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("health status = %d, want 503 when database is unavailable", response.Code)
	}
}

func newAccountPortfolioServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	clearAccountPortfolio(t, pool)
	handler, err := accountportfolio.New(accountportfolio.Config{
		Database: pool,
		ClientID: "portal-gateway",
		Keys:     map[string]string{"account-key": serviceSecret},
	})
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return httptest.NewServer(handler), pool
}

func send(t *testing.T, baseURL, actorID, route, nonce string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+route, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Actor-User-Id", actorID)
	request.Header.Set("X-Request-Id", "req_account_test")
	sign(t, request, nonce)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func sign(t *testing.T, request *http.Request, nonce string) {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonceDigest := sha256.Sum256([]byte(nonce))
	nonce = base64.RawURLEncoding.EncodeToString(nonceDigest[:24])
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{
		request.Method,
		request.URL.RequestURI(),
		timestamp,
		nonce,
		hex.EncodeToString(digest[:]),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(serviceSecret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth("portal-gateway", serviceSecret)
	request.Header.Set("X-Service-Id", "portal-gateway")
	request.Header.Set("X-Key-Id", "account-key")
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}

func readBody(t *testing.T, response *http.Response) []byte {
	t.Helper()
	defer response.Body.Close()
	var payload json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

type statusError struct {
	status int
	body   string
}

func (e *statusError) Error() string {
	return "unexpected status " + strconv.Itoa(e.status) + ": " + e.body
}
