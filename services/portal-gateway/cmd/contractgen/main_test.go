package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readPortalGatewayContract(t *testing.T) []byte {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "packages", "api-contracts", "openapi", "portal-gateway.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestParseDocumentRejectsDuplicatePortalNoticePath(t *testing.T) {
	source := readPortalGatewayContract(t)
	duplicate := bytes.Replace(source, []byte("\n  /api/v1/account/summary:"), []byte(`
  /api/v1/notices:
    get:
      operationId: noticeList

  /api/v1/account/summary:`), 1)
	if bytes.Equal(source, duplicate) {
		t.Fatal("test fixture did not insert duplicate /api/v1/notices path")
	}
	_, err := parseDocument(duplicate)
	if err == nil || !strings.Contains(err.Error(), "duplicate OpenAPI mapping key \"/api/v1/notices\"") {
		t.Fatalf("duplicate path error = %v", err)
	}
}

func TestPortalNoticeOperationHasOnlyTheSafeFeedContract(t *testing.T) {
	spec, err := parseDocument(readPortalGatewayContract(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePortalNoticeOperation(spec.Paths); err != nil {
		t.Fatalf("safe Portal notice operation rejected: %v", err)
	}

	path := spec.Paths["/api/v1/notices"]
	legacy := *path.Get
	legacy.OperationID = "noticeList"
	path.Get = &legacy
	spec.Paths["/api/v1/notices"] = path
	if err := validatePortalNoticeOperation(spec.Paths); err == nil || !strings.Contains(err.Error(), "operationId") {
		t.Fatalf("legacy notice operation error = %v", err)
	}
}

func TestPortalNoticeCompatibilityEnvelopeRetainsLegacyTopLevelContract(t *testing.T) {
	spec, err := parseDocument(readPortalGatewayContract(t))
	if err != nil {
		t.Fatal(err)
	}
	envelope, ok := spec.Components.Schemas["NoticeListResponse"]
	if !ok || !contains(envelope.Required, "request_id") || !contains(envelope.Required, "notices") {
		t.Fatalf("NoticeListResponse must retain required request_id and notices: %#v", envelope)
	}
	if contains(envelope.Required, "data") {
		t.Fatal("NoticeListResponse.data must remain additive for legacy clients")
	}
	if notices := envelope.Properties["notices"]; !notices.Type.isExactly("array") || notices.Items == nil || notices.Items.Ref != "#/components/schemas/NoticeSummary" {
		t.Fatalf("NoticeListResponse.notices must retain NoticeSummary items: %#v", notices)
	}
	if data := envelope.Properties["data"]; data.Ref != "#/components/schemas/PortalNoticeFeed" {
		t.Fatalf("NoticeListResponse.data must add the rich PortalNoticeFeed: %#v", data)
	}
	summary, ok := spec.Components.Schemas["NoticeSummary"]
	if !ok || !contains(summary.Required, "id") || !contains(summary.Required, "title") || !contains(summary.Required, "source") || !contains(summary.Required, "published_at") {
		t.Fatalf("NoticeSummary legacy properties must remain required: %#v", summary)
	}
	if summary.Properties["source"].Type.isExactly("string") == false || summary.Properties["published_at"].Format != "date-time" {
		t.Fatalf("NoticeSummary legacy fields changed: %#v", summary)
	}
}

func TestPortalNoticeSourceContractRequiresBoundedIRIURL(t *testing.T) {
	spec, err := parseDocument(readPortalGatewayContract(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePortalNoticeSourceSchema(spec.Components.Schemas["PortalNoticeSource"]); err != nil {
		t.Fatalf("safe Portal notice source rejected: %v", err)
	}

	for _, testCase := range []struct {
		name string
		edit func(*schema)
		want string
	}{
		{
			name: "URL byte-bound declaration removed",
			edit: func(source *schema) {
				url := source.Properties["url"]
				url.MaxLength = 0
				source.Properties["url"] = url
			},
			want: "maxLength",
		},
		{
			name: "UTF-8 source URL contract reverted to URI",
			edit: func(source *schema) {
				url := source.Properties["url"]
				url.Format = "uri"
				source.Properties["url"] = url
			},
			want: "IRI",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			spec, err := parseDocument(readPortalGatewayContract(t))
			if err != nil {
				t.Fatal(err)
			}
			source := spec.Components.Schemas["PortalNoticeSource"]
			testCase.edit(&source)
			if err := validatePortalNoticeSourceSchema(source); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("source URL contract error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestLearningStateOperationRequiresSessionAndHonestOwnerFailures(t *testing.T) {
	spec, err := parseDocument(readPortalGatewayContract(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateLearningStateOperation(spec.Paths); err != nil {
		t.Fatalf("safe learning-state operation rejected: %v", err)
	}

	path := spec.Paths["/api/v1/learning-state"]
	unsafe := *path.Get
	unsafe.Security = nil
	path.Get = &unsafe
	spec.Paths["/api/v1/learning-state"] = path
	if err := validateLearningStateOperation(spec.Paths); err == nil || !strings.Contains(err.Error(), "portalSession") {
		t.Fatalf("unsigned learning-state operation error = %v", err)
	}

	path = spec.Paths["/api/v1/learning-state"]
	unpaged := *path.Get
	unpaged.Security = []map[string][]string{{"portalSession": {}}}
	unpaged.Parameters = nil
	path.Get = &unpaged
	spec.Paths["/api/v1/learning-state"] = path
	if err := validateLearningStateOperation(spec.Paths); err == nil || !strings.Contains(err.Error(), "page") {
		t.Fatalf("unpaginated learning-state operation error = %v", err)
	}

	path = spec.Paths["/api/v1/learning-state"]
	missingBadRequest := *path.Get
	missingBadRequest.Parameters = []parameter{{Ref: "#/components/parameters/LearningStatePageQuery"}, {Ref: "#/components/parameters/LearningStatePageSizeQuery"}, {Ref: "#/components/parameters/LearningStateWrongQuery"}}
	delete(missingBadRequest.Responses, "400")
	path.Get = &missingBadRequest
	spec.Paths["/api/v1/learning-state"] = path
	if err := validateLearningStateOperation(spec.Paths); err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("learning-state operation without 400 error = %v", err)
	}
}

func TestLearningStateSchemasContainOnlyCoreOwnedFacts(t *testing.T) {
	spec, err := parseDocument(readPortalGatewayContract(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateLearningStateSchemas(spec.Components.Schemas); err != nil {
		t.Fatalf("safe learning-state schemas rejected: %v", err)
	}

	item := spec.Components.Schemas["LearningStateItem"]
	item.Properties["question_content"] = schema{Type: schemaType{"string"}}
	spec.Components.Schemas["LearningStateItem"] = item
	if err := validateLearningStateSchemas(spec.Components.Schemas); err == nil || !strings.Contains(err.Error(), "additional properties") {
		t.Fatalf("fabricated question-content schema error = %v", err)
	}
}

func TestLearningStateSchemasAcceptOnlyStandardPaginatedCollection(t *testing.T) {
	spec, err := parseDocument(readPortalGatewayContract(t))
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	envelope := spec.Components.Schemas["LearningStateEnvelope"]
	envelope.Properties["data"] = schema{Ref: "#/components/schemas/LearningStatePage"}
	spec.Components.Schemas["LearningStateEnvelope"] = envelope
	spec.Components.Schemas["LearningStatePage"] = schema{
		Type:                 schemaType{"object"},
		AdditionalProperties: &closed,
		Required:             []string{"items", "pagination"},
		Properties: map[string]schema{
			"items":      {Type: schemaType{"array"}, Items: &schema{Ref: "#/components/schemas/LearningStateItem"}},
			"pagination": {Ref: "#/components/schemas/LearningStatePagination"},
		},
	}
	spec.Components.Schemas["LearningStatePagination"] = schema{
		Type:                 schemaType{"object"},
		AdditionalProperties: &closed,
		Required:             []string{"page", "page_size", "total", "total_pages"},
		Properties: map[string]schema{
			"page":        {Type: schemaType{"integer"}},
			"page_size":   {Type: schemaType{"integer"}},
			"total":       {Type: schemaType{"integer"}},
			"total_pages": {Type: schemaType{"integer"}},
		},
	}
	if err := validateLearningStateSchemas(spec.Components.Schemas); err != nil {
		t.Fatalf("standard paginated learning-state schemas rejected: %v", err)
	}
}
