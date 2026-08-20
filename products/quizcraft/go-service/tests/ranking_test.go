package tests

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
)

// TestRankingHTTPCountsNewSessionsOnceAndCarriesInternalUserID verifies the
// internal Portal-read ranking contract: every correct answer counts once per
// session/question, tied scores share a rank, and each entry carries the
// signed-in learner's user_id (the Portal Gateway must strip it externally).
func TestRankingHTTPCountsNewSessionsOnceAndCarriesInternalUserID(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "ranking-"+uuid.NewString())
	handler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), CatalogClientID: portalCatalogClientID, CatalogKeys: map[string]string{portalCatalogKeyID: portalCatalogSecret}})
	server := httptest.NewServer(handler)
	defer server.Close()
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)

	firstSession, firstQuestion := createRankingSession(t, server.URL, auth, report, "ranking-session-first")
	const workers = 10
	statuses := make(chan int, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			request, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, firstSession), bytes.NewReader(mustJSON(map[string]any{"question_id": firstQuestion.QuestionID, "question_version_id": firstQuestion.QuestionVersionID, "answer": 1})))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Cookie", auth)
			request.Header.Set("Idempotency-Key", fmt.Sprintf("ranking-answer-race-%04d", index))
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				statuses <- 0
				return
			}
			_ = response.Body.Close()
			statuses <- response.StatusCode
		}(index)
	}
	wait.Wait()
	close(statuses)
	for workerStatus := range statuses {
		if workerStatus != http.StatusOK {
			t.Fatalf("ranking race = %d", workerStatus)
		}
	}
	secondSession, secondQuestion := createRankingSession(t, server.URL, auth, report, "ranking-session-second")
	if secondQuestion.QuestionID != firstQuestion.QuestionID {
		t.Fatalf("expected stable repeated question: %s / %s", firstQuestion.QuestionID, secondQuestion.QuestionID)
	}
	answerStatus, answerBody := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, secondSession), map[string]string{"Cookie": auth, "Idempotency-Key": "ranking-answer-second"}, map[string]any{"question_id": secondQuestion.QuestionID, "question_version_id": secondQuestion.QuestionVersionID, "answer": 1})
	if answerStatus != http.StatusOK || !bytes.Contains(answerBody, []byte(`"correct":true`)) {
		t.Fatalf("second answer = %d %s", answerStatus, answerBody)
	}
	secondUserID := uuid.NewString()
	secondAuth := "quizcraft_session=" + practiceToken(t, secondUserID)
	for index := 0; index < 2; index++ {
		sessionID, question := createRankingSession(t, server.URL, secondAuth, report, fmt.Sprintf("ranking-tie-session-%02d", index))
		status, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, sessionID), map[string]string{"Cookie": secondAuth, "Idempotency-Key": fmt.Sprintf("ranking-tie-answer-%03d", index)}, map[string]any{"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": 1})
		if status != http.StatusOK {
			t.Fatalf("tie answer %d = %d", index, status)
		}
	}

	// The per-bank ranking is isolated to this test's bank, so exact rank and
	// count assertions are safe there. The overall ranking is shared with every
	// other test in the package; assert only presence and the internal shape.
	bankStatus, bankBody := requestPortalRead(t, server.URL, fmt.Sprintf("/api/v1/banks/%s/rankings?period=lifetime", report.BankID), "portal.practice.read", "req_ranking_bank_isolated")
	if bankStatus != http.StatusOK || !bytes.Contains(bankBody, []byte(`"user_id":"`+userID+`"`)) || !bytes.Contains(bankBody, []byte(`"user_id":"`+secondUserID+`"`)) || bytes.Contains(bankBody, []byte(`"guest_key":"guest:`)) || !bytes.Contains(bankBody, []byte(`"guest_key":null`)) || bytes.Contains(bankBody, []byte(`"nickname"`)) || bytes.Contains(bankBody, []byte(`"system_avatar"`)) || bytes.Count(bankBody, []byte(`"correct_answer_count":2`)) != 2 {
		t.Fatalf("internal bank ranking = %d %s", bankStatus, bankBody)
	}
	if bytes.Count(bankBody, []byte(`"rank":1`)) != 2 {
		t.Fatalf("equal scores are not tied: %s", bankBody)
	}
	overallStatus, overallBody := requestPortalRead(t, server.URL, "/api/v1/rankings/overall", "portal.practice.read", "req_ranking_overall_shape")
	if overallStatus != http.StatusOK || !bytes.Contains(overallBody, []byte(`"user_id":"`+userID+`"`)) || !bytes.Contains(overallBody, []byte(`"user_id":"`+secondUserID+`"`)) || bytes.Contains(overallBody, []byte(`"nickname"`)) || bytes.Contains(overallBody, []byte(`"system_avatar"`)) {
		t.Fatalf("internal overall ranking = %d %s", overallStatus, overallBody)
	}
	invalidPeriod, _ := requestPortalRead(t, server.URL, "/api/v1/rankings/overall?period=monthly", "portal.practice.read", "req_ranking_invalid_period")
	if invalidPeriod != http.StatusBadRequest {
		t.Fatalf("invalid period = %d", invalidPeriod)
	}

	futureNow := time.Now().UTC().AddDate(0, 0, 8)
	futureHandler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), CatalogClientID: portalCatalogClientID, CatalogKeys: map[string]string{portalCatalogKeyID: portalCatalogSecret}, Now: func() time.Time { return futureNow }})
	future := httptest.NewServer(futureHandler)
	defer future.Close()
	_, weeklyBody := requestPortalReadAt(t, future.URL, "/api/v1/rankings/overall", "portal.practice.read", "req_ranking_future_weekly", futureNow)
	_, lifetimeBody := requestPortalReadAt(t, future.URL, "/api/v1/rankings/overall?period=lifetime", "portal.practice.read", "req_ranking_future_lifetime", futureNow)
	if bytes.Contains(weeklyBody, []byte(userID)) || !bytes.Contains(lifetimeBody, []byte(userID)) {
		t.Fatalf("weekly/lifetime boundary = %s / %s", weeklyBody, lifetimeBody)
	}
}

