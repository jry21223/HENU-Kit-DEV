package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTrainingWorkshopImportsRealListeningAtomically(t *testing.T) {
	pool := isolatedQuizcraftV2Database(t)
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), AllowTestWorkshopClaims: true, CatalogClientID: portalCatalogClientID, CatalogKeys: map[string]string{portalCatalogKeyID: portalCatalogSecret}, PortalCommandsEnabled: true, PortalCommandClientID: portalCommandClientID, PortalCommandKeys: map[string]string{portalCommandKeyID: portalCommandSecret}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	actor := uuid.NewString()
	auth := "quizcraft_session=" + workshopToken(t, actor, []string{"quizcraft.workshop.write", "quizcraft.workshop.read", "quizcraft.workshop.publish"}, []map[string]string{{"kind": "product", "product_code": "quizcraft"}})
	status, body := requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks", map[string]string{"Cookie": auth, "Idempotency-Key": "training-bank-create-" + actor}, map[string]any{"bank_key": "listening-" + actor, "name": "English listening"})
	if status != 201 {
		t.Fatalf("bank %d %s", status, body)
	}
	bank := operationResourceID(t, body)
	raw, err := os.ReadFile("../content/english-training/north-wind.import.json")
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}

	authoredTraining := input["training"]
	input["training"] = nil
	status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": "training-null-source-" + actor}, input)
	if status != 400 {
		t.Fatalf("explicit null training silently imported ordinary content %d %s", status, body)
	}
	input["training"] = authoredTraining
	status, body = requestJSON(t, http.MethodPost, server.URL+"/api/v1/workshop/banks/"+bank+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": "training-unit-import-" + actor}, input)
	if status != 201 {
		t.Fatalf("training import %d %s", status, body)
	}

	acceptedImportBody := append([]byte(nil), body...)
	downBeforeLearners, err := os.ReadFile("../db/migrations/000012_english_training.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(context.Background(), string(downBeforeLearners)); err == nil {
		t.Fatal("down migration erased authored training before any learner session")
	}

	// JSON object key order is not a semantic edit: the same authored source must
	// resolve to the existing immutable bank version, not create another draft.
	authoredMap := input["training"].(map[string]any)
	title := authoredMap["title"]
	delete(authoredMap, "title")
	rest, _ := json.Marshal(authoredMap)
	titleRaw, _ := json.Marshal(title)
	authoredMap["title"] = title
	reordered := append([]byte(`{"title":`), titleRaw...)
	reordered = append(reordered, ',')
	reordered = append(reordered, rest[1:]...)
	input["training"] = json.RawMessage(reordered)
	input["expected_version"] = 2
	status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": "training-order-only-" + actor}, input)
	if status != 409 {
		t.Fatalf("object key order changed immutable identity %d %s", status, body)
	}
	input["training"] = authoredMap
	input["expected_version"] = 1
	var report apiEnvelope[quizcraft.ImportReport]
	decodeJSON(t, acceptedImportBody, &report)
	status, body = requestJSON(t, "GET", server.URL+"/api/v1/workshop/banks/"+bank+"/versions/"+report.Data.BankVersionID, map[string]string{"Cookie": auth}, nil)
	if status != 200 {
		t.Fatalf("detail %d %s", status, body)
	}
	var detail struct {
		Data struct {
			Training struct {
				Unit struct {
					UnitVersionID string `json:"unit_version_id"`
					UnitID        string `json:"unit_id"`
					Questions     []any  `json:"questions"`
				} `json:"unit"`
			} `json:"training"`
		} `json:"data"`
	}
	decodeJSON(t, body, &detail)
	if detail.Data.Training.Unit.UnitVersionID == "" || len(detail.Data.Training.Unit.Questions) != 4 {
		t.Fatalf("resolved training missing %s", body)
	}
	for i, action := range []string{"validate", "publish"} {
		status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/versions/"+report.Data.BankVersionID+"/"+action, map[string]string{"Cookie": auth, "Idempotency-Key": "training-" + action + actor}, map[string]any{"expected_version": 2 + i, "note": "Isolated automated acceptance; human ear review remains pending"})
		if status != 200 {
			t.Fatalf("%s %d %s", action, status, body)
		}
	}
	const prefix = "/api/v1/portal/practice/training"
	status, body = sendCatalogRequest(t, newPortalReadRequest(t, server.URL, prefix+"/catalog", "portal.practice.read", "req_training_catalog"))
	if status != 200 || !bytes.Contains(body, []byte(detail.Data.Training.Unit.UnitVersionID)) {
		t.Fatalf("catalog %d %s", status, body)
	}
	for _, secret := range []string{"expected_answer", "analysis", "segments", "layers", "evidence", "The North Wind and the Sun were disputing"} {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatalf("catalog exposed %s", secret)
		}
	}

	for _, mode := range []string{"random", "chapter", "difficult"} {
		status, body = requestJSON(t, "POST", server.URL+"/api/v1/practice/sessions", map[string]string{"Idempotency-Key": "ordinary-bypass-" + mode + actor}, map[string]any{"bank_id": bank, "bank_version_id": report.Data.BankVersionID, "mode": mode, "chapter_id": "north-wind"})
		if status < 400 {
			t.Fatalf("ordinary %s accepted training: %d %s", mode, status, body)
		}
	}
	create := mustJSON(map[string]any{"unit_id": detail.Data.Training.Unit.UnitID, "unit_version_id": detail.Data.Training.Unit.UnitVersionID})

	nilActor := newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions", create, "00000000-0000-0000-0000-000000000000", "training-nil-actor-"+actor)
	nilStatus, nilBody, _ := sendPortalPracticeCommand(t, nilActor)
	if nilStatus != 401 {
		t.Fatalf("nil signed actor accepted %d %s", nilStatus, nilBody)
	}
	status, body, cookies := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions", create, "", "training-create-session"+actor))
	if status != 201 {
		t.Fatalf("create %d %s", status, body)
	}
	cookie := findCookie(t, cookies, "quizcraft_anonymous")
	var session struct {
		Data struct {
			SessionID  string `json:"session_id"`
			SavedCount int    `json:"saved_count"`
			Phase      string `json:"phase"`
			Unit       struct {
				Questions []struct {
					QuestionID        string `json:"question_id"`
					QuestionVersionID string `json:"question_version_id"`
				} `json:"questions"`
			} `json:"unit"`
		} `json:"data"`
	}
	decodeJSON(t, body, &session)

	// Private GETs never mint guests, and a forged actor cannot acquire identity
	// without the sixth HMAC line. Nonce reuse is rejected independently of keys.
	noOwner := newPortalReadRequest(t, server.URL, prefix+"/sessions/"+session.Data.SessionID, "portal.practice.read", "req_no_owner")
	rs, rb := sendCatalogRequest(t, noOwner)
	if rs != 404 {
		t.Fatalf("no owner read %d %s", rs, rb)
	}
	spoof := newPortalReadRequest(t, server.URL, prefix+"/sessions/"+session.Data.SessionID, "portal.practice.read", "req_spoof")
	spoof.Header.Set("X-Actor-User-Id", actor)
	spoof.AddCookie(cookie)
	rs, rb = sendCatalogRequest(t, spoof)
	if rs != 401 {
		t.Fatalf("unsigned actor %d %s", rs, rb)
	}
	nonce := newPortalReadRequest(t, server.URL, prefix+"/sessions/"+session.Data.SessionID, "portal.practice.read", "req_nonce")
	nonce.AddCookie(cookie)
	rs, rb = sendCatalogRequest(t, nonce)
	if rs != 200 {
		t.Fatalf("initial nonce %d %s", rs, rb)
	}
	rs, rb = sendCatalogRequest(t, nonce)
	if rs != 409 || !bytes.Contains(rb, []byte("SERVICE_REPLAY")) {
		t.Fatalf("nonce replay %d %s", rs, rb)
	}
	for i, q := range session.Data.Unit.Questions {
		answer := mustJSON(map[string]any{"unit_version_id": detail.Data.Training.Unit.UnitVersionID, "question_id": q.QuestionID, "question_version_id": q.QuestionVersionID, "answer": []int{1, 2, 0, 0}[i]})
		req := newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions/"+session.Data.SessionID+"/answers", answer, "", fmt.Sprintf("training-answer-%s-%d", actor, i))
		req.AddCookie(cookie)
		status, body, _ = sendPortalPracticeCommand(t, req)
		if status != 200 {
			t.Fatalf("answer %d %d %s", i, status, body)
		}

		if i == 0 {
			// A new key cannot change a first answer, and ordinary answer URLs cannot
			// reveal private training review before the unit is complete.
			replay := newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions/"+session.Data.SessionID+"/answers", answer, "", "training-new-key-replay-"+actor)
			replay.AddCookie(cookie)
			rs, rb, _ := sendPortalPracticeCommand(t, replay)
			if rs != 200 || !bytes.Contains(rb, []byte(`"replayed":true`)) {
				t.Fatalf("new-key replay %d %s", rs, rb)
			}
			var changed map[string]any
			decodeJSON(t, answer, &changed)
			changed["answer"] = 0
			replay = newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions/"+session.Data.SessionID+"/answers", mustJSON(changed), "", "training-changed-answer-"+actor)
			replay.AddCookie(cookie)
			rs, rb, _ = sendPortalPracticeCommand(t, replay)
			if rs != 409 || !bytes.Contains(rb, []byte("FIRST_ANSWER_IMMUTABLE")) {
				t.Fatalf("changed first %d %s", rs, rb)
			}
			rs, rb = requestJSON(t, "POST", server.URL+"/api/v1/practice/sessions/"+session.Data.SessionID+"/answers", map[string]string{"Cookie": cookie.Name + "=" + cookie.Value, "Idempotency-Key": "ordinary-answer-bypass-" + actor}, map[string]any{"question_id": q.QuestionID, "question_version_id": q.QuestionVersionID, "answer": 1})
			if rs < 400 || bytes.Contains(rb, []byte("expected_answer")) {
				t.Fatalf("ordinary answer bypass %d %s", rs, rb)
			}
		}
		if bytes.Contains(body, []byte("analysis")) || bytes.Contains(body, []byte("expected_answer")) || bytes.Contains(body, []byte("evidence")) {
			t.Fatalf("receipt leaked %s", body)
		}
		req = newPortalReadRequest(t, server.URL, prefix+"/sessions/"+session.Data.SessionID, "portal.practice.read", "req_training_recovery")
		req.AddCookie(cookie)
		status, body = sendCatalogRequest(t, req)
		if status != 200 {
			t.Fatalf("recover %d %s", status, body)
		}
		decodeJSON(t, body, &session)
		if session.Data.SavedCount != i+1 {
			t.Fatalf("saved %s", body)
		}
		req = newPortalReadRequest(t, server.URL, prefix+"/sessions/"+session.Data.SessionID+"/review", "portal.practice.read", "req_training_review")
		req.AddCookie(cookie)
		status, body = sendCatalogRequest(t, req)
		expectedStatus := 409
		if i == 3 {
			expectedStatus = 200
		}
		if status != expectedStatus {
			t.Fatalf("review after %d = %d %s", i+1, status, body)
		}
	}

	// GET by a valid account with the original guest cookie proves access only.
	req := trainingAccountRead(t, server.URL, prefix+"/sessions/"+session.Data.SessionID, actor)
	req.AddCookie(cookie)
	status, body = sendCatalogRequest(t, req)
	if status != 200 {
		t.Fatalf("account guest proof %d %s", status, body)
	}
	var guestOriginal string
	if err := pool.QueryRow(context.Background(), `SELECT actor_key FROM quizcraft_practice_sessions WHERE id=$1`, session.Data.SessionID).Scan(&guestOriginal); err != nil {
		t.Fatal(err)
	}
	var claims int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_practice_session_claims WHERE session_id=$1`, session.Data.SessionID).Scan(&claims); err != nil || claims != 0 {
		t.Fatalf("GET claimed training %d %v", claims, err)
	}
	q := session.Data.Unit.Questions[0]
	claimBody := mustJSON(map[string]any{"unit_version_id": detail.Data.Training.Unit.UnitVersionID, "question_id": q.QuestionID, "question_version_id": q.QuestionVersionID, "answer": 1})
	command := newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions/"+session.Data.SessionID+"/answers", claimBody, actor, "training-claim-replay-"+actor)
	command.AddCookie(cookie)
	status, body, _ = sendPortalPracticeCommand(t, command)
	if status != 200 {
		t.Fatalf("claim replay %d %s", status, body)
	}

	staleCreate := newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions", create, "", "training-create-session"+actor)
	staleCreate.AddCookie(cookie)
	rs, rb, _ = sendPortalPracticeCommand(t, staleCreate)
	if rs != 404 {
		t.Fatalf("old guest create replay accessed claimed account session %d %s", rs, rb)
	}
	claimReconcile, err := quizcraft.ReconcileLearningState(context.Background(), pool)
	if err != nil || !claimReconcile.Clean() {
		t.Fatalf("claim lost guest learning facts %+v %v", claimReconcile, err)
	}
	// Account training retains first-answer learning facts, while ordinary metrics
	// must equal a real ordinary-only baseline, even with mixed facts.
	ordinary := importPracticeBank(t, pool, "mixed-ordinary-"+actor)
	ordinarySession, ordinaryQuestion := createRankingSession(t, server.URL, auth, ordinary, "mixed-ordinary-session-"+actor)
	status, body = requestJSON(t, "POST", server.URL+"/api/v1/practice/sessions/"+ordinarySession+"/answers", map[string]string{"Cookie": auth, "Idempotency-Key": "mixed-ordinary-answer-" + actor}, map[string]any{"question_id": ordinaryQuestion.QuestionID, "question_version_id": ordinaryQuestion.QuestionVersionID, "answer": 1})
	if status != 200 {
		t.Fatalf("ordinary baseline answer %d %s", status, body)
	}
	status, baseline := requestPersonalStats(t, server.URL, actor)
	if status != 200 {
		t.Fatalf("baseline %d %s", status, baseline)
	}
	status, body, _ = sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions", create, actor, "training-account-create-"+actor))
	if status != 201 {
		t.Fatalf("account create %d %s", status, body)
	}
	decodeJSON(t, body, &session)
	for i, q := range session.Data.Unit.Questions {
		raw := mustJSON(map[string]any{"unit_version_id": detail.Data.Training.Unit.UnitVersionID, "question_id": q.QuestionID, "question_version_id": q.QuestionVersionID, "answer": []int{1, 2, 0, 0}[i]})
		status, body, _ = sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions/"+session.Data.SessionID+"/answers", raw, actor, fmt.Sprintf("training-account-answer-%s-%d", actor, i)))
		if status != 200 {
			t.Fatalf("account answer %d %s", status, body)
		}
	}
	status, body = requestPersonalStats(t, server.URL, actor)
	var before, after map[string]any
	decodeJSON(t, baseline, &before)
	decodeJSON(t, body, &after)
	b, _ := json.Marshal(before["data"])
	a, _ := json.Marshal(after["data"])
	if status != 200 || !bytes.Equal(b, a) {
		t.Fatalf("training changed ordinary stats: before=%s after=%s", b, a)
	}
	reconciled, err := quizcraft.ReconcileLearningState(context.Background(), pool)
	if err != nil || !reconciled.Clean() {
		t.Fatalf("training reconcile %+v %v", reconciled, err)
	}

	rebuilt, err := quizcraft.RebuildLearningState(context.Background(), pool)
	if err != nil || !rebuilt.Clean() {
		t.Fatalf("training rebuild %+v %v", rebuilt, err)
	}
	core, err := quizcraft.New(quizcraft.Config{Database: pool})
	if err != nil {
		t.Fatal(err)
	}
	count, err := core.SettlePreviousUTCWeek(context.Background(), time.Now().UTC().AddDate(0, 0, 7))
	if err != nil || count != 2 {
		t.Fatalf("training generated bank settlement %d %v", count, err)
	}
	var standings []byte
	if err = pool.QueryRow(context.Background(), `SELECT standings FROM quizcraft_ranking_settlement_events WHERE scope='overall'`).Scan(&standings); err != nil || !bytes.Contains(standings, []byte(`"correct_answer_count": 1`)) || bytes.Contains(standings, []byte(`"correct_answer_count": 3`)) {
		t.Fatalf("training entered settlement %s %v", standings, err)
	}
	var difficultyFacts int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM quizcraft_question_stats s JOIN quizcraft_questions q ON q.id=s.question_id WHERE q.bank_id=$1`, bank).Scan(&difficultyFacts); err != nil || difficultyFacts != 0 {
		t.Fatalf("training changed ordinary difficulty %d %v", difficultyFacts, err)
	}

	for _, route := range []string{"/api/v1/rankings/overall", "/api/v1/banks/" + ordinary.BankID + "/rankings", "/api/v1/banks/" + bank + "/rankings"} {
		for _, period := range []string{"weekly", "lifetime"} {
			status, body = sendCatalogRequest(t, newPortalReadRequest(t, server.URL, route+"?period="+period, "portal.practice.read", "req_training_ranking"))
			var ranking struct {
				Data struct {
					Entries []struct {
						Count int `json:"correct_answer_count"`
					} `json:"entries"`
				} `json:"data"`
			}
			decodeJSON(t, body, &ranking)
			expected := 1
			if strings.Contains(route, bank) {
				expected = 0
			}
			if status != 200 || len(ranking.Data.Entries) != expected || (expected == 1 && ranking.Data.Entries[0].Count != 1) {
				t.Fatalf("training changed %s %s ranking %d %s", route, period, status, body)
			}
		}
	}

	// Ordinary favorites creation rejects even a saved favorite from this bank.
	question := session.Data.Unit.Questions[0]
	status, body = requestJSON(t, "PUT", server.URL+"/api/v1/banks/"+bank+"/favorites/"+question.QuestionID, map[string]string{"Cookie": auth, "Idempotency-Key": "training-favorite-" + actor}, map[string]any{})
	if status != 200 {
		t.Fatalf("favorite setup %d %s", status, body)
	}
	status, body = requestJSON(t, "POST", server.URL+"/api/v1/banks/"+bank+"/favorites/practice-sessions", map[string]string{"Cookie": auth, "Idempotency-Key": "training-favorite-bypass-" + actor}, map[string]any{})
	if status < 400 {
		t.Fatalf("favorites accepted training %d %s", status, body)
	}

	input["expected_version"] = 4
	training := input["training"].(map[string]any)
	firstReview := training["question_reviews"].([]any)[0].(map[string]any)
	evidence := firstReview["evidence"].(map[string]any)
	originalEvidence := evidence["segment_ids"]
	evidence["segment_ids"] = []string{"unknown-sentence"}
	status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": "training-invalid-evidence-" + actor}, input)
	if status != 422 {
		t.Fatalf("invalid evidence %d %s", status, body)
	}
	evidence["segment_ids"] = originalEvidence
	training["title"] = "The North Wind and the Sun · revised"
	status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": "training-revision-" + actor}, input)
	if status != 201 {
		t.Fatalf("revision after invalid atomic import %d %s", status, body)
	}
	var revised apiEnvelope[quizcraft.ImportReport]
	decodeJSON(t, body, &revised)
	if revised.Data.BankVersionID == report.Data.BankVersionID {
		t.Fatal("training-only semantic edit did not create bank version")
	}
	for i, q := range report.Data.Questions {
		if q.QuestionID != revised.Data.Questions[i].QuestionID || q.QuestionVersionID != revised.Data.Questions[i].QuestionVersionID {
			t.Fatal("training-only edit renamed pinned questions")
		}
	}
	for i, action := range []string{"validate", "publish"} {
		status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/versions/"+revised.Data.BankVersionID+"/"+action, map[string]string{"Cookie": auth, "Idempotency-Key": "training-revised-" + action + actor}, map[string]any{"expected_version": 5 + i})
		if status != 200 {
			t.Fatalf("revised %s %d %s", action, status, body)
		}
	}
	req = trainingAccountRead(t, server.URL, prefix+"/sessions/"+session.Data.SessionID, actor)
	status, body = sendCatalogRequest(t, req)
	if status != 200 || bytes.Contains(body, []byte("revised")) || !bytes.Contains(body, []byte(report.Data.BankVersionID)) {
		t.Fatalf("old pinned recovery changed %d %s", status, body)
	}
	status, body, _ = sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, prefix+"/sessions", create, actor, "training-old-version-create-"+actor))
	if status != 409 || !bytes.Contains(body, []byte("VERSION_UNAVAILABLE")) {
		t.Fatalf("old version new session %d %s", status, body)
	}
	req = trainingAccountRead(t, server.URL, prefix+"/review-items", actor)
	status, body = sendCatalogRequest(t, req)
	if status != 200 || !bytes.Contains(body, []byte(`"review_required_count":1`)) || !bytes.Contains(body, []byte(detail.Data.Training.Unit.UnitVersionID)) {
		t.Fatalf("durable wrong index %d %s", status, body)
	}

	mediaURL := server.URL + "/practice-media/92aa98d9bf83c7f1532dcd2ae2169190b649b5214c13481405ce68d14a630336.mp3"
	request, _ := http.NewRequest("GET", mediaURL, nil)
	request.Header.Set("Range", "bytes=100-199")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 206 || response.Header.Get("Content-Range") != "bytes 100-199/584180" || response.Header.Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("real range response %#v", response)
	}

}

