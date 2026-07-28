package accountportfolio

import (
	"fmt"
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
	assertPointsPaginationContract(t, owner, portal)
}

type accountContractDocument struct {
	Paths      map[string]map[string]accountContractOperation `yaml:"paths"`
	Components struct {
		Schemas map[string]accountContractSchema `yaml:"schemas"`
	} `yaml:"components"`
}

type accountContractOperation struct {
	Parameters []accountContractParameter         `yaml:"parameters"`
	Responses  map[string]accountContractResponse `yaml:"responses"`
}

type accountContractParameter struct {
	Ref      string                `yaml:"$ref"`
	Name     string                `yaml:"name"`
	In       string                `yaml:"in"`
	Required bool                  `yaml:"required"`
	Schema   accountContractSchema `yaml:"schema"`
}

type accountContractResponse struct {
	Content map[string]accountContractMediaType `yaml:"content"`
}

type accountContractMediaType struct {
	Schema accountContractSchema `yaml:"schema"`
}

type accountContractSchema struct {
	Ref                  string                           `yaml:"$ref"`
	Type                 accountContractType              `yaml:"type"`
	Format               string                           `yaml:"format"`
	Required             []string                         `yaml:"required"`
	Properties           map[string]accountContractSchema `yaml:"properties"`
	Items                *accountContractSchema           `yaml:"items"`
	OneOf                []accountContractSchema          `yaml:"oneOf"`
	Enum                 []string                         `yaml:"enum"`
	Minimum              *float64                         `yaml:"minimum"`
	Maximum              *float64                         `yaml:"maximum"`
	MinLength            *int                             `yaml:"minLength"`
	MaxLength            *int                             `yaml:"maxLength"`
	AdditionalProperties *bool                            `yaml:"additionalProperties"`
}

type accountContractType []string

func (value *accountContractType) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*value = accountContractType{node.Value}
		return nil
	case yaml.SequenceNode:
		result := make(accountContractType, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("OpenAPI schema type item must be a scalar")
			}
			result = append(result, item.Value)
		}
		*value = result
		return nil
	default:
		return fmt.Errorf("OpenAPI schema type must be a scalar or sequence")
	}
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
	if !sameStrings([]string(left.Type), []string(right.Type)) || left.Format != right.Format || !sameStrings(left.Enum, right.Enum) || !sameMinimum(left.Minimum, right.Minimum) || !sameMinimum(left.Maximum, right.Maximum) || !sameInt(left.MinLength, right.MinLength) || !sameInt(left.MaxLength, right.MaxLength) || !sameBool(left.AdditionalProperties, right.AdditionalProperties) || !sameStrings(left.Required, right.Required) {
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
	if len(left.OneOf) != len(right.OneOf) {
		t.Fatalf("%s oneOf alternative count differs: owner=%d gateway=%d", location, len(left.OneOf), len(right.OneOf))
	}
	for index := range left.OneOf {
		assertSchemaEquivalent(t, leftDocument, left.OneOf[index], rightDocument, right.OneOf[index], fmt.Sprintf("%s.oneOf[%d]", location, index))
	}
}

func assertPointsPaginationContract(t *testing.T, owner, portal accountContractDocument) {
	t.Helper()
	for _, name := range []string{"cursor", "limit"} {
		ownerParameter := queryParameter(t, owner, PointsPath, name)
		portalParameter := queryParameter(t, portal, PointsPath, name)
		if ownerParameter.Required != portalParameter.Required {
			t.Fatalf("%s query parameter required differs: owner=%t portal=%t", name, ownerParameter.Required, portalParameter.Required)
		}
		assertSchemaEquivalent(t, owner, ownerParameter.Schema, portal, portalParameter.Schema, PointsPath+"?"+name)
	}
	if _, ok := portal.Paths[PointsPath]["get"].Responses["400"]; !ok {
		t.Fatalf("%s public contract must document invalid pagination query as 400", PointsPath)
	}
}

func queryParameter(t *testing.T, document accountContractDocument, path, name string) accountContractParameter {
	t.Helper()
	operation, ok := document.Paths[path]["get"]
	if !ok {
		t.Fatalf("%s GET operation is missing", path)
	}
	for _, parameter := range operation.Parameters {
		if parameter.In == "query" && parameter.Name == name {
			return parameter
		}
	}
	t.Fatalf("%s query parameter %q is missing", path, name)
	return accountContractParameter{}
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

func sameInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameBool(left, right *bool) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