// TestRankingHTTPStaysDarkUntilPortalGatewayAndRanksUsersAndGuests verifies the
// ranking routes stay dark until the Portal read identity is configured, then
// shows that both a signed-in learner (user_id set, no profile needed) and a
// guest (user_id null) enter the internal ranking.
func TestRankingHTTPStaysDarkUntilPortalGatewayAndRanksUsersAndGuests(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "ranking-dark-"+uuid.NewString())

	darkHandler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:       pool,
		AuthHMACSecret: []byte(practiceAuthSecret),
	})
	if err != nil {
		t.Fatal(err)
	}
	darkServer := httptest.NewServer(darkHandler)
	defer darkServer.Close()
	darkStatus, _ := requestJSON(t, http.MethodGet, darkServer.URL+"/api/v1/rankings/overall", nil, nil)
	if darkStatus != http.StatusNotFound {
		t.Fatalf("unconfigured V2 ranking route = %d, want 404", darkStatus)
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

	unsignedStatus, unsignedBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/rankings/overall", nil, nil)
	if unsignedStatus != http.StatusUnauthorized || bytes.Contains(unsignedBody, []byte(`"data"`)) {
		t.Fatalf("unsigned V2 ranking = %d %s", unsignedStatus, unsignedBody)
	}
	forbiddenStatus, forbiddenBody := requestPortalRead(t, server.URL, "/api/v1/rankings/overall", "portal.library.read", "req_ranking_forbidden")
	if forbiddenStatus != http.StatusForbidden || bytes.Contains(forbiddenBody, []byte(`"data"`)) {
		t.Fatalf("forbidden V2 ranking = %d %s", forbiddenStatus, forbiddenBody)
	}

	// A signed-in learner with no ranking profile still ranks, carrying user_id.
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	sessionID, question := createRankingSession(t, server.URL, auth, report, "ranking-user-session")
	answerStatus, answerBody := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, sessionID), map[string]string{"Cookie": auth, "Idempotency-Key": "ranking-user-answer"}, map[string]any{"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": 1})
	if answerStatus != http.StatusOK || !bytes.Contains(answerBody, []byte(`"correct":true`)) {
		t.Fatalf("user ranking answer = %d %s", answerStatus, answerBody)
	}

	// A guest (anonymous cookie) ranks with user_id null.
	guestSessionID, guestQuestion := createRankingSession(t, server.URL, "", report, "ranking-guest-session")
	guestAnswerStatus, guestAnswerBody := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, guestSessionID), map[string]string{"Idempotency-Key": "ranking-guest-answer"}, map[string]any{"question_id": guestQuestion.QuestionID, "question_version_id": guestQuestion.QuestionVersionID, "answer": 1})
	if guestAnswerStatus != http.StatusOK || !bytes.Contains(guestAnswerBody, []byte(`"correct":true`)) {
		t.Fatalf("guest ranking answer = %d %s", guestAnswerStatus, guestAnswerBody)
	}

	status, body := requestPortalRead(t, server.URL, "/api/v1/rankings/overall?period=lifetime", "portal.practice.read", "req_ranking_users_and_guests")
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"user_id":"`+userID+`"`)) || !bytes.Contains(body, []byte(`"user_id":null`)) || !bytes.Contains(body, []byte(`"guest_key":null`)) || !bytes.Contains(body, []byte(`"guest_key":"guest:`)) || bytes.Contains(body, []byte(`"nickname"`)) || bytes.Contains(body, []byte(`"system_avatar"`)) {
		t.Fatalf("internal ranking with user and guest = %d %s", status, body)
	}
	// The guest entry's guest_key is the session's immutable actor_key
	// ("guest:<anonymous subject>"), distinct from the signed-in user's null.
	guestSubject := uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name())).String()
	if !bytes.Contains(body, []byte(`"guest_key":"guest:`+guestSubject+`"`)) {
		t.Fatalf("guest ranking lacks the stable guest_key: %s", body)
	}
	bankStatus, bankBody := requestPortalRead(t, server.URL, fmt.Sprintf("/api/v1/banks/%s/rankings?period=lifetime", report.BankID), "portal.practice.read", "req_ranking_bank_users_and_guests")
	if bankStatus != http.StatusOK || !bytes.Contains(bankBody, []byte(`"user_id":"`+userID+`"`)) || !bytes.Contains(bankBody, []byte(`"user_id":null`)) || !bytes.Contains(bankBody, []byte(`"guest_key":null`)) || !bytes.Contains(bankBody, []byte(`"guest_key":"guest:`+guestSubject+`"`)) || bytes.Count(bankBody, []byte(`"correct_answer_count":1`)) != 2 {
		t.Fatalf("internal bank ranking with user and guest = %d %s", bankStatus, bankBody)
	}
}

