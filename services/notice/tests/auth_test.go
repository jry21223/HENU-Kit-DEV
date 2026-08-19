package tests

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	notice "henukit.dev/notice"
)

// serviceRequest is one signed service call, expressed so a test can corrupt
// exactly one field and leave everything else valid.
type serviceRequest struct {
	method      string
	path        string
	body        string
	secret      string // signs the canonical string
	basicUser   string
	basicSecret string
	serviceID   string
	keyID       string
	timestamp   string
	nonce       string
	signature   string // when set, replaces the computed signature
	actor       string
	scopeKind   string
	productCode string
}

func validRequest() serviceRequest {
	return serviceRequest{
		method:      http.MethodGet,
		path:        "/api/v1/console-notices",
		body:        "",
		secret:      testSecret,
		basicUser:   "console-gateway",
		basicSecret: testSecret,
		serviceID:   "console-gateway",
		keyID:       "active",
		timestamp:   strconv.FormatInt(time.Now().Unix(), 10),
		nonce:       base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24])),
		actor:       uuid.NewString(),
		scopeKind:   "product",
		productCode: "notice",
	}
}

func (request serviceRequest) send(t *testing.T, baseURL string) *http.Response {
	t.Helper()

	httpRequest, err := http.NewRequest(request.method, baseURL+request.path, strings.NewReader(request.body))
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.SetBasicAuth(request.basicUser, request.basicSecret)

	signature := request.signature
	if signature == "" {
		digest := sha256.Sum256([]byte(request.body))
		canonical := strings.Join([]string{
			request.method, request.path, request.timestamp, request.nonce,
			hex.EncodeToString(digest[:]),
		}, "\n")
		mac := hmac.New(sha256.New, []byte(request.secret))
		_, _ = mac.Write([]byte(canonical))
		signature = base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Service-Id", request.serviceID)
	httpRequest.Header.Set("X-Key-Id", request.keyID)
	httpRequest.Header.Set("X-Timestamp", request.timestamp)
	httpRequest.Header.Set("X-Nonce", request.nonce)
	httpRequest.Header.Set("X-Signature", signature)
	httpRequest.Header.Set("X-Actor-User-Id", request.actor)
	httpRequest.Header.Set("X-Permission-Code", "notice.read")
	httpRequest.Header.Set("X-Scope-Kind", request.scopeKind)
	httpRequest.Header.Set("X-Product-Code", request.productCode)
	httpRequest.Header.Set("X-Request-Id", "req_"+uuid.NewString())

	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func newAuthServer(t *testing.T) *httptest.Server {
	t.Helper()

	pool, err := pgxpool.New(context.Background(), os.Getenv("NOTICE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("NOTICE_TEST_REDIS_ADDR")})
	t.Cleanup(func() { redisClient.Close() })
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}

	handler, err := notice.New(notice.Config{
		Database: pool,
		Redis:    redisClient,
		ClientID: "console-gateway",
		Keys:     map[string]string{"active": testSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func assertRejected(t *testing.T, response *http.Response, wantStatus int, wantCode string) {
	t.Helper()
	defer response.Body.Close()

	if response.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d", response.StatusCode, wantStatus)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != wantCode {
		t.Fatalf("error code = %q, want %q", envelope.Error.Code, wantCode)
	}
}

// The happy path must pass, or the rejection tests below prove nothing: a
// request that fails for an unrelated reason would still look "rejected".
func TestValidServiceRequestIsAccepted(t *testing.T) {
	server := newAuthServer(t)

	response := validRequest().send(t, server.URL)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid signed request status = %d, want 200", response.StatusCode)
	}
}

func TestServiceCredentialsAreRejected(t *testing.T) {
	server := newAuthServer(t)

	for _, tc := range []struct {
		name   string
		mutate func(*serviceRequest)
	}{
		{"wrong basic secret", func(r *serviceRequest) { r.basicSecret = "not-the-shared-secret-padded-out" }},
		{"empty basic secret", func(r *serviceRequest) { r.basicSecret = "" }},
		{"wrong basic user", func(r *serviceRequest) { r.basicUser = "portal-gateway" }},
		{"unknown key id", func(r *serviceRequest) { r.keyID = "retired" }},
		{"empty key id", func(r *serviceRequest) { r.keyID = "" }},
		{"service id disagrees with basic user", func(r *serviceRequest) { r.serviceID = "portal-gateway" }},
		{"empty service id", func(r *serviceRequest) { r.serviceID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := validRequest()
			tc.mutate(&request)
			assertRejected(t, request.send(t, server.URL), http.StatusUnauthorized, "INVALID_SERVICE_AUTH")
		})
	}
}

// The timestamp bounds the replay window, so a stale or future-dated request
// must be refused even when it is otherwise perfectly signed.
func TestServiceTimestampOutsideTheWindowIsRejected(t *testing.T) {
	server := newAuthServer(t)

	for _, tc := range []struct {
		name      string
		timestamp string
	}{
		{"far past", strconv.FormatInt(time.Now().Add(-6*time.Minute).Unix(), 10)},
		{"far future", strconv.FormatInt(time.Now().Add(6*time.Minute).Unix(), 10)},
		{"not a number", "yesterday"},
		{"empty", ""},
		{"unix epoch", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := validRequest()
			request.timestamp = tc.timestamp
			assertRejected(t, request.send(t, server.URL), http.StatusUnauthorized, "INVALID_SERVICE_AUTH")
		})
	}
}

// A request signed just inside the window is still accepted, so the bound is a
// window rather than a demand for perfectly synchronized clocks.
func TestServiceTimestampInsideTheWindowIsAccepted(t *testing.T) {
	server := newAuthServer(t)

	request := validRequest()
	request.timestamp = strconv.FormatInt(time.Now().Add(-4*time.Minute).Unix(), 10)

	response := request.send(t, server.URL)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a timestamp inside the window", response.StatusCode)
	}
}

// The nonce is the replay key, so anything that is not 24 decoded bytes must be
// refused rather than stored.
func TestServiceNonceShapeIsEnforced(t *testing.T) {
	server := newAuthServer(t)

	for _, tc := range []struct {
		name  string
		nonce string
	}{
		{"empty", ""},
		{"not base64url", "!!!not-base64!!!"},
		{"padded base64", base64.StdEncoding.EncodeToString([]byte(uuid.NewString()[:25]))},
		{"base64 of the right length but wrong alphabet", strings.Repeat("+", 32)},
		{"too short", base64.RawURLEncoding.EncodeToString([]byte("short"))},
		{"too long", base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := validRequest()
			request.nonce = tc.nonce
			assertRejected(t, request.send(t, server.URL), http.StatusUnauthorized, "INVALID_SERVICE_AUTH")
		})
	}
}

// The signature covers method, path, timestamp, nonce and body digest. Changing
// any of them after signing must invalidate the request.
func TestServiceSignatureCoversTheWholeCanonicalString(t *testing.T) {
	server := newAuthServer(t)

	for _, tc := range []struct {
		name   string
		mutate func(*serviceRequest)
	}{
		{"forged signature", func(r *serviceRequest) {
			r.signature = base64.RawURLEncoding.EncodeToString([]byte("forged-signature-bytes-here"))
		}},
		{"empty signature", func(r *serviceRequest) { r.signature = "not-a-signature" }},
		{"signed with the wrong secret", func(r *serviceRequest) {
			r.secret = "a-different-secret-at-least-32-bytes"
		}},
		{"path changed after signing", func(r *serviceRequest) {
			r.secret = testSecret
			r.signature = signatureFor(testSecret, r.method, "/api/v1/sources", r.timestamp, r.nonce, r.body)
		}},
		{"timestamp changed after signing", func(r *serviceRequest) {
			r.signature = signatureFor(testSecret, r.method, r.path,
				strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10), r.nonce, r.body)
		}},
		{"nonce changed after signing", func(r *serviceRequest) {
			other := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
			r.signature = signatureFor(testSecret, r.method, r.path, r.timestamp, other, r.body)
		}},
		{"body changed after signing", func(r *serviceRequest) {
			r.signature = signatureFor(testSecret, r.method, r.path, r.timestamp, r.nonce, `{"tampered":true}`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := validRequest()
			tc.mutate(&request)
			assertRejected(t, request.send(t, server.URL), http.StatusUnauthorized, "INVALID_SERVICE_AUTH")
		})
	}
}

func signatureFor(secret, method, path, timestamp, nonce, body string) string {
	digest := sha256.Sum256([]byte(body))
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// A correctly signed service may still carry an unusable actor or the wrong
// Scope; those are separate refusals from the credential check.
func TestSignedRequestStillNeedsAValidActorAndScope(t *testing.T) {
	server := newAuthServer(t)

	for _, tc := range []struct {
		name       string
		mutate     func(*serviceRequest)
		wantStatus int
		wantCode   string
	}{
		{"actor is not a UUID", func(r *serviceRequest) { r.actor = "admin" },
			http.StatusUnauthorized, "INVALID_ACTOR"},
		{"actor is empty", func(r *serviceRequest) { r.actor = "" },
			http.StatusUnauthorized, "INVALID_ACTOR"},
		{"scope kind is not product", func(r *serviceRequest) { r.scopeKind = "platform" },
			http.StatusForbidden, "SCOPE_DENIED"},
		{"product code belongs to another product", func(r *serviceRequest) { r.productCode = "library" },
			http.StatusForbidden, "SCOPE_DENIED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := validRequest()
			tc.mutate(&request)
			assertRejected(t, request.send(t, server.URL), tc.wantStatus, tc.wantCode)
		})
	}
}

// The body is read through a 256KiB MaxBytesReader before it is hashed, so an
// oversized body is refused rather than buffered.
func TestOversizedBodyIsRefused(t *testing.T) {
	server := newAuthServer(t)

	request := validRequest()
	request.method = http.MethodPost
	request.path = "/api/v1/sources"
	request.body = `{"code":"` + strings.Repeat("x", 300<<10) + `"}`

	assertRejected(t, request.send(t, server.URL), http.StatusBadRequest, "INVALID_REQUEST")
}
