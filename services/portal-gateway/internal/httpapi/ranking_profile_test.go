package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"henukit.dev/portal-gateway/internal/config"
)

// rankingProfileOperationEnvelopeJSON is an OperationEnvelope-shaped Core
// write result matching validatePracticeOperationEnvelope.
const rankingProfileOperationEnvelopeJSON = `{"request_id":"req_core_profile","data":{"operation_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","state":"succeeded","idempotency_key":"ranking-profile-0001","request_id":"req_core_profile","resource_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}}`

const rankingProfileInputJSON = `{"nickname":"认真刷题","system_avatar":"scholar-blue","visible":true}`

// newRankingProfileHandler enables both the V2 read gate (which registers the
// PATCH route) and the practice command client (which forwards the write).
func newRankingProfileHandler(t *testing.T, coreURL string) *Handler {
	t.Helper()
	handler, err := New(config.Config{
		SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
		QuizCraftV2ReadsEnabled: true,
		QuizCraftCoreURL:        coreURL,
		QuizCraftCoreAuth:       config.ServiceAuth{ClientID: "portal-gateway", ClientSecret: "portal-v2-read-secret-with-enough-entropy", KeyID: "portal-v2-read-key"},
		PracticeURL:             coreURL,
		PracticeCommandAuth:     config.ServiceAuth{ClientID: "portal-gateway", ClientSecret: practiceCommandSecret, KeyID: "portal-practice-command-key"},
		PracticeCommandsEnabled: true,
		LocalOAuthCookieName:    "portal_oauth_local",
		LocalSessionCookieName:  "portal_session_local",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestUpdateRankingProfileRelaysSignedInWrite(t *testing.T) {
	var gotBody []byte
	var quizCraftCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		quizCraftCalls.Add(1)
		if request.Method != http.MethodPatch || request.URL.Path != "/api/v1/ranking-profile" {
			t.Fatalf("Core request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("X-Actor-User-Id"); got != practiceCommandUserID {
			t.Fatalf("Core actor = %q, want Portal Session user", got)
		}
		gotBody = readPracticeCommandBody(t, request)
		assertSignedPracticeCommand(t, request, gotBody, practiceCommandUserID)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(rankingProfileOperationEnvelopeJSON))
	}))
	defer core.Close()

	handler := newRankingProfileHandler(t, core.URL)
	request := authenticatedPracticeCommandRequest(t, handler, http.MethodPatch, "/api/v1/ranking-profile", rankingProfileInputJSON, "ranking-profile-write-0001")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("update ranking profile status = %d: %s", response.Code, response.Body.String())
	}
	if string(gotBody) != rankingProfileInputJSON {
		t.Fatalf("Core body = %s, want exact client command %s", gotBody, rankingProfileInputJSON)
	}
	if response.Body.String() != rankingProfileOperationEnvelopeJSON {
		t.Fatalf("Gateway did not relay Core OperationEnvelope: %s", response.Body.String())
	}
	if quizCraftCalls.Load() != 1 {
		t.Fatalf("Core calls = %d, want 1", quizCraftCalls.Load())
	}
}

func TestUpdateRankingProfileRequiresPortalSession(t *testing.T) {
	var quizCraftCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		quizCraftCalls.Add(1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer core.Close()

	handler := newRankingProfileHandler(t, core.URL)
	request := httptest.NewRequest(http.MethodPatch, "https://portal.test/api/v1/ranking-profile", strings.NewReader(rankingProfileInputJSON))
	request.TLS = practiceCommandTLSState()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "ranking-profile-anon-0001")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous ranking profile status = %d, want 401: %s", response.Code, response.Body.String())
	}
	if quizCraftCalls.Load() != 0 {
		t.Fatalf("anonymous write reached Core %d times, want 0", quizCraftCalls.Load())
	}
}

func TestUpdateRankingProfileStaysDarkBeforeV2ReadsGate(t *testing.T) {
	var quizCraftCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		quizCraftCalls.Add(1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer core.Close()

	handler, err := New(config.Config{
		SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
		PracticeURL:             core.URL,
		PracticeCommandAuth:     config.ServiceAuth{ClientID: "portal-gateway", ClientSecret: practiceCommandSecret, KeyID: "portal-practice-command-key"},
		PracticeCommandsEnabled: true,
		LocalOAuthCookieName:    "portal_oauth_local",
		LocalSessionCookieName:  "portal_session_local",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := authenticatedPracticeCommandRequest(t, handler, http.MethodPatch, "/api/v1/ranking-profile", rankingProfileInputJSON, "ranking-profile-dark-0001")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("dark ranking profile status = %d, want 404: %s", response.Code, response.Body.String())
	}
	if quizCraftCalls.Load() != 0 {
		t.Fatalf("dark write reached Core %d times, want 0", quizCraftCalls.Load())
	}
}
