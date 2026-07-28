package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
)

func TestFavoritesHTTPAuthenticationIdempotencyConcurrencyAndUnavailablePrivacy(t *testing.T) {
	pool := practicePool(t)
	bankKey := "favorites-" + uuid.NewString()
	report := importPracticeBank(t, pool, bankKey)
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	question := report.Questions[0]
	availableQuestion := report.Questions[1]
	path := fmt.Sprintf("%s/api/v1/banks/%s/favorites/%s", server.URL, report.BankID, question.QuestionID)

	guestStatus, _ := requestJSON(t, http.MethodPut, path, map[string]string{"Idempotency-Key": "guest-favorite-key-0001"}, nil)
	if guestStatus != http.StatusUnauthorized {
		t.Fatalf("guest favorite = %d", guestStatus)
	}
	auth := "quizcraft_session=" + practiceToken(t, uuid.NewString())
	status, body := requestJSON(t, http.MethodPut, path, map[string]string{"Cookie": auth, "Idempotency-Key": "favorite-key-00000001"}, nil)
	if status != http.StatusOK {
		t.Fatalf("favorite = %d %s", status, body)
	}
	repeatStatus, repeatBody := requestJSON(t, http.MethodPut, path, map[string]string{"Cookie": auth, "Idempotency-Key": "favorite-key-00000001"}, nil)
	if repeatStatus != status || !bytes.Equal(repeatBody, body) {
		t.Fatalf("favorite replay changed = %d %s / %s", repeatStatus, repeatBody, body)
	}

	const workers = 10
	statuses := make(chan int, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			workerStatus, _ := requestJSON(t, http.MethodPut, path, map[string]string{"Cookie": auth, "Idempotency-Key": fmt.Sprintf("favorite-concurrent-%04d", index)}, nil)
			statuses <- workerStatus
		}(index)
	}
	wait.Wait()
	close(statuses)
	for workerStatus := range statuses {
		if workerStatus != http.StatusOK {
			t.Fatalf("concurrent favorite = %d", workerStatus)
		}
	}
	secondPath := fmt.Sprintf("%s/api/v1/banks/%s/favorites/%s", server.URL, report.BankID, availableQuestion.QuestionID)
	if secondStatus, secondBody := requestJSON(t, http.MethodPut, secondPath, map[string]string{"Cookie": auth, "Idempotency-Key": "favorite-second-key-001"}, nil); secondStatus != http.StatusOK {
		t.Fatalf("favorite second = %d %s", secondStatus, secondBody)
	}
	var relationCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_favorites WHERE bank_id=$1 AND question_id=$2`, report.BankID, question.QuestionID).Scan(&relationCount); err != nil || relationCount != 1 {
		t.Fatalf("favorite relation count = %d, err=%v", relationCount, err)
	}

	overviewStatus, overviewBody := requestJSON(t, http.MethodGet, server.URL+"/api/v1/favorites", map[string]string{"Cookie": auth}, nil)
	if overviewStatus != http.StatusOK || !bytes.Contains(overviewBody, []byte(`"available_count":2`)) || bytes.Contains(overviewBody, []byte(question.QuestionID)) {
		t.Fatalf("overview = %d %s", overviewStatus, overviewBody)
	}
	listStatus, listBody := requestJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/banks/%s/favorites", server.URL, report.BankID), map[string]string{"Cookie": auth}, nil)
	if listStatus != http.StatusOK || !bytes.Contains(listBody, []byte(`"available":true`)) || !bytes.Contains(listBody, []byte(question.QuestionVersionID)) {
		t.Fatalf("favorite list = %d %s", listStatus, listBody)
	}

	var updated map[string]any
	decodeJSON(t, []byte(validBank), &updated)
	updated["meta"].(map[string]any)["total"] = float64(3)
	updated["questions"] = updated["questions"].([]any)[1:]
	service, _ := quizcraft.New(quizcraft.Config{Database: pool, AllowTestBootstrapActivation: true})
	if _, err := service.ImportJSON(context.Background(), bankKey, mustJSON(updated)); err != nil {
		t.Fatal(err)
	}
	_, unavailableBody := requestJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/banks/%s/favorites", server.URL, report.BankID), map[string]string{"Cookie": auth}, nil)
	var favorites apiEnvelope[[]map[string]any]
	decodeJSON(t, unavailableBody, &favorites)
	var unavailable map[string]any
	for _, item := range favorites.Data {
		if item["question_id"] == question.QuestionID {
			unavailable = item
		}
	}
	if len(favorites.Data) != 2 || unavailable == nil || unavailable["available"] != false {
		t.Fatalf("unavailable favorite = %#v", favorites.Data)
	}
	for _, forbidden := range []string{"question_version_id", "content", "options", "answer", "analysis"} {
		if _, exposed := unavailable[forbidden]; exposed {
			t.Fatalf("unavailable favorite exposed %s: %#v", forbidden, unavailable)
		}
	}
	practiceStatus, practiceBody := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/banks/%s/favorites/practice-sessions", server.URL, report.BankID), map[string]string{"Cookie": auth, "Idempotency-Key": "favorites-session-filter-01"}, nil)
	var filteredSession apiEnvelope[practiceSessionResponse]
	decodeJSON(t, practiceBody, &filteredSession)
	if practiceStatus != http.StatusCreated || filteredSession.Data.ExcludedUnavailableCount != 1 || len(filteredSession.Data.Questions) != 1 || filteredSession.Data.Questions[0].QuestionID != availableQuestion.QuestionID {
		t.Fatalf("filtered favorites session = %d %+v", practiceStatus, filteredSession.Data)
	}
}

func TestFavoritesPracticeStaysInsideBankAndUnfavoriteIsIdempotent(t *testing.T) {
	pool := practicePool(t)
	first := importPracticeBank(t, pool, "favorites-first-"+uuid.NewString())
	second := importPracticeBank(t, pool, "favorites-second-"+uuid.NewString())
	handler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	server := httptest.NewServer(handler)
	defer server.Close()
	auth := "quizcraft_session=" + practiceToken(t, uuid.NewString())
	for index, report := range []quizcraft.ImportReport{first, second} {
		path := fmt.Sprintf("%s/api/v1/banks/%s/favorites/%s", server.URL, report.BankID, report.Questions[0].QuestionID)
		status, _ := requestJSON(t, http.MethodPut, path, map[string]string{"Cookie": auth, "Idempotency-Key": fmt.Sprintf("favorite-bank-key-%04d", index)}, nil)
		if status != http.StatusOK {
			t.Fatalf("favorite bank %d = %d", index, status)
		}
	}
	status, body := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/banks/%s/favorites/practice-sessions", server.URL, first.BankID), map[string]string{"Cookie": auth, "Idempotency-Key": "favorites-session-key-001"}, nil)
	if status != http.StatusCreated {
		t.Fatalf("favorites session = %d %s", status, body)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, body, &session)
	if session.Data.BankID != first.BankID || session.Data.Mode != "favorites" || len(session.Data.Questions) != 1 || session.Data.Questions[0].QuestionID != first.Questions[0].QuestionID {
		t.Fatalf("cross-bank favorites session = %+v", session.Data)
	}

	deletePath := fmt.Sprintf("%s/api/v1/banks/%s/favorites/%s", server.URL, first.BankID, first.Questions[0].QuestionID)
	for _, key := range []string{"unfavorite-key-000001", "unfavorite-key-000002"} {
		deleteStatus, _ := requestJSON(t, http.MethodDelete, deletePath, map[string]string{"Cookie": auth, "Idempotency-Key": key}, nil)
		if deleteStatus != http.StatusOK {
			t.Fatalf("unfavorite = %d", deleteStatus)
		}
	}
	listStatus, listBody := requestJSON(t, http.MethodGet, fmt.Sprintf("%s/api/v1/banks/%s/favorites", server.URL, first.BankID), map[string]string{"Cookie": auth}, nil)
	var list apiEnvelope[[]json.RawMessage]
	decodeJSON(t, listBody, &list)
	if listStatus != http.StatusOK || len(list.Data) != 0 {
		t.Fatalf("favorites after delete = %d %s", listStatus, listBody)
	}
}

func TestFavoritesPersistAcrossIndependentSignedInDevices(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "favorites-devices-"+uuid.NewString())
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	userID := uuid.NewString()
	deviceA := "quizcraft_session=" + favoriteDeviceToken(t, userID, "device-a")
	deviceB := "quizcraft_session=" + favoriteDeviceToken(t, userID, "device-b")
	otherUser := "quizcraft_session=" + favoriteDeviceToken(t, uuid.NewString(), "device-c")
	question := report.Questions[0]
	mutationPath := fmt.Sprintf("%s/api/v1/banks/%s/favorites/%s", server.URL, report.BankID, question.QuestionID)
	if status, body := requestJSON(t, http.MethodPut, mutationPath, map[string]string{"Cookie": deviceA, "Idempotency-Key": "favorite-device-a-write-001"}, nil); status != http.StatusOK {
		t.Fatalf("device A favorite = %d %s", status, body)
	}

	listPath := fmt.Sprintf("%s/api/v1/banks/%s/favorites", server.URL, report.BankID)
	if status, body := requestJSON(t, http.MethodGet, listPath, map[string]string{"Cookie": deviceB}, nil); status != http.StatusOK || !bytes.Contains(body, []byte(question.QuestionID)) {
		t.Fatalf("device B did not receive the server favorite = %d %s", status, body)
	}
	if status, body := requestJSON(t, http.MethodGet, listPath, map[string]string{"Cookie": otherUser}, nil); status != http.StatusOK || bytes.Contains(body, []byte(question.QuestionID)) {
		t.Fatalf("other user observed another user's favorite = %d %s", status, body)
	}
	if status, body := requestJSON(t, http.MethodDelete, mutationPath, map[string]string{"Cookie": deviceB, "Idempotency-Key": "favorite-device-b-delete-001"}, nil); status != http.StatusOK {
		t.Fatalf("device B unfavorite = %d %s", status, body)
	}
	if status, body := requestJSON(t, http.MethodGet, listPath, map[string]string{"Cookie": deviceA}, nil); status != http.StatusOK || bytes.Contains(body, []byte(question.QuestionID)) {
		t.Fatalf("device A retained a deleted favorite = %d %s", status, body)
	}
}

func favoriteDeviceToken(t *testing.T, userID, deviceID string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"iss": "quizcraft-session",
		"exp": time.Now().Add(time.Hour).Unix(),
		"aud": "quizcraft",
		"jti": deviceID,
	})
	signed, err := token.SignedString([]byte(practiceAuthSecret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestGuestPracticeSessionCanBeClaimedAfterLoginWithOriginalAnonymousCookie(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "favorite-login-return-"+uuid.NewString())
	handler, _ := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret)})
	server := httptest.NewServer(handler)
	defer server.Close()
	guestSubject := uuid.NewString()
	guestCookie := "quizcraft_anonymous=" + practiceAnonymousTokenForSubject(t, guestSubject)
	status, body := requestJSON(t, http.MethodPost, server.URL+"/api/v1/practice/sessions", map[string]string{"Cookie": guestCookie, "Idempotency-Key": "guest-before-login-session"}, map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 1,
	})
	if status != http.StatusCreated {
		t.Fatalf("guest session = %d %s", status, body)
	}
	var session apiEnvelope[practiceSessionResponse]
	decodeJSON(t, body, &session)
	question := session.Data.Questions[0]
	guestAnswerKey := "guest-before-login-answer"
	guestAnswerStatus, guestAnswerBody := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, session.Data.SessionID), map[string]string{"Cookie": guestCookie, "Idempotency-Key": guestAnswerKey}, map[string]any{
		"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": "before-login",
	})
	if guestAnswerStatus != http.StatusOK {
		t.Fatalf("guest answer before login = %d %s", guestAnswerStatus, guestAnswerBody)
	}
	userID := uuid.NewString()
	loginCookies := guestCookie + "; quizcraft_session=" + practiceToken(t, userID)
	answerStatus, answerBody := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, session.Data.SessionID), map[string]string{"Cookie": loginCookies, "Idempotency-Key": "answer-after-login-key"}, map[string]any{
		"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": "after-login",
	})
	if answerStatus != http.StatusOK {
		t.Fatalf("answer after login = %d %s", answerStatus, answerBody)
	}
	var storedUserID uuid.UUID
	var actorKey string
	if err := pool.QueryRow(context.Background(), `SELECT user_id,user_actor_key FROM quizcraft_practice_session_claims WHERE session_id=$1`, session.Data.SessionID).Scan(&storedUserID, &actorKey); err != nil {
		t.Fatal(err)
	}
	if storedUserID.String() != userID || actorKey != "user:"+userID {
		t.Fatalf("claimed session owner = %s / %s", storedUserID, actorKey)
	}
	for _, mutation := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE quizcraft_practice_session_claims SET user_actor_key=user_actor_key WHERE session_id=$1`, []any{session.Data.SessionID}},
		{`DELETE FROM quizcraft_practice_session_claims WHERE session_id=$1`, []any{session.Data.SessionID}},
		{`TRUNCATE quizcraft_practice_session_claims`, nil},
	} {
		if _, err := pool.Exec(context.Background(), mutation.sql, mutation.args...); err == nil {
			t.Fatalf("immutable session claim mutation succeeded: %s", mutation.sql)
		}
	}

	otherStatus, _ := requestJSON(t, http.MethodPost, fmt.Sprintf("%s/api/v1/practice/sessions/%s/answers", server.URL, session.Data.SessionID), map[string]string{"Cookie": guestCookie, "Idempotency-Key": guestAnswerKey}, map[string]any{
		"question_id": question.QuestionID, "question_version_id": question.QuestionVersionID, "answer": "before-login",
	})
	if otherStatus != http.StatusForbidden {
		t.Fatalf("guest retained claimed session access = %d", otherStatus)
	}
}
