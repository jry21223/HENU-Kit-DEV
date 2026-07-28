package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type operation struct {
	OperationID string                `yaml:"operationId"`
	Responses   map[string]response   `yaml:"responses"`
	Security    []map[string][]string `yaml:"security"`
}

type response struct {
	Content map[string]mediaType `yaml:"content"`
	Ref     string               `yaml:"$ref"`
}

type mediaType struct {
	Schema schema `yaml:"schema"`
}

type securityScheme struct {
	Type   string `yaml:"type"`
	Scheme string `yaml:"scheme"`
	In     string `yaml:"in"`
	Name   string `yaml:"name"`
}

type schemaType string

func (value *schemaType) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*value = schemaType(node.Value)
		return nil
	case yaml.SequenceNode:
		// Some unrelated schemas use a nullable union such as [string, 'null'].
		// This narrow generator validates only the catalog's scalar types, while
		// still parsing the complete source contract for its fingerprint.
		values := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			values = append(values, item.Value)
		}
		*value = schemaType(strings.Join(values, "|"))
		return nil
	default:
		return fmt.Errorf("unsupported schema type node kind %d", node.Kind)
	}
}

type schema struct {
	Type       schemaType        `yaml:"type"`
	Required   []string          `yaml:"required"`
	Properties map[string]schema `yaml:"properties"`
	Items      *schema           `yaml:"items"`
	Ref        string            `yaml:"$ref"`
}

type document struct {
	Paths      map[string]map[string]operation `yaml:"paths"`
	Components struct {
		Schemas         map[string]schema         `yaml:"schemas"`
		SecuritySchemes map[string]securityScheme `yaml:"securitySchemes"`
	} `yaml:"components"`
}

type catalogSecurityRequirement struct {
	name   string
	kind   string
	scheme string
	in     string
	header string
}

var catalogSecurityRequirements = []catalogSecurityRequirement{
	{name: "portalCatalogBasic", kind: "http", scheme: "basic"},
	{name: "portalCatalogSignature", kind: "apiKey", in: "header", header: "X-Signature"},
	{name: "portalCatalogPermission", kind: "apiKey", in: "header", header: "X-Permission-Code"},
	{name: "portalCatalogScope", kind: "apiKey", in: "header", header: "X-Scope-Kind"},
	{name: "portalCatalogProduct", kind: "apiKey", in: "header", header: "X-Product-Code"},
}

func main() {
	contractPath := flag.String("contract", "../../packages/api-contracts/openapi/quizcraft.yaml", "QuizCraft OpenAPI contract")
	outputPath := flag.String("output", "internal/practice/contract_generated.go", "generated Go output")
	flag.Parse()

	source, err := os.ReadFile(*contractPath)
	fail(err)
	var spec document
	fail(yaml.Unmarshal(source, &spec))

	catalogPath, catalogMethod, catalogOperation := requireOperation(spec.Paths, "listPracticeBanks")
	statsPath, statsMethod, statsOperation := requireOperation(spec.Paths, "getPersonalPracticeStats")
	validatePortalReadOperation("listPracticeBanks", catalogMethod, catalogOperation, "BankListEnvelope")
	validatePortalReadOperation("getPersonalPracticeStats", statsMethod, statsOperation, "PersonalPracticeStatsEnvelope")
	validateCatalogSecurity(spec.Components.SecuritySchemes)
	validateCatalogSchema(spec.Components.Schemas)
	validatePersonalStatsSchema(spec.Components.Schemas)

	digest := fmt.Sprintf("%x", sha256.Sum256(source))
	generated, err := format.Source([]byte(render(catalogPath, statsPath, digest)))
	fail(err)
	fail(os.WriteFile(*outputPath, generated, 0o644))
}

