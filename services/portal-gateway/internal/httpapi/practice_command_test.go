package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/session"
)

const (
	practiceCommandSecret = "portal-practice-command-secret-with-enough-entropy"
	practiceCommandUserID = "11111111-1111-4111-8111-111111111111"
)

func TestPracticeSessionCommandUsesPortalSessionActorAndDoesNotExposeAnswers(t *testing.T) {
	var gotBody []byte
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/portal/practice/sessions" {
			t.Fatalf("Core request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("X-Actor-User-Id"); got != practiceCommandUserID {
			t.Fatalf("Core actor = %q, want Portal Session user", got)
		}
		gotBody = readPracticeCommandBody(t, request)
		assertSignedPracticeCommand(t, request, gotBody, practiceCommandUserID)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"request_id":"req_core_session","data":{"session_id":"22222222-2222-4222-8222-222222222222","bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random","excluded_unavailable_count":0,"questions":[{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","type":"single","chapter_id":"ch01","chapter":"基础","content":"服务端选择的题目","options":["甲","乙"]}]}}`))
	}))
	defer core.Close()

	handler := newPracticeCommandHandler(t, core.URL)
	input := `{"bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random","question_count":1}`
	request := authenticatedPracticeCommandRequest(t, handler, http.MethodPost, "/api/v1/practice/sessions", input, "practice-session-retry-0001")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("create session status = %d: %s", response.Code, response.Body.String())
	}
	if string(gotBody) != input {
		t.Fatalf("Core body = %s, want exact client command %s", gotBody, input)
	}
	if strings.Contains(response.Body.String(), `"answer"`) || strings.Contains(response.Body.String(), `"expected_answer"`) {
		t.Fatalf("Gateway leaked an answer before submission: %s", response.Body.String())
	}
}

func TestPracticeSessionGuestForwardsOnlyCoreAnonymousCookie(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/portal/practice/sessions" {
			t.Fatalf("Core request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-Actor-User-Id") != "" {
			t.Fatalf("guest Core request unexpectedly supplied an actor header")
		}
		body := readPracticeCommandBody(t, request)
		assertSignedPracticeCommand(t, request, body, "")
		if cookie := request.Header.Get("Cookie"); cookie != "quizcraft_anonymous=core-managed-anonymous-token" {
			t.Fatalf("Core guest cookie = %q, want only the Core anonymous identity", cookie)
		}
		writer.Header().Add("Set-Cookie", "quizcraft_anonymous=rotated-core-token; Path=/; HttpOnly; Secure; SameSite=Lax")
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"request_id":"req_core_guest","data":{"session_id":"22222222-2222-4222-8222-222222222222","bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random","excluded_unavailable_count":0,"questions":[]}}`))
	}))
	defer core.Close()

	handler := newPracticeCommandHandler(t, core.URL)
	request := httptest.NewRequest(http.MethodPost, "https://portal.test/api/v1/practice/sessions", strings.NewReader(`{"bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random"}`))
	request.TLS = practiceCommandTLSState()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "practice-guest-session-0001")
	request.AddCookie(&http.Cookie{Name: "quizcraft_anonymous", Value: "core-managed-anonymous-token"})
	request.AddCookie(&http.Cookie{Name: "unrelated_browser_cookie", Value: "must-not-cross-boundary"})
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("guest create session status = %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Set-Cookie"); !strings.Contains(got, "quizcraft_anonymous=rotated-core-token") || strings.Contains(got, "Domain=") {
		t.Fatalf("Gateway did not safely forward Core anonymous cookie: %q", got)
	}
}

func TestPracticeSessionCommandFailsClosedWhenCoreIsUnavailable(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"request_id":"req_core_down","error":{"code":"writes_disabled","message":"not enabled"}}`))
	}))
	defer core.Close()

	handler := newPracticeCommandHandler(t, core.URL)
	request := authenticatedPracticeCommandRequest(t, handler, http.MethodPost, "/api/v1/practice/sessions", `{"bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random"}`, "practice-session-down-0001")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), `"questions"`) || strings.Contains(strings.ToLower(response.Body.String()), "mock") {
		t.Fatalf("Core failure was not fail-closed: %d %s", response.Code, response.Body.String())
	}
}

