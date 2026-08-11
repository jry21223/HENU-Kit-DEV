package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type schema struct {
	Type       schemaType        `yaml:"type"`
	Format     string            `yaml:"format"`
	Const      string            `yaml:"const"`
	Required   []string          `yaml:"required"`
	Properties map[string]schema `yaml:"properties"`
	Items      *schema           `yaml:"items"`
	Ref        string            `yaml:"$ref"`
}

// schemaType accepts the OpenAPI 3.1 scalar and union forms. This generator
// only emits PortalSession, but it must still parse the full public contract.
type schemaType []string

func (value *schemaType) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*value = schemaType{node.Value}
		return nil
	case yaml.SequenceNode:
		result := make(schemaType, 0, len(node.Content))
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

func (value schemaType) isExactly(want string) bool {
	return len(value) == 1 && value[0] == want
}

type document struct {
	Paths      map[string]pathItem `yaml:"paths"`
	Components struct {
		Schemas map[string]schema `yaml:"schemas"`
	} `yaml:"components"`
}

type pathItem struct {
	Get *operation `yaml:"get"`
}

type operation struct {
	Security  []map[string][]string `yaml:"security"`
	Responses map[string]response   `yaml:"responses"`
}

type response struct {
	Ref     string            `yaml:"$ref"`
	Headers map[string]header `yaml:"headers"`
}

type header struct {
	Required bool   `yaml:"required"`
	Schema   schema `yaml:"schema"`
}

func main() {
	contractPath := flag.String("contract", "../../packages/api-contracts/openapi/portal-gateway.yaml", "OpenAPI contract")
	goOutput := flag.String("go-output", "internal/contract/portal_session.generated.go", "generated Go output")
	tsOutput := flag.String("ts-output", "../../apps/portal/src/lib/api/portal-session.generated.ts", "generated TypeScript output")
	flag.Parse()

	source, err := os.ReadFile(*contractPath)
	if err != nil {
		fail(err)
	}
	var spec document
	if err := yaml.Unmarshal(source, &spec); err != nil {
		fail(fmt.Errorf("parse OpenAPI: %w", err))
	}
	portalSession, ok := spec.Components.Schemas["PortalSession"]
	if !ok || !portalSession.Type.isExactly("object") {
		fail(fmt.Errorf("PortalSession object schema is missing"))
	}
	for _, property := range []string{"user_id", "display_name", "expires_at"} {
		if _, exists := portalSession.Properties[property]; !exists {
			fail(fmt.Errorf("PortalSession.%s is missing", property))
		}
	}
	validatePersonalPracticeStats(spec.Components.Schemas)
	validateLibraryDownloadFacade(spec.Paths)

	digest := fmt.Sprintf("%x", sha256.Sum256(source))
	goSource, err := format.Source([]byte(renderGo(portalSession, digest)))
	if err != nil {
		fail(fmt.Errorf("format generated Go: %w", err))
	}
	write(*goOutput, goSource)
	write(*tsOutput, []byte(renderTypeScript(portalSession, digest)))
}

func validateLibraryDownloadFacade(paths map[string]pathItem) {
	const route = "/api/v1/library/materials/{material_id}/download"
	operation := paths[route].Get
	if operation == nil {
		fail(fmt.Errorf("Library download facade GET %s is missing", route))
	}
	if operation.Security == nil || len(operation.Security) != 0 {
		fail(fmt.Errorf("Library download facade must be explicitly anonymous"))
	}
	for _, status := range []string{"303", "404", "503"} {
		if _, ok := operation.Responses[status]; !ok {
			fail(fmt.Errorf("Library download facade response %s is missing", status))
		}
	}
	if operation.Responses["404"].Ref != "#/components/responses/MaterialNotAvailable" || operation.Responses["503"].Ref != "#/components/responses/DownloadTemporarilyUnavailable" {
		fail(fmt.Errorf("Library download facade must use its frozen 404 and 503 responses"))
	}
	redirect := operation.Responses["303"]
	for name, expected := range map[string]string{
		"Cache-Control":          "no-store",
		"Referrer-Policy":        "no-referrer",
		"X-Content-Type-Options": "nosniff",
	} {
		value, ok := redirect.Headers[name]
		if !ok || !value.Required || value.Schema.Const != expected {
			fail(fmt.Errorf("Library download facade 303 header %s must be required const %q", name, expected))
		}
	}
	location, ok := redirect.Headers["Location"]
	if !ok || !location.Required || !location.Schema.Type.isExactly("string") || location.Schema.Format != "uri" {
		fail(fmt.Errorf("Library download facade 303 Location must be a required URI string"))
	}
}

