package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
)

const practiceAuthSecret = "practice-test-secret-at-least-32-bytes"

type practiceQuestionResponse struct {
	QuestionID        string   `json:"question_id"`
	QuestionVersionID string   `json:"question_version_id"`
	Type              string   `json:"type"`
	ChapterID         string   `json:"chapter_id"`
	Chapter           string   `json:"chapter"`
	Content           string   `json:"content"`
	Options           []string `json:"options"`
}

type practiceSessionResponse struct {
	SessionID                string                     `json:"session_id"`
	BankID                   string                     `json:"bank_id"`
	BankVersionID            string                     `json:"bank_version_id"`
	Mode                     string                     `json:"mode"`
	ExcludedUnavailableCount int                        `json:"excluded_unavailable_count"`
	Questions                []practiceQuestionResponse `json:"questions"`
}

type answerResultResponse struct {
	QuestionID        string `json:"question_id"`
	QuestionVersionID string `json:"question_version_id"`
	Correct           bool   `json:"correct"`
	Replayed          bool   `json:"replayed"`
	ExpectedAnswer    any    `json:"expected_answer"`
	Analysis          string `json:"analysis"`
}

type apiEnvelope[T any] struct {
	RequestID string `json:"request_id"`
	Data      T      `json:"data"`
}

func TestPracticeHTTPGuestFourTypesAndReplayProtection(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "practice-four-types-"+uuid.NewString())
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
	listStatus, listBody := requestCatalog(t, server.URL, "portal.practice.read")
	if listStatus != http.StatusOK || !bytes.Contains(listBody, []byte(report.BankID)) || !bytes.Contains(listBody, []byte(`"question_count":4`)) || !bytes.Contains(listBody, []byte(`"chapters"`)) {
		t.Fatalf("bank list = %d %s", listStatus, listBody)
	}

	sessionStatus, sessionBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Idempotency-Key": "guest-session-key-0001"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 4, "future_client_field": "ignored",
	})
	if sessionStatus != http.StatusCreated {
		t.Fatalf("create session = %d %s", sessionStatus, sessionBody)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, sessionBody, &session)
	if session.RequestID == "" || len(session.Data.Questions) != 4 || session.Data.Mode != "random" {
		t.Fatalf("session = %+v", session)
	}
	foreignQuestion := session.Data.Questions[0]
	foreignStatus, _ := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{
		"Cookie": "quizcraft_anonymous=" + practiceAnonymousTokenForSubject(t, uuid.NewString()), "Idempotency-Key": "foreign-guest-answer-0001",
	}, map[string]any{"question_id": foreignQuestion.QuestionID, "question_version_id": foreignQuestion.QuestionVersionID, "answer": 0})
	if foreignStatus != http.StatusForbidden {
		t.Fatalf("foreign guest submitted another session: %d", foreignStatus)
	}
	repeatStatus, repeatBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Idempotency-Key": "guest-session-key-0001"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 4, "future_client_field": "ignored",
	})
	if repeatStatus != sessionStatus || !bytes.Equal(repeatBody, sessionBody) {
		t.Fatalf("session replay changed original result: %d %s / %s", repeatStatus, repeatBody, sessionBody)
	}
	conflictStatus, _ := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Idempotency-Key": "guest-session-key-0001"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 1,
	})
	if conflictStatus != http.StatusConflict {
		t.Fatalf("idempotency payload conflict status = %d", conflictStatus)
	}
	operationStatus, operationBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/operations/create_practice_session", map[string]string{"Idempotency-Key": "guest-session-key-0001"}, nil)
	if operationStatus != http.StatusOK || !bytes.Contains(operationBody, []byte(session.Data.SessionID)) || !bytes.Contains(operationBody, []byte(`"state":"succeeded"`)) {
		t.Fatalf("operation lookup = %d %s", operationStatus, operationBody)
	}
	var rawSession map[string]any
	decodeJSON(t, sessionBody, &rawSession)
	for _, rawQuestion := range rawSession["data"].(map[string]any)["questions"].([]any) {
		question := rawQuestion.(map[string]any)
		if _, exposed := question["answer"]; exposed {
			t.Fatal("practice session exposed an answer")
		}
		if question["content"] == "" || question["type"] == "" || question["chapter_id"] == "" || question["chapter"] == "" {
			t.Fatalf("unrenderable question: %#v", question)
		}
	}

	answers := map[string]any{}
	for _, question := range report.Questions {
		switch question.SourceQuestionID {
		case "q0001":
			answers[question.QuestionID] = float64(1)
		case "q0002":
			answers[question.QuestionID] = []any{float64(2), float64(1)}
		case "q0003":
			answers[question.QuestionID] = true
		case "q0004":
			answers[question.QuestionID] = " main "
		}
	}
	for index, question := range session.Data.Questions {
		payload := map[string]any{"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": answers[question.QuestionID]}
		key := fmt.Sprintf("guest-answer-key-%04d", index)
		status, body := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{"Idempotency-Key": key}, payload)
		if status != http.StatusOK {
			t.Fatalf("submit %s = %d %s", question.Type, status, body)
		}
		var result apiEnvelope[answerResultResponse]
		decodeJSON(t, body, &result)
		if !result.Data.Correct || result.Data.Replayed || result.Data.QuestionVersionID != question.QuestionVersionID {
			t.Fatalf("result %s = %+v", question.Type, result)
		}
		if index == 0 {
			retryStatus, retryBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{"Idempotency-Key": key}, payload)
			if retryStatus != status || !bytes.Equal(retryBody, body) {
				t.Fatalf("same-key replay changed response: %d %s / %s", retryStatus, retryBody, body)
			}
			_, otherBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{"Idempotency-Key": "guest-answer-other-0001"}, payload)
			var other apiEnvelope[answerResultResponse]
			decodeJSON(t, otherBody, &other)
			if !other.Data.Replayed || !other.Data.Correct {
				t.Fatalf("different-key duplicate = %+v", other)
			}
		}
	}
	var attempts, learningRows int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_practice_attempts WHERE session_id=$1`, session.Data.SessionID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_learning_state WHERE bank_id=$1`, report.BankID).Scan(&learningRows); err != nil {
		t.Fatal(err)
	}
	if attempts != 4 || learningRows != 0 {
		t.Fatalf("guest facts attempts/learning = %d/%d", attempts, learningRows)
	}
	difficultStatus, difficultBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Idempotency-Key": "guest-difficult-key-01"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "difficult", "question_count": 2,
	})
	if difficultStatus != http.StatusCreated || !bytes.Contains(difficultBody, []byte(`"mode":"difficult"`)) {
		t.Fatalf("difficult session = %d %s", difficultStatus, difficultBody)
	}
}

