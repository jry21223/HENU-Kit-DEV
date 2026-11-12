package tests

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
	tc "henukit.dev/quizcraft/internal/trainingcontract"
)

// Every fixture publishes real authored content through the owner lifecycle;
// learner assertions below exercise only the signed HTTP boundary.
func publishedListening(t *testing.T) (*httptest.Server, tc.TrainingCatalogItem) {
	t.Helper()
	server, unit, _ := publishedListeningDatabase(t)
	return server, unit
}

func publishedListeningDatabase(t *testing.T) (*httptest.Server, tc.TrainingCatalogItem, *pgxpool.Pool) {
	t.Helper()
	pool := isolatedQuizcraftV2Database(t)
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), AllowTestWorkshopClaims: true, CatalogClientID: portalCatalogClientID, CatalogKeys: map[string]string{portalCatalogKeyID: portalCatalogSecret}, PortalCommandsEnabled: true, PortalCommandClientID: portalCommandClientID, PortalCommandKeys: map[string]string{portalCommandKeyID: portalCommandSecret}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	actor := uuid.NewString()
	auth := "quizcraft_session=" + workshopToken(t, actor, []string{"quizcraft.workshop.write", "quizcraft.workshop.publish"}, []map[string]string{{"kind": "product", "product_code": "quizcraft"}})
	status, body := requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks", map[string]string{"Cookie": auth, "Idempotency-Key": "reveal-bank-" + actor}, map[string]string{"bank_key": "reveal-" + actor, "name": "Listening"})
	if status != 201 {
		t.Fatalf("bank %d %s", status, body)
	}
	bank := operationResourceID(t, body)
	raw, err := os.ReadFile("../content/english-training/north-wind.import.json")
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	decodeJSON(t, raw, &input)
	status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": "reveal-import-" + actor}, input)
	if status != 201 {
		t.Fatalf("import %d %s", status, body)
	}
	var report apiEnvelope[quizcraft.ImportReport]
	decodeJSON(t, body, &report)
	for i, action := range []string{"validate", "publish"} {
		status, body = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/versions/"+report.Data.BankVersionID+"/"+action, map[string]string{"Cookie": auth, "Idempotency-Key": "reveal-" + action + actor}, map[string]any{"expected_version": 2 + i, "note": "Isolated acceptance; new human ear review remains pending"})
		if status != 200 {
			t.Fatalf("%s %d %s", action, status, body)
		}
	}
	status, body = sendCatalogRequest(t, newPortalReadRequest(t, server.URL, trainingTestPrefix+"/catalog", "portal.practice.read", "req_reveal_catalog"))
	var catalog tc.TrainingCatalogEnvelope
	decodeJSON(t, body, &catalog)
	if status != 200 || len(catalog.Data.Units) != 1 {
		t.Fatalf("catalog %d %s", status, body)
	}
	return server, catalog.Data.Units[0], pool
}

const trainingTestPrefix = "/api/v1/portal/practice/training"