func trainingAccountRead(t *testing.T, base, path, actor string) *http.Request {
	request := newPortalReadRequest(t, base, path, "portal.practice.read", "req_training_account")
	request.Header.Set("X-Actor-User-Id", actor)
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{"GET", request.URL.RequestURI(), request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:]), actor}, "\n")
	mac := hmac.New(sha256.New, []byte(portalCatalogSecret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	return request
}

func TestTrainingMigrationDownReapplyPreservesOrdinaryFacts(t *testing.T) {
	pool := isolatedArtifactDatabase(t)
	ctx := context.Background()
	if _, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations"); err != nil {
		t.Fatal(err)
	}
	report := importPracticeBank(t, pool, "migration-ordinary-"+uuid.NewString())
	raw, err := os.ReadFile("../db/migrations/000012_english_training.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	var version string
	if err = pool.QueryRow(ctx, `SELECT active_version_id::text FROM quizcraft_banks WHERE id=$1`, report.BankID).Scan(&version); err != nil || version != report.BankVersionID {
		t.Fatalf("ordinary bank changed %s %v", version, err)
	}
	result, err := quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations")
	if err != nil || len(result.Applied) != 1 || result.Applied[0].Version != "000012" {
		t.Fatalf("training down restore %+v %v", result, err)
	}
	result, err = quizcraft.ApplyVersionedMigrations(ctx, pool, "../db/migrations")
	if err != nil || len(result.Applied) != 0 || len(result.Skipped) != 12 {
		t.Fatalf("training repeat %+v %v", result, err)
	}
}
