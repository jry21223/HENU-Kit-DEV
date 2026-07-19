package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type operation struct{ ID, Method, Path, Constant string }

var required = []operation{
	{"noticeHealth", "GET", "/healthz", "HealthRoute"},
	{"getNoticeConsoleSummary", "GET", "/api/v1/console-summary", "ConsoleSummaryRoute"},
	{"listConsoleNotices", "GET", "/api/v1/console-notices", "SnapshotRoute"},
	{"createNoticeSource", "POST", "/api/v1/sources", "SourceCreateRoute"},
	{"createNoticeVersion", "POST", "/api/v1/sources/{source_id}/versions", "VersionCreateRoute"},
	{"reviewNoticeVersion", "POST", "/api/v1/versions/{version_id}/reviews", "ReviewRoute"},
	{"distributeNoticeVersion", "POST", "/api/v1/versions/{version_id}/distributions", "DistributionRoute"},
	{"getNoticeOperation", "GET", "/api/v1/operations/{operation}", "OperationRoute"},
}

func main() {
	contractPath := filepath.Clean("../../packages/api-contracts/openapi/notice.yaml")
	payload, err := os.ReadFile(contractPath)
	fail(err)
	loader := openapi3.NewLoader()
	document, err := loader.LoadFromFile(contractPath)
	fail(err)
	fail(document.Validate(context.Background()))
	requiredSecurity := []string{"serviceAuth", "serviceId", "keyId", "timestamp", "nonce", "signature"}
	if len(document.Security) != 1 {
		fail(errors.New("notice contract requires one AND-composed service security requirement"))
	}
	for _, name := range requiredSecurity {
		if document.Components.SecuritySchemes[name] == nil {
			fail(fmt.Errorf("missing service security scheme %s", name))
		}
		if _, ok := document.Security[0][name]; !ok {
			fail(fmt.Errorf("global service security requirement omits %s", name))
		}
	}
	for _, expected := range required {
		item := document.Paths.Find(expected.Path)
		if item == nil || item.GetOperation(expected.Method) == nil || item.GetOperation(expected.Method).OperationID != expected.ID {
			fail(fmt.Errorf("missing %s %s operationId %s", expected.Method, expected.Path, expected.ID))
		}
	}
	for _, name := range []string{"CreateSourceRequest", "CreateVersionRequest", "ReviewRequest", "DistributionRequest"} {
		schemaRef := document.Components.Schemas[name]
		if schemaRef == nil || schemaRef.Value == nil || schemaRef.Value.Example == nil {
			fail(fmt.Errorf("%s requires a valid example", name))
		}
		fail(schemaRef.Value.VisitJSON(schemaRef.Value.Example))
		invalid, ok := schemaRef.Value.Extensions["x-invalid-examples"].([]any)
		if !ok || len(invalid) == 0 {
			fail(fmt.Errorf("%s requires invalid examples", name))
		}
		for index, example := range invalid {
			if schemaRef.Value.VisitJSON(example) == nil {
				fail(fmt.Errorf("%s invalid example %d unexpectedly validates", name, index))
			}
		}
	}
	sort.Slice(required, func(i, j int) bool { return required[i].Constant < required[j].Constant })
	var constants strings.Builder
	for _, item := range required {
		fmt.Fprintf(&constants, "\t%s = %q\n", item.Constant, item.Path)
	}
	digest := sha256.Sum256(payload)
	generated := fmt.Sprintf("// Code generated from notice.yaml (SHA256 %x); DO NOT EDIT.\n\npackage contract\n\nconst (\n%s)\n", digest, constants.String())
	fail(os.MkdirAll("internal/contract", 0o755))
	formatted, err := format.Source([]byte(generated))
	fail(err)
	fail(os.WriteFile("internal/contract/generated.go", formatted, 0o644))
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