func TestListeningLayersRequireCompleteFirstAnswersAndRecoverMonotonically(t *testing.T) {
	server, unit := publishedListening(t)
	create := mustJSON(tc.TrainingCreateSession{UnitId: unit.UnitId, UnitVersionId: unit.UnitVersionId})
	status, body, cookies := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, trainingTestPrefix+"/sessions", create, "", "reveal-create-"+uuid.NewString()))
	if status != 201 {
		t.Fatalf("create %d %s", status, body)
	}
	cookie := findCookie(t, cookies, "quizcraft_anonymous")
	var session tc.TrainingSessionEnvelope
	decodeJSON(t, body, &session)
	path := trainingTestPrefix + "/sessions/" + session.Data.SessionId.String()
	command := func(layer, key, actor string, c *http.Cookie) (int, []byte) {
		req := newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/reveals", mustJSON(tc.TrainingRevealRequest{UnitVersionId: unit.UnitVersionId, Layer: tc.TrainingRevealRequestLayer(layer)}), actor, key)
		if c != nil {
			req.AddCookie(c)
		}
		s, b, _ := sendPortalPracticeCommand(t, req)
		return s, b
	}
	read := func(suffix, actor string, c *http.Cookie) (int, []byte) {
		req := newPortalReadRequest(t, server.URL, path+suffix, "portal.practice.read", "req_reveal_recovery")
		if actor != "" {
			req = trainingAccountRead(t, server.URL, path+suffix, actor)
		}
		if c != nil {
			req.AddCookie(c)
		}
		return sendCatalogRequest(t, req)
	}
	for i, q := range mustListeningUnit(t, session.Data).Questions {
		status, body = command("english", "pending-reveal-"+uuid.NewString(), "", cookie)
		if status != 409 || !bytes.Contains(body, []byte("UNIT_INCOMPLETE")) {
			t.Fatalf("pending/partial reveal %d %s", status, body)
		}
		if bytes.Contains(body, []byte("were disputing")) {
			t.Fatalf("partial transcript leak %s", body)
		}
		req := newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/answers", mustJSON(tc.TrainingAnswerSubmission{UnitVersionId: unit.UnitVersionId, QuestionId: q.QuestionId, QuestionVersionId: q.QuestionVersionId, Answer: []int{1, 2, 0, 0}[i]}), "", fmt.Sprintf("reveal-answer-%s-%d", session.Data.SessionId, i))
		req.AddCookie(cookie)
		status, body, _ = sendPortalPracticeCommand(t, req)
		if status != 200 {
			t.Fatalf("answer %d %s", status, body)
		}
	}
	status, body = read("/review", "", cookie)
	// Decode the wire union into its concrete listening shape for assertions.
	var initial apiEnvelope[tc.TrainingListeningReview]
	decodeJSON(t, body, &initial)
	if status != 200 || len(initial.Data.Layers) != 0 || len(initial.Data.Segments) != 5 {
		t.Fatalf("unrevealed review %d %s", status, body)
	}
	status, body = command("chinese", "locked-chinese-"+uuid.NewString(), "", cookie)
	if status != 409 || !bytes.Contains(body, []byte("LAYER_LOCKED")) {
		t.Fatalf("skipped English %d %s", status, body)
	}
	key := "english-reveal-" + uuid.NewString()
	status, body = command("english", key, "", cookie)
	if status != 200 {
		t.Fatalf("English reveal %d %s", status, body)
	}
	var english tc.TrainingLayerDisclosureEnvelope
	decodeJSON(t, body, &english)
	content, err := english.Data.Content.AsTrainingListeningLayer()
	if err != nil || content.Layer != "english" || len(content.Segments) != 5 || content.Segments[0].SegmentId != "s1" {
		t.Fatalf("pinned English %+v %v", content, err)
	}
	for _, replayKey := range []string{key, "new-key-" + uuid.NewString()} {
		status, body = command("english", replayKey, "", cookie)
		var replay tc.TrainingLayerDisclosureEnvelope
		decodeJSON(t, body, &replay)
		if status != 200 || replay.Data.DisclosureId != english.Data.DisclosureId || !replay.Data.DisclosedAt.Equal(english.Data.DisclosedAt) {
			t.Fatalf("changed disclosure on replay %d %s", status, body)
		}
	}
	status, body = command("chinese", key, "", cookie)
	if status != 409 || !bytes.Contains(body, []byte("IDEMPOTENCY_CONFLICT")) {
		t.Fatalf("changed reveal body %d %s", status, body)
	}
	status, body = read("", "", cookie)
	decodeJSON(t, body, &session)
	if status != 200 || len(session.Data.RevealedLayers) != 1 || session.Data.RevealedLayers[0] != "english" || bytes.Contains(body, []byte("were disputing")) {
		t.Fatalf("safe revealed recovery %d %s", status, body)
	}
	actor := uuid.NewString()
	status, body = command("chinese", "claim-chinese-"+uuid.NewString(), actor, cookie)
	if status != 200 {
		t.Fatalf("claim Chinese %d %s", status, body)
	}
	status, body = command("english", key, "", cookie)
	if status != 404 {
		t.Fatalf("old guest reveal replay leaked claimed text %d %s", status, body)
	}
	status, body = command("english", "foreign-reveal-"+uuid.NewString(), uuid.NewString(), cookie)
	if status != 404 {
		t.Fatalf("foreign reveal %d %s", status, body)
	}
	status, body = read("/review", actor, nil)
	var recovered apiEnvelope[tc.TrainingListeningReview]
	decodeJSON(t, body, &recovered)
	if status != 200 || len(recovered.Data.Layers) != 2 || recovered.Data.Layers[0].DisclosureId != english.Data.DisclosureId {
		t.Fatalf("claimed layer recovery %d %s", status, body)
	}
	status, body = sendCatalogRequest(t, trainingAccountRead(t, server.URL, trainingTestPrefix+"/review-items", actor))
	if status != 200 || !bytes.Contains(body, []byte(`"review_required_count":1`)) {
		t.Fatalf("claim reveal lost review state %d %s", status, body)
	}
	// Post-submit text never changes the saved first answers or their outcomes.
	status, body = read("", actor, nil)
	decodeJSON(t, body, &session)
	if status != 200 || len(session.Data.Receipts) != 4 || session.Data.Receipts[0].Assisted || session.Data.Receipts[0].Outcome != "independent_correct" || session.Data.Receipts[3].Outcome != "review_required" {
		t.Fatalf("reveal changed facts %d %s", status, body)
	}
}

