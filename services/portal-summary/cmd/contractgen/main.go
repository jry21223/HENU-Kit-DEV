package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type schema struct {
	Ref            string            `yaml:"$ref"`
	Const          any               `yaml:"const"`
	Enum           []string          `yaml:"enum"`
	MinItems       int               `yaml:"minItems"`
	MaxItems       int               `yaml:"maxItems"`
	MaxLength      int               `yaml:"maxLength"`
	Pattern        string            `yaml:"pattern"`
	Properties     map[string]schema `yaml:"properties"`
	AllOf          []schema          `yaml:"allOf"`
	Example        any               `yaml:"example"`
	InvalidExample any               `yaml:"x-invalid-example"`
}

type header struct {
	Required bool   `yaml:"required"`
	Schema   schema `yaml:"schema"`
}

type response struct {
	Ref         string            `yaml:"$ref"`
	Headers     map[string]schema `yaml:"headers"`
	XErrorCodes []string          `yaml:"x-error-codes"`
	Content     map[string]media  `yaml:"content"`
}

type media struct {
	Schema schema `yaml:"schema"`
}

type operation struct {
	Responses map[string]response `yaml:"responses"`
}

type pathItem struct {
	Get operation `yaml:"get"`
}

type document struct {
	Paths      map[string]pathItem `yaml:"paths"`
	Components struct {
		Schemas   map[string]schema   `yaml:"schemas"`
		Headers   map[string]header   `yaml:"headers"`
		Responses map[string]response `yaml:"responses"`
	} `yaml:"components"`
}

type limits struct {
	metricCount, labelLength, valueLength, hintLength, messageLength, requestIDLength int
	requestIDPattern                                                                  string
	statuses                                                                          map[string]bool
}

type metric struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Hint  string `json:"hint"`
}
type portalSummary struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	StatusMessage string    `json:"status_message"`
	RequestID     string    `json:"request_id"`
	Metrics       []metric  `json:"metrics"`
	AsOf          time.Time `json:"as_of"`
}
type envelope struct {
	Data      portalSummary `json:"data"`
	RequestID string        `json:"request_id"`
}

func main() {
	portalPath := flag.String("portal-contract", "../../packages/api-contracts/openapi/portal-summary.yaml", "Portal Summary OpenAPI")
	gatewayPath := flag.String("gateway-contract", "../../packages/api-contracts/openapi/console-gateway.yaml", "Console Gateway OpenAPI")
	outputPath := flag.String("output", "internal/contract/generated.go", "generated Go output")
	flag.Parse()
	portalSource, portal := read(*portalPath)
	gatewaySource, gateway := read(*gatewayPath)
	portalSummarySchema := requiredSchema(portal, "PortalSummary")
	if len(portalSummarySchema.AllOf) != 2 || portalSummarySchema.AllOf[0].Ref != "./console-gateway.yaml#/components/schemas/ConsoleModuleSummary" {
		fail(errors.New("PortalSummary must inherit ConsoleModuleSummary"))
	}
	specialization := portalSummarySchema.AllOf[1]
	gatewaySummary := requiredSchema(gateway, "ConsoleModuleSummary")
	gatewayMetric := requiredSchema(gateway, "ConsoleModuleMetric")
	values := limits{
		metricCount: specialization.Properties["metrics"].MinItems,
		labelLength: gatewayMetric.Properties["label"].MaxLength, valueLength: gatewayMetric.Properties["value"].MaxLength,
		hintLength: gatewayMetric.Properties["hint"].MaxLength, messageLength: gatewaySummary.Properties["status_message"].MaxLength,
		requestIDLength: gatewaySummary.Properties["request_id"].MaxLength, requestIDPattern: gatewaySummary.Properties["request_id"].Pattern,
		statuses: stringSet(specialization.Properties["status"].Enum),
	}
	if values.metricCount != 8 || specialization.Properties["metrics"].MaxItems != values.metricCount || values.labelLength <= 0 || values.valueLength <= 0 || values.hintLength <= 0 || values.messageLength <= 0 || values.requestIDLength <= 0 || values.requestIDPattern == "" || specialization.Properties["id"].Const != "portal" || len(values.statuses) == 0 {
		fail(errors.New("PortalSummary specialization or inherited limits are incomplete"))
	}
	errorCodes := validateErrorContract(portal, values)
	envelopeSchema := requiredSchema(portal, "PortalSummaryEnvelope")
	if envelopeSchema.Example == nil || envelopeSchema.InvalidExample == nil {
		fail(errors.New("PortalSummaryEnvelope requires example and x-invalid-example"))
	}
	if err := validateExample(decodeExample(envelopeSchema.Example), values); err != nil {
		fail(fmt.Errorf("valid example: %w", err))
	}
	if err := validateExample(decodeExample(envelopeSchema.InvalidExample), values); err == nil {
		fail(errors.New("x-invalid-example unexpectedly satisfies the contract"))
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(append(append([]byte{}, portalSource...), gatewaySource...)))
	source := generatedSource(digest, values, errorCodes)
	formatted, err := format.Source([]byte(source))
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*outputPath, formatted, 0o644); err != nil {
		fail(err)
	}
}

