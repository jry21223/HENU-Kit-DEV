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
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
)

const (
	portalCommandClientID = "portal-gateway-practice-command"
	portalCommandKeyID    = "portal-practice-command-key-1"
	portalCommandSecret   = "portal-practice-command-secret-at-least-32-bytes"
)

func TestPortalPracticeCommandsAreDarkUntilEnabledAndBindTrustedActors(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "portal-command-"+uuid.NewString())
	payload := mustJSON(map[string]any{
		"bank_id": report.BankID, "bank_version_id": report.BankVersionID, "mode": "random", "question_count": 1,
	})

	darkHandler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:              pool,
		AuthHMACSecret:        []byte(practiceAuthSecret),
		PortalCommandClientID: portalCommandClientID,
		PortalCommandKeys:     map[string]string{portalCommandKeyID: portalCommandSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	darkServer := httptest.NewServer(darkHandler)
	defer darkServer.Close()
	darkRequest := newPortalPracticeCommandRequest(t, http.MethodPost, darkServer.URL, "/api/v1/portal/practice/sessions", payload, "", "portal-command-dark-0001")
	darkStatus, darkBody, _ := sendPortalPracticeCommand(t, darkRequest)
	if darkStatus != http.StatusNotFound || bytes.Contains(darkBody, []byte(`"questions"`)) {
		t.Fatalf("default-dark Portal command = %d %s", darkStatus, darkBody)
	}

	writesDisabledHandler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:              pool,
		AuthHMACSecret:        []byte(practiceAuthSecret),
		PortalCommandsEnabled: true,
		PortalCommandClientID: portalCommandClientID,
		PortalCommandKeys:     map[string]string{portalCommandKeyID: portalCommandSecret},
		WritesDisabled:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	writesDisabledServer := httptest.NewServer(writesDisabledHandler)
	defer writesDisabledServer.Close()
	writesDisabledRequest := newPortalPracticeCommandRequest(t, http.MethodPost, writesDisabledServer.URL, "/api/v1/portal/practice/sessions", payload, "", "portal-command-writes-disabled-0001")
	writesDisabledStatus, writesDisabledBody, _ := sendPortalPracticeCommand(t, writesDisabledRequest)
	if writesDisabledStatus != http.StatusServiceUnavailable || !bytes.Contains(writesDisabledBody, []byte(`"code":"writes_disabled"`)) {
		t.Fatalf("writes-disabled Portal command = %d %s", writesDisabledStatus, writesDisabledBody)
	}

	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:              pool,
		AuthHMACSecret:        []byte(practiceAuthSecret),
		PortalCommandsEnabled: true,
		PortalCommandClientID: portalCommandClientID,
		PortalCommandKeys:     map[string]string{portalCommandKeyID: portalCommandSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	guestRequest := newPortalPracticeCommandRequest(t, http.MethodPost, server.URL, "/api/v1/portal/practice/sessions", payload, "", "portal-command-guest-0001")
	guestStatus, guestBody, guestCookies := sendPortalPracticeCommand(t, guestRequest)
	if guestStatus != http.StatusCreated {
		t.Fatalf("guest Portal command = %d %s", guestStatus, guestBody)
	}
	guestCookie := findCookie(t, guestCookies, "quizcraft_anonymous")
	if !guestCookie.HttpOnly || !guestCookie.Secure || guestCookie.SameSite != http.SameSiteLaxMode || guestCookie.Path != "/" || guestCookie.Domain != "" {
		t.Fatalf("guest Portal command cookie = %#v", guestCookie)
	}
	var guestSession apiEnvelope[practiceSessionResponse]
	decodeJSON(t, guestBody, &guestSession)
	if guestSession.Data.SessionID == "" || len(guestSession.Data.Questions) != 1 {
		t.Fatalf("guest Portal command session = %+v", guestSession)
	}
	var guestRaw map[string]any
	decodeJSON(t, guestBody, &guestRaw)
	if _, exposed := guestRaw["data"].(map[string]any)["questions"].([]any)[0].(map[string]any)["answer"]; exposed {
		t.Fatal("Portal command session exposed a correct answer")
	}
	var guestActorKey string
	if err := pool.QueryRow(context.Background(), `SELECT actor_key FROM quizcraft_practice_sessions WHERE id=$1`, guestSession.Data.SessionID).Scan(&guestActorKey); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(guestActorKey, "guest:") {
		t.Fatalf("guest Portal command actor = %q", guestActorKey)
	}

	// The middleware must consume Basic service credentials without allowing
	// actor() to treat them as a browser Bearer credential.
	actorID := uuid.NewString()
	userRequest := newPortalPracticeCommandRequest(t, http.MethodPost, server.URL, "/api/v1/portal/practice/sessions", payload, actorID, "portal-command-user-0001")
	userRequest.AddCookie(guestCookie)
	userStatus, userBody, _ := sendPortalPracticeCommand(t, userRequest)
	if userStatus != http.StatusCreated {
		t.Fatalf("signed Portal actor command = %d %s", userStatus, userBody)
	}
	var userSession apiEnvelope[practiceSessionResponse]
	decodeJSON(t, userBody, &userSession)
	var gotUserID uuid.UUID
	var userActorKey string
	if err := pool.QueryRow(context.Background(), `SELECT user_id,actor_key FROM quizcraft_practice_sessions WHERE id=$1`, userSession.Data.SessionID).Scan(&gotUserID, &userActorKey); err != nil {
		t.Fatal(err)
	}
	if gotUserID.String() != actorID || userActorKey != "user:"+actorID {
		t.Fatalf("signed Portal actor persisted %s / %q", gotUserID, userActorKey)
	}

	// A logged-in command keeps the Core-issued guest cookie on the request so
	// the existing one-way guest-to-user claim remains available. The answer
	// body also proves the middleware restored its exact signed bytes before
	// the existing command handler decoded them.
	guestQuestion := guestSession.Data.Questions[0]
	answerPayload := mustJSON(map[string]any{
		"question_id": guestQuestion.QuestionID, "question_version_id": guestQuestion.QuestionVersionID, "answer": 0,
	})
	claimRequest := newPortalPracticeCommandRequest(t, http.MethodPost, server.URL, "/api/v1/portal/practice/sessions/"+guestSession.Data.SessionID+"/answers", answerPayload, actorID, "portal-command-claim-answer-0001")
	claimRequest.AddCookie(guestCookie)
	claimStatus, claimBody, _ := sendPortalPracticeCommand(t, claimRequest)
	if claimStatus != http.StatusOK || !bytes.Contains(claimBody, []byte(`"expected_answer"`)) {
		t.Fatalf("signed Portal actor claimed guest session = %d %s", claimStatus, claimBody)
	}
	var claimedUserID uuid.UUID
	var claimedActorKey string
	if err := pool.QueryRow(context.Background(), `SELECT user_id,user_actor_key FROM quizcraft_practice_session_claims WHERE session_id=$1`, guestSession.Data.SessionID).Scan(&claimedUserID, &claimedActorKey); err != nil {
		t.Fatal(err)
	}
	if claimedUserID.String() != actorID || claimedActorKey != "user:"+actorID {
		t.Fatalf("Portal command claim = %s / %q", claimedUserID, claimedActorKey)
	}

	// A service client cannot spoof a user by changing the header after the
	// six-part signature has been calculated.
	spoofed := newPortalPracticeCommandRequest(t, http.MethodPost, server.URL, "/api/v1/portal/practice/sessions", payload, actorID, "portal-command-spoof-0001")
	spoofed.Header.Set("X-Actor-User-Id", uuid.NewString())
	spoofedStatus, spoofedBody, _ := sendPortalPracticeCommand(t, spoofed)
	if spoofedStatus != http.StatusUnauthorized || bytes.Contains(spoofedBody, []byte(`"questions"`)) {
		t.Fatalf("tampered Portal actor = %d %s", spoofedStatus, spoofedBody)
	}
	bodyTampered := newPortalPracticeCommandRequest(t, http.MethodPost, server.URL, "/api/v1/portal/practice/sessions", payload, "", "portal-command-body-tamper-0001")
	changedPayload := append([]byte(nil), payload...)
	changedPayload[len(changedPayload)-2] = '2'
	bodyTampered.Body = io.NopCloser(bytes.NewReader(changedPayload))
	bodyTampered.ContentLength = int64(len(changedPayload))
	bodyTamperedStatus, bodyTamperedResponse, _ := sendPortalPracticeCommand(t, bodyTampered)
	if bodyTamperedStatus != http.StatusUnauthorized || bytes.Contains(bodyTamperedResponse, []byte(`"questions"`)) {
		t.Fatalf("tampered Portal command body = %d %s", bodyTamperedStatus, bodyTamperedResponse)
	}

	// Direct routes retain their browser-session boundary: a Basic service
	// credential is not a substitute for a user or anonymous Core session.
	direct, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/practice/sessions", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	direct.SetBasicAuth(portalCommandClientID, portalCommandSecret)
	direct.Header.Set("Content-Type", "application/json")
	direct.Header.Set("Idempotency-Key", "portal-command-direct-0001")
	directStatus, directBody, _ := sendPortalPracticeCommand(t, direct)
	if directStatus != http.StatusUnauthorized || bytes.Contains(directBody, []byte(`"questions"`)) {
		t.Fatalf("direct Basic route = %d %s", directStatus, directBody)
	}

	replayStatus, replayBody, _ := sendPortalPracticeCommand(t, guestRequest)
	if replayStatus != http.StatusConflict || !bytes.Contains(replayBody, []byte(`"code":"service_replay"`)) {
		t.Fatalf("Portal command nonce replay = %d %s", replayStatus, replayBody)
	}
}

func TestPortalPracticeCommandsRequireCompleteDedicatedConfiguration(t *testing.T) {
	pool := practicePool(t)
	_, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:              pool,
		AuthHMACSecret:        []byte(practiceAuthSecret),
		PortalCommandsEnabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "Portal command") {
		t.Fatalf("incomplete enabled Portal command config error = %v", err)
	}
}