func mustListeningUnit(t *testing.T, s tc.TrainingSession) tc.TrainingListeningUnit {
	t.Helper()
	u, err := s.Unit.AsTrainingListeningUnit()
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestListeningRevealSerializesWithLastAnswerAndFreshIndependentPracticeClearsReview(t *testing.T) {
	server, unit := publishedListening(t)
	actor := uuid.NewString()
	create := func() tc.TrainingSession {
		status, body, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, trainingTestPrefix+"/sessions", mustJSON(tc.TrainingCreateSession{UnitId: unit.UnitId, UnitVersionId: unit.UnitVersionId}), actor, "fresh-listening-"+uuid.NewString()))
		if status != 201 {
			t.Fatalf("create %d %s", status, body)
		}
		var result tc.TrainingSessionEnvelope
		decodeJSON(t, body, &result)
		return result.Data
	}
	first := create()
	path := trainingTestPrefix + "/sessions/" + first.SessionId.String()
	questions := mustListeningUnit(t, first).Questions
	answer := func(session tc.TrainingSession, index, choice int) *http.Request {
		q := questions[index]
		return newPortalPracticeCommandRequest(t, "POST", server.URL, trainingTestPrefix+"/sessions/"+session.SessionId.String()+"/answers", mustJSON(tc.TrainingAnswerSubmission{UnitVersionId: unit.UnitVersionId, QuestionId: q.QuestionId, QuestionVersionId: q.QuestionVersionId, Answer: choice}), actor, "race-answer-"+uuid.NewString())
	}
	for i, choice := range []int{1, 2, 0} {
		status, body, _ := sendPortalPracticeCommand(t, answer(first, i, choice))
		if status != 200 {
			t.Fatalf("answer %d %s", status, body)
		}
	}
	reveal := func(key string) *http.Request {
		return newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/reveals", mustJSON(tc.TrainingRevealRequest{UnitVersionId: unit.UnitVersionId, Layer: "english"}), actor, key)
	}
	// The reveal may run before or after the last answer, but it may never release
	// text while only three exact first answers exist.
	requests := []*http.Request{answer(first, 3, 0), reveal("race-before-complete-" + uuid.NewString())}
	statuses := make([]int, 2)
	bodies := make([][]byte, 2)
	var group sync.WaitGroup
	for i, request := range requests {
		group.Add(1)
		go func(i int, r *http.Request) {
			defer group.Done()
			statuses[i], bodies[i], _ = sendPortalPracticeCommand(t, r)
		}(i, request)
	}
	group.Wait()
	if statuses[0] != 200 || (statuses[1] != 200 && statuses[1] != 409) {
		t.Fatalf("last answer/reveal race %v %q", statuses, bodies)
	}
	if statuses[1] == 409 && !bytes.Contains(bodies[1], []byte("UNIT_INCOMPLETE")) {
		t.Fatalf("race denial %s", bodies[1])
	}
	ids := make([]uuid.UUID, 8)
	for i := range ids {
		group.Add(1)
		request := reveal("concurrent-reveal-" + uuid.NewString())
		go func(i int, r *http.Request) {
			defer group.Done()
			status, body, _ := sendPortalPracticeCommand(t, r)
			if status != 200 {
				t.Errorf("concurrent reveal %d %s", status, body)
				return
			}
			var result tc.TrainingLayerDisclosureEnvelope
			decodeJSON(t, body, &result)
			ids[i] = result.Data.DisclosureId
		}(i, request)
	}
	group.Wait()
	for _, id := range ids {
		if id == uuid.Nil || id != ids[0] {
			t.Fatalf("duplicate disclosure facts %v", ids)
		}
	}
	status, body := sendCatalogRequest(t, trainingAccountRead(t, server.URL, trainingTestPrefix+"/review-items", actor))
	var index tc.TrainingReviewItemsEnvelope
	decodeJSON(t, body, &index)
	if status != 200 || len(index.Data.Items) != 1 || index.Data.Items[0].SessionId != first.SessionId || index.Data.Items[0].ReviewRequiredCount != 1 {
		t.Fatalf("pinned wrong review %d %s", status, body)
	}
	second := create()
	if second.SessionId == first.SessionId || second.SavedCount != 0 || len(second.RevealedLayers) != 0 {
		t.Fatalf("re-practice reused old facts %+v", second)
	}
	for i, choice := range []int{1, 2, 0, 3} {
		status, body, _ := sendPortalPracticeCommand(t, answer(second, i, choice))
		if status != 200 {
			t.Fatalf("independent answer %d %s", status, body)
		}
	}
	status, body = sendCatalogRequest(t, trainingAccountRead(t, server.URL, trainingTestPrefix+"/review-items", actor))
	decodeJSON(t, body, &index)
	if status != 200 || len(index.Data.Items) != 0 {
		t.Fatalf("fresh independent success did not clear review %d %s", status, body)
	}
	status, body = sendCatalogRequest(t, trainingAccountRead(t, server.URL, path, actor))
	var old tc.TrainingSessionEnvelope
	decodeJSON(t, body, &old)
	if status != 200 || !old.Data.Receipts[3].ReviewRequired || old.Data.Receipts[3].Answer != 0 || old.Data.RevealedLayers[0] != "english" {
		t.Fatalf("new practice rewrote old first facts %d %s", status, body)
	}
}

