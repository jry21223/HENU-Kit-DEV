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

func TestRankingHTTPCountsNewSessionsOnceAndProtectsPublicIdentity(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "ranking-"+uuid.NewString())
	handler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	server := httptest.NewServer(handler)
	defer server.Close()
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	profile := map[string]any{"visible": true, "nickname": "认真刷题", "system_avatar": "scholar-blue"}
	guestStatus, _ := requestJSON(t, http.MethodPatch, server.URL+"/api/v1/ranking-profile", map[string]string{"Idempotency-Key": "guest-ranking-profile"}, profile)
	if guestStatus != http.StatusUnauthorized {
		t.Fatalf("guest profile update = %d", guestStatus)
	}
	status, body := requestJSON(t, http.MethodPatch, server.URL+"/api/v1/ranking-profile", map[string]string{"Cookie": auth, "Idempotency-Key": "ranking-profile-key-001"}, profile)
	if status != http.StatusOK {
		t.Fatalf("profile = %d %s", status, body)
	}
	replayStatus, replayBody := requestJSON(t, http.MethodPatch, server.URL+"/api/v1/ranking-profile", map[string]string{"Cookie": auth, "Idempotency-Key": "ranking-profile-key-001"}, profile)
	if replayStatus != status || !bytes.Equal(replayBody, body) {
		t.Fatalf("profile replay changed = %d %s", replayStatus, replayBody)
	}
	for _, invalid := range []map[string]any{{"visible": true, "nickname": "HENUKit官方", "system_avatar": "scholar-blue"}, {"visible": true, "nickname": "HENU\u200BKit", "system_avatar": "scholar-blue"}, {"visible": true, "nickname": "ＱｕｉｚＣｒａｆｔ", "system_avatar": "scholar-blue"}, {"visible": true, "nickname": "аdmin", "system_avatar": "scholar-blue"}, {"visible": true, "nickname": "admın", "system_avatar": "scholar-blue"}, {"visible": true, "nickname": "正常昵称", "system_avatar": "uploaded-url"}} {
		invalidStatus, _ := requestJSON(t, http.MethodPatch, server.URL+"/api/v1/ranking-profile", map[string]string{"Cookie": auth, "Idempotency-Key": "invalid-ranking-" + uuid.NewString()}, invalid)
		if invalidStatus != http.StatusBadRequest {
			t.Fatalf("invalid profile accepted = %d", invalidStatus)
		}
	}

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
	_, _ = requestJSON(t, http.MethodPatch, server.URL+"/api/v1/ranking-profile", map[string]string{"Cookie": secondAuth, "Idempotency-Key": "ranking-second-profile"}, map[string]any{"visible": true, "nickname": "并列用户", "system_avatar": "coder-green"})
	for index := 0; index < 2; index++ {
		sessionID, question := createRankingSession(t, server.URL, secondAuth, report, fmt.Sprintf("ranking-tie-session-%02d", index))
		status, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, sessionID), map[string]string{"Cookie": secondAuth, "Idempotency-Key": fmt.Sprintf("ranking-tie-answer-%03d", index)}, map[string]any{"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": 1})
		if status != http.StatusOK {
			t.Fatalf("tie answer %d = %d", index, status)
		}
	}

	for _, path := range []string{"/api/v1/rankings/overall", fmt.Sprintf("/api/v1/banks/%s/rankings?period=lifetime", report.BankID)} {
		rankStatus, rankBody := requestJSON(t, http.MethodGet, server.URL+path, nil, nil)
		if rankStatus != http.StatusOK || !bytes.Contains(rankBody, []byte(`"nickname":"认真刷题"`)) || !bytes.Contains(rankBody, []byte(`"system_avatar":"scholar-blue"`)) || !bytes.Contains(rankBody, []byte(`"correct_answer_count":2`)) || bytes.Contains(rankBody, []byte(userID)) || bytes.Contains(rankBody, []byte(`"user_id"`)) {
			t.Fatalf("public ranking = %d %s", rankStatus, rankBody)
		}
		if bytes.Count(rankBody, []byte(`"rank":1`)) != 2 {
			t.Fatalf("equal scores are not tied: %s", rankBody)
		}
	}
	invalidPeriod, _ := requestJSON(t, http.MethodGet, server.URL+"/api/v1/rankings/overall?period=monthly", nil, nil)
	if invalidPeriod != http.StatusBadRequest {
		t.Fatalf("invalid period = %d", invalidPeriod)
	}

	futureHandler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), Now: func() time.Time { return time.Now().UTC().AddDate(0, 0, 8) }})
	future := httptest.NewServer(futureHandler)
	defer future.Close()
	_, weeklyBody := requestJSON(t, http.MethodGet, future.URL+"/api/v1/rankings/overall", nil, nil)
	_, lifetimeBody := requestJSON(t, http.MethodGet, future.URL+"/api/v1/rankings/overall?period=lifetime", nil, nil)
	if bytes.Contains(weeklyBody, []byte("认真刷题")) || !bytes.Contains(lifetimeBody, []byte("认真刷题")) {
		t.Fatalf("weekly/lifetime boundary = %s / %s", weeklyBody, lifetimeBody)
	}

	optOutStatus, _ := requestJSON(t, http.MethodPatch, server.URL+"/api/v1/ranking-profile", map[string]string{"Cookie": auth, "Idempotency-Key": "ranking-profile-optout"}, map[string]any{"visible": false, "nickname": "认真刷题", "system_avatar": "scholar-blue"})
	_, hiddenBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/rankings/overall?period=lifetime", nil, nil)
	if optOutStatus != http.StatusOK || bytes.Contains(hiddenBody, []byte("认真刷题")) {
		t.Fatalf("ranking opt out = %d %s", optOutStatus, hiddenBody)
	}
}

