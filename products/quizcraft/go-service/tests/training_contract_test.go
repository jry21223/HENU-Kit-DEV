package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func trainingContract(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile("../../../../packages/api-contracts/openapi/quizcraft-training.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestTrainingSavedAnswerContractRejectsSharedReview(t *testing.T) {
	document := trainingContract(t)
	var receipt map[string]any
	if err := json.Unmarshal([]byte(`{"session_id":"10000000-0000-4000-8000-000000000001","unit_version_id":"10000000-0000-4000-8000-000000000002","question_id":"10000000-0000-4000-8000-000000000003","question_version_id":"10000000-0000-4000-8000-000000000004","answer":1,"correct":true,"assisted":false,"outcome":"independent_correct","review_required":false,"submitted_at":"2026-11-12T08:00:00Z","replayed":false,"saved_count":1,"total_count":2,"phase":"partial"}`), &receipt); err != nil {
		t.Fatal(err)
	}
	schema := document.Components.Schemas["TrainingAnswerReceipt"].Value
	if err := schema.VisitJSON(receipt); err != nil {
		t.Fatalf("saved answer receipt rejected: %v", err)
	}
	for _, field := range []string{"expected_answer", "analysis", "transcript", "evidence", "review", "layers"} {
		receipt[field] = "answer-bearing content"
		if err := schema.VisitJSON(receipt); err == nil {
			t.Errorf("saved answer receipt accepted shared %s", field)
		}
		delete(receipt, field)
	}
}

func TestTrainingImportUsesClosedSourceReferences(t *testing.T) {
	trainingContract(t)
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile("../../../../packages/api-contracts/openapi/quizcraft.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(`{
		"expected_version":1,"source_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"questions":[{"source_question_id":"q1","type":"single","chapter_id":"unit","chapter":"Listening","content":"What happened?","options":["A","B"],"answer":1,"analysis":"The speaker says B."}],
		"training":{"source_unit_id":"north-wind","kind":"listening","title":"North Wind",
		"attributions":[{"component":"audio","original":false,"source_url":"https://example.org/audio","license_id":"CC-BY-SA-3.0","license_url":"https://creativecommons.org/licenses/by-sa/3.0/","attribution":"Source reader","modifications":"Trimmed"}],
		"media":{"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","content_type":"audio/mpeg","duration_ms":2000},
		"segments":[{"segment_id":"sentence-a","start_ms":0,"end_ms":2000}],
		"question_reviews":[{"source_question_id":"q1","error_category":"detail","evidence":{"kind":"listening","segment_ids":["sentence-a"]}}],
		"layers":[{"kind":"listening","layer":"english","segments":[{"segment_id":"sentence-a","text":"The answer is B."}]},{"kind":"listening","layer":"chinese","segments":[{"segment_id":"sentence-a","text":"答案为 B。"}]}]}}
	`), &input); err != nil {
		t.Fatal(err)
	}
	schema := document.Paths.Find("/api/v1/workshop/banks/{bank_id}/imports").Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	if err := schema.VisitJSON(input); err != nil {
		t.Fatalf("source-ID training import rejected: %v", err)
	}
	question := input["questions"].([]any)[0].(map[string]any)
	question["answer"] = map[string]any{"correct": true}
	if err := schema.VisitJSON(input); err == nil {
		t.Fatal("training import accepted an arbitrary JSON answer")
	}
	question["answer"] = float64(1)
	input["training"].(map[string]any)["unit_version_id"] = "10000000-0000-4000-8000-000000000001"
	if err := schema.VisitJSON(input); err == nil {
		t.Fatal("source input accepted a client-selected generated version")
	}
}

func TestTrainingBrowserFacadeResolvesSharedClosedSchemas(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile("../../../../packages/api-contracts/openapi/portal-training.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/practice/training/catalog", "/api/v1/practice/training/review-items", "/api/v1/practice/training/sessions/{session_id}", "/api/v1/practice/training/sessions/{session_id}/review"} {
		if document.Paths.Find(path).Get == nil {
			t.Fatalf("bounded read %s missing", path)
		}
	}
	for _, name := range []string{"TrainingSessionEnvelope", "TrainingAnswerReceiptEnvelope", "TrainingReviewEnvelope"} {
		if err := document.Components.Schemas[name].Value.VisitJSON(map[string]any{"request_id": "req_training", "data": map[string]any{"transcript": "leaked"}}); err == nil {
			t.Errorf("%s accepted an untyped leaked payload", name)
		}
	}
}