func TestListeningRevealRetainsWriteGates(t *testing.T) {
	pool := isolatedQuizcraftV2Database(t)
	path := trainingTestPrefix + "/sessions/" + uuid.NewString() + "/reveals"
	raw := mustJSON(tc.TrainingRevealRequest{UnitVersionId: uuid.New(), Layer: "english"})
	for _, gate := range []struct {
		name             string
		enabled, stopped bool
	}{{"default off", false, false}, {"migration stop write", true, true}} {
		t.Run(gate.name, func(t *testing.T) {
			handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), CatalogClientID: portalCatalogClientID, CatalogKeys: map[string]string{portalCatalogKeyID: portalCatalogSecret}, PortalCommandsEnabled: gate.enabled, WritesDisabled: gate.stopped, PortalCommandClientID: portalCommandClientID, PortalCommandKeys: map[string]string{portalCommandKeyID: portalCommandSecret}})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			status, body, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, path, raw, "", "disabled-reveal-"+uuid.NewString()))
			if status != 503 || !bytes.Contains(body, []byte("TRAINING_DISABLED")) {
				t.Fatalf("gate %d %s", status, body)
			}
		})
	}
}

func TestListeningRevealRejectsClientDisclosureAuthority(t *testing.T) {
	server, unit := publishedListening(t)
	path := trainingTestPrefix + "/sessions/" + uuid.NewString() + "/reveals"
	for _, body := range [][]byte{
		[]byte(fmt.Sprintf(`{"unit_version_id":%q,"layer":"english","submitted":true}`, unit.UnitVersionId)),
		[]byte(fmt.Sprintf(`{"unit_version_id":%q,"layer":"english","layer":"chinese"}`, unit.UnitVersionId)),
	} {
		status, response, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, path, body, "", "closed-reveal-"+uuid.NewString()))
		if status != 400 || !bytes.Contains(response, []byte("INVALID_REQUEST")) {
			t.Fatalf("client authority accepted %d %s", status, response)
		}
	}
}

