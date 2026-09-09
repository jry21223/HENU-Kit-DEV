package tests

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
)

type portalLearningStateItem struct {
	BankID            string `json:"bank_id"`
	QuestionID        string `json:"question_id"`
	QuestionVersionID string `json:"question_version_id"`
	Wrong             bool   `json:"wrong"`
	AttemptCount      int64  `json:"attempt_count"`
	CorrectCount      int64  `json:"correct_count"`
	UpdatedAt         string `json:"updated_at"`
}

// TestPortalLearningStateRequiresTheSignedActorAndIsScopedToThatAccount pins
// the behaviour Portal Gateway actually depends on. Gateway's actorBoundRead
// sends a brand-new request carrying the six-part signature and no browser
// cookie, so the Portal learning-state route must derive its identity from the
// signed X-Actor-User-Id. Core's own /api/v1/learning-state cannot serve that
// caller: it authenticates a cookie and answers 401 for a cookie-less request.
func TestPortalLearningStateRequiresTheSignedActorAndIsScopedToThatAccount(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "portal-learning-state-"+uuid.NewString())

	darkHandler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:       pool,
		AuthHMACSecret: []byte(practiceAuthSecret),
	})
	if err != nil {
		t.Fatal(err)
	}
	darkServer := httptest.NewServer(darkHandler)
	defer darkServer.Close()
	if status, _ := requestJSON(t, http.MethodGet, darkServer.URL+"/api/v1/portal/practice/learning-state", nil, nil); status != http.StatusNotFound {
		t.Fatalf("unconfigured Portal learning-state route = %d, want 404", status)
	}

	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:        pool,
		AuthHMACSecret:  []byte(practiceAuthSecret),
		CatalogClientID: portalCatalogClientID,
		CatalogKeys:     map[string]string{portalCatalogKeyID: portalCatalogSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	answered := answerOnePracticeQuestionWrong(t, server.URL, auth, report, "portal-learning-state")

	// Without any signature the route must refuse. This is the exact shape of
	// request that the old /api/v1/learning-state proxy produced, minus the
	// signature, and it must never fall back to an anonymous actor.
	unsignedStatus, unsignedBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/portal/practice/learning-state", nil, nil)
	if unsignedStatus != http.StatusUnauthorized || bytes.Contains(unsignedBody, []byte(`"data"`)) {
		t.Fatalf("unsigned Portal learning state = %d %s", unsignedStatus, unsignedBody)
	}

	// A signed request whose actor line was swapped after signing must fail the
	// six-part HMAC rather than serve the substituted account.
	tampered := newPortalActorReadRequest(t, server.URL, "/api/v1/portal/practice/learning-state", userID)
	tampered.Header.Set("X-Actor-User-Id", uuid.NewString())
	tamperedStatus, tamperedBody := sendCatalogRequest(t, tampered)
	if tamperedStatus != http.StatusUnauthorized || bytes.Contains(tamperedBody, []byte(`"data"`)) {
		t.Fatalf("tampered Portal learning-state actor = %d %s", tamperedStatus, tamperedBody)
	}

	missingActor := newPortalActorReadRequest(t, server.URL, "/api/v1/portal/practice/learning-state", userID)
	missingActor.Header.Del("X-Actor-User-Id")
	missingStatus, missingBody := sendCatalogRequest(t, missingActor)
	if missingStatus != http.StatusUnauthorized || bytes.Contains(missingBody, []byte(`"data"`)) {
		t.Fatalf("missing Portal learning-state actor = %d %s", missingStatus, missingBody)
	}

	for _, actor := range []string{"anonymous", "not-a-uuid", uuid.Nil.String()} {
		status, body := sendCatalogRequest(t, newPortalActorReadRequest(t, server.URL, "/api/v1/portal/practice/learning-state", actor))
		if status != http.StatusUnauthorized || bytes.Contains(body, []byte(`"data"`)) {
			t.Fatalf("invalid Portal learning-state actor %q = %d %s", actor, status, body)
		}
	}

	// The correctly signed request returns this actor's own facts, written by
	// the real answer flow above.
	signedStatus, signedBody := sendCatalogRequest(t, newPortalActorReadRequest(t, server.URL, "/api/v1/portal/practice/learning-state", userID))
	if signedStatus != http.StatusOK {
		t.Fatalf("signed Portal learning state = %d %s", signedStatus, signedBody)
	}
	var signed apiEnvelope[[]portalLearningStateItem]
	decodeJSON(t, signedBody, &signed)
	if signed.RequestID == "" {
		t.Fatalf("signed Portal learning state carried no request id: %s", signedBody)
	}
	if len(signed.Data) != 1 {
		t.Fatalf("signed Portal learning state rows = %d, want 1: %s", len(signed.Data), signedBody)
	}
	row := signed.Data[0]
	if row.BankID != report.BankID || row.QuestionID != answered.QuestionID || row.QuestionVersionID != answered.QuestionVersionID {
		t.Fatalf("signed Portal learning state row = %+v, want bank %s question %s", row, report.BankID, answered.QuestionID)
	}
	if !row.Wrong || row.AttemptCount != 1 || row.CorrectCount != 0 || row.UpdatedAt == "" {
		t.Fatalf("signed Portal learning state facts = %+v", row)
	}

	// A different signed actor must not see the first account's rows.
	otherStatus, otherBody := sendCatalogRequest(t, newPortalActorReadRequest(t, server.URL, "/api/v1/portal/practice/learning-state", uuid.NewString()))
	if otherStatus != http.StatusOK {
		t.Fatalf("other-actor Portal learning state = %d %s", otherStatus, otherBody)
	}
	var other apiEnvelope[[]portalLearningStateItem]
	decodeJSON(t, otherBody, &other)
	if other.Data == nil || len(other.Data) != 0 {
		t.Fatalf("other actor received another account's learning state: %s", otherBody)
	}

	// The cookie-authenticated route QuizCraft's own web app calls is untouched
	// and still serves the same facts to the same account.
	cookieStatus, cookieBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/learning-state", map[string]string{"Cookie": auth}, nil)
	if cookieStatus != http.StatusOK || !bytes.Contains(cookieBody, []byte(answered.QuestionID)) {
		t.Fatalf("cookie learning state = %d %s", cookieStatus, cookieBody)
	}

	// And it still rejects the cookie-less request that Portal Gateway sends,
	// which is precisely why the Portal route above has to exist.
	guestStatus, _ := requestJSON(t, http.MethodGet, server.URL+"/api/v1/learning-state", nil, nil)
	if guestStatus != http.StatusUnauthorized {
		t.Fatalf("cookie-less learning state = %d, want 401", guestStatus)
	}
}

// answerOnePracticeQuestionWrong drives the real session/answer flow so the
// learning-state assertions read facts the service itself wrote.
func answerOnePracticeQuestionWrong(t *testing.T, baseURL, auth string, report quizcraft.ImportReport, prefix string) practiceQuestionResponse {
	t.Helper()
	sessionStatus, sessionBody := requestJSON(t, http.MethodPost, baseURL+"/api/v1/practice/sessions", map[string]string{
		"Cookie": auth, "Idempotency-Key": prefix + "-session-0001",
	}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 1,
	})
	if sessionStatus != http.StatusCreated {
		t.Fatalf("create practice session = %d %s", sessionStatus, sessionBody)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, sessionBody, &session)
	if len(session.Data.Questions) != 1 {
		t.Fatalf("practice session questions = %d, want 1", len(session.Data.Questions))
	}
	question := session.Data.Questions[0]
	answerStatus, answerBody := requestJSON(t, http.MethodPost, baseURL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{
		"Cookie": auth, "Idempotency-Key": fmt.Sprintf("%s-answer-0001", prefix),
	}, map[string]any{"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": "definitely-wrong"})
	if answerStatus != http.StatusOK {
		t.Fatalf("submit answer = %d %s", answerStatus, answerBody)
	}
	return question
}