func TestPracticeHTTPIssuesSecureAnonymousCookie(t *testing.T) {
	pool := practicePool(t)
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/learning-state", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("guest learning state = %d", response.StatusCode)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "quizcraft_anonymous" || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" || cookies[0].Domain != "" {
		t.Fatalf("anonymous cookie = %#v", cookies)
	}
}

func TestPracticeHTTPRejectsMissingSessionWithoutWritingFacts(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "practice-missing-session-"+uuid.NewString())
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	var attemptsBefore, idempotencyFactsBefore int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM quizcraft_practice_attempts),(SELECT count(*) FROM quizcraft_idempotency_results)`).Scan(&attemptsBefore, &idempotencyFactsBefore); err != nil {
		t.Fatal(err)
	}
	question := report.Questions[0]
	status, body := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+uuid.NewString()+"/answers", map[string]string{"Idempotency-Key": "missing-session-answer-0001"}, map[string]any{
		"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": 0,
	})
	if status != http.StatusNotFound || !bytes.Contains(body, []byte(`"code":"session_not_found"`)) {
		t.Fatalf("missing session = %d %s", status, body)
	}
	var attempts, idempotencyFacts int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM quizcraft_practice_attempts),(SELECT count(*) FROM quizcraft_idempotency_results)`).Scan(&attempts, &idempotencyFacts); err != nil {
		t.Fatal(err)
	}
	if attempts != attemptsBefore || idempotencyFacts != idempotencyFactsBefore {
		t.Fatalf("missing session changed attempt/idempotency facts from %d/%d to %d/%d", attemptsBefore, idempotencyFactsBefore, attempts, idempotencyFacts)
	}
}

