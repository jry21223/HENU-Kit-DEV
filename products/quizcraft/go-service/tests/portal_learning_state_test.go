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

const portalLearningStatePath = "/api/v1/portal/practice/learning-state"

type portalLearningStateItem struct {
	BankID            string `json:"bank_id"`
	QuestionID        string `json:"question_id"`
	QuestionVersionID string `json:"question_version_id"`
	Wrong             bool   `json:"wrong"`
	AttemptCount      int64  `json:"attempt_count"`
	CorrectCount      int64  `json:"correct_count"`
	UpdatedAt         string `json:"updated_at"`
}

type portalLearningStatePage struct {
	Items      []portalLearningStateItem `json:"items"`
	Pagination struct {
		Page       int64 `json:"page"`
		PageSize   int64 `json:"page_size"`
		Total      int64 `json:"total"`
		TotalPages int64 `json:"total_pages"`
	} `json:"pagination"`
}

func TestPortalLearningStateUsesRealServiceAuthenticationAndSignedActor(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "portal-learning-state-"+uuid.NewString())
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

	ownerID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, ownerID)
	sessionStatus, sessionBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{
		"Cookie": auth, "Idempotency-Key": "portal-learning-state-session-0001",
	}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 2,
	})
	if sessionStatus != http.StatusCreated {
		t.Fatalf("create practice session = %d %s", sessionStatus, sessionBody)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, sessionBody, &session)
	questions := session.Data.Questions
	if len(questions) != 2 {
		t.Fatalf("practice session questions = %d, want 2", len(questions))
	}
	for index, question := range questions {
		answerStatus, answerBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{
			"Cookie": auth, "Idempotency-Key": "portal-learning-state-answer-000" + strconv.Itoa(index+1),
		}, map[string]any{
			"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": "definitely-wrong",
		})
		if answerStatus != http.StatusOK || !bytes.Contains(answerBody, []byte(`"correct":false`)) {
			t.Fatalf("submit wrong answer %d = %d %s", index+1, answerStatus, answerBody)
		}
	}

	request := newPortalLearningStateRequest(t, server.URL, ownerID, "portal.practice.read", "page=1&page_size=1")
	request.Header.Set("X-Request-Id", "req_portal_learning_state_owner")
	status, body, headers := sendCatalogRequestWithHeaders(t, request)
	if status != http.StatusOK {
		t.Fatalf("owner learning state = %d %s", status, body)
	}
	var state apiEnvelope[portalLearningStatePage]
	decodeJSON(t, body, &state)
	if state.RequestID != "req_portal_learning_state_owner" || headers.Get("X-Request-Id") != state.RequestID || len(state.Data.Items) != 1 || state.Data.Pagination.Page != 1 || state.Data.Pagination.PageSize != 1 || state.Data.Pagination.Total != 2 || state.Data.Pagination.TotalPages != 2 {
		t.Fatalf("owner learning state envelope = header:%q body:%+v", headers.Get("X-Request-Id"), state)
	}
	item := state.Data.Items[0]
	if item.BankID != report.BankID || !containsPortalQuestion(questions, item.QuestionID, item.QuestionVersionID) || !item.Wrong || item.AttemptCount != 1 || item.CorrectCount != 0 || strings.TrimSpace(item.UpdatedAt) == "" {
		t.Fatalf("owner learning state item = %+v", item)
	}
	secondRequest := newPortalLearningStateRequest(t, server.URL, ownerID, "portal.practice.read", "page=2&page_size=1")
	secondRequest.Header.Set("X-Request-Id", "req_portal_learning_state_owner_2")
	secondStatus, secondBody, secondHeaders := sendCatalogRequestWithHeaders(t, secondRequest)
	var second apiEnvelope[portalLearningStatePage]
	decodeJSON(t, secondBody, &second)
	if secondStatus != http.StatusOK || second.RequestID != "req_portal_learning_state_owner_2" || secondHeaders.Get("X-Request-Id") != second.RequestID || len(second.Data.Items) != 1 || second.Data.Items[0].QuestionID == item.QuestionID || second.Data.Pagination.Page != 2 || second.Data.Pagination.Total != 2 {
		t.Fatalf("second owner learning-state page = %d header:%q body:%+v", secondStatus, secondHeaders.Get("X-Request-Id"), second)
	}

	otherStatus, otherBody := sendCatalogRequest(t, newPortalLearningStateRequest(t, server.URL, uuid.NewString(), "portal.practice.read"))
	if otherStatus != http.StatusOK {
		t.Fatalf("other actor learning state = %d %s", otherStatus, otherBody)
	}
	var other apiEnvelope[portalLearningStatePage]
	decodeJSON(t, otherBody, &other)
	if other.Data.Items == nil || len(other.Data.Items) != 0 || other.Data.Pagination.Total != 0 || bytes.Contains(otherBody, []byte(questions[0].QuestionID)) || bytes.Contains(otherBody, []byte(questions[1].QuestionID)) {
		t.Fatalf("other actor saw owner learning state = %+v", other)
	}

	for _, invalidQuery := range []string{"page_size=101", "page=1;page_size=20"} {
		invalidPageRequest := newPortalLearningStateRequest(t, server.URL, ownerID, "portal.practice.read", invalidQuery)
		invalidPageStatus, invalidPageBody, invalidPageHeaders := sendCatalogRequestWithHeaders(t, invalidPageRequest)
		var invalidPageEnvelope struct {
			RequestID string `json:"request_id"`
		}
		decodeJSON(t, invalidPageBody, &invalidPageEnvelope)
		if invalidPageStatus != http.StatusBadRequest || bytes.Contains(invalidPageBody, []byte(`"data"`)) || invalidPageHeaders.Get("X-Request-Id") != "req_portal_learning_state_test" || invalidPageEnvelope.RequestID != "req_portal_learning_state_test" {
			t.Fatalf("invalid owner learning-state pagination %q = %d header:%q body:%s", invalidQuery, invalidPageStatus, invalidPageHeaders.Get("X-Request-Id"), invalidPageBody)
		}
	}

	tampered := newPortalLearningStateRequest(t, server.URL, ownerID, "portal.practice.read")
	tampered.Header.Set("X-Actor-User-Id", uuid.NewString())
	tamperedStatus, tamperedBody, tamperedHeaders := sendCatalogRequestWithHeaders(t, tampered)
	var tamperedEnvelope struct {
		RequestID string `json:"request_id"`
	}
	decodeJSON(t, tamperedBody, &tamperedEnvelope)
	if tamperedStatus != http.StatusUnauthorized || bytes.Contains(tamperedBody, []byte(`"data"`)) || tamperedHeaders.Get("X-Request-Id") != "req_portal_learning_state_test" || tamperedEnvelope.RequestID != "req_portal_learning_state_test" {
		t.Fatalf("tampered actor learning state = %d header:%q body:%s", tamperedStatus, tamperedHeaders.Get("X-Request-Id"), tamperedBody)
	}

	forbiddenStatus, forbiddenBody, forbiddenHeaders := sendCatalogRequestWithHeaders(t, newPortalLearningStateRequest(t, server.URL, ownerID, "portal.library.read"))
	var forbiddenEnvelope struct {
		RequestID string `json:"request_id"`
	}
	decodeJSON(t, forbiddenBody, &forbiddenEnvelope)
	if forbiddenStatus != http.StatusForbidden || bytes.Contains(forbiddenBody, []byte(`"data"`)) || forbiddenHeaders.Get("X-Request-Id") != "req_portal_learning_state_test" || forbiddenEnvelope.RequestID != "req_portal_learning_state_test" {
		t.Fatalf("wrong permission learning state = %d header:%q body:%s", forbiddenStatus, forbiddenHeaders.Get("X-Request-Id"), forbiddenBody)
	}

	unsigned, err := http.NewRequest(http.MethodGet, server.URL+portalLearningStatePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	unsigned.Header.Set("X-Actor-User-Id", ownerID)
	unsignedStatus, unsignedBody := sendCatalogRequest(t, unsigned)
	if unsignedStatus != http.StatusUnauthorized || bytes.Contains(unsignedBody, []byte(`"data"`)) {
		t.Fatalf("unsigned learning state = %d %s", unsignedStatus, unsignedBody)
	}
}