func TestPracticeCommandsStayDarkUntilTheExplicitGatewayGateIsEnabled(t *testing.T) {
	called := false
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		writer.WriteHeader(http.StatusCreated)
	}))
	defer core.Close()

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	handler, err := New(config.Config{
		SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
		PracticeURL:             core.URL,
		LocalOAuthCookieName:    "portal_oauth_local",
		LocalSessionCookieName:  "portal_session_local",
		PracticeCommandsEnabled: false,
	}, redisClient)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://portal.test/api/v1/practice/sessions", strings.NewReader(`{"bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random"}`))
	request.TLS = practiceCommandTLSState()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "practice-dark-gate-0001")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable || called || strings.Contains(response.Body.String(), `"questions"`) {
		t.Fatalf("dark practice command = %d called=%t body=%s", response.Code, called, response.Body.String())
	}
}

func TestPracticeCommandRejectsInvalidPortalSessionInsteadOfDowngradingToGuest(t *testing.T) {
	called := false
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		writer.WriteHeader(http.StatusCreated)
	}))
	defer core.Close()
	handler := newPracticeCommandHandler(t, core.URL)
	request := httptest.NewRequest(http.MethodPost, "https://portal.test/api/v1/practice/sessions", strings.NewReader(`{"bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random"}`))
	request.TLS = practiceCommandTLSState()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "practice-invalid-session-0001")
	request.AddCookie(&http.Cookie{Name: "__Host-henukit_portal_session", Value: "not-a-portal-session"})
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("invalid Portal Session status/called = %d/%t: %s", response.Code, called, response.Body.String())
	}
}

func TestPracticeCommandRejectsEncryptedPortalSessionWithNonUUIDUserID(t *testing.T) {
	called := false
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		writer.WriteHeader(http.StatusCreated)
	}))
	defer core.Close()
	handler := newPracticeCommandHandler(t, core.URL)
	encoded, err := handler.sessionCodec.Encode(session.Value{
		UserID:        "legacy-user-id",
		DisplayName:   "小河同学",
		ExchangeToken: strings.Repeat("x", 32),
		ExpiresAt:     time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://portal.test/api/v1/practice/sessions", strings.NewReader(`{"bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random"}`))
	request.TLS = practiceCommandTLSState()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "practice-invalid-user-id-0001")
	request.AddCookie(&http.Cookie{Name: "__Host-henukit_portal_session", Value: encoded})
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("non-UUID Portal user status/called = %d/%t: %s", response.Code, called, response.Body.String())
	}
}

func TestPracticeCommandFailsClosedForIncompleteCoreResponse(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"request_id":"req_core_session","data":{"session_id":"22222222-2222-4222-8222-222222222222","bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-8444-8444-444444444444","mode":"random","excluded_unavailable_count":0,"questions":[{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","type":"single","chapter":"基础","content":"服务端选择的题目"}]}}`))
	}))
	defer core.Close()
	handler := newPracticeCommandHandler(t, core.URL)
	request := authenticatedPracticeCommandRequest(t, handler, http.MethodPost, "/api/v1/practice/sessions", `{"bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random"}`, "practice-incomplete-response-0001")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), `"questions"`) {
		t.Fatalf("incomplete Core response = %d: %s", response.Code, response.Body.String())
	}
}