func TestPracticeHTTPScoresImportedChoiceTextAnswers(t *testing.T) {
	pool := practicePool(t)
	var bank map[string]any
	decodeJSON(t, []byte(validBank), &bank)
	questions := bank["questions"].([]any)
	questions[0].(map[string]any)["answer"] = "2"
	questions[1].(map[string]any)["answer"] = []any{"2", "4"}
	service, err := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.ImportJSON(context.Background(), "practice-text-answer-"+uuid.NewString(), mustJSON(bank))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	status, body := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Idempotency-Key": "text-answer-session-0001"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 4,
	})
	if status != http.StatusCreated {
		t.Fatalf("create session = %d %s", status, body)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, body, &session)
	for index, question := range session.Data.Questions {
		var answer any
		switch question.Type {
		case "single":
			answer = 1
		case "multi":
			answer = []any{float64(2), float64(1)}
		default:
			continue
		}
		answerStatus, answerBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{"Idempotency-Key": fmt.Sprintf("text-answer-submit-%04d", index)}, map[string]any{
			"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": answer,
		})
		var result apiEnvelope[answerResultResponse]
		decodeJSON(t, answerBody, &result)
		if answerStatus != http.StatusOK || !result.Data.Correct {
			t.Fatalf("text %s result = %d %+v", question.Type, answerStatus, result)
		}
	}
	for index, question := range session.Data.Questions {
		if question.Type != "judge" {
			continue
		}
		nullStatus, nullBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{"Idempotency-Key": fmt.Sprintf("null-answer-submit-%04d", index)}, map[string]any{
			"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": nil, "future_client_field": "ignored",
		})
		var result apiEnvelope[answerResultResponse]
		decodeJSON(t, nullBody, &result)
		if nullStatus != http.StatusOK || result.Data.Correct {
			t.Fatalf("null answer result = %d %+v", nullStatus, result)
		}
	}
}