func TestPortalLearningStateFiltersWrongFactsBeforePagination(t *testing.T) {
	pool := practicePool(t)
	questions := make([]map[string]any, 21)
	for index := range questions {
		questions[index] = map[string]any{
			"id":         fmt.Sprintf("q%04d", index+1),
			"type":       "blank",
			"chapter_id": "ch01",
			"chapter":    "基础",
			"content":    fmt.Sprintf("第 %d 题", index+1),
			"answer":     "main",
			"analysis":   "",
		}
	}
	document := map[string]any{
		"meta": map[string]any{
			"name": "过滤后分页", "version": "v1", "color": "#2563eb", "total": 21,
			"source_files": []string{"filter-pagination.md"},
			"chapters":     []map[string]any{{"id": "ch01", "name": "基础"}},
		},
		"questions": questions,
	}
	service, err := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.ImportJSON(context.Background(), "wrong-filter-pagination-"+uuid.NewString(), mustJSON(document))
	if err != nil {
		t.Fatal(err)
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

	ownerID := uuid.NewString()
	auth := "quizcraft_session=" + practiceToken(t, ownerID)
	sessionStatus, sessionBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{
		"Cookie": auth, "Idempotency-Key": "wrong-filter-session-0001",
	}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 21,
	})
	if sessionStatus != http.StatusCreated {
		t.Fatalf("create filter-pagination session = %d %s", sessionStatus, sessionBody)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, sessionBody, &session)
	if len(session.Data.Questions) != 21 {
		t.Fatalf("filter-pagination session questions = %d, want 21", len(session.Data.Questions))
	}
	wrongQuestionID := session.Data.Questions[0].QuestionID
	for index, question := range session.Data.Questions {
		answer := "main"
		wantCorrect := true
		if index == 0 {
			answer = "definitely-wrong"
			wantCorrect = false
		}
		answerStatus, answerBody := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{
			"Cookie": auth, "Idempotency-Key": fmt.Sprintf("wrong-filter-answer-%04d", index+1),
		}, map[string]any{
			"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": answer,
		})
		if answerStatus != http.StatusOK || !bytes.Contains(answerBody, []byte(fmt.Sprintf(`"correct":%t`, wantCorrect))) {
			t.Fatalf("submit filter-pagination answer %d = %d %s", index+1, answerStatus, answerBody)
		}
		if index == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}

	request := newPortalLearningStateRequest(t, server.URL, ownerID, "portal.practice.read", "wrong=true&page=1&page_size=20")
	status, body, _ := sendCatalogRequestWithHeaders(t, request)
	var state apiEnvelope[portalLearningStatePage]
	decodeJSON(t, body, &state)
	if status != http.StatusOK || state.Data.Pagination.Total != 1 || state.Data.Pagination.TotalPages != 1 || len(state.Data.Items) != 1 || state.Data.Items[0].QuestionID != wrongQuestionID || !state.Data.Items[0].Wrong {
		t.Fatalf("filtered owner learning state = %d %+v; want the older wrong fact on page 1", status, state)
	}
}

func newPortalLearningStateRequest(t *testing.T, baseURL, actor, permission string, rawQuery ...string) *http.Request {
	t.Helper()
	target := baseURL + portalLearningStatePath
	if len(rawQuery) == 1 && rawQuery[0] != "" {
		target += "?" + rawQuery[0]
	}
	request, err := http.NewRequest(http.MethodGet, target, nil)
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
	request.Header.Set("X-Permission-Code", permission)
	request.Header.Set("X-Scope-Kind", "product")
	request.Header.Set("X-Product-Code", "quizcraft")
	request.Header.Set("X-Actor-User-Id", actor)
	request.Header.Set("X-Request-Id", "req_portal_learning_state_test")
	return request
}

func containsPortalQuestion(questions []practiceQuestionResponse, questionID, versionID string) bool {
	for _, question := range questions {
		if question.QuestionID == questionID && question.QuestionVersionID == versionID {
			return true
		}
	}
	return false
}