func read(path string) ([]byte, document) {
	source, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	var value document
	if err := yaml.Unmarshal(source, &value); err != nil {
		fail(err)
	}
	return source, value
}
func requiredSchema(document document, name string) schema {
	value, ok := document.Components.Schemas[name]
	if !ok {
		fail(fmt.Errorf("missing schema %s", name))
	}
	return value
}
func decodeExample(value any) envelope {
	encoded, _ := json.Marshal(value)
	var result envelope
	if err := json.Unmarshal(encoded, &result); err != nil {
		fail(err)
	}
	return result
}
func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}

func validateErrorContract(portal document, values limits) map[string]int {
	expected := map[string]struct {
		status int
		codes  []string
		schema string
	}{
		"Unauthorized": {status: 401, codes: []string{"INVALID_SERVICE_AUTH"}, schema: "UnauthorizedErrorEnvelope"},
		"Conflict":     {status: 409, codes: []string{"REPLAY_DETECTED"}, schema: "ConflictErrorEnvelope"},
		"Unavailable":  {status: 503, codes: []string{"DEPENDENCY_UNAVAILABLE", "INVALID_OWNER_SUMMARY"}, schema: "UnavailableErrorEnvelope"},
	}
	requestHeader, ok := portal.Components.Headers["RequestId"]
	if !ok || !requestHeader.Required || requestHeader.Schema.MaxLength != values.requestIDLength || requestHeader.Schema.Pattern != values.requestIDPattern {
		fail(errors.New("response RequestId header must match the inherited request ID contract"))
	}
	operationResponses := portal.Paths["/console-summary"].Get.Responses
	_, rejectsInvalidRequestID := operationResponses["400"]
	if operationResponses["200"].Headers["X-Request-Id"].Ref != "#/components/headers/RequestId" || rejectsInvalidRequestID {
		fail(errors.New("success must trace its response and invalid request IDs must be normalized rather than rejected"))
	}
	codeStatuses := map[string]int{}
	for name, expectation := range expected {
		definition, ok := portal.Components.Responses[name]
		if !ok || definition.Headers["X-Request-Id"].Ref != "#/components/headers/RequestId" || !sameStrings(definition.XErrorCodes, expectation.codes) || definition.Content["application/json"].Schema.Ref != "#/components/schemas/"+expectation.schema {
			fail(fmt.Errorf("response %s must declare its stable error codes and trace header", name))
		}
		if !sameStrings(specializedErrorCodes(requiredSchema(portal, expectation.schema)), expectation.codes) {
			fail(fmt.Errorf("response %s must use a standard OpenAPI schema with status-specific error codes", name))
		}
		if operationResponses[fmt.Sprint(expectation.status)].Ref != "#/components/responses/"+name {
			fail(fmt.Errorf("status %d must reference response %s", expectation.status, name))
		}
		for _, code := range expectation.codes {
			codeStatuses[code] = expectation.status
		}
	}
	contractCodes := requiredSchema(portal, "ErrorEnvelope").Properties["error"].Properties["code"].Enum
	allCodes := make([]string, 0, len(codeStatuses))
	for code := range codeStatuses {
		allCodes = append(allCodes, code)
	}
	if !sameStrings(contractCodes, allCodes) {
		fail(errors.New("ErrorEnvelope code enum must equal the response-specific stable codes"))
	}
	return codeStatuses
}

func specializedErrorCodes(value schema) []string {
	if len(value.AllOf) != 2 || value.AllOf[0].Ref != "#/components/schemas/ErrorEnvelope" {
		return nil
	}
	code := value.AllOf[1].Properties["error"].Properties["code"]
	if text, ok := code.Const.(string); ok && text != "" {
		return []string{text}
	}
	return code.Enum
}

