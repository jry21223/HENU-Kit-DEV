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
	Type                 schemaType        `yaml:"type"`
	Format               string            `yaml:"format"`
	Pattern              string            `yaml:"pattern"`
	MaxLength            int               `yaml:"maxLength"`
	Required             []string          `yaml:"required"`
	Properties           map[string]schema `yaml:"properties"`
	AdditionalProperties *bool             `yaml:"additionalProperties"`
	Items                *schema           `yaml:"items"`
	Ref                  string            `yaml:"$ref"`
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
	OperationID string                `yaml:"operationId"`
	Security    []map[string][]string `yaml:"security"`
	Responses   map[string]response   `yaml:"responses"`
}

type response struct {
	Ref     string               `yaml:"$ref"`
	Content map[string]mediaType `yaml:"content"`
}

type mediaType struct {
	Schema schema `yaml:"schema"`
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
	spec, err := parseDocument(source)
	if err != nil {
		fail(err)
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
	validatePortalNoticeFeed(spec.Components.Schemas)
	if err := validatePortalNoticeOperation(spec.Paths); err != nil {
		fail(err)
	}

	digest := fmt.Sprintf("%x", sha256.Sum256(source))
	goSource, err := format.Source([]byte(renderGo(portalSession, digest)))
	if err != nil {
		fail(fmt.Errorf("format generated Go: %w", err))
	}
	write(*goOutput, goSource)
	write(*tsOutput, []byte(renderTypeScript(portalSession, digest)))
}

func parseDocument(source []byte) (document, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(source, &root); err != nil {
		return document{}, fmt.Errorf("parse OpenAPI: %w", err)
	}
	if err := rejectDuplicateMappingKeys(&root); err != nil {
		return document{}, err
	}
	var spec document
	if err := yaml.Unmarshal(source, &spec); err != nil {
		return document{}, fmt.Errorf("parse OpenAPI: %w", err)
	}
	return spec, nil
}

// rejectDuplicateMappingKeys runs on the YAML syntax tree before unmarshalling
// into maps. yaml.v3 otherwise accepts a repeated path key and silently keeps
// only the final operation, which could replace an authenticated route's
// contract during generation.
func rejectDuplicateMappingKeys(node *yaml.Node) error {
	switch node.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range node.Content {
			if err := rejectDuplicateMappingKeys(child); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := make(map[string]bool, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if seen[key.Value] {
				return fmt.Errorf("duplicate OpenAPI mapping key %q at line %d", key.Value, key.Line)
			}
			seen[key.Value] = true
			if err := rejectDuplicateMappingKeys(value); err != nil {
				return err
			}
		}
	}
	return nil
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

type PortalNoticeSource struct {
	Name string `+"`json:\"name\"`"+`
	URL  string `+"`json:\"url\"`"+`
}

type PortalNotice struct {
	ID        string             `+"`json:\"id\"`"+`
	Title     string             `+"`json:\"title\"`"+`
	Body      string             `+"`json:\"body\"`"+`
	Source    PortalNoticeSource `+"`json:\"source\"`"+`
	CreatedAt time.Time          `+"`json:\"created_at\"`"+`
}

type PortalNoticeFeed struct {
	Notices []PortalNotice `+"`json:\"notices\"`"+`
}

type NoticeSummary struct {
	ID          string    `+"`json:\"id\"`"+`
	Title       string    `+"`json:\"title\"`"+`
	Source      string    `+"`json:\"source\"`"+`
	PublishedAt time.Time `+"`json:\"published_at\"`"+`
}

type PortalNoticeFeedEnvelope struct {
	RequestID string           `+"`json:\"request_id\"`"+`
	Notices   []NoticeSummary  `+"`json:\"notices\"`"+`
	Data      PortalNoticeFeed `+"`json:\"data\"`"+`
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

export interface PortalNoticeSource {
  name: string;
  url: string;
}

export interface PortalNotice {
  id: string;
  title: string;
  body: string;
  source: PortalNoticeSource;
  created_at: string;
}

export interface PortalNoticeFeed {
  notices: PortalNotice[];
}

export interface NoticeSummary {
  id: string;
  title: string;
  source: string;
  published_at: string;
}

export interface PortalNoticeFeedEnvelope {
  request_id: string;
  notices: NoticeSummary[];
  data?: PortalNoticeFeed;
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

func validatePortalNoticeFeed(schemas map[string]schema) {
	requireObjectSchema(schemas, "NoticeListResponse", []string{"request_id", "notices"})
	envelope := schemas["NoticeListResponse"]
	if envelope.AdditionalProperties != nil && !*envelope.AdditionalProperties {
		fail(fmt.Errorf("NoticeListResponse must keep additive properties compatible with legacy clients"))
	}
	if notices := envelope.Properties["notices"]; !notices.Type.isExactly("array") || notices.Items == nil || notices.Items.Ref != "#/components/schemas/NoticeSummary" {
		fail(fmt.Errorf("NoticeListResponse.notices must retain NoticeSummary items"))
	}
	if data := envelope.Properties["data"]; data.Ref != "#/components/schemas/PortalNoticeFeed" || contains(envelope.Required, "data") {
		fail(fmt.Errorf("NoticeListResponse.data must add optional PortalNoticeFeed"))
	}
	requireObjectSchema(schemas, "NoticeSummary", []string{"id", "title", "source", "published_at"})
	summary := schemas["NoticeSummary"]
	for _, property := range []string{"id", "title", "source"} {
		if !summary.Properties[property].Type.isExactly("string") {
			fail(fmt.Errorf("NoticeSummary.%s must be string", property))
		}
	}
	if publishedAt := summary.Properties["published_at"]; !publishedAt.Type.isExactly("string") || publishedAt.Format != "date-time" {
		fail(fmt.Errorf("NoticeSummary.published_at must be date-time"))
	}
	requireObjectSchema(schemas, "PortalNoticeFeed", []string{"notices"})
	feed := schemas["PortalNoticeFeed"]
	if notices := feed.Properties["notices"]; !notices.Type.isExactly("array") || notices.Items == nil || notices.Items.Ref != "#/components/schemas/PortalNotice" {
		fail(fmt.Errorf("PortalNoticeFeed.notices must reference PortalNotice items"))
	}
	requireObjectSchema(schemas, "PortalNotice", []string{"id", "title", "body", "source", "created_at"})
	notice := schemas["PortalNotice"]
	for _, property := range []string{"id", "title", "body"} {
		if !notice.Properties[property].Type.isExactly("string") {
			fail(fmt.Errorf("PortalNotice.%s must be string", property))
		}
	}
	if notice.Properties["title"].MaxLength != 200 || notice.Properties["body"].MaxLength != 100000 {
		fail(fmt.Errorf("PortalNotice title/body bounds must be 200/100000"))
	}
	if source := notice.Properties["source"]; source.Ref != "#/components/schemas/PortalNoticeSource" {
		fail(fmt.Errorf("PortalNotice.source must reference PortalNoticeSource"))
	}
	if createdAt := notice.Properties["created_at"]; !createdAt.Type.isExactly("string") || createdAt.Format != "date-time" {
		fail(fmt.Errorf("PortalNotice.created_at must be date-time"))
	}
	requireObjectSchema(schemas, "PortalNoticeSource", []string{"name", "url"})
	source := schemas["PortalNoticeSource"]
	if err := validatePortalNoticeSourceSchema(source); err != nil {
		fail(err)
	}
}

func validatePortalNoticeSourceSchema(source schema) error {
	for _, property := range []string{"name", "url"} {
		if !source.Properties[property].Type.isExactly("string") {
			return fmt.Errorf("PortalNoticeSource.%s must be string", property)
		}
	}
	if source.Properties["name"].MaxLength != 120 || source.Properties["url"].Pattern != `^https://` {
		return fmt.Errorf("PortalNoticeSource bounds must be https and name <= 120")
	}
	if source.Properties["url"].MaxLength != 2048 {
		return fmt.Errorf("PortalNoticeSource.url maxLength must be 2048")
	}
	if source.Properties["url"].Format != "iri" {
		return fmt.Errorf("PortalNoticeSource.url must use IRI format")
	}
	return nil
}

func validatePortalNoticeOperation(paths map[string]pathItem) error {
	path, ok := paths["/api/v1/notices"]
	if !ok || path.Get == nil {
		return fmt.Errorf("/api/v1/notices GET operation is missing")
	}
	operation := path.Get
	if operation.OperationID != "getPortalNotices" {
		return fmt.Errorf("/api/v1/notices GET operationId must be getPortalNotices")
	}
	hasPortalSession := false
	if len(operation.Security) == 1 && len(operation.Security[0]) == 1 {
		_, hasPortalSession = operation.Security[0]["portalSession"]
	}
	if !hasPortalSession {
		return fmt.Errorf("/api/v1/notices GET must require portalSession")
	}
	response, ok := operation.Responses["200"]
	if !ok || response.Content["application/json"].Schema.Ref != "#/components/schemas/NoticeListResponse" {
		return fmt.Errorf("/api/v1/notices GET 200 must return NoticeListResponse")
	}
	for status, wantRef := range map[string]string{
		"401": "#/components/responses/Unauthorized",
		"403": "#/components/responses/Forbidden",
		"502": "#/components/responses/BadGateway",
		"503": "#/components/responses/ServiceUnavailable",
	} {
		if response, ok := operation.Responses[status]; !ok || response.Ref != wantRef {
			return fmt.Errorf("/api/v1/notices GET %s must reference %s", status, wantRef)
		}
	}
	return nil
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
