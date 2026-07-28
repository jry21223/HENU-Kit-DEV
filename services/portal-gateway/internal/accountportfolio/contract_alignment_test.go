package accountportfolio

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The Account owner and public Gateway deliberately expose different security
// boundaries, but their successful payloads must remain identical. Neither
// side is generated from the other, so this test makes schema drift a CI
// failure before a hand-written client or runtime validator can go stale.
func TestOwnerAndGatewayAccountContractsStaySemanticallyAligned(t *testing.T) {
	owner := readAccountContract(t, "account-portfolio.yaml")
	portal := readAccountContract(t, "portal-gateway.yaml")

	for _, path := range []string{
		SummaryPath,
		PointsPath,
		MembershipPath,
		NotificationsPath,
		TicketsPath,
		MembershipOrdersPath,
	} {
		ownerSchema := successSchema(t, owner, path)
		portalSchema := successSchema(t, portal, path)
		assertSchemaEquivalent(t, owner, ownerSchema, portal, portalSchema, path)

		if _, ok := portal.Paths[path]["get"].Responses["502"]; !ok {
			t.Fatalf("%s public contract must document the Gateway's invalid-owner 502", path)
		}
	}
}

type accountContractDocument struct {
	Paths      map[string]map[string]accountContractOperation `yaml:"paths"`
	Components struct {
		Schemas map[string]accountContractSchema `yaml:"schemas"`
	} `yaml:"components"`
}

type accountContractOperation struct {
	Responses map[string]accountContractResponse `yaml:"responses"`
}

type accountContractResponse struct {
	Content map[string]accountContractMediaType `yaml:"content"`
}

type accountContractMediaType struct {
	Schema accountContractSchema `yaml:"schema"`
}

type accountContractSchema struct {
	Ref        string                           `yaml:"$ref"`
	Type       string                           `yaml:"type"`
	Format     string                           `yaml:"format"`
	Required   []string                         `yaml:"required"`
	Properties map[string]accountContractSchema `yaml:"properties"`
	Items      *accountContractSchema           `yaml:"items"`
	Enum       []string                         `yaml:"enum"`
	Minimum    *float64                         `yaml:"minimum"`
}

func readAccountContract(t *testing.T, name string) accountContractDocument {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate account contract test source")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../../../packages/api-contracts/openapi", name))
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var document accountContractDocument
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return document
}

func successSchema(t *testing.T, document accountContractDocument, path string) accountContractSchema {
	t.Helper()
	operation, ok := document.Paths[path]["get"]
	if !ok {
		t.Fatalf("%s GET operation is missing", path)
	}
	response, ok := operation.Responses["200"]
	if !ok {
		t.Fatalf("%s 200 response is missing", path)
	}
	media, ok := response.Content["application/json"]
	if !ok {
		t.Fatalf("%s 200 application/json schema is missing", path)
	}
	return media.Schema
}

func assertSchemaEquivalent(t *testing.T, leftDocument accountContractDocument, left accountContractSchema, rightDocument accountContractDocument, right accountContractSchema, location string) {
	t.Helper()
	left = resolveAccountContractSchema(t, leftDocument, left, location)
	right = resolveAccountContractSchema(t, rightDocument, right, location)
	if left.Type != right.Type || left.Format != right.Format || !sameStrings(left.Enum, right.Enum) || !sameMinimum(left.Minimum, right.Minimum) || !sameStrings(left.Required, right.Required) {
		t.Fatalf("%s schema metadata differs: owner=%+v gateway=%+v", location, left, right)
	}
	if len(left.Properties) != len(right.Properties) {
		t.Fatalf("%s property count differs: owner=%d gateway=%d", location, len(left.Properties), len(right.Properties))
	}
	for name, leftProperty := range left.Properties {
		rightProperty, ok := right.Properties[name]
		if !ok {
			t.Fatalf("%s owner property %q is missing from Gateway", location, name)
		}
		assertSchemaEquivalent(t, leftDocument, leftProperty, rightDocument, rightProperty, location+"."+name)
	}
	if (left.Items == nil) != (right.Items == nil) {
		t.Fatalf("%s item schema differs", location)
	}
	if left.Items != nil {
		assertSchemaEquivalent(t, leftDocument, *left.Items, rightDocument, *right.Items, location+"[]")
	}
}

func resolveAccountContractSchema(t *testing.T, document accountContractDocument, value accountContractSchema, location string) accountContractSchema {
	t.Helper()
	for value.Ref != "" {
		const prefix = "#/components/schemas/"
		if !strings.HasPrefix(value.Ref, prefix) {
			t.Fatalf("%s has unsupported schema reference %q", location, value.Ref)
		}
		name := strings.TrimPrefix(value.Ref, prefix)
		resolved, ok := document.Components.Schemas[name]
		if !ok {
			t.Fatalf("%s references absent schema %q", location, name)
		}
		value = resolved
	}
	return value
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameMinimum(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