func TestPracticeHTTPAuthenticatedLearningStateAndConcurrentReplay(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "practice-auth-"+uuid.NewString())
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	invalidAuthStatus, _ := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Authorization": "Bearer invalid", "Idempotency-Key": "invalid-auth-key-0001"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 1,
	})
	if invalidAuthStatus != http.StatusUnauthorized {
		t.Fatalf("invalid bearer was treated as guest: %d", invalidAuthStatus)
	}
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	status, body := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Cookie": auth, "Idempotency-Key": "auth-session-key-0001"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 1,
	})
	if status != http.StatusCreated {
		t.Fatalf("auth session = %d %s", status, body)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, body, &session)
	question := session.Data.Questions[0]
	payload := map[string]any{"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": "definitely-wrong"}

	const workers = 12
	statuses := make(chan int, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", bytes.NewReader(mustJSON(payload)))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Cookie", auth)
			request.Header.Set("Idempotency-Key", fmt.Sprintf("concurrent-answer-%04d", index))
			response, requestErr := http.DefaultClient.Do(request)
			if requestErr != nil {
				statuses <- 0
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			statuses <- response.StatusCode
		}(index)
	}
	wait.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent submit status = %d", status)
		}
	}
	stateStatus, stateBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/learning-state", map[string]string{"Cookie": auth}, nil)
	if stateStatus != http.StatusOK || !bytes.Contains(stateBody, []byte(question.QuestionID)) || !bytes.Contains(stateBody, []byte(`"wrong":true`)) {
		t.Fatalf("learning state = %d %s", stateStatus, stateBody)
	}
	guestStatus, _ := requestJSON(t, http.MethodGet, server.URL+"/api/v1/learning-state", nil, nil)
	if guestStatus != http.StatusUnauthorized {
		t.Fatalf("guest learning state status = %d", guestStatus)
	}
	var attempts, statAttempts, learningRows int
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM quizcraft_practice_attempts WHERE session_id=$1),(SELECT attempt_count FROM quizcraft_question_stats WHERE question_id=$2),(SELECT count(*) FROM quizcraft_learning_state WHERE user_id=$3)`, session.Data.SessionID, question.QuestionID, userID).Scan(&attempts, &statAttempts, &learningRows); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || statAttempts != 1 || learningRows != 1 {
		t.Fatalf("concurrent facts = %d/%d/%d", attempts, statAttempts, learningRows)
	}
}

func TestPracticeHTTPConcurrentSessionsKeepLatestLearningStateReconciled(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "practice-learning-state-order-"+uuid.NewString())
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	userID := uuid.New()
	auth := "quizcraft_session=" + practiceToken(t, userID.String())
	blockedSession, blockedQuestion := createRankingSession(t, server.URL, auth, report, "learning-state-blocked-session")
	laterSession, laterQuestion := createRankingSession(t, server.URL, auth, report, "learning-state-later-session")
	if laterQuestion.QuestionID != blockedQuestion.QuestionID || laterQuestion.QuestionVersionID != blockedQuestion.QuestionVersionID {
		t.Fatalf("sessions did not select the same stable question: %+v / %+v", blockedQuestion, laterQuestion)
	}

	blocker, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(context.Background()) }()
	if _, err := blocker.Exec(context.Background(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, blockedSession); err != nil {
		t.Fatal(err)
	}

	type submissionOutcome struct {
		status int
		body   []byte
		err    error
	}
	blockedResult := make(chan submissionOutcome, 1)
	go func() {
		request, requestErr := http.NewRequest(http.MethodPost, server.URL+"/api/v1/practice/sessions/"+blockedSession+"/answers", bytes.NewReader(mustJSON(map[string]any{
			"question_id": blockedQuestion.QuestionID, "question_version_id": blockedQuestion.QuestionVersionID, "answer": correctAnswerFor(blockedQuestion),
		})))
		if requestErr != nil {
			blockedResult <- submissionOutcome{err: requestErr}
			return
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Cookie", auth)
		request.Header.Set("Idempotency-Key", "learning-state-blocked-answer")
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			blockedResult <- submissionOutcome{err: requestErr}
			return
		}
		defer response.Body.Close()
		body, requestErr := io.ReadAll(response.Body)
		blockedResult <- submissionOutcome{status: response.StatusCode, body: body, err: requestErr}
	}()
	waitForAdvisoryWaiters(t, pool, 1)

	laterStatus, laterBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+laterSession+"/answers", map[string]string{
		"Cookie": auth, "Idempotency-Key": "learning-state-later-answer",
	}, map[string]any{"question_id": laterQuestion.QuestionID, "question_version_id": laterQuestion.QuestionVersionID, "answer": "definitely-wrong"})
	if laterStatus != http.StatusOK || !bytes.Contains(laterBody, []byte(`"correct":false`)) {
		t.Fatalf("later answer = %d %s", laterStatus, laterBody)
	}
	if err := blocker.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	blocked := <-blockedResult
	if blocked.err != nil || blocked.status != http.StatusOK || !bytes.Contains(blocked.body, []byte(`"correct":true`)) {
		t.Fatalf("blocked answer = %d %s err=%v", blocked.status, blocked.body, blocked.err)
	}

	reconciliation, err := quizcraft.ReconcileLearningState(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if !reconciliation.Clean() {
		t.Fatalf("concurrent learning state reconciliation = %+v", reconciliation)
	}

	var wrong bool
	var attempts, correct int64
	var latestAttemptID, expectedLatestAttemptID uuid.UUID
	var expectedWrong bool
	if err := pool.QueryRow(context.Background(), `
SELECT state.wrong,state.attempt_count,state.correct_count,state.latest_attempt_id,
       latest.id,(NOT latest.correct)
