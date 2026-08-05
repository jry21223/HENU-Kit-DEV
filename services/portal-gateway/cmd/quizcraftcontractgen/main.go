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
	Internal    bool                  `yaml:"x-internal"`
	Parameters  []parameter           `yaml:"parameters"`
}

type parameter struct {
	Name     string `yaml:"name"`
	In       string `yaml:"in"`
	Required bool   `yaml:"required"`
	Schema   schema `yaml:"schema"`
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

type additionalProperties struct {
	specified bool
	falseOnly bool
}

func (value *additionalProperties) UnmarshalYAML(node *yaml.Node) error {
	value.specified = true
	value.falseOnly = node.Kind == yaml.ScalarNode && node.Tag == "!!bool" && node.Value == "false"
	return nil
}

type schema struct {
	Type                 schemaType           `yaml:"type"`
	Format               string               `yaml:"format"`
	Required             []string             `yaml:"required"`
	Properties           map[string]schema    `yaml:"properties"`
	Items                *schema              `yaml:"items"`
	Ref                  string               `yaml:"$ref"`
	Enum                 []string             `yaml:"enum"`
	Const                string               `yaml:"const"`
	AdditionalProperties additionalProperties `yaml:"additionalProperties"`
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

var portalPracticeCommandSecurityRequirements = []catalogSecurityRequirement{
	{name: "portalPracticeBasic", kind: "http", scheme: "basic"},
	{name: "portalPracticeSignature", kind: "apiKey", in: "header", header: "X-Signature"},
}

var personalStatsSecurityRequirements = append(
	append([]catalogSecurityRequirement{}, catalogSecurityRequirements...),
	catalogSecurityRequirement{name: "portalCatalogActor", kind: "apiKey", in: "header", header: "X-Actor-User-Id"},
)

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
	overallRankingPath, overallRankingMethod, overallRankingOperation := requireOperation(spec.Paths, "getOverallRanking")
	bankRankingPath, bankRankingMethod, bankRankingOperation := requireOperation(spec.Paths, "getBankRanking")
	createPortalPracticeSessionPath, createPortalPracticeSessionMethod, createPortalPracticeSessionOperation := requireOperation(spec.Paths, "createPortalPracticeSession")
	submitPortalPracticeAnswerPath, submitPortalPracticeAnswerMethod, submitPortalPracticeAnswerOperation := requireOperation(spec.Paths, "submitPortalPracticeAnswer")
	createPortalPracticeFeedbackPath, createPortalPracticeFeedbackMethod, createPortalPracticeFeedbackOperation := requireOperation(spec.Paths, "createPortalPracticeFeedback")
	getPortalPracticeFeedbackStatusPath, getPortalPracticeFeedbackStatusMethod, getPortalPracticeFeedbackStatusOperation := requireOperation(spec.Paths, "getPortalPracticeFeedbackStatus")
	getPortalFavoritesOverviewPath, getPortalFavoritesOverviewMethod, getPortalFavoritesOverviewOperation := requireOperation(spec.Paths, "getPortalFavoritesOverview")
	listPortalFavoriteQuestionsPath, listPortalFavoriteQuestionsMethod, listPortalFavoriteQuestionsOperation := requireOperation(spec.Paths, "listPortalFavoriteQuestions")
	favoritePortalQuestionPath, favoritePortalQuestionMethod, favoritePortalQuestionOperation := requireOperation(spec.Paths, "favoritePortalQuestion")
	unfavoritePortalQuestionPath, unfavoritePortalQuestionMethod, unfavoritePortalQuestionOperation := requireOperation(spec.Paths, "unfavoritePortalQuestion")
	createPortalFavoritesSessionPath, createPortalFavoritesSessionMethod, createPortalFavoritesSessionOperation := requireOperation(spec.Paths, "createPortalFavoritesSession")
	validatePortalReadOperation("listPracticeBanks", catalogMethod, catalogOperation, "BankListEnvelope", catalogSecurityRequirements)
	validatePortalReadOperation("getPersonalPracticeStats", statsMethod, statsOperation, "PersonalPracticeStatsEnvelope", personalStatsSecurityRequirements)
	validatePersonalStatsActorBinding(statsOperation)
	validatePortalReadOperation("getOverallRanking", overallRankingMethod, overallRankingOperation, "RankingEnvelope", catalogSecurityRequirements)
	validatePortalReadOperation("getBankRanking", bankRankingMethod, bankRankingOperation, "RankingEnvelope", catalogSecurityRequirements)
	validatePortalPracticeCommandOperation("createPortalPracticeSession", createPortalPracticeSessionPath, createPortalPracticeSessionMethod, createPortalPracticeSessionOperation, "201", "PracticeSessionEnvelope")
	validatePortalPracticeCommandOperation("submitPortalPracticeAnswer", submitPortalPracticeAnswerPath, submitPortalPracticeAnswerMethod, submitPortalPracticeAnswerOperation, "200", "AnswerResultEnvelope")
	validatePortalPracticeCommandOperation("createPortalPracticeFeedback", createPortalPracticeFeedbackPath, createPortalPracticeFeedbackMethod, createPortalPracticeFeedbackOperation, "202", "OperationEnvelope")
	validatePortalReadOperation("getPortalPracticeFeedbackStatus", getPortalPracticeFeedbackStatusMethod, getPortalPracticeFeedbackStatusOperation, "FeedbackStatusEnvelope", personalStatsSecurityRequirements)
	validatePersonalStatsActorBinding(getPortalPracticeFeedbackStatusOperation)
	validatePortalReadOperation("getPortalFavoritesOverview", getPortalFavoritesOverviewMethod, getPortalFavoritesOverviewOperation, "FavoritesOverviewEnvelope", personalStatsSecurityRequirements)
	validatePersonalStatsActorBinding(getPortalFavoritesOverviewOperation)
	validatePortalReadOperation("listPortalFavoriteQuestions", listPortalFavoriteQuestionsMethod, listPortalFavoriteQuestionsOperation, "FavoriteListEnvelope", personalStatsSecurityRequirements)
	validatePersonalStatsActorBinding(listPortalFavoriteQuestionsOperation)
	validatePortalPracticeCommandOperation("favoritePortalQuestion", favoritePortalQuestionPath, favoritePortalQuestionMethod, favoritePortalQuestionOperation, "200", "OperationEnvelope")
	validatePortalPracticeCommandOperation("unfavoritePortalQuestion", unfavoritePortalQuestionPath, unfavoritePortalQuestionMethod, unfavoritePortalQuestionOperation, "200", "OperationEnvelope")
	validatePortalPracticeCommandOperation("createPortalFavoritesSession", createPortalFavoritesSessionPath, createPortalFavoritesSessionMethod, createPortalFavoritesSessionOperation, "201", "PracticeSessionEnvelope")
	validateCatalogSecurity(spec.Components.SecuritySchemes, personalStatsSecurityRequirements)
	validatePortalPracticeCommandSecurity(spec.Components.SecuritySchemes)
	validateCatalogSchema(spec.Components.Schemas)
	validateRankingSchema(spec.Components.Schemas)
	validatePersonalStatsSchema(spec.Components.Schemas)
	validateFeedbackStatusSchema(spec.Components.Schemas)

	digest := fmt.Sprintf("%x", sha256.Sum256(source))
	generated, err := format.Source([]byte(render(catalogPath, statsPath, overallRankingPath, bankRankingPath, createPortalPracticeSessionPath, submitPortalPracticeAnswerPath, createPortalPracticeFeedbackPath, getPortalPracticeFeedbackStatusPath, getPortalFavoritesOverviewPath, listPortalFavoriteQuestionsPath, favoritePortalQuestionPath, unfavoritePortalQuestionPath, createPortalFavoritesSessionPath, digest)))
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

func validatePortalReadOperation(operationID, method string, operation operation, responseSchema string, requirements []catalogSecurityRequirement) {
	if method != "get" {
		fail(fmt.Errorf("%s must use GET, found %s", operationID, method))
	}
	response, ok := operation.Responses["200"]
	if !ok || response.Content["application/json"].Schema.Ref != "#/components/schemas/"+responseSchema {
		fail(fmt.Errorf("%s 200 response must be %s", operationID, responseSchema))
	}
	if unauthorized, ok := operation.Responses["401"]; !ok || unauthorized.Ref != "#/components/responses/Unauthorized" {
		fail(fmt.Errorf("%s must document unauthorized Portal reads", operationID))
	}
	if forbidden, ok := operation.Responses["403"]; !ok || forbidden.Ref != "#/components/responses/Forbidden" {
		fail(fmt.Errorf("%s must document forbidden Portal reads", operationID))
	}
	if conflict, ok := operation.Responses["409"]; !ok || conflict.Ref != "#/components/responses/ServiceReplay" {
		fail(fmt.Errorf("%s must document the service replay conflict", operationID))
	}
	if len(operation.Security) != 1 || len(operation.Security[0]) != len(requirements) {
		fail(fmt.Errorf("%s must require the complete Portal read security requirement", operationID))
	}
	for _, requirement := range requirements {
		if scopes, found := operation.Security[0][requirement.name]; !found || len(scopes) != 0 {
			fail(fmt.Errorf("%s is missing security scheme %s", operationID, requirement.name))
		}
	}
}

func validatePortalPracticeCommandOperation(operationID, path, method string, operation operation, successStatus, envelope string) {
	if (method != "post" && method != "put" && method != "delete") || !operation.Internal || !strings.HasPrefix(path, "/api/v1/portal/practice/") {
		fail(fmt.Errorf("%s must be an internal Portal practice command (%s)", operationID, method))
	}
	response, ok := operation.Responses[successStatus]
	if !ok || response.Content["application/json"].Schema.Ref != "#/components/schemas/"+envelope {
		fail(fmt.Errorf("%s %s response must be %s", operationID, successStatus, envelope))
	}
	for _, status := range []string{"400", "401", "409", "503"} {
		if _, ok := operation.Responses[status]; !ok {
			fail(fmt.Errorf("%s must document %s", operationID, status))
		}
	}
	if operationID == "submitPortalPracticeAnswer" {
		for _, status := range []string{"403", "404"} {
			if _, ok := operation.Responses[status]; !ok {
				fail(fmt.Errorf("%s must document %s", operationID, status))
			}
		}
	}
	if len(operation.Security) != 1 || len(operation.Security[0]) != len(portalPracticeCommandSecurityRequirements) {
		fail(fmt.Errorf("%s must require the dedicated Portal practice command security", operationID))
	}
	for _, requirement := range portalPracticeCommandSecurityRequirements {
		if scopes, found := operation.Security[0][requirement.name]; !found || len(scopes) != 0 {
			fail(fmt.Errorf("%s is missing security scheme %s", operationID, requirement.name))
		}
	}
}

func validatePersonalStatsActorBinding(operation operation) {
	for _, parameter := range operation.Parameters {
		if parameter.Name == "X-Actor-User-Id" && parameter.In == "header" && parameter.Required && parameter.Schema.Type == "string" && parameter.Schema.Format == "uuid" {
			return
		}
	}
	fail(fmt.Errorf("getPersonalPracticeStats must require UUID X-Actor-User-Id header binding"))
}

func validateCatalogSecurity(schemes map[string]securityScheme, requirements []catalogSecurityRequirement) {
	for _, requirement := range requirements {
		scheme, found := schemes[requirement.name]
		if !found || scheme.Type != requirement.kind || scheme.Scheme != requirement.scheme || scheme.In != requirement.in || scheme.Name != requirement.header {
			fail(fmt.Errorf("%s security scheme does not match the Portal read client", requirement.name))
		}
	}
}

func validatePortalPracticeCommandSecurity(schemes map[string]securityScheme) {
	for _, requirement := range portalPracticeCommandSecurityRequirements {
		scheme, found := schemes[requirement.name]
		if !found || scheme.Type != requirement.kind || scheme.Scheme != requirement.scheme || scheme.In != requirement.in || scheme.Name != requirement.header {
			fail(fmt.Errorf("%s security scheme does not match the Portal practice command client", requirement.name))
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

func validateRankingSchema(schemas map[string]schema) {
	requireObject(schemas, "RankingEnvelope", []string{"request_id", "data"})
	envelope := schemas["RankingEnvelope"]
	requireClosedObject(envelope, "RankingEnvelope")
	if data, ok := envelope.Properties["data"]; !ok || data.Ref != "#/components/schemas/RankingPage" {
		fail(fmt.Errorf("RankingEnvelope.data must reference RankingPage"))
	}

	requireObject(schemas, "RankingPage", []string{"scope", "period", "metric", "entries"})
	page := schemas["RankingPage"]
	requireClosedObject(page, "RankingPage")
	requireProperty(page, "scope", "string")
	if period, ok := page.Properties["period"]; !ok || period.Ref != "#/components/schemas/RankingPeriod" {
		fail(fmt.Errorf("RankingPage.period must reference RankingPeriod"))
	}
	requireProperty(page, "metric", "string")
	if page.Properties["metric"].Const != "correct_answer_count" {
		fail(fmt.Errorf("RankingPage.metric must be correct_answer_count"))
	}
	entries, ok := page.Properties["entries"]
	if !ok || entries.Type != "array" || entries.Items == nil || entries.Items.Type != "object" {
		fail(fmt.Errorf("RankingPage.entries must be an array of public ranking entries"))
	}
	requireSchemaObject(*entries.Items, "RankingPage.entries[]", []string{"rank", "nickname", "system_avatar", "correct_answer_count"})
	requireClosedObject(*entries.Items, "RankingPage.entries[]")
	requireProperty(*entries.Items, "rank", "integer")
	requireProperty(*entries.Items, "nickname", "string")
	requireProperty(*entries.Items, "system_avatar", "string")
	requireProperty(*entries.Items, "correct_answer_count", "integer")

	period, ok := schemas["RankingPeriod"]
	if !ok || period.Type != "string" || !contains(period.Enum, "weekly") || !contains(period.Enum, "lifetime") {
		fail(fmt.Errorf("RankingPeriod must be a weekly/lifetime string enum"))
	}
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

func validateFeedbackStatusSchema(schemas map[string]schema) {
	requireObject(schemas, "FeedbackStatusEnvelope", []string{"request_id", "data"})
	envelope := schemas["FeedbackStatusEnvelope"]
	requireClosedObject(envelope, "FeedbackStatusEnvelope")
	if data := envelope.Properties["data"]; data.Ref != "#/components/schemas/FeedbackStatus" {
		fail(fmt.Errorf("FeedbackStatusEnvelope.data must be FeedbackStatus"))
	}

	requireObject(schemas, "FeedbackStatus", []string{"feedback_id", "bank_id", "question_id", "question_version_id", "category", "status", "created_at", "updated_at"})
	feedback := schemas["FeedbackStatus"]
	requireClosedObject(feedback, "FeedbackStatus")
	for _, property := range []string{"feedback_id", "bank_id", "question_id", "question_version_id", "category", "status"} {
		requireProperty(feedback, property, "string")
	}
	if category, ok := feedback.Properties["category"]; !ok || !contains(category.Enum, "wrong_answer") || !contains(category.Enum, "ambiguous") || !contains(category.Enum, "typo") || !contains(category.Enum, "outdated") || !contains(category.Enum, "other") {
		fail(fmt.Errorf("FeedbackStatus.category must be the QuestionFeedback category enum"))
	}
	if status, ok := feedback.Properties["status"]; !ok || !contains(status.Enum, "pending") || !contains(status.Enum, "in_progress") || !contains(status.Enum, "blocked") || !contains(status.Enum, "resolved") || !contains(status.Enum, "archived") {
		fail(fmt.Errorf("FeedbackStatus.status must be the processing status enum"))
	}
}

func requireObject(schemas map[string]schema, name string, required []string) {
	value, ok := schemas[name]
	if !ok {
		fail(fmt.Errorf("%s must be an object schema", name))
	}
	requireSchemaObject(value, name, required)
}

func requireSchemaObject(value schema, name string, required []string) {
	if value.Type != "object" {
		fail(fmt.Errorf("%s must be an object schema", name))
	}
	for _, property := range required {
		if !contains(value.Required, property) || value.Properties[property].Type == "" && value.Properties[property].Ref == "" {
			fail(fmt.Errorf("%s.%s must be required", name, property))
		}
	}
}

func requireClosedObject(value schema, name string) {
	if !value.AdditionalProperties.specified || !value.AdditionalProperties.falseOnly {
		fail(fmt.Errorf("%s must forbid unspecified public fields", name))
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

func render(catalogPath, statsPath, overallRankingPath, bankRankingPath, createPortalPracticeSessionPath, submitPortalPracticeAnswerPath, createPortalPracticeFeedbackPath, getPortalPracticeFeedbackStatusPath, getPortalFavoritesOverviewPath, listPortalFavoriteQuestionsPath, favoritePortalQuestionPath, unfavoritePortalQuestionPath, createPortalFavoritesSessionPath, digest string) string {
	return fmt.Sprintf(`// Code generated by cmd/quizcraftcontractgen from quizcraft.yaml; DO NOT EDIT.
package practice

const QuizCraftCatalogContractSHA256 = %q
const QuizCraftRankingContractSHA256 = QuizCraftCatalogContractSHA256
const ListPracticeBanksPath = %q
const GetPersonalPracticeStatsPath = %q
const OverallRankingPath = %q
const BankRankingPath = %q
const CreatePortalPracticeSessionPath = %q
const SubmitPortalPracticeAnswerPath = %q
const CreatePortalPracticeFeedbackPath = %q
const GetPortalPracticeFeedbackStatusPath = %q
const GetPortalFavoritesOverviewPath = %q
const ListPortalFavoriteQuestionsPath = %q
const FavoritePortalQuestionPath = %q
const UnfavoritePortalQuestionPath = %q
const CreatePortalFavoritesSessionPath = %q

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

type RankingPeriod string

const (
	RankingPeriodWeekly   RankingPeriod = "weekly"
	RankingPeriodLifetime RankingPeriod = "lifetime"
)

// RankingEnvelope contains public fields only; internal account identifiers are
// deliberately absent from the Portal read contract.
type RankingEnvelope struct {
	RequestID string      `+"`json:\"request_id\"`"+`
	Data      RankingPage `+"`json:\"data\"`"+`
}

type RankingPage struct {
	Scope   string        `+"`json:\"scope\"`"+`
	BankID  string        `+"`json:\"bank_id,omitempty\"`"+`
	Period  RankingPeriod `+"`json:\"period\"`"+`
	Metric  string        `+"`json:\"metric\"`"+`
	Entries []RankingEntry `+"`json:\"entries\"`"+`
}

type RankingEntry struct {
	Rank               int64  `+"`json:\"rank\"`"+`
	Nickname           string `+"`json:\"nickname\"`"+`
	SystemAvatar       string `+"`json:\"system_avatar\"`"+`
	CorrectAnswerCount int64  `+"`json:\"correct_answer_count\"`"+`
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

// FeedbackStatusEnvelope is one authenticated Portal user's persisted
// correction processing status. It never represents a mock response.
type FeedbackStatusEnvelope struct {
	RequestID string         `+"`json:\"request_id\"`"+`
	Data      FeedbackStatus `+"`json:\"data\"`"+`
}

type FeedbackStatus struct {
	FeedbackID        string `+"`json:\"feedback_id\"`"+`
	BankID            string `+"`json:\"bank_id\"`"+`
	QuestionID        string `+"`json:\"question_id\"`"+`
	QuestionVersionID string `+"`json:\"question_version_id\"`"+`
	Category          string `+"`json:\"category\"`"+`
	Status            string `+"`json:\"status\"`"+`
	CreatedAt         string `+"`json:\"created_at\"`"+`
	UpdatedAt         string `+"`json:\"updated_at\"`"+`
}

// FavoritesOverviewEnvelope lists one signed-in Portal user's automatic
// per-bank favorite folders. It never represents a mock response.
type FavoritesOverviewEnvelope struct {
	RequestID string           `+"`json:\"request_id\"`"+`
	Data      []FavoriteFolder `+"`json:\"data\"`"+`
}

type FavoriteFolder struct {
	BankID            string `+"`json:\"bank_id\"`"+`
	BankName          string `+"`json:\"bank_name\"`"+`
	AvailableCount    int    `+"`json:\"available_count\"`"+`
	UnavailableCount  int    `+"`json:\"unavailable_count\"`"+`
}

// FavoriteListEnvelope lists one bank's favorite references for one user.
// Unavailable items expose references only; available items carry the content
// version id.
type FavoriteListEnvelope struct {
	RequestID string             `+"`json:\"request_id\"`"+`
	Data      []FavoriteQuestion `+"`json:\"data\"`"+`
}

type FavoriteQuestion struct {
	BankID            string `+"`json:\"bank_id\"`"+`
	QuestionID        string `+"`json:\"question_id\"`"+`
	Available         bool   `+"`json:\"available\"`"+`
	QuestionVersionID string `+"`json:\"question_version_id,omitempty\"`"+`
}
`, digest, catalogPath, statsPath, overallRankingPath, bankRankingPath, createPortalPracticeSessionPath, submitPortalPracticeAnswerPath, createPortalPracticeFeedbackPath, getPortalPracticeFeedbackStatusPath, getPortalFavoritesOverviewPath, listPortalFavoriteQuestionsPath, favoritePortalQuestionPath, unfavoritePortalQuestionPath, createPortalFavoritesSessionPath)
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
