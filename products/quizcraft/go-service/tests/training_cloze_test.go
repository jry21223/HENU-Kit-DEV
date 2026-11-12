package tests

import (
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	quizcraft "henukit.dev/quizcraft"
	tc "henukit.dev/quizcraft/internal/trainingcontract"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"
)

func TestClozePublishesAndHintsPersistBeforeAssistedFirstAnswers(t *testing.T) {
	server, unit, _ := publishedTrainingDatabase(t, "campus-repair.import.json")
	actor := uuid.NewString()
	status, body, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, trainingTestPrefix+"/sessions", mustJSON(tc.TrainingCreateSession{UnitId: unit.UnitId, UnitVersionId: unit.UnitVersionId}), actor, "cloze-start-"+uuid.NewString()))
	var session tc.TrainingSessionEnvelope
	decodeJSON(t, body, &session)
	if status != 201 {
		t.Fatalf("create %d %s", status, body)
	}
	u, err := session.Data.Unit.AsTrainingClozeUnit()
	if err != nil || u.Kind != "cloze" || len(u.Parts) != 35 || len(u.Blanks) != 10 {
		t.Fatalf("safe cloze %+v %v", u, err)
	}
	for _, secret := range []string{"expected_answer", "analysis", "evidence", "levels"} {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatalf("pending leak %s", body)
		}
	}
	path := trainingTestPrefix + "/sessions/" + session.Data.SessionId.String()
	answer := func(i, choice int) tc.TrainingAnswerReceipt {
		q := u.Questions[i]
		s, b, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/answers", mustJSON(tc.TrainingAnswerSubmission{UnitVersionId: unit.UnitVersionId, QuestionId: q.QuestionId, QuestionVersionId: q.QuestionVersionId, Answer: choice}), actor, "cloze-answer-"+uuid.NewString()))
		if s != 200 {
			t.Fatalf("answer %d %s", s, b)
		}
		var receipt tc.TrainingAnswerReceiptEnvelope
		decodeJSON(t, b, &receipt)
		return receipt.Data
	}
	first := answer(0, 1)
	if first.Assisted || first.Outcome != "independent_correct" {
		t.Fatalf("independent %+v", first)
	}
	hint := tc.TrainingHintRequest{UnitVersionId: unit.UnitVersionId, HintId: u.Hints[1].HintId, Level: 1}
	key := "cloze-hint-" + uuid.NewString()
	status, body, _ = sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/hints", mustJSON(hint), actor, key))
	if status != 200 {
		t.Fatalf("hint %d %s", status, body)
	}
	var disclosure tc.TrainingHintDisclosureEnvelope
	decodeJSON(t, body, &disclosure)
	if len(disclosure.Data.AffectedQuestionIds) != 9 || disclosure.Data.Text == "" {
		t.Fatalf("hint %+v", disclosure.Data)
	}
	for _, k := range []string{key, "cloze-hint-replay-" + uuid.NewString()} {
		s, b, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/hints", mustJSON(hint), actor, k))
		var replay tc.TrainingHintDisclosureEnvelope
		decodeJSON(t, b, &replay)
		if s != 200 || replay.Data.DisclosureId != disclosure.Data.DisclosureId {
			t.Fatalf("hint replay %d %s", s, b)
		}
	}
	status, body = sendCatalogRequest(t, trainingAccountRead(t, server.URL, path, actor))
	decodeJSON(t, body, &session)
	if status != 200 || len(session.Data.HintDisclosures) != 1 || session.Data.Receipts[0].Assisted {
		t.Fatalf("recovery %d %s", status, body)
	}
	status, body = sendCatalogRequest(t, trainingAccountRead(t, server.URL, path+"/review", actor))
	if status != 409 {
		t.Fatalf("partial review %d %s", status, body)
	}
	for i, choice := range []int{1, 2, 0, 3, 1, 0, 2, 3, 0, 2} {
		if i == 0 {
			continue
		}
		receipt := answer(i, choice)
		if !receipt.Assisted || receipt.Outcome != "assisted_completion" || !receipt.ReviewRequired {
			t.Fatalf("assisted %+v", receipt)
		}
	}
	status, body = sendCatalogRequest(t, trainingAccountRead(t, server.URL, path+"/review", actor))
	var review apiEnvelope[tc.TrainingClozeReview]
	decodeJSON(t, body, &review)
	if status != 200 || review.Data.Kind != "cloze" || len(review.Data.Questions) != 10 || len(review.Data.Layers) != 0 {
		t.Fatalf("review %d %s", status, body)
	}
	status, body = sendCatalogRequest(t, trainingAccountRead(t, server.URL, trainingTestPrefix+"/review-items", actor))
	if status != 200 || !bytes.Contains(body, []byte(`"review_required_count":9`)) || !bytes.Contains(body, []byte(`"kind":"cloze"`)) {
		t.Fatalf("review index %d %s", status, body)
	}
}