FROM quizcraft_learning_state AS state
JOIN LATERAL (
    SELECT id,correct
    FROM quizcraft_practice_attempts
    WHERE user_id=$1 AND bank_id=$2 AND question_id=$3
    ORDER BY submitted_at DESC,id DESC
    LIMIT 1
) AS latest ON true
WHERE state.user_id=$1 AND state.bank_id=$2 AND state.question_id=$3
`, userID, report.BankID, blockedQuestion.QuestionID).Scan(&wrong, &attempts, &correct, &latestAttemptID, &expectedLatestAttemptID, &expectedWrong); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || correct != 1 || wrong != expectedWrong || latestAttemptID != expectedLatestAttemptID {
		t.Fatalf("latest learning state = wrong:%v attempts:%d correct:%d latest:%s; want wrong:%v attempts:2 correct:1 latest:%s", wrong, attempts, correct, latestAttemptID, expectedWrong, expectedLatestAttemptID)
	}

	if rebuilt, err := quizcraft.RebuildLearningState(context.Background(), pool); err != nil || !rebuilt.Clean() {
		t.Fatalf("rebuilt concurrent learning state = %+v err=%v", rebuilt, err)
	}
	if reconciled, err := quizcraft.ReconcileLearningState(context.Background(), pool); err != nil || !reconciled.Clean() {
		t.Fatalf("post-rebuild concurrent learning state = %+v err=%v", reconciled, err)
	}
}

func TestPracticeHTTPLegacyShadowComparison(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "practice-shadow-"+uuid.NewString())
	legacy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/practice/shadow-compare" || request.Header.Get("X-QuizCraft-Shadow-Secret") != "shadow-compare-secret-at-least-32-bytes" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"correct":true,"correct_answer":1,"analysis":""}`))
	}))
	defer legacy.Close()
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), LegacyBaseURL: legacy.URL, LegacyCompareSecret: "shadow-compare-secret-at-least-32-bytes"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	_, body := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Idempotency-Key": "shadow-session-key-001"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "chapter", "chapter_id": "ch01", "question_count": 4,
	})
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, body, &session)
	var selected practiceQuestionResponse
	for _, question := range session.Data.Questions {
		if question.Type == "single" {
			selected = question
		}
	}
	status, submitBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{"Idempotency-Key": "shadow-answer-key-0001"}, map[string]any{
		"question_id": selected.QuestionID, "question_version_id": selected.QuestionVersionID, "answer": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("shadow submit = %d %s", status, submitBody)
	}
	var outcome string
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := pool.QueryRow(context.Background(), `SELECT outcome FROM quizcraft_shadow_comparisons WHERE session_id=$1 AND question_id=$2`, session.Data.SessionID, selected.QuestionID).Scan(&outcome)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if outcome != "match" {
		t.Fatalf("shadow outcome = %s", outcome)
	}
}

func practicePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("QUIZCRAFT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func importPracticeBank(t *testing.T, pool *pgxpool.Pool, key string) quizcraft.ImportReport {
	t.Helper()
	service, err := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.ImportJSON(context.Background(), key, []byte(validBank))
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func practiceToken(t *testing.T, userID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": userID, "iss": "quizcraft-session", "exp": time.Now().Add(time.Hour).Unix(), "aud": "quizcraft"})
	signed, err := token.SignedString([]byte(practiceAuthSecret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func requestJSON(t *testing.T, method, url string, headers map[string]string, payload any) (int, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(mustJSON(payload))
	}
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if request.Header.Get("Authorization") == "" && request.Header.Get("Cookie") == "" {
		request.Header.Set("Cookie", "quizcraft_anonymous="+practiceAnonymousToken(t))
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, responseBody
}

func practiceAnonymousToken(t *testing.T) string {
	t.Helper()
	subject := uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name())).String()
	return practiceAnonymousTokenForSubject(t, subject)
}

func practiceAnonymousTokenForSubject(t *testing.T, subject string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": subject, "iss": "quizcraft-anonymous", "exp": time.Now().Add(time.Hour).Unix(), "aud": "quizcraft"})
	signed, err := token.SignedString([]byte(practiceAuthSecret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func decodeJSON(t *testing.T, raw []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode %s: %v", strings.TrimSpace(string(raw)), err)
	}
}