func TestRankingProfileMutationsAreSerializedPerUser(t *testing.T) {
	pool := practicePool(t)
	handler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	server := httptest.NewServer(handler)
	defer server.Close()
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	blocker, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback(context.Background()) }()
	if _, err := blocker.Exec(context.Background(), `SELECT pg_advisory_xact_lock(hashtextextended('ranking-profile-user:' || $1::text,0))`, userID); err != nil {
		t.Fatal(err)
	}
	type result struct {
		status int
		err    error
	}
	send := func(key, nickname string, visible bool, results chan<- result) {
		request, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/ranking-profile", bytes.NewReader(mustJSON(map[string]any{"visible": visible, "nickname": nickname, "system_avatar": "scholar-blue"})))
		if err != nil {
			results <- result{err: err}
			return
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Cookie", auth)
		request.Header.Set("Idempotency-Key", key)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			results <- result{err: err}
			return
		}
		_ = response.Body.Close()
		results <- result{status: response.StatusCode}
	}
	results := make(chan result, 2)
	go send("profile-older-visible", "较早公开", true, results)
	waitForAdvisoryWaiters(t, pool, 1)
	go send("profile-newer-hidden", "最新退出", false, results)
	waitForAdvisoryWaiters(t, pool, 2)
	if err := blocker.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		outcome := <-results
		if outcome.err != nil || outcome.status != http.StatusOK {
			t.Fatalf("serialized profile mutation = %d err=%v", outcome.status, outcome.err)
		}
	}
	var nickname string
	var visible bool
	if err := pool.QueryRow(context.Background(), `SELECT nickname,visible FROM quizcraft_ranking_profiles WHERE user_id=$1`, userID).Scan(&nickname, &visible); err != nil || nickname != "最新退出" || visible {
		t.Fatalf("latest profile mutation did not win: %q visible=%v err=%v", nickname, visible, err)
	}
}

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

func TestRankingSettlementFactsAreImmutableAndRewardFree(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "ranking-settlement-"+uuid.NewString())
	hiddenReport := importPracticeBank(t, pool, "ranking-settlement-hidden-"+uuid.NewString())
	handler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	server := httptest.NewServer(handler)
	defer server.Close()
	userID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, userID)
	_, _ = requestJSON(t, http.MethodPatch, server.URL+"/api/v1/ranking-profile", map[string]string{"Cookie": auth, "Idempotency-Key": "settlement-profile-key"}, map[string]any{"visible": true, "nickname": "结算用户", "system_avatar": "owl-purple"})
	sessionID, question := createRankingSession(t, server.URL, auth, report, "settlement-session-key")
	answerStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, sessionID), map[string]string{"Cookie": auth, "Idempotency-Key": "settlement-answer-key"}, map[string]any{"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": 1})
	if answerStatus != http.StatusOK {
		t.Fatalf("settlement source answer = %d", answerStatus)
	}
	hiddenUserID := uuid.NewString()
	hiddenAuth := "quizcraft_session=" + practiceToken(t, hiddenUserID)
	_, _ = requestJSON(t, http.MethodPatch, server.URL+"/api/v1/ranking-profile", map[string]string{"Cookie": hiddenAuth, "Idempotency-Key": "settlement-hidden-profile"}, map[string]any{"visible": true, "nickname": "隐身用户", "system_avatar": "reader-amber"})
	hiddenSessionID, hiddenQuestion := createRankingSession(t, server.URL, hiddenAuth, hiddenReport, "settlement-hidden-session")
	hiddenAnswerStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, hiddenSessionID), map[string]string{"Cookie": hiddenAuth, "Idempotency-Key": "settlement-hidden-answer"}, map[string]any{"question_id": hiddenQuestion.QuestionID, "question_version_id": hiddenQuestion.QuestionVersionID, "answer": 1})
	if hiddenAnswerStatus != http.StatusOK {
		t.Fatalf("hidden settlement source answer = %d", hiddenAnswerStatus)
	}
	_, _ = requestJSON(t, http.MethodPatch, server.URL+"/api/v1/ranking-profile", map[string]string{"Cookie": hiddenAuth, "Idempotency-Key": "settlement-hidden-optout"}, map[string]any{"visible": false, "nickname": "隐身用户", "system_avatar": "reader-amber"})
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_banks SET active_version_id=NULL WHERE id=$1`, report.BankID); err != nil {
		t.Fatal(err)
	}
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
	service, _ := quizcraft.New(quizcraft.Config{Database: pool})
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
		t.Fatalf("unpublished bank settlement events = %d err=%v", bankEvents, err)
	}
	var hiddenStandings string
	if err := pool.QueryRow(context.Background(), `SELECT standings::text FROM quizcraft_ranking_settlement_events WHERE scope='bank' AND bank_id=$1 AND period_end=$2`, hiddenReport.BankID, at).Scan(&hiddenStandings); err != nil || hiddenStandings != "[]" {
		t.Fatalf("fully opted-out bank settlement = %s err=%v", hiddenStandings, err)
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