func requireOperation(paths map[string]map[string]operation, operationID string) (string, string, operation) {
	type candidate struct {
		path      string
		method    string
		operation operation
	}
	pathsWithOperation := make([]candidate, 0, 1)
	for path, methods := range paths {
		for method, operation := range methods {
			if operation.OperationID == operationID {
				pathsWithOperation = append(pathsWithOperation, candidate{path: path, method: method, operation: operation})
			}
		}
	}
	sort.Slice(pathsWithOperation, func(left, right int) bool {
		if pathsWithOperation[left].path == pathsWithOperation[right].path {
			return pathsWithOperation[left].method < pathsWithOperation[right].method
		}
		return pathsWithOperation[left].path < pathsWithOperation[right].path
	})
	if len(pathsWithOperation) != 1 {
		fail(fmt.Errorf("QuizCraft operation %s must occur exactly once, found %d", operationID, len(pathsWithOperation)))
	}
	return pathsWithOperation[0].path, pathsWithOperation[0].method, pathsWithOperation[0].operation
}

func validatePortalReadOperation(operationID, method string, operation operation, responseSchema string) {
	if method != "get" {
		fail(fmt.Errorf("%s must use GET, found %s", operationID, method))
	}
	response, ok := operation.Responses["200"]
	if !ok || response.Content["application/json"].Schema.Ref != "#/components/schemas/"+responseSchema {
		fail(fmt.Errorf("%s 200 response must be %s", operationID, responseSchema))
	}
	if conflict, ok := operation.Responses["409"]; !ok || conflict.Ref != "#/components/responses/ServiceReplay" {
		fail(fmt.Errorf("%s must document the service replay conflict", operationID))
	}
	if len(operation.Security) != 1 || len(operation.Security[0]) != len(catalogSecurityRequirements) {
		fail(fmt.Errorf("%s must require the complete Portal catalog security requirement", operationID))
	}
	for _, requirement := range catalogSecurityRequirements {
		if scopes, found := operation.Security[0][requirement.name]; !found || len(scopes) != 0 {
			fail(fmt.Errorf("%s is missing security scheme %s", operationID, requirement.name))
		}
	}
}

func validateCatalogSecurity(schemes map[string]securityScheme) {
	for _, requirement := range catalogSecurityRequirements {
		scheme, found := schemes[requirement.name]
		if !found || scheme.Type != requirement.kind || scheme.Scheme != requirement.scheme || scheme.In != requirement.in || scheme.Name != requirement.header {
			fail(fmt.Errorf("%s security scheme does not match the Portal catalog client", requirement.name))
		}
	}
}

func validateCatalogSchema(schemas map[string]schema) {
	requireObject(schemas, "BankListEnvelope", []string{"request_id", "data"})
	envelope := schemas["BankListEnvelope"]
	data, ok := envelope.Properties["data"]
	if !ok || data.Type != "array" || data.Items == nil || data.Items.Ref != "#/components/schemas/BankVersion" {
		fail(fmt.Errorf("BankListEnvelope.data must be an array of BankVersion"))
	}

	requireObject(schemas, "BankVersion", []string{"bank_id", "bank_version_id", "bank_key", "name", "content_sha256", "question_count", "chapters"})
	bank := schemas["BankVersion"]
	requireProperty(bank, "bank_id", "string")
	requireProperty(bank, "bank_version_id", "string")
	requireProperty(bank, "bank_key", "string")
	requireProperty(bank, "name", "string")
	requireProperty(bank, "content_sha256", "string")
	requireProperty(bank, "question_count", "integer")
	chapters, ok := bank.Properties["chapters"]
	if !ok || chapters.Type != "array" || chapters.Items == nil || chapters.Items.Ref != "#/components/schemas/Chapter" {
		fail(fmt.Errorf("BankVersion.chapters must be an array of Chapter"))
	}

	requireObject(schemas, "Chapter", []string{"id", "name"})
	chapter := schemas["Chapter"]
	requireProperty(chapter, "id", "string")
	requireProperty(chapter, "name", "string")
}