func TestListeningDisclosureTimeFollowsSessionSerialization(t *testing.T) {
	server, unit, pool := publishedListeningDatabase(t)
	actor := uuid.NewString()
	status, body, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, trainingTestPrefix+"/sessions", mustJSON(tc.TrainingCreateSession{UnitId: unit.UnitId, UnitVersionId: unit.UnitVersionId}), actor, "clock-session-"+uuid.NewString()))
	if status != 201 {
		t.Fatalf("create %d %s", status, body)
	}
	var session tc.TrainingSessionEnvelope
	decodeJSON(t, body, &session)
	path := trainingTestPrefix + "/sessions/" + session.Data.SessionId.String()
	for i, q := range mustListeningUnit(t, session.Data).Questions {
		status, body, _ = sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/answers", mustJSON(tc.TrainingAnswerSubmission{UnitVersionId: unit.UnitVersionId, QuestionId: q.QuestionId, QuestionVersionId: q.QuestionVersionId, Answer: []int{1, 2, 0, 3}[i]}), actor, "clock-answer-"+uuid.NewString()))
		if status != 200 {
			t.Fatalf("answer %d %s", status, body)
		}
	}
	// Hold the real session serialization resource as a scheduling fixture. We
	// assert the returned HTTP fact, not table contents or collaborator calls.
	lock, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Rollback(context.Background()) }()
	if _, err = lock.Exec(context.Background(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, session.Data.SessionId.String()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/reveals", mustJSON(tc.TrainingRevealRequest{UnitVersionId: unit.UnitVersionId, Layer: "english"}), actor, "clock-reveal-"+uuid.NewString()).WithContext(ctx)
	type result struct {
		status int
		body   []byte
	}
	done := make(chan result, 1)
	go func() { s, b, _ := sendPortalPracticeCommand(t, request); done <- result{s, b} }()
	for {
		var waiting bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database()))`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("reveal did not reach the held session lock")
		case <-time.After(time.Millisecond):
		}
	}
	var releasedAt time.Time
	if err = lock.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&releasedAt); err != nil {
		t.Fatal(err)
	}
	if err = lock.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	var response result
	select {
	case response = <-done:
	case <-ctx.Done():
		t.Fatal("reveal did not complete after releasing the session lock")
	}
	var disclosure tc.TrainingLayerDisclosureEnvelope
	decodeJSON(t, response.body, &disclosure)
	if response.status != 200 || disclosure.Data.DisclosedAt.Before(releasedAt) {
		t.Fatalf("disclosure predates session authorization: released=%s status=%d body=%s", releasedAt, response.status, response.body)
	}
}