func TestClozeHintReplayClaimAndLayerRecoveryStayOwnerPinned(t *testing.T) {
	server, unit, _ := publishedTrainingDatabase(t, "campus-repair.import.json")
	s, b, cookies := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, trainingTestPrefix+"/sessions", mustJSON(tc.TrainingCreateSession{UnitId: unit.UnitId, UnitVersionId: unit.UnitVersionId}), "", "cloze-guest-"+uuid.NewString()))
	if s != 201 {
		t.Fatalf("create %d %s", s, b)
	}
	var session tc.TrainingSessionEnvelope
	decodeJSON(t, b, &session)
	u, _ := session.Data.Unit.AsTrainingClozeUnit()
	cookie := findCookie(t, cookies, "quizcraft_anonymous")
	path := trainingTestPrefix + "/sessions/" + session.Data.SessionId.String()
	command := func(suffix string, input any, key, actor string, guest bool) (int, []byte) {
		r := newPortalPracticeCommandRequest(t, "POST", server.URL, path+suffix, mustJSON(input), actor, key)
		if guest {
			r.AddCookie(cookie)
		}
		s, b, _ := sendPortalPracticeCommand(t, r)
		return s, b
	}
	hint := tc.TrainingHintRequest{UnitVersionId: unit.UnitVersionId, HintId: u.Hints[0].HintId, Level: 2}
	s, b = command("/hints", hint, "cloze-skip-"+uuid.NewString(), "", true)
	if s != 409 || !bytes.Contains(b, []byte("LAYER_LOCKED")) {
		t.Fatalf("skip %d %s", s, b)
	}
	hint.Level = 1
	key := "cloze-hint-claim-" + uuid.NewString()
	s, b = command("/hints", hint, key, "", true)
	if s != 200 {
		t.Fatalf("hint %d %s", s, b)
	}
	var original tc.TrainingHintDisclosureEnvelope
	decodeJSON(t, b, &original)
	hint.Level = 2
	s, b = command("/hints", hint, key, "", true)
	if s != 409 || !bytes.Contains(b, []byte("IDEMPOTENCY_CONFLICT")) {
		t.Fatalf("changed key %d %s", s, b)
	}
	actor := uuid.NewString()
	r := trainingAccountRead(t, server.URL, path, actor)
	r.AddCookie(cookie)
	s, b = sendCatalogRequest(t, r)
	decodeJSON(t, b, &session)
	if s != 200 || session.Data.RecoveryScope != "browser" {
		t.Fatalf("GET claimed guest %d %s", s, b)
	}
	s, b = command("/hints", hint, "cloze-claim-level-"+uuid.NewString(), actor, true)
	if s != 200 {
		t.Fatalf("claim hint %d %s", s, b)
	}
	hint.Level = 1
	s, b = command("/hints", hint, key, "", true)
	if s != 404 {
		t.Fatalf("old guest leaked %d %s", s, b)
	}
	s, b = command("/hints", hint, "cloze-foreign-"+uuid.NewString(), uuid.NewString(), true)
	if s != 404 {
		t.Fatalf("foreign hint %d %s", s, b)
	}
	s, b = sendCatalogRequest(t, trainingAccountRead(t, server.URL, path, actor))
	decodeJSON(t, b, &session)
	if s != 200 || len(session.Data.HintDisclosures) != 2 || session.Data.HintDisclosures[0].DisclosureId != original.Data.DisclosureId || session.Data.RecoveryScope != "account" {
		t.Fatalf("claim lost hints %d %s", s, b)
	}
	for i, q := range u.Questions {
		s, b = command("/answers", tc.TrainingAnswerSubmission{UnitVersionId: unit.UnitVersionId, QuestionId: q.QuestionId, QuestionVersionId: q.QuestionVersionId, Answer: []int{1, 2, 0, 3, 1, 0, 2, 3, 0, 2}[i]}, "cloze-claim-answer-"+uuid.NewString(), actor, false)
		if s != 200 {
			t.Fatalf("answer %d %s", s, b)
		}
	}
	reveal := tc.TrainingRevealRequest{UnitVersionId: unit.UnitVersionId, Layer: "chinese"}
	s, b = command("/reveals", reveal, "cloze-skip-language-"+uuid.NewString(), actor, false)
	if s != 409 {
		t.Fatalf("Chinese before English %d %s", s, b)
	}
	for _, language := range []string{"english", "chinese"} {
		reveal.Layer = tc.TrainingRevealRequestLayer(language)
		s, b = command("/reveals", reveal, "cloze-language-"+uuid.NewString(), actor, false)
		var disclosed tc.TrainingLayerDisclosureEnvelope
		decodeJSON(t, b, &disclosed)
		layer, err := disclosed.Data.Content.AsTrainingClozeLayer()
		if s != 200 || err != nil || len(layer.Parts) != 35 || string(layer.Layer) != language {
			t.Fatalf("cloze reveal %d %s", s, b)
		}
	}
	s, b = sendCatalogRequest(t, trainingAccountRead(t, server.URL, path+"/review", actor))
	var review apiEnvelope[tc.TrainingClozeReview]
	decodeJSON(t, b, &review)
	if s != 200 || len(review.Data.Layers) != 2 {
		t.Fatalf("recovery layers %d %s", s, b)
	}
}

