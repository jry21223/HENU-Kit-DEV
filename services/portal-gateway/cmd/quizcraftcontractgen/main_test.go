package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLearningStateOperationRequiresStandardPagination(t *testing.T) {
	valid := operation{
		Parameters: []parameter{
			{Name: "X-Actor-User-Id", In: "header", Required: true, Schema: schema{Type: "string", Format: "uuid"}},
			{Ref: "#/components/parameters/LearningStatePageQuery"},
			{Ref: "#/components/parameters/LearningStatePageSizeQuery"},
			{Ref: "#/components/parameters/LearningStateWrongQuery"},
		},
		Responses: map[string]response{"400": {Ref: "#/components/responses/BadRequest"}},
	}
	if err := validateLearningStatePaginationOperation(valid); err != nil {
		t.Fatalf("standard owner pagination rejected: %v", err)
	}

	unpaged := valid
	unpaged.Parameters = nil
	if err := validateLearningStatePaginationOperation(unpaged); err == nil || !strings.Contains(err.Error(), "page") {
		t.Fatalf("unpaginated owner operation error = %v", err)
	}

	withoutBadRequest := valid
	withoutBadRequest.Responses = map[string]response{}
	if err := validateLearningStatePaginationOperation(withoutBadRequest); err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("owner operation without 400 error = %v", err)
	}
}

func TestLearningStateSchemaRequiresClosedEnvelope(t *testing.T) {
	if os.Getenv("QUIZCRAFT_CONTRACTGEN_OPEN_LEARNING_STATE_ENVELOPE") == "1" {
		schemas := validLearningStateSchemas()
		envelope := schemas["PortalLearningStateEnvelope"]
		envelope.AdditionalProperties = additionalProperties{}
		schemas["PortalLearningStateEnvelope"] = envelope
		validateLearningStateSchema(schemas)
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestLearningStateSchemaRequiresClosedEnvelope$")
	command.Env = append(os.Environ(), "QUIZCRAFT_CONTRACTGEN_OPEN_LEARNING_STATE_ENVELOPE=1")
	if err := command.Run(); err == nil {
		t.Fatal("validateLearningStateSchema accepted an open PortalLearningStateEnvelope")
	}
}

func TestLearningStateSchemaRequiresClosedItem(t *testing.T) {
	if os.Getenv("QUIZCRAFT_CONTRACTGEN_OPEN_LEARNING_STATE_ITEM") == "1" {
		schemas := validLearningStateSchemas()
		item := schemas["PortalLearningStateItem"]
		item.AdditionalProperties = additionalProperties{}
		schemas["PortalLearningStateItem"] = item
		validateLearningStateSchema(schemas)
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestLearningStateSchemaRequiresClosedItem$")
	command.Env = append(os.Environ(), "QUIZCRAFT_CONTRACTGEN_OPEN_LEARNING_STATE_ITEM=1")
	if err := command.Run(); err == nil {
		t.Fatal("validateLearningStateSchema accepted an open PortalLearningStateItem")
	}
}

func TestLearningStateSchemaAcceptsStandardPaginatedCollection(t *testing.T) {
	schemas := validLearningStateSchemas()
	envelope := schemas["PortalLearningStateEnvelope"]
	envelope.Properties["data"] = schema{Ref: "#/components/schemas/PortalLearningStatePage"}
	schemas["PortalLearningStateEnvelope"] = envelope
	closed := additionalProperties{specified: true, falseOnly: true}
	schemas["PortalLearningStatePage"] = schema{
		Type:                 "object",
		Required:             []string{"items", "pagination"},
		AdditionalProperties: closed,
		Properties: map[string]schema{
			"items":      {Type: "array", Items: &schema{Ref: "#/components/schemas/PortalLearningStateItem"}},
			"pagination": {Ref: "#/components/schemas/PortalLearningStatePagination"},
		},
	}
	schemas["PortalLearningStatePagination"] = schema{
		Type:                 "object",
		Required:             []string{"page", "page_size", "total", "total_pages"},
		AdditionalProperties: closed,
		Properties: map[string]schema{
			"page":        {Type: "integer"},
			"page_size":   {Type: "integer"},
			"total":       {Type: "integer"},
			"total_pages": {Type: "integer"},
		},
	}
	validateLearningStateSchema(schemas)
}

func validLearningStateSchemas() map[string]schema {
	closed := additionalProperties{specified: true, falseOnly: true}
	return map[string]schema{
		"PortalLearningStateEnvelope": {
			Type:                 "object",
			Required:             []string{"request_id", "data"},
			AdditionalProperties: closed,
			Properties: map[string]schema{
				"request_id": {Type: "string"},
				"data":       {Ref: "#/components/schemas/PortalLearningStatePage"},
			},
		},
		"PortalLearningStatePage": {
			Type:                 "object",
			Required:             []string{"items", "pagination"},
			AdditionalProperties: closed,
			Properties: map[string]schema{
				"items":      {Type: "array", Items: &schema{Ref: "#/components/schemas/PortalLearningStateItem"}},
				"pagination": {Ref: "#/components/schemas/PortalLearningStatePagination"},
			},
		},
		"PortalLearningStatePagination": {
			Type:                 "object",
			Required:             []string{"page", "page_size", "total", "total_pages"},
			AdditionalProperties: closed,
			Properties: map[string]schema{
				"page":        {Type: "integer"},
				"page_size":   {Type: "integer"},
				"total":       {Type: "integer"},
				"total_pages": {Type: "integer"},
			},
		},
		"PortalLearningStateItem": {
			Type:                 "object",
			Required:             []string{"bank_id", "question_id", "question_version_id", "wrong", "attempt_count", "correct_count", "updated_at"},
			AdditionalProperties: closed,
			Properties: map[string]schema{
				"bank_id":             {Type: "string"},
				"question_id":         {Type: "string"},
				"question_version_id": {Type: "string"},
				"wrong":               {Type: "boolean"},
				"attempt_count":       {Type: "integer"},
				"correct_count":       {Type: "integer"},
				"updated_at":          {Type: "string"},
			},
		},
	}
}