func TestPracticeCommandRejectsUnsafeCoreCookieWithoutReturningSessionData(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Add("Set-Cookie", "quizcraft_anonymous=unexpected-domain; Path=/; Domain=core.internal; HttpOnly; Secure; SameSite=Lax")
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(practiceSessionEnvelopeJSON))
	}))
	defer core.Close()
	handler := newPracticeCommandHandler(t, core.URL)
	request := authenticatedPracticeCommandRequest(t, handler, http.MethodPost, "/api/v1/practice/sessions", `{"bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random"}`, "practice-unsafe-cookie-0001")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), `"questions"`) || response.Header().Get("Set-Cookie") != "" {
		t.Fatalf("unsafe Core cookie response = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func TestPracticeAnswerCommandReturnsOnlyServerConfirmedScore(t *testing.T) {
	const sessionID = "22222222-2222-4222-8222-222222222222"
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/portal/practice/sessions/"+sessionID+"/answers" {
			t.Fatalf("Core answer path = %s", request.URL.Path)
		}
		body := readPracticeCommandBody(t, request)
		assertSignedPracticeCommand(t, request, body, practiceCommandUserID)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"request_id":"req_core_answer","data":{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","correct":false,"replayed":false,"expected_answer":1,"analysis":"服务端讲解"}}`))
	}))
	defer core.Close()
	handler := newPracticeCommandHandler(t, core.URL)
	request := authenticatedPracticeCommandRequest(t, handler, http.MethodPost, "/api/v1/practice/sessions/"+sessionID+"/answers", `{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","answer":0}`, "practice-answer-retry-0001")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"correct":false`) || !strings.Contains(response.Body.String(), `"expected_answer":1`) {
		t.Fatalf("answer result = %d: %s", response.Code, response.Body.String())
	}
}

const practiceSessionEnvelopeJSON = `{"request_id":"req_core_session","data":{"session_id":"22222222-2222-4222-8222-222222222222","bank_id":"33333333-3333-4333-8333-333333333333","bank_version_id":"44444444-4444-4444-8444-444444444444","mode":"random","excluded_unavailable_count":0,"questions":[{"question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","type":"single","chapter_id":"ch01","chapter":"基础","content":"服务端选择的题目","options":["甲","乙"]}]}}`

func newPracticeCommandHandler(t *testing.T, coreURL string) *Handler {
	t.Helper()
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	handler, err := New(config.Config{
		SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
		PracticeURL:             coreURL,
		PracticeCommandAuth:     config.ServiceAuth{ClientID: "portal-gateway", ClientSecret: practiceCommandSecret, KeyID: "portal-practice-command-key"},
		PracticeCommandsEnabled: true,
		LocalOAuthCookieName:    "portal_oauth_local",
		LocalSessionCookieName:  "portal_session_local",
	}, redisClient)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func authenticatedPracticeCommandRequest(t *testing.T, handler *Handler, method, path, body, idempotencyKey string) *http.Request {
	t.Helper()
	encoded, err := handler.sessionCodec.Encode(session.Value{
		UserID:        practiceCommandUserID,
		DisplayName:   "小河同学",
		ExchangeToken: strings.Repeat("x", 32),
		ExpiresAt:     time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, "https://portal.test"+path, strings.NewReader(body))
	request.TLS = practiceCommandTLSState()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request.AddCookie(&http.Cookie{Name: "__Host-henukit_portal_session", Value: encoded})
	return request
}

func practiceCommandTLSState() *tls.ConnectionState {
	return &tls.ConnectionState{}
}

func readPracticeCommandBody(t *testing.T, request *http.Request) []byte {
	t.Helper()
	var payload json.RawMessage
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertSignedPracticeCommand(t *testing.T, request *http.Request, body []byte, actor string) {
	t.Helper()
	user, password, basic := request.BasicAuth()
	if !basic || user != "portal-gateway" || password != practiceCommandSecret {
		t.Fatal("Core command omitted service credentials")
	}
	digest := sha256.Sum256(body)
	parts := []string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:])}
	if actor != "" {
		parts = append(parts, actor)
	}
	mac := hmac.New(sha256.New, []byte(practiceCommandSecret))
	_, _ = mac.Write([]byte(strings.Join(parts, "\n")))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if request.Header.Get("X-Signature") != want {
		t.Fatalf("Core command HMAC does not bind request body/actor")
	}
	if request.Header.Get("Idempotency-Key") == "" {
		t.Fatal("Core command omitted Idempotency-Key")
	}
}
