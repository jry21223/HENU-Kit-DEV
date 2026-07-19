package tests

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestQuizCraftImportContractExamples(t *testing.T) {
	loader := openapi3.NewLoader()
	document, err := loader.LoadFromFile("../../../../packages/api-contracts/openapi/quizcraft.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	operation := document.Paths.Find("/api/v1/workshop/banks/{bank_id}/imports").Post
	media := operation.RequestBody.Value.Content.Get("application/json")
	schema := media.Schema.Value
	valid := media.Examples["valid"].Value.Value
	if err := schema.VisitJSON(valid); err != nil {
		t.Fatalf("valid import example rejected: %v", err)
	}
	invalidExamples, ok := media.Extensions["x-invalid-examples"].([]any)
	if !ok || len(invalidExamples) == 0 {
		t.Fatalf("x-invalid-examples missing or malformed: %#v", media.Extensions["x-invalid-examples"])
	}
	for index, invalid := range invalidExamples {
		if err := schema.VisitJSON(invalid); err == nil {
			t.Fatalf("invalid import example %d unexpectedly accepted", index)
		}
	}
}