func TestClozeHintAnswerRaceMatchesCommittedAffectedQuestionsAndRepair(t *testing.T) {
	server, unit, pool := publishedTrainingDatabase(t, "campus-repair.import.json")
	actor := uuid.NewString()
	status, baseline := requestPersonalStats(t, server.URL, actor)
	if status != 200 {
		t.Fatalf("stats %d %s", status, baseline)
	}
	for round := 0; round < 6; round++ {
		s, b, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, trainingTestPrefix+"/sessions", mustJSON(tc.TrainingCreateSession{UnitId: unit.UnitId, UnitVersionId: unit.UnitVersionId}), actor, "cloze-race-create-"+uuid.NewString()))
		if s != 201 {
			t.Fatalf("create %d %s", s, b)
		}
		var session tc.TrainingSessionEnvelope
		decodeJSON(t, b, &session)
		u, _ := session.Data.Unit.AsTrainingClozeUnit()
		path := trainingTestPrefix + "/sessions/" + session.Data.SessionId.String()
		q := u.Questions[0]
		requests := []*http.Request{newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/hints", mustJSON(tc.TrainingHintRequest{UnitVersionId: unit.UnitVersionId, HintId: u.Hints[0].HintId, Level: 1}), actor, "cloze-race-hint-"+uuid.NewString()), newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/answers", mustJSON(tc.TrainingAnswerSubmission{UnitVersionId: unit.UnitVersionId, QuestionId: q.QuestionId, QuestionVersionId: q.QuestionVersionId, Answer: 1}), actor, "cloze-race-answer-"+uuid.NewString())}
		statuses := make([]int, 2)
		bodies := make([][]byte, 2)
		var group sync.WaitGroup
		for i, r := range requests {
			group.Add(1)
			go func(i int, r *http.Request) {
				defer group.Done()
				statuses[i], bodies[i], _ = sendPortalPracticeCommand(t, r)
			}(i, r)
		}
		group.Wait()
		if statuses[0] != 200 || statuses[1] != 200 {
			t.Fatalf("race %v %q", statuses, bodies)
		}
		var hint tc.TrainingHintDisclosureEnvelope
		var receipt tc.TrainingAnswerReceiptEnvelope
		decodeJSON(t, bodies[0], &hint)
		decodeJSON(t, bodies[1], &receipt)
		affected := false
		for _, id := range hint.Data.AffectedQuestionIds {
			if id == q.QuestionId {
				affected = true
			}
		}
		if receipt.Data.Assisted != affected || receipt.Data.ReviewRequired != affected || !receipt.Data.Correct {
			t.Fatalf("race assistance disagrees hint=%+v receipt=%+v", hint.Data, receipt.Data)
		}
		if affected && receipt.Data.SubmittedAt.Before(hint.Data.DisclosedAt) || !affected && hint.Data.DisclosedAt.Before(receipt.Data.SubmittedAt) {
			t.Fatalf("serialized facts have reversed times: %+v %+v", hint.Data, receipt.Data)
		}
		s, b = sendCatalogRequest(t, trainingAccountRead(t, server.URL, path, actor))
		decodeJSON(t, b, &session)
		if s != 200 || len(session.Data.HintDisclosures) != 1 || session.Data.Receipts[0].Assisted != affected {
			t.Fatalf("race recovery %d %s", s, b)
		}
	}
	reconciled, err := quizcraft.ReconcileLearningState(context.Background(), pool)
	if err != nil || !reconciled.Clean() {
		t.Fatalf("assisted reconcile %+v %v", reconciled, err)
	}
	rebuilt, err := quizcraft.RebuildLearningState(context.Background(), pool)
	if err != nil || !rebuilt.Clean() {
		t.Fatalf("assisted rebuild %+v %v", rebuilt, err)
	}
	status, after := requestPersonalStats(t, server.URL, actor)
	var beforeJSON, afterJSON map[string]any
	decodeJSON(t, baseline, &beforeJSON)
	decodeJSON(t, after, &afterJSON)
	if status != 200 || !reflect.DeepEqual(beforeJSON["data"], afterJSON["data"]) {
		t.Fatalf("training changed ordinary stats %s %s", baseline, after)
	}
	// A new independent first answer clears review for the same stable question.
	s, b, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, trainingTestPrefix+"/sessions", mustJSON(tc.TrainingCreateSession{UnitId: unit.UnitId, UnitVersionId: unit.UnitVersionId}), actor, "cloze-fresh-clear-"+uuid.NewString()))
	if s != 201 {
		t.Fatalf("fresh session %d %s", s, b)
	}
	var fresh tc.TrainingSessionEnvelope
	decodeJSON(t, b, &fresh)
	u, _ := fresh.Data.Unit.AsTrainingClozeUnit()
	path := trainingTestPrefix + "/sessions/" + fresh.Data.SessionId.String()
	for i, q := range u.Questions {
		s, b, _ = sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/answers", mustJSON(tc.TrainingAnswerSubmission{UnitVersionId: unit.UnitVersionId, QuestionId: q.QuestionId, QuestionVersionId: q.QuestionVersionId, Answer: []int{1, 2, 0, 3, 1, 0, 2, 3, 0, 2}[i]}), actor, "cloze-clear-answer-"+uuid.NewString()))
		if s != 200 {
			t.Fatalf("fresh answer %d %s", s, b)
		}
	}
	s, b = sendCatalogRequest(t, trainingAccountRead(t, server.URL, trainingTestPrefix+"/review-items", actor))
	var index tc.TrainingReviewItemsEnvelope
	decodeJSON(t, b, &index)
	if s != 200 || len(index.Data.Items) != 0 {
		t.Fatalf("fresh independent did not clear review %d %s", s, b)
	}
}