// TestRankingAttributesGuestsViaImmutableSessionActorKey verifies the guest
// attribution path: a guest identity (immutable sessions.actor_key) aggregates
// correct answers across multiple sessions under user_id null, and mixed
// guest/signed-in entries share one ranking.
func TestRankingAttributesGuestsViaImmutableSessionActorKey(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "ranking-guest-dedup-"+uuid.NewString())
	handler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), CatalogClientID: portalCatalogClientID, CatalogKeys: map[string]string{portalCatalogKeyID: portalCatalogSecret}})
	server := httptest.NewServer(handler)
	defer server.Close()

	// The same guest answers the same question across two separate sessions;
	// UNIQUE(session_id, question_id) dedupes within a session, so the guest
	// identity accumulates one count per session.
	firstSession, firstQuestion := createRankingSession(t, server.URL, "", report, "guest-dedup-session-first")
	firstStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, firstSession), map[string]string{"Idempotency-Key": "guest-dedup-answer-first"}, map[string]any{"question_id": firstQuestion.QuestionID, "question_version_id": firstQuestion.QuestionVersionID, "answer": 1})
	if firstStatus != http.StatusOK {
		t.Fatalf("guest first answer = %d", firstStatus)
	}
	secondSession, secondQuestion := createRankingSession(t, server.URL, "", report, "guest-dedup-session-second")
	if secondQuestion.QuestionID != firstQuestion.QuestionID {
		t.Fatalf("expected stable repeated question: %s / %s", firstQuestion.QuestionID, secondQuestion.QuestionID)
	}
	secondStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, secondSession), map[string]string{"Idempotency-Key": "guest-dedup-answer-second"}, map[string]any{"question_id": secondQuestion.QuestionID, "question_version_id": secondQuestion.QuestionVersionID, "answer": 1})
	if secondStatus != http.StatusOK {
		t.Fatalf("guest second answer = %d", secondStatus)
	}

	// A signed-in learner on the same bank provides a mixed-identity ranking.
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	userSession, userQuestion := createRankingSession(t, server.URL, auth, report, "guest-dedup-user-session")
	userStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, userSession), map[string]string{"Cookie": auth, "Idempotency-Key": "guest-dedup-user-answer"}, map[string]any{"question_id": userQuestion.QuestionID, "question_version_id": userQuestion.QuestionVersionID, "answer": 1})
	if userStatus != http.StatusOK {
		t.Fatalf("user answer = %d", userStatus)
	}

	// A second, distinct guest (a different anonymous subject) ranks too, so
	// the test can pin that different guests get different guest_keys.
	otherGuestSubject := uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name()+"-other-guest")).String()
	otherGuestAuth := "quizcraft_anonymous=" + practiceAnonymousTokenForSubject(t, otherGuestSubject)
	otherSession, otherQuestion := createRankingSession(t, server.URL, otherGuestAuth, report, "guest-other-session")
	otherStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, otherSession), map[string]string{"Cookie": otherGuestAuth, "Idempotency-Key": "guest-other-answer"}, map[string]any{"question_id": otherQuestion.QuestionID, "question_version_id": otherQuestion.QuestionVersionID, "answer": 1})
	if otherStatus != http.StatusOK {
		t.Fatalf("other guest answer = %d", otherStatus)
	}

	// The repeated guest aggregates across sessions under one stable guest_key
	// ("guest:<anonymous subject>"); the other guest and the signed-in user
	// keep their own keys, and the signed-in user's guest_key is null.
	guestSubject := uuid.NewSHA1(uuid.NameSpaceOID, []byte(t.Name())).String()
	bankStatus, bankBody := requestPortalRead(t, server.URL, fmt.Sprintf("/api/v1/banks/%s/rankings?period=lifetime", report.BankID), "portal.practice.read", "req_ranking_guest_dedup")
	if bankStatus != http.StatusOK || !bytes.Contains(bankBody, []byte(`"user_id":null`)) || !bytes.Contains(bankBody, []byte(`"user_id":"`+userID+`"`)) || !bytes.Contains(bankBody, []byte(`"guest_key":"guest:`+guestSubject+`"`)) || !bytes.Contains(bankBody, []byte(`"guest_key":"guest:`+otherGuestSubject+`"`)) || !bytes.Contains(bankBody, []byte(`"guest_key":null`)) || !bytes.Contains(bankBody, []byte(`"correct_answer_count":2`)) || !bytes.Contains(bankBody, []byte(`"correct_answer_count":1`)) || bytes.Contains(bankBody, []byte(`"nickname"`)) || bytes.Contains(bankBody, []byte(`"system_avatar"`)) {
		t.Fatalf("mixed guest/user bank ranking = %d %s", bankStatus, bankBody)
	}
}