func validatePersonalStatsSchema(schemas map[string]schema) {
	requireObject(schemas, "PersonalPracticeStatsEnvelope", []string{"request_id", "data"})
	envelope := schemas["PersonalPracticeStatsEnvelope"]
	if data := envelope.Properties["data"]; data.Ref != "#/components/schemas/PersonalPracticeStats" {
		fail(fmt.Errorf("PersonalPracticeStatsEnvelope.data must be PersonalPracticeStats"))
	}

	requireObject(schemas, "PersonalPracticeStats", []string{"total_answers", "correct_answers", "accuracy", "streak_days", "mastery"})
	stats := schemas["PersonalPracticeStats"]
	for _, property := range []string{"total_answers", "correct_answers", "accuracy", "streak_days"} {
		requireProperty(stats, property, "integer")
	}
	mastery, ok := stats.Properties["mastery"]
	if !ok || mastery.Type != "array" || mastery.Items == nil || mastery.Items.Ref != "#/components/schemas/MasterySubject" {
		fail(fmt.Errorf("PersonalPracticeStats.mastery must be an array of MasterySubject"))
	}

	requireObject(schemas, "MasterySubject", []string{"bank_id", "label", "value", "total_questions", "correct_questions"})
	subject := schemas["MasterySubject"]
	requireProperty(subject, "bank_id", "string")
	requireProperty(subject, "label", "string")
	for _, property := range []string{"value", "total_questions", "correct_questions"} {
		requireProperty(subject, property, "integer")
	}
}

func requireObject(schemas map[string]schema, name string, required []string) {
	value, ok := schemas[name]
	if !ok || value.Type != "object" {
		fail(fmt.Errorf("%s must be an object schema", name))
	}
	for _, property := range required {
		if !contains(value.Required, property) || value.Properties[property].Type == "" && value.Properties[property].Ref == "" {
			fail(fmt.Errorf("%s.%s must be required", name, property))
		}
	}
}

func requireProperty(value schema, name, wantType string) {
	property, ok := value.Properties[name]
	if !ok || property.Type != schemaType(wantType) {
		fail(fmt.Errorf("catalog property %s must be %s", name, wantType))
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func render(catalogPath, statsPath, digest string) string {
	return fmt.Sprintf(`// Code generated by cmd/quizcraftcontractgen from quizcraft.yaml; DO NOT EDIT.
package practice

const QuizCraftCatalogContractSHA256 = %q
const ListPracticeBanksPath = %q
const GetPersonalPracticeStatsPath = %q

// BankListEnvelope is the generated read-only QuizCraft catalog response.
// Its data members are the published, and therefore available, bank versions.
type BankListEnvelope struct {
	RequestID string        `+"`json:\"request_id\"`"+`
	Data      []BankVersion `+"`json:\"data\"`"+`
}

// BankVersion is the stable published version of one QuizCraft bank.
type BankVersion struct {
	BankID        string    `+"`json:\"bank_id\"`"+`
	BankVersionID string    `+"`json:\"bank_version_id\"`"+`
	BankKey       string    `+"`json:\"bank_key\"`"+`
	Name          string    `+"`json:\"name\"`"+`
	ContentSHA256 string    `+"`json:\"content_sha256\"`"+`
	QuestionCount int       `+"`json:\"question_count\"`"+`
	Chapters      []Chapter `+"`json:\"chapters\"`"+`
}

// Chapter is a published QuizCraft bank chapter.
type Chapter struct {
	ID   string `+"`json:\"id\"`"+`
	Name string `+"`json:\"name\"`"+`
}

// PersonalPracticeStatsEnvelope is one authenticated Portal user's
// fact-derived QuizCraft statistics. It never represents a mock response.
type PersonalPracticeStatsEnvelope struct {
	RequestID string                `+"`json:\"request_id\"`"+`
	Data      PersonalPracticeStats `+"`json:\"data\"`"+`
}

type PersonalPracticeStats struct {
	TotalAnswers   int64            `+"`json:\"total_answers\"`"+`
	CorrectAnswers int64            `+"`json:\"correct_answers\"`"+`
	Accuracy       int              `+"`json:\"accuracy\"`"+`
	StreakDays     int              `+"`json:\"streak_days\"`"+`
	Mastery        []MasterySubject `+"`json:\"mastery\"`"+`
}

type MasterySubject struct {
	BankID           string `+"`json:\"bank_id\"`"+`
	Label            string `+"`json:\"label\"`"+`
	Value            int    `+"`json:\"value\"`"+`
	TotalQuestions   int64  `+"`json:\"total_questions\"`"+`
	CorrectQuestions int64  `+"`json:\"correct_questions\"`"+`
}
`, digest, catalogPath, statsPath)
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