func TestClozePublicationRejectsInvalidBindingsWithoutPartialWrites(t *testing.T) {
	pool := isolatedQuizcraftV2Database(t)
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{Database: pool, AuthHMACSecret: []byte(practiceAuthSecret), AllowTestWorkshopClaims: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	actor := uuid.NewString()
	auth := "quizcraft_session=" + workshopToken(t, actor, []string{"quizcraft.workshop.write", "quizcraft.workshop.read"}, []map[string]string{{"kind": "product", "product_code": "quizcraft"}})
	s, b := requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks", map[string]string{"Cookie": auth, "Idempotency-Key": "cloze-invalid-bank-" + actor}, map[string]string{"bank_key": "cloze-invalid-" + actor, "name": "Original cloze"})
	if s != 201 {
		t.Fatalf("bank %d %s", s, b)
	}
	bank := operationResourceID(t, b)
	raw, err := os.ReadFile("../content/english-training/campus-repair.import.json")
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(map[string]any){
		func(a map[string]any) { a["parts"].([]any)[1] = a["parts"].([]any)[0] },
		func(a map[string]any) { a["blanks"].([]any)[1] = a["blanks"].([]any)[0] },
		func(a map[string]any) {
			a["question_reviews"].([]any)[0].(map[string]any)["evidence"].(map[string]any)["part_ids"] = []string{"unknown-part"}
		},
		func(a map[string]any) { a["attributions"] = []any{} },
		func(a map[string]any) {
			a["layers"].([]any)[0].(map[string]any)["parts"].([]any)[3].(map[string]any)["text"] = "wrong filled answer"
		},
		func(a map[string]any) { a["hints"].([]any)[1] = a["hints"].([]any)[0] },
	}
	for i, mutate := range mutations {
		var input map[string]any
		decodeJSON(t, raw, &input)
		mutate(input["training"].(map[string]any))
		s, b = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": fmt.Sprintf("cloze-invalid-%s-%d", actor, i)}, input)
		if s < 400 {
			t.Fatalf("invalid import %d accepted %d %s", i, s, b)
		}
	}
	var valid map[string]any
	decodeJSON(t, raw, &valid)
	s, b = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+bank+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": "cloze-valid-after-invalid-" + actor}, valid)
	if s != 201 {
		t.Fatalf("invalid imports partially wrote bank: %d %s", s, b)
	}
}

func TestClozeNewPublicationPreservesOldPinnedHintsPassageAndEvidence(t *testing.T) {
	server, unit, _ := publishedTrainingDatabase(t, "campus-repair.import.json")
	actor := uuid.NewString()
	create := func(selected tc.TrainingCatalogItem) tc.TrainingSession {
		s, b, _ := sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, trainingTestPrefix+"/sessions", mustJSON(tc.TrainingCreateSession{UnitId: selected.UnitId, UnitVersionId: selected.UnitVersionId}), actor, "cloze-version-session-"+uuid.NewString()))
		if s != 201 {
			t.Fatalf("create %d %s", s, b)
		}
		var result tc.TrainingSessionEnvelope
		decodeJSON(t, b, &result)
		return result.Data
	}
	old := create(unit)
	oldUnit, _ := old.Unit.AsTrainingClozeUnit()
	auth := "quizcraft_session=" + workshopToken(t, actor, []string{"quizcraft.workshop.write", "quizcraft.workshop.publish"}, []map[string]string{{"kind": "product", "product_code": "quizcraft"}})
	raw, err := os.ReadFile("../content/english-training/campus-repair.import.json")
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	decodeJSON(t, raw, &input)
	input["expected_version"] = 4
	training := input["training"].(map[string]any)
	training["hints"].([]any)[0].(map[string]any)["levels"].([]any)[0] = "新版本提示：结合全句判断替代关系。"
	s, b := requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+unit.BankId.String()+"/imports", map[string]string{"Cookie": auth, "Idempotency-Key": "cloze-revised-import-" + uuid.NewString()}, input)
	if s != 201 {
		t.Fatalf("revision %d %s", s, b)
	}
	var imported apiEnvelope[quizcraft.ImportReport]
	decodeJSON(t, b, &imported)
	for i, action := range []string{"validate", "publish"} {
		s, b = requestJSON(t, "POST", server.URL+"/api/v1/workshop/banks/"+unit.BankId.String()+"/versions/"+imported.Data.BankVersionID+"/"+action, map[string]string{"Cookie": auth, "Idempotency-Key": "cloze-revised-" + action + uuid.NewString()}, map[string]any{"expected_version": 5 + i, "note": "Isolated cloze version acceptance"})
		if s != 200 {
			t.Fatalf("%s %d %s", action, s, b)
		}
	}
	s, b = sendCatalogRequest(t, newPortalReadRequest(t, server.URL, trainingTestPrefix+"/catalog", "portal.practice.read", "req_cloze_revised"))
	var catalog tc.TrainingCatalogEnvelope
	decodeJSON(t, b, &catalog)
	if s != 200 || len(catalog.Data.Units) != 1 {
		t.Fatalf("catalog %d %s", s, b)
	}
	current := catalog.Data.Units[0]
	if current.UnitId != unit.UnitId || current.UnitVersionId == unit.UnitVersionId || current.ContentSha256 == unit.ContentSha256 {
		t.Fatalf("hint edit did not create exact new version old=%+v new=%+v", unit, current)
	}
	fresh := create(current)
	freshUnit, _ := fresh.Unit.AsTrainingClozeUnit()
	if freshUnit.Blanks[0] != oldUnit.Blanks[0] || freshUnit.Questions[0].QuestionId != oldUnit.Questions[0].QuestionId {
		t.Fatal("semantic edit renamed stable question/blank identity")
	}
	for _, item := range []struct {
		session  tc.TrainingSession
		version  uuid.UUID
		expected string
	}{{old, unit.UnitVersionId, "观察空格后的 throwing，判断这里需要哪类连接短语。"}, {fresh, current.UnitVersionId, "新版本提示：结合全句判断替代关系。"}} {
		path := trainingTestPrefix + "/sessions/" + item.session.SessionId.String()
		s, b, _ = sendPortalPracticeCommand(t, newPortalPracticeCommandRequest(t, "POST", server.URL, path+"/hints", mustJSON(tc.TrainingHintRequest{UnitVersionId: item.version, HintId: oldUnit.Hints[0].HintId, Level: 1}), actor, "cloze-pinned-hint-"+uuid.NewString()))
		var hint tc.TrainingHintDisclosureEnvelope
		decodeJSON(t, b, &hint)
		if s != 200 || hint.Data.Text != item.expected {
			t.Fatalf("pinned hint %d %s", s, b)
		}
		s, b = sendCatalogRequest(t, trainingAccountRead(t, server.URL, path, actor))
		var recovered tc.TrainingSessionEnvelope
		decodeJSON(t, b, &recovered)
		safe, _ := recovered.Data.Unit.AsTrainingClozeUnit()
		if s != 200 || safe.UnitVersionId != item.version || !reflect.DeepEqual(safe.Parts, oldUnit.Parts) {
			t.Fatalf("old context replaced %d %s", s, b)
		}
	}
}