func TestRankingSettlementFactsAreImmutableAndRewardFree(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "ranking-settlement-"+uuid.NewString())
	emptyReport := importPracticeBank(t, pool, "ranking-settlement-empty-"+uuid.NewString())
	handler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	server := httptest.NewServer(handler)
	defer server.Close()
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	sessionID, question := createRankingSession(t, server.URL, auth, report, "settlement-session-key")
	answerStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, sessionID), map[string]string{"Cookie": auth, "Idempotency-Key": "settlement-answer-key"}, map[string]any{"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": 1})
	if answerStatus != http.StatusOK {
		t.Fatalf("settlement source answer = %d", answerStatus)
	}
	// emptyReport receives no answers: with no profile mechanism there is no
	// "opted-out" state, so an unscored bank simply gets no settlement event.
	columns, err := pool.Query(context.Background(), `SELECT column_name FROM information_schema.columns WHERE table_name='quizcraft_ranking_settlement_events' ORDER BY column_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer columns.Close()
	for columns.Next() {
		var column string
		_ = columns.Scan(&column)
		if strings.Contains(column, "point") || strings.Contains(column, "reward") || strings.Contains(column, "member") {
			t.Fatalf("settlement exposes reward column %s", column)
		}
	}
	service, _ := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	now := time.Now().UTC()
	daysUntilNextMonday := (8 - int(now.Weekday())) % 7
	if daysUntilNextMonday == 0 {
		daysUntilNextMonday = 7
	}
	at := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, daysUntilNextMonday)
	created, err := service.SettlePreviousUTCWeek(context.Background(), at)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.SettlePreviousUTCWeek(context.Background(), at)
	if err != nil || created < 2 || replayed != 0 {
		t.Fatalf("settlement create/replay = %d/%d err=%v", created, replayed, err)
	}
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id FROM quizcraft_ranking_settlement_events WHERE scope='overall' AND period_end=$1`, at).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var standings string
	if err := pool.QueryRow(context.Background(), `SELECT standings::text FROM quizcraft_ranking_settlement_events WHERE id=$1`, id).Scan(&standings); err != nil || !strings.Contains(standings, userID) {
		t.Fatalf("settlement lacks stable actor evidence: %s err=%v", standings, err)
	}
	var bankEvents int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_ranking_settlement_events WHERE scope='bank' AND bank_id=$1 AND period_end=$2`, report.BankID, at).Scan(&bankEvents); err != nil || bankEvents != 1 {
		t.Fatalf("scored bank settlement events = %d err=%v", bankEvents, err)
	}
	var emptyBankEvents int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_ranking_settlement_events WHERE scope='bank' AND bank_id=$1 AND period_end=$2`, emptyReport.BankID, at).Scan(&emptyBankEvents); err != nil || emptyBankEvents != 0 {
		t.Fatalf("unscored bank settlement events = %d err=%v", emptyBankEvents, err)
	}
	for _, mutation := range []string{`UPDATE quizcraft_ranking_settlement_events SET standings='[]' WHERE id=$1`, `DELETE FROM quizcraft_ranking_settlement_events WHERE id=$1`} {
		if _, err := pool.Exec(context.Background(), mutation, id); err == nil {
			t.Fatalf("settlement mutation succeeded: %s", mutation)
		}
	}
}

func createRankingSession(t *testing.T, baseURL, auth string, report quizcraft.ImportReport, key string) (string, practiceQuestionResponse) {
	t.Helper()
	status, body := requestJSON(t, http.MethodPost, baseURL+"/api/v1/practice/sessions", map[string]string{"Cookie": auth, "Idempotency-Key": key}, map[string]any{"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 4})
	if status != http.StatusCreated {
		t.Fatalf("ranking session = %d %s", status, body)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, body, &session)
	for _, question := range session.Data.Questions {
		if question.Type == "single" {
			return session.Data.SessionID, question
		}
	}
	t.Fatal("ranking session contains no single-choice question")
	return "", practiceQuestionResponse{}
}

// waitForAdvisoryWaiters is shared with practice_test.go's serialization test.
func waitForAdvisoryWaiters(t *testing.T, pool *pgxpool.Pool, expected int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var count int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count >= expected {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("advisory waiters = %d, want >= %d", count, expected)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