func TestPortalFavoriteCommandsSignPutDeleteAndStayBoundToTheSignedActor(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "portal-favorites-put-delete-"+uuid.NewString())
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:              pool,
		AuthHMACSecret:        []byte(practiceAuthSecret),
		PortalCommandsEnabled: true,
		PortalCommandClientID: portalCommandClientID,
		PortalCommandKeys:     map[string]string{portalCommandKeyID: portalCommandSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	actorID := uuid.NewString()
	question := report.Questions[0]
	favoritePath := fmt.Sprintf("/api/v1/portal/practice/banks/%s/favorites/%s", report.BankID, question.QuestionID)

	putRequest := newPortalPracticeCommandRequest(t, http.MethodPut, server.URL, favoritePath, []byte(`{}`), actorID, "portal-favorite-put-0001")
	putStatus, putBody, _ := sendPortalPracticeCommand(t, putRequest)
	if putStatus != http.StatusOK || !bytes.Contains(putBody, []byte(`"state":"succeeded"`)) {
		t.Fatalf("signed Portal PUT favorite = %d %s", putStatus, putBody)
	}
	var relationCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_favorites WHERE bank_id=$1 AND question_id=$2`, report.BankID, question.QuestionID).Scan(&relationCount); err != nil || relationCount != 1 {
		t.Fatalf("favorite relation after PUT = %d, err=%v", relationCount, err)
	}

	// The PUT command shares the middleware's nonce replay protection with the
	// POST commands: resending the identical signed request must conflict.
	replayStatus, replayBody, _ := sendPortalPracticeCommand(t, putRequest)
	if replayStatus != http.StatusConflict || !bytes.Contains(replayBody, []byte(`"code":"service_replay"`)) {
		t.Fatalf("Portal PUT favorite nonce replay = %d %s", replayStatus, replayBody)
	}

	// The method is one of the signed canonical lines: a signature computed for
	// PUT must be rejected when the request is sent as POST.
	methodTampered := newPortalPracticeCommandRequest(t, http.MethodPut, server.URL, favoritePath, []byte(`{}`), actorID, "portal-favorite-method-tamper-0001")
	methodTampered.Method = http.MethodPost
	tamperStatus, tamperBody, _ := sendPortalPracticeCommand(t, methodTampered)
	if tamperStatus != http.StatusUnauthorized || !bytes.Contains(tamperBody, []byte(`"code":"invalid_service_auth"`)) {
		t.Fatalf("method-tampered Portal favorite = %d %s", tamperStatus, tamperBody)
	}

	deleteStatus, deleteBody, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, http.MethodDelete, server.URL, favoritePath, []byte(`{}`), actorID, "portal-favorite-delete-0001"))
	if deleteStatus != http.StatusOK || !bytes.Contains(deleteBody, []byte(`"state":"succeeded"`)) {
		t.Fatalf("signed Portal DELETE favorite = %d %s", deleteStatus, deleteBody)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_favorites WHERE bank_id=$1 AND question_id=$2`, report.BankID, question.QuestionID).Scan(&relationCount); err != nil || relationCount != 0 {
		t.Fatalf("favorite relation after DELETE = %d, err=%v", relationCount, err)
	}
}