func sameStrings(left, right []string) bool {
	leftCopy, rightCopy := append([]string{}, left...), append([]string{}, right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	return len(leftCopy) == len(rightCopy) && strings.Join(leftCopy, "\x00") == strings.Join(rightCopy, "\x00")
}
func validateExample(value envelope, limits limits) error {
	if value.Data.ID != "portal" || !limits.statuses[value.Data.Status] || len(value.Data.Metrics) != limits.metricCount || value.Data.AsOf.IsZero() || value.Data.RequestID != value.RequestID {
		return errors.New("envelope identity, status, metrics, time, or trace is invalid")
	}
	pattern, err := regexp.Compile(limits.requestIDPattern)
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(value.RequestID) > limits.requestIDLength || !pattern.MatchString(value.RequestID) || utf8.RuneCountInString(value.Data.StatusMessage) > limits.messageLength {
		return errors.New("request ID or status message is invalid")
	}
	for _, metric := range value.Data.Metrics {
		if metric.Label == "" || metric.Value == "" || utf8.RuneCountInString(metric.Label) > limits.labelLength || utf8.RuneCountInString(metric.Value) > limits.valueLength || utf8.RuneCountInString(metric.Hint) > limits.hintLength {
			return errors.New("metric is invalid")
		}
	}
	return nil
}

func generatedSource(digest string, values limits, errorCodes map[string]int) string {
	statuses := make([]string, 0, len(values.statuses))
	for status := range values.statuses {
		statuses = append(statuses, fmt.Sprintf("%q: true", status))
	}
	sort.Strings(statuses)
	codeConstants := make([]string, 0, len(errorCodes))
	codeStatuses := make([]string, 0, len(errorCodes))
	for code, status := range errorCodes {
		name := goName(code)
		codeConstants = append(codeConstants, fmt.Sprintf("Error%s = %q", name, code))
		codeStatuses = append(codeStatuses, fmt.Sprintf("%q: %d", code, status))
	}
	sort.Strings(codeConstants)
	sort.Strings(codeStatuses)
	return strings.NewReplacer(
		"{{HASH}}", digest, "{{COUNT}}", fmt.Sprint(values.metricCount), "{{LABEL}}", fmt.Sprint(values.labelLength), "{{VALUE}}", fmt.Sprint(values.valueLength),
		"{{HINT}}", fmt.Sprint(values.hintLength), "{{MESSAGE}}", fmt.Sprint(values.messageLength), "{{REQUEST_LENGTH}}", fmt.Sprint(values.requestIDLength),
		"{{REQUEST_PATTERN}}", fmt.Sprintf("%q", values.requestIDPattern), "{{STATUSES}}", strings.Join(statuses, ", "),
		"{{ERROR_CONSTANTS}}", strings.Join(codeConstants, "\n"), "{{ERROR_STATUSES}}", strings.Join(codeStatuses, ", "),
	).Replace(generatedTemplate)
}

func goName(value string) string {
	parts := strings.Split(strings.ToLower(value), "_")
	for index, part := range parts {
		if part != "" {
			parts[index] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

const generatedTemplate = `// Code generated from portal-summary.yaml and console-gateway.yaml (SHA256 {{HASH}}); DO NOT EDIT.
package contract

import (
    "errors"
    "regexp"
    "time"
    "unicode/utf8"
)

const (
    ContractSHA256 = "{{HASH}}"
    {{ERROR_CONSTANTS}}
)

type Metric struct { Label string ` + "`json:\"label\"`" + `; Value string ` + "`json:\"value\"`" + `; Hint string ` + "`json:\"hint,omitempty\"`" + ` }
type PortalSummary struct { ID string ` + "`json:\"id\"`" + `; Status string ` + "`json:\"status\"`" + `; Metrics []Metric ` + "`json:\"metrics\"`" + `; StatusMessage string ` + "`json:\"status_message\"`" + `; AsOf time.Time ` + "`json:\"as_of\"`" + `; RequestID string ` + "`json:\"request_id\"`" + ` }
type PortalSummaryEnvelope struct { Data PortalSummary ` + "`json:\"data\"`" + `; RequestID string ` + "`json:\"request_id\"`" + ` }

var requestIDPattern = regexp.MustCompile({{REQUEST_PATTERN}})
var liveStatuses = map[string]bool{ {{STATUSES}} }
var errorStatuses = map[string]int{ {{ERROR_STATUSES}} }

func ValidErrorStatus(status int, code string) bool { return errorStatuses[code] == status }

func ValidatePortalSummaryEnvelope(value PortalSummaryEnvelope) error {
    if value.Data.ID != "portal" || !liveStatuses[value.Data.Status] || len(value.Data.Metrics) != {{COUNT}} || value.Data.AsOf.IsZero() || value.Data.RequestID != value.RequestID { return errors.New("portal summary identity, status, metrics, time, or trace is invalid") }
    if utf8.RuneCountInString(value.RequestID) > {{REQUEST_LENGTH}} || !requestIDPattern.MatchString(value.RequestID) || utf8.RuneCountInString(value.Data.StatusMessage) > {{MESSAGE}} { return errors.New("portal summary trace or message is invalid") }
    for _, metric := range value.Data.Metrics { if metric.Label == "" || metric.Value == "" || utf8.RuneCountInString(metric.Label) > {{LABEL}} || utf8.RuneCountInString(metric.Value) > {{VALUE}} || utf8.RuneCountInString(metric.Hint) > {{HINT}} { return errors.New("portal summary metric is invalid") } }
    return nil
}
`

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