func renderGo(value schema, digest string) string {
	required := stringSet(value.Required)
	var fields strings.Builder
	for _, property := range sortedProperties(value.Properties) {
		fieldType := goType(value.Properties[property])
		tag := property
		if !required[property] {
			fieldType = "*" + fieldType
			tag += ",omitempty"
		}
		fmt.Fprintf(&fields, "\t%s %s `json:\"%s\"`\n", goName(property), fieldType, tag)
	}
	return fmt.Sprintf(`// Code generated by cmd/contractgen from portal-gateway.yaml; DO NOT EDIT.
package contract

import "time"

const PortalSessionSourceSHA256 = %q
const LibraryDownloadRoute = "/api/v1/library/materials/{material_id}/download"

type PortalSession struct {
%s}

type MasterySubject struct {
	BankID           string `+"`json:\"bank_id\"`"+`
	Label            string `+"`json:\"label\"`"+`
	Value            int    `+"`json:\"value\"`"+`
	TotalQuestions   int64  `+"`json:\"total_questions\"`"+`
	CorrectQuestions int64  `+"`json:\"correct_questions\"`"+`
}

type PersonalPracticeStats struct {
	TotalAnswers   int64            `+"`json:\"total_answers\"`"+`
	CorrectAnswers int64            `+"`json:\"correct_answers\"`"+`
	Accuracy       int              `+"`json:\"accuracy\"`"+`
	StreakDays     int              `+"`json:\"streak_days\"`"+`
	Mastery        []MasterySubject `+"`json:\"mastery\"`"+`
}

type PersonalPracticeStatsEnvelope struct {
	RequestID string                `+"`json:\"request_id\"`"+`
	Data      PersonalPracticeStats `+"`json:\"data\"`"+`
}
`, digest, fields.String())
}

func renderTypeScript(value schema, digest string) string {
	required := stringSet(value.Required)
	var fields strings.Builder
	for _, property := range sortedProperties(value.Properties) {
		optional := ""
		if !required[property] {
			optional = "?"
		}
		fmt.Fprintf(&fields, "  %s%s: %s;\n", property, optional, tsType(value.Properties[property]))
	}
	return fmt.Sprintf(`// Code generated by portal-gateway/cmd/contractgen from portal-gateway.yaml (SHA256 %s); DO NOT EDIT.
export interface PortalSession {
%s}

export interface MasterySubject {
  bank_id: string;
  label: string;
  value: number;
  total_questions: number;
  correct_questions: number;
}

export interface PersonalPracticeStats {
  total_answers: number;
  correct_answers: number;
  accuracy: number;
  streak_days: number;
  mastery: MasterySubject[];
}

export interface PersonalPracticeStatsEnvelope {
  request_id: string;
  data: PersonalPracticeStats;
}
`, digest, fields.String())
}

func validatePersonalPracticeStats(schemas map[string]schema) {
	requireObjectSchema(schemas, "PersonalPracticeStatsEnvelope", []string{"request_id", "data"})
	envelope := schemas["PersonalPracticeStatsEnvelope"]
	if data := envelope.Properties["data"]; data.Ref != "#/components/schemas/PersonalPracticeStats" {
		fail(fmt.Errorf("PersonalPracticeStatsEnvelope.data must reference PersonalPracticeStats"))
	}

	requireObjectSchema(schemas, "PersonalPracticeStats", []string{"total_answers", "correct_answers", "accuracy", "streak_days", "mastery"})
	stats := schemas["PersonalPracticeStats"]
	for _, property := range []string{"total_answers", "correct_answers", "accuracy", "streak_days"} {
		if !stats.Properties[property].Type.isExactly("integer") {
			fail(fmt.Errorf("PersonalPracticeStats.%s must be integer", property))
		}
	}
	if mastery := stats.Properties["mastery"]; !mastery.Type.isExactly("array") || mastery.Items == nil || mastery.Items.Ref != "#/components/schemas/MasterySubject" {
		fail(fmt.Errorf("PersonalPracticeStats.mastery must reference MasterySubject items"))
	}

	requireObjectSchema(schemas, "MasterySubject", []string{"bank_id", "label", "value", "total_questions", "correct_questions"})
	subject := schemas["MasterySubject"]
	for _, property := range []string{"bank_id", "label"} {
		if !subject.Properties[property].Type.isExactly("string") {
			fail(fmt.Errorf("MasterySubject.%s must be string", property))
		}
	}
	for _, property := range []string{"value", "total_questions", "correct_questions"} {
		if !subject.Properties[property].Type.isExactly("integer") {
			fail(fmt.Errorf("MasterySubject.%s must be integer", property))
		}
	}
}

func requireObjectSchema(schemas map[string]schema, name string, required []string) {
	value, ok := schemas[name]
	if !ok || !value.Type.isExactly("object") {
		fail(fmt.Errorf("%s object schema is missing", name))
	}
	for _, property := range required {
		if !contains(value.Required, property) || (len(value.Properties[property].Type) == 0 && value.Properties[property].Ref == "") {
			fail(fmt.Errorf("%s.%s is missing or not required", name, property))
		}
	}
}

func goType(value schema) string {
	if value.Type.isExactly("string") && value.Format == "date-time" {
		return "time.Time"
	}
	if value.Type.isExactly("string") {
		return "string"
	}
	fail(fmt.Errorf("unsupported Go schema type %q format %q", []string(value.Type), value.Format))
	return ""
}

func tsType(value schema) string {
	if value.Type.isExactly("string") {
		return "string"
	}
	fail(fmt.Errorf("unsupported TypeScript schema type %q", []string(value.Type)))
	return ""
}

func sortedProperties(properties map[string]schema) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func goName(value string) string {
	var output []rune
	upper := true
	for _, char := range value {
		if char == '_' || char == '-' {
			upper = true
			continue
		}
		if upper {
			char = unicode.ToUpper(char)
			upper = false
		}
		output = append(output, char)
	}
	return strings.ReplaceAll(string(output), "Id", "ID")
}

func write(path string, content []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