func TestPortalGuestFavoriteCommandsRequireASignedInActor(t *testing.T) {
	pool := practicePool(t)
	report := importPracticeBank(t, pool, "portal-favorites-guest-"+uuid.NewString())
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:              pool,
		AuthHMACSecret:        []byte(practiceAuthSecret),
		PortalCommandsEnabled: true,
		PortalCommandClientID: portalCommandClientID,
		PortalCommandKeys:     map[string]string{portalCommandKeyID: portalCommandSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	question := report.Questions[0]
	favoritePath := fmt.Sprintf("/api/v1/portal/practice/banks/%s/favorites/%s", report.BankID, question.QuestionID)

	// Five-part guest commands carry no actor user ID; the favorites handlers
	// must answer 401 instead of dereferencing a nil userID.
	guestPutStatus, guestPutBody, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, http.MethodPut, server.URL, favoritePath, []byte(`{}`), "", "portal-favorite-guest-0001"))
	if guestPutStatus != http.StatusUnauthorized || !bytes.Contains(guestPutBody, []byte(`"code":"authentication_required"`)) {
		t.Fatalf("guest Portal PUT favorite = %d %s", guestPutStatus, guestPutBody)
	}
	guestDeleteStatus, guestDeleteBody, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, http.MethodDelete, server.URL, favoritePath, []byte(`{}`), "", "portal-favorite-guest-delete-0001"))
	if guestDeleteStatus != http.StatusUnauthorized || !bytes.Contains(guestDeleteBody, []byte(`"code":"authentication_required"`)) {
		t.Fatalf("guest Portal DELETE favorite = %d %s", guestDeleteStatus, guestDeleteBody)
	}
	guestSessionStatus, guestSessionBody, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, http.MethodPost, server.URL, fmt.Sprintf("/api/v1/portal/practice/banks/%s/favorites/practice-sessions", report.BankID), []byte(`{}`), "", "portal-favorites-session-guest-0001"))
	if guestSessionStatus != http.StatusUnauthorized || !bytes.Contains(guestSessionBody, []byte(`"code":"authentication_required"`)) {
		t.Fatalf("guest Portal favorites session = %d %s", guestSessionStatus, guestSessionBody)
	}
}

func newPortalPracticeCommandRequest(t *testing.T, method, baseURL, path string, raw []byte, actor, idempotencyKey string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	nonceText := base64.RawURLEncoding.EncodeToString(nonce)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	digest := sha256.Sum256(raw)
	canonicalParts := []string{method, request.URL.RequestURI(), timestamp, nonceText, hex.EncodeToString(digest[:])}
	if actor != "" {
		canonicalParts = append(canonicalParts, actor)
		request.Header.Set("X-Actor-User-Id", actor)
	}
	mac := hmac.New(sha256.New, []byte(portalCommandSecret))
	_, _ = mac.Write([]byte(strings.Join(canonicalParts, "\n")))
	request.SetBasicAuth(portalCommandClientID, portalCommandSecret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request.Header.Set("X-Service-Id", portalCommandClientID)
	request.Header.Set("X-Key-Id", portalCommandKeyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonceText)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	return request
}

func sendPortalPracticeCommand(t *testing.T, request *http.Request) (int, []byte, []*http.Cookie) {
	t.Helper()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, body, response.Cookies()
}
