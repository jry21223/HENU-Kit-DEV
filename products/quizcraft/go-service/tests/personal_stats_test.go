package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
)

type personalStatsResponse struct {
	TotalAnswers   int64 `json:"total_answers"`
	CorrectAnswers int64 `json:"correct_answers"`
	Accuracy       int   `json:"accuracy"`
	StreakDays     int   `json:"streak_days"`
	Mastery        []struct {
		BankID           string `json:"bank_id"`
		Label            string `json:"label"`
		Value            int    `json:"value"`
		TotalQuestions   int64  `json:"total_questions"`
		CorrectQuestions int64  `json:"correct_questions"`
	} `json:"mastery"`
}

func TestPracticeHTTPPersonalStatsAggregatesImmutableAttempts(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "personal-stats-"+uuid.NewString())
	darkHandler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:       pool,
		AuthHMACSecret: []byte(practiceAuthSecret),
	})
	if err != nil {
		t.Fatal(err)
	}
	darkServer := httptest.NewServer(darkHandler)
	defer darkServer.Close()
	if status, _ := requestJSON(t, http.MethodGet, darkServer.URL+"/api/v1/portal/practice/stats", nil, nil); status != http.StatusNotFound {
		t.Fatalf("unconfigured V2 personal-stats route = %d, want 404", status)
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
	sessionStatus, sessionBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{
		"Cookie": auth, "Idempotency-Key": "personal-stats-session-0001",
	}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 4,
	})
	if sessionStatus != http.StatusCreated {
		t.Fatalf("create practice session = %d %s", sessionStatus, sessionBody)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, sessionBody, &session)
	if len(session.Data.Questions) != 4 {
		t.Fatalf("practice session questions = %d, want 4", len(session.Data.Questions))
	}

	for index, question := range session.Data.Questions {
		answer := any("definitely-wrong")
		if index == 0 {
			answer = correctAnswerFor(question)
		}
		status, body := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{
			"Cookie": auth, "Idempotency-Key": fmt.Sprintf("personal-stats-answer-%04d", index),
		}, map[string]any{"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": answer})
		if status != http.StatusOK {
			t.Fatalf("submit answer %d = %d %s", index, status, body)
		}
	}

	first := session.Data.Questions[0]
	replayStatus, replayBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{
		"Cookie": auth, "Idempotency-Key": "personal-stats-answer-replay",
	}, map[string]any{"question_id": first.QuestionID, "question_version_id": first.QuestionVersionID, "answer": correctAnswerFor(first)})
	if replayStatus != http.StatusOK || !bytes.Contains(replayBody, []byte(`"replayed":true`)) {
		t.Fatalf("replayed answer = %d %s", replayStatus, replayBody)
	}

	status, body := requestPersonalStats(t, server.URL, userID)
	if status != http.StatusOK {
		t.Fatalf("personal stats = %d %s", status, body)
	}
	var stats apiEnvelope[personalStatsResponse]
	decodeJSON(t, body, &stats)
	if stats.Data.TotalAnswers != 4 || stats.Data.CorrectAnswers != 1 || stats.Data.Accuracy != 25 || stats.Data.StreakDays != 1 {
		t.Fatalf("personal stats aggregate = %+v", stats.Data)
	}
	if len(stats.Data.Mastery) != 1 {
		t.Fatalf("mastery = %+v, want one bank", stats.Data.Mastery)
	}
	mastery := stats.Data.Mastery[0]
	if mastery.BankID != report.BankID || mastery.Label == "" || mastery.Value != 25 || mastery.TotalQuestions != 4 || mastery.CorrectQuestions != 1 {
		t.Fatalf("mastery row = %+v", mastery)
	}

	newUserStatus, newUserBody := requestPersonalStats(t, server.URL, uuid.NewString())
	if newUserStatus != http.StatusOK {
		t.Fatalf("new user stats = %d %s", newUserStatus, newUserBody)
	}
	var newUser apiEnvelope[personalStatsResponse]
	decodeJSON(t, newUserBody, &newUser)
	if newUser.Data.TotalAnswers != 0 || newUser.Data.CorrectAnswers != 0 || newUser.Data.Accuracy != 0 || newUser.Data.StreakDays != 0 || newUser.Data.Mastery == nil || len(newUser.Data.Mastery) != 0 {
		t.Fatalf("new user received non-zero or non-empty stats: %+v", newUser.Data)
	}

	guestStatus, guestBody := requestPersonalStats(t, server.URL, "anonymous")
	if guestStatus != http.StatusUnauthorized || bytes.Contains(guestBody, []byte(`"data"`)) {
		t.Fatalf("guest personal stats = %d %s", guestStatus, guestBody)
	}

	tamperedActor := newPersonalStatsRequest(t, server.URL, userID)
	tamperedActor.Header.Set("X-Actor-User-Id", uuid.NewString())
	tamperedStatus, tamperedBody := sendCatalogRequest(t, tamperedActor)
	if tamperedStatus != http.StatusUnauthorized || bytes.Contains(tamperedBody, []byte(`"data"`)) {
		t.Fatalf("tampered personal-stats actor = %d %s", tamperedStatus, tamperedBody)
	}

	beforeRepair, err := quizcraft.ReconcileLearningState(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if !beforeRepair.Clean() {
		t.Fatalf("fresh learning state reconciliation = %+v", beforeRepair)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE quizcraft_learning_state SET correct_count=0,wrong=true WHERE user_id=$1 AND question_id=$2`, userID, first.QuestionID); err != nil {
		t.Fatal(err)
	}
	drifted, err := quizcraft.ReconcileLearningState(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if drifted.MismatchedRows != 1 || drifted.Clean() {
		t.Fatalf("drifted learning state reconciliation = %+v", drifted)
	}
	rebuilt, err := quizcraft.RebuildLearningState(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if !rebuilt.Clean() {
		t.Fatalf("rebuilt learning state reconciliation = %+v", rebuilt)
	}
}

func correctAnswerFor(question practiceQuestionResponse) any {
	switch question.Type {
	case "single":
		return 1
	case "multi":
		return []any{2, 1}
	case "judge":
		return true
	case "blank":
		return "main"
	default:
		return nil
	}
}

func requestPersonalStats(t *testing.T, baseURL, actor string) (int, []byte) {
	t.Helper()
	return sendCatalogRequest(t, newPersonalStatsRequest(t, baseURL, actor))
}

func newPersonalStatsRequest(t *testing.T, baseURL, actor string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/portal/practice/stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	nonceText := base64.RawURLEncoding.EncodeToString(nonce)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{http.MethodGet, request.URL.RequestURI(), timestamp, nonceText, hex.EncodeToString(digest[:]), actor}, "\n")
	mac := hmac.New(sha256.New, []byte(portalCatalogSecret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth(portalCatalogClientID, portalCatalogSecret)
	request.Header.Set("X-Service-Id", portalCatalogClientID)
	request.Header.Set("X-Key-Id", portalCatalogKeyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonceText)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-Permission-Code", "portal.practice.read")
	request.Header.Set("X-Scope-Kind", "product")
	request.Header.Set("X-Product-Code", "quizcraft")
	request.Header.Set("X-Actor-User-Id", actor)
	request.Header.Set("X-Request-Id", "req_personal_stats_test")
	return request
}
