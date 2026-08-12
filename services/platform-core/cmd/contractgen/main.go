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

type document struct {
	Paths      map[string]pathItem `yaml:"paths"`
	Components struct {
		Schemas         map[string]*schema        `yaml:"schemas"`
		Parameters      map[string]parameter      `yaml:"parameters"`
		SecuritySchemes map[string]securityScheme `yaml:"securitySchemes"`
	} `yaml:"components"`
}

type pathItem struct {
	Get  *operation `yaml:"get"`
	Post *operation `yaml:"post"`
}

type operation struct {
	OperationID string                 `yaml:"operationId"`
	Security    []map[string][]string  `yaml:"security"`
	Parameters  []parameter            `yaml:"parameters"`
	RequestBody requestBody            `yaml:"requestBody"`
	Responses   map[string]apiResponse `yaml:"responses"`
}

type parameter struct {
	Ref      string  `yaml:"$ref"`
	Name     string  `yaml:"name"`
	In       string  `yaml:"in"`
	Required bool    `yaml:"required"`
	Schema   *schema `yaml:"schema"`
}
type securityScheme struct {
	Type   string `yaml:"type"`
	Scheme string `yaml:"scheme"`
	In     string `yaml:"in"`
	Name   string `yaml:"name"`
}
type requestBody struct {
	Required bool                 `yaml:"required"`
	Content  map[string]mediaType `yaml:"content"`
}
type apiResponse struct {
	Content map[string]mediaType `yaml:"content"`
}
type mediaType struct {
	Schema *schema `yaml:"schema"`
}
type schema struct {
	Ref        string             `yaml:"$ref"`
	Type       any                `yaml:"type"`
	Format     string             `yaml:"format"`
	Const      any                `yaml:"const"`
	MinLength  *int               `yaml:"minLength"`
	MaxLength  *int               `yaml:"maxLength"`
	Required   []string           `yaml:"required"`
	Properties map[string]*schema `yaml:"properties"`
	AllOf      []*schema          `yaml:"allOf"`
	Nullable   bool               `yaml:"nullable"`
}

func main() {
	contractPath := flag.String("contract", "../../packages/api-contracts/openapi/platform-core.yaml", "OpenAPI contract")
	outputPath := flag.String("output", "internal/contract/generated.go", "generated Go output")
	flag.Parse()
	source, err := os.ReadFile(*contractPath)
	if err != nil {
		fail(err)
	}
	var spec document
	if err := yaml.Unmarshal(source, &spec); err != nil {
		fail(fmt.Errorf("parse OpenAPI: %w", err))
	}
	authorizePath, authorize := findOperation(spec.Paths, "authorizeOAuthClient")
	tokenPath, token := findOperation(spec.Paths, "exchangeAuthorizationCode")
	authorizationCheckPath, authorizationCheck := findOperation(spec.Paths, "checkAuthorization")
	requestVerificationPath, requestVerification := findOperation(spec.Paths, "requestVerificationCode")
	verifyVerificationPath, verifyVerification := findOperation(spec.Paths, "verifyVerificationCode")
	recordDeliveryPath, recordDelivery := findOperation(spec.Paths, "recordMailDelivery")
	listInboxPath, listInbox := findOperation(spec.Paths, "listOperationsInboxItems")
	getInboxPath, getInbox := findOperation(spec.Paths, "getOperationsInboxItem")
	createInboxPath, createInbox := findOperation(spec.Paths, "createOperationsInboxItem")
	updateInboxPath, updateInbox := findOperation(spec.Paths, "updateOperationsInboxItem")
	operationStatusPath, operationStatus := findOperation(spec.Paths, "getOperationsInboxOperationStatus")
	platformOperationsPath, platformOperations := findOperation(spec.Paths, "getPlatformOperations")
	revokePlatformSessionPath, revokePlatformSession := findOperation(spec.Paths, "revokePlatformOperationSession")
	updatePlatformAccessPath, updatePlatformAccess := findOperation(spec.Paths, "updatePlatformOperationAccess")
	platformOperationStatusPath, platformOperationStatus := findOperation(spec.Paths, "getPlatformOperationStatus")
	accountLookupPath, accountLookup := findOperation(spec.Paths, "lookupPlatformOperationAccount")
	consoleIdentityResolutionPath, consoleIdentityResolution := findOperation(spec.Paths, "resolveConsoleUserIdentities")
	membershipAccountsPath, membershipAccounts := findOperation(spec.Paths, "listPlatformOperationMembershipAccounts")
	if authorize == nil || token == nil || authorizationCheck == nil || requestVerification == nil || verifyVerification == nil || recordDelivery == nil || listInbox == nil || getInbox == nil || createInbox == nil || updateInbox == nil || operationStatus == nil || platformOperations == nil || revokePlatformSession == nil || updatePlatformAccess == nil || platformOperationStatus == nil || accountLookup == nil || consoleIdentityResolution == nil || membershipAccounts == nil {
		fail(fmt.Errorf("required authorization operations are missing"))
	}
	validateTokenOperation(token, spec.Components.Parameters, spec.Components.SecuritySchemes)
	validateAuthorizationCheckOperation(authorizationCheck, spec.Components.Parameters)
	validateIdempotentPublicWrite(requestVerification, spec.Components.Parameters, "verification-code request")
	validateIdempotentPublicWrite(verifyVerification, spec.Components.Parameters, "verification-code verification")
	validateSignedDeliveryOperation(recordDelivery, spec.Components.Parameters, spec.Components.SecuritySchemes)
	validateInboxOperation(listInbox, spec.Components.Parameters, false, false)
	validateInboxOperation(getInbox, spec.Components.Parameters, false, false)
	validateInboxOperation(createInbox, spec.Components.Parameters, true, true)
	validateInboxOperation(updateInbox, spec.Components.Parameters, true, true)
	validateInboxOperation(operationStatus, spec.Components.Parameters, true, false)
	validateInboxOperation(platformOperations, spec.Components.Parameters, false, false)
	validateInboxOperation(revokePlatformSession, spec.Components.Parameters, true, true)
	validateInboxOperation(updatePlatformAccess, spec.Components.Parameters, true, true)
	validateInboxOperation(platformOperationStatus, spec.Components.Parameters, true, false)
	validateInboxOperation(accountLookup, spec.Components.Parameters, false, true)
	validateInboxOperation(consoleIdentityResolution, spec.Components.Parameters, false, false)
	validateInboxOperation(membershipAccounts, spec.Components.Parameters, false, true)
	requestSchema := token.RequestBody.Content["application/json"].Schema
	if requestSchema == nil {
		fail(fmt.Errorf("token request application/json schema is missing"))
	}
	userSchema := spec.Components.Schemas["PlatformUser"]
	if userSchema == nil {
		fail(fmt.Errorf("PlatformUser schema is missing"))
	}
	responseSchema := responseDataSchema(token.Responses["200"].Content["application/json"].Schema)
	if responseSchema == nil {
		fail(fmt.Errorf("token 200 response data schema is missing"))
	}
	authorizationCheckRequest := resolveSchema(authorizationCheck.RequestBody.Content["application/json"].Schema, spec.Components.Schemas)
	authorizationScope := spec.Components.Schemas["AuthorizationScope"]
	authorizationDecision := resolveSchema(responseDataSchema(authorizationCheck.Responses["200"].Content["application/json"].Schema), spec.Components.Schemas)
	if authorizationCheckRequest == nil || authorizationScope == nil || authorizationDecision == nil {
		fail(fmt.Errorf("authorization check schemas are missing"))
	}
	requestVerificationRequest := resolveSchema(requestVerification.RequestBody.Content["application/json"].Schema, spec.Components.Schemas)
	requestVerificationResponse := resolveSchema(responseDataSchema(requestVerification.Responses["202"].Content["application/json"].Schema), spec.Components.Schemas)
	verifyVerificationRequest := resolveSchema(verifyVerification.RequestBody.Content["application/json"].Schema, spec.Components.Schemas)
	verifyVerificationResponse := resolveSchema(responseDataSchema(verifyVerification.Responses["200"].Content["application/json"].Schema), spec.Components.Schemas)
	recordDeliveryRequest := resolveSchema(recordDelivery.RequestBody.Content["application/json"].Schema, spec.Components.Schemas)
	recordDeliveryResponse := resolveSchema(responseDataSchema(recordDelivery.Responses["202"].Content["application/json"].Schema), spec.Components.Schemas)
	if requestVerificationRequest == nil || requestVerificationResponse == nil || verifyVerificationRequest == nil || verifyVerificationResponse == nil || recordDeliveryRequest == nil || recordDeliveryResponse == nil {
		fail(fmt.Errorf("verification-code schemas are missing"))
	}
	inboxItem := spec.Components.Schemas["OperationsInboxItem"]
	createInboxRequest := resolveSchema(createInbox.RequestBody.Content["application/json"].Schema, spec.Components.Schemas)
	updateInboxRequest := resolveSchema(updateInbox.RequestBody.Content["application/json"].Schema, spec.Components.Schemas)
	operationStatusResponse := resolveSchema(responseDataSchema(operationStatus.Responses["200"].Content["application/json"].Schema), spec.Components.Schemas)
	if inboxItem == nil || createInboxRequest == nil || updateInboxRequest == nil || operationStatusResponse == nil {
		fail(fmt.Errorf("operations inbox schemas are missing"))
	}
	accountLookupRequest := resolveSchema(accountLookup.RequestBody.Content["application/json"].Schema, spec.Components.Schemas)
	accountLookupAccount := spec.Components.Schemas["PlatformOperationsAccountLookupAccount"]
	accountLookupResult := resolveSchema(responseDataSchema(accountLookup.Responses["200"].Content["application/json"].Schema), spec.Components.Schemas)
	if accountLookupRequest == nil || accountLookupAccount == nil || accountLookupResult == nil {
		fail(fmt.Errorf("account lookup schemas are missing"))
	}
	successEnvelope := spec.Components.Schemas["SuccessEnvelope"]
	errorObject := spec.Components.Schemas["ErrorObject"]
	errorEnvelope := spec.Components.Schemas["ErrorEnvelope"]
	if successEnvelope == nil || errorObject == nil || errorEnvelope == nil {
		fail(fmt.Errorf("response envelope schemas are missing"))
	}
	headerSupport := renderHeaders(token.Parameters, spec.Components.Parameters, spec.Components.SecuritySchemes["serviceHmac"])
	digest := sha256.Sum256(source)
	generated := fmt.Sprintf(`// Code generated by cmd/contractgen from platform-core.yaml; DO NOT EDIT.
package contract

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	AuthorizeRoute = %q
	TokenRoute = %q
	AuthorizationCheckRoute = %q
	RequestVerificationCodeRoute = %q
	VerifyVerificationCodeRoute = %q
	RecordMailDeliveryRoute = %q
	ListOperationsInboxRoute = %q
	GetOperationsInboxRoute = %q
	CreateOperationsInboxRoute = %q
	UpdateOperationsInboxRoute = %q
	OperationsInboxOperationStatusRoute = %q
	PlatformOperationsRoute = %q
	RevokePlatformOperationSessionRoute = %q
	UpdatePlatformOperationAccessRoute = %q
	PlatformOperationStatusRoute = %q
	PlatformOperationsAccountLookupRoute = %q
	ConsoleUserIdentityResolutionRoute = %q
	PlatformOperationsMembershipAccountsRoute = %q
	SourceSHA256 = %q
)

const SessionExchangeTokenHeader = "X-Session-Exchange-Token"

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s

%s
`, authorizePath, tokenPath, authorizationCheckPath, requestVerificationPath, verifyVerificationPath, recordDeliveryPath,
		listInboxPath, getInboxPath, createInboxPath, updateInboxPath, operationStatusPath,
		platformOperationsPath, revokePlatformSessionPath, updatePlatformAccessPath, platformOperationStatusPath, accountLookupPath, consoleIdentityResolutionPath, membershipAccountsPath, fmt.Sprintf("%x", digest),
		headerSupport,
		renderQuery("AuthorizeOAuthClientQuery", authorize.Parameters),
		renderStruct("ExchangeAuthorizationCodeRequest", requestSchema),
		renderStruct("PlatformUser", userSchema),
		renderStruct("ExchangeAuthorizationCodeResponse", responseSchema),
		renderStruct("AuthorizationScope", authorizationScope),
		renderStruct("AuthorizationCheckRequest", authorizationCheckRequest),
		renderStruct("AuthorizationDecision", authorizationDecision),
		renderSuccessEnvelope(successEnvelope),
		renderStruct("ErrorObject", errorObject),
		renderStruct("ErrorEnvelope", errorEnvelope),
		renderStruct("RequestVerificationCodeRequest", requestVerificationRequest),
		renderStruct("VerificationCodeAccepted", requestVerificationResponse),
		renderStruct("VerifyVerificationCodeRequest", verifyVerificationRequest),
		renderStruct("VerificationCodeVerified", verifyVerificationResponse),
		renderStruct("RecordMailDeliveryRequest", recordDeliveryRequest),
		renderStruct("MailDeliveryAccepted", recordDeliveryResponse),
		renderStruct("OperationsInboxItem", inboxItem),
		renderStruct("CreateOperationsInboxItemRequest", createInboxRequest),
		renderStruct("UpdateOperationsInboxItemRequest", updateInboxRequest),
		renderStruct("OperationsInboxOperationStatus", operationStatusResponse),
		renderStruct("PlatformOperationsAccountLookupRequest", accountLookupRequest),
		renderStruct("PlatformOperationsAccountLookupAccount", accountLookupAccount),
		renderStruct("PlatformOperationsAccountLookupResult", accountLookupResult),
	)
	formatted, err := format.Source([]byte(generated))
	if err != nil {
		fail(fmt.Errorf("format generated contract: %w", err))
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*outputPath, formatted, 0o644); err != nil {
		fail(err)
	}
}

func renderQuery(name string, parameters []parameter) string {
	var query []parameter
	for _, parameter := range parameters {
		if parameter.Ref == "" && parameter.In == "query" {
			query = append(query, parameter)
		}
	}
	if len(query) == 0 {
		fail(fmt.Errorf("%s has no query parameters", name))
	}
	var output strings.Builder
	fmt.Fprintf(&output, "type %s struct {\n", name)
	for _, parameter := range query {
		if parameter.Schema == nil {
			fail(fmt.Errorf("query parameter %s has no schema", parameter.Name))
		}
		fmt.Fprintf(&output, "\t%s %s\n", goName(parameter.Name), schemaType(parameter.Schema))
	}
	output.WriteString("}\n\n")
	fmt.Fprintf(&output, "func Parse%s(values url.Values) (%s, error) {\n\tvar result %s\n", name, name, name)
	for _, parameter := range query {
		field := goName(parameter.Name)
		fmt.Fprintf(&output, "\tresult.%s = values.Get(%q)\n", field, parameter.Name)
		if parameter.Required {
			fmt.Fprintf(&output, "\tif result.%s == \"\" { return %s{}, fmt.Errorf(%q) }\n", field, name, parameter.Name+" is required")
		}
		if constant, ok := parameter.Schema.Const.(string); ok {
			fmt.Fprintf(&output, "\tif result.%s != %q { return %s{}, fmt.Errorf(%q) }\n", field, constant, name, parameter.Name+" has an invalid value")
		}
		if parameter.Schema.MinLength != nil {
			fmt.Fprintf(&output, "\tif len(result.%s) < %d { return %s{}, fmt.Errorf(%q) }\n", field, *parameter.Schema.MinLength, name, parameter.Name+" is too short")
		}
		if parameter.Schema.MaxLength != nil {
			fmt.Fprintf(&output, "\tif len(result.%s) > %d { return %s{}, fmt.Errorf(%q) }\n", field, *parameter.Schema.MaxLength, name, parameter.Name+" is too long")
		}
		if parameter.Schema.Format == "uri" {
			fmt.Fprintf(&output, "\tif _, err := url.ParseRequestURI(result.%s); err != nil { return %s{}, fmt.Errorf(%q) }\n", field, name, parameter.Name+" must be a URI")
		}
	}
	output.WriteString("\treturn result, nil\n}\n")
	return output.String()
}

func findOperation(paths map[string]pathItem, operationID string) (string, *operation) {
	for path, item := range paths {
		for _, operation := range []*operation{item.Get, item.Post} {
			if operation != nil && operation.OperationID == operationID {
				return "/api/v1" + path, operation
			}
		}
	}
	return "", nil
}

func validateTokenOperation(operation *operation, parameters map[string]parameter, securitySchemes map[string]securityScheme) {
	validSecurity := false
	for _, requirement := range operation.Security {
		_, basic := requirement["clientSecret"]
		_, hmac := requirement["serviceHmac"]
		validSecurity = validSecurity || basic && hmac
	}
	if !validSecurity {
		fail(fmt.Errorf("token operation must require clientSecret and serviceHmac together"))
	}
	if scheme := securitySchemes["clientSecret"]; scheme.Type != "http" || scheme.Scheme != "basic" {
		fail(fmt.Errorf("clientSecret must be HTTP Basic"))
	}
	if scheme := securitySchemes["serviceHmac"]; scheme.Type != "apiKey" || scheme.In != "header" || scheme.Name == "" {
		fail(fmt.Errorf("serviceHmac must be a named header apiKey"))
	}
	requiredParameters := map[string]bool{
		"#/components/parameters/RequiredIdempotencyKey": false,
		"#/components/parameters/ServiceId":              false, "#/components/parameters/KeyId": false,
		"#/components/parameters/Timestamp": false, "#/components/parameters/Nonce": false,
	}
	for _, parameter := range operation.Parameters {
		if _, ok := requiredParameters[parameter.Ref]; ok {
			requiredParameters[parameter.Ref] = true
		}
	}
	for parameter, present := range requiredParameters {
		if !present {
			fail(fmt.Errorf("token operation is missing %s", parameter))
		}
		name := strings.TrimPrefix(parameter, "#/components/parameters/")
		definition, ok := parameters[name]
		if !ok || definition.In != "header" || !definition.Required || definition.Name == "" || definition.Schema == nil {
			fail(fmt.Errorf("%s must resolve to a required header parameter", parameter))
		}
	}
	if !operation.RequestBody.Required {
		fail(fmt.Errorf("token request body must be required"))
	}
}

func validateAuthorizationCheckOperation(operation *operation, parameters map[string]parameter) {
	validSecurity := false
	for _, requirement := range operation.Security {
		_, basic := requirement["clientSecret"]
		_, hmac := requirement["serviceHmac"]
		validSecurity = validSecurity || basic && hmac
	}
	if !validSecurity {
		fail(fmt.Errorf("authorization check must require clientSecret and serviceHmac together"))
	}
	requiredParameters := map[string]bool{
		"#/components/parameters/ServiceId": false, "#/components/parameters/KeyId": false,
		"#/components/parameters/Timestamp": false, "#/components/parameters/Nonce": false,
	}
	for _, parameter := range operation.Parameters {
		if _, ok := requiredParameters[parameter.Ref]; ok {
			requiredParameters[parameter.Ref] = true
		}
	}
	for reference, present := range requiredParameters {
		if !present {
			fail(fmt.Errorf("authorization check is missing %s", reference))
		}
		name := strings.TrimPrefix(reference, "#/components/parameters/")
		definition, ok := parameters[name]
		if !ok || definition.In != "header" || !definition.Required || definition.Name == "" || definition.Schema == nil {
			fail(fmt.Errorf("%s must resolve to a required header parameter", reference))
		}
	}
	if !operation.RequestBody.Required {
		fail(fmt.Errorf("authorization check request body must be required"))
	}
}

func validateIdempotentPublicWrite(operation *operation, parameters map[string]parameter, name string) {
	found := false
	for _, reference := range operation.Parameters {
		if reference.Ref == "#/components/parameters/RequiredIdempotencyKey" {
			found = true
		}
	}
	definition, ok := parameters["RequiredIdempotencyKey"]
	if !found || !ok || definition.In != "header" || !definition.Required || definition.Name != "Idempotency-Key" || definition.Schema == nil {
		fail(fmt.Errorf("%s must require Idempotency-Key", name))
	}
	if !operation.RequestBody.Required {
		fail(fmt.Errorf("%s request body must be required", name))
	}
}

func validateSignedDeliveryOperation(operation *operation, parameters map[string]parameter, schemes map[string]securityScheme) {
	found := false
	for _, requirement := range operation.Security {
		_, found = requirement["mailDeliveryHmac"]
		if found {
			break
		}
	}
	scheme := schemes["mailDeliveryHmac"]
	if !found || scheme.Type != "apiKey" || scheme.In != "header" || scheme.Name != "X-Signature" {
		fail(fmt.Errorf("%s must require delivery HMAC authentication", operation.OperationID))
	}
	required := map[string]bool{"#/components/parameters/KeyId": false, "#/components/parameters/Timestamp": false, "#/components/parameters/Nonce": false}
	for _, parameter := range operation.Parameters {
		if _, ok := required[parameter.Ref]; ok {
			required[parameter.Ref] = true
		}
	}
	for reference, present := range required {
		if !present {
			fail(fmt.Errorf("%s is missing %s", operation.OperationID, reference))
		}
	}
	if !operation.RequestBody.Required {
		fail(fmt.Errorf("%s request body must be required", operation.OperationID))
	}
}

func validateInboxOperation(operation *operation, parameters map[string]parameter, requireIdempotency, requireBody bool) {
	validSecurity := false
	for _, requirement := range operation.Security {
		_, basic := requirement["clientSecret"]
		_, hmac := requirement["serviceHmac"]
		validSecurity = validSecurity || basic && hmac
	}
	if !validSecurity {
		fail(fmt.Errorf("%s must require clientSecret and serviceHmac together", operation.OperationID))
	}
	required := map[string]bool{
		"#/components/parameters/ServiceId":            false,
		"#/components/parameters/KeyId":                false,
		"#/components/parameters/Timestamp":            false,
		"#/components/parameters/Nonce":                false,
		"#/components/parameters/SessionExchangeToken": false,
	}
	if requireIdempotency {
		required["#/components/parameters/RequiredIdempotencyKey"] = false
	}
	for _, candidate := range operation.Parameters {
		if _, ok := required[candidate.Ref]; ok {
			required[candidate.Ref] = true
		}
	}
	for reference, present := range required {
		name := strings.TrimPrefix(reference, "#/components/parameters/")
		definition, ok := parameters[name]
		if !present || !ok || definition.In != "header" || !definition.Required {
			fail(fmt.Errorf("%s is missing required header %s", operation.OperationID, reference))
		}
	}
	if requireBody && !operation.RequestBody.Required {
		fail(fmt.Errorf("%s request body must be required", operation.OperationID))
	}
}

func renderHeaders(references []parameter, parameters map[string]parameter, hmac securityScheme) string {
	var headers []parameter
	for _, reference := range references {
		name := strings.TrimPrefix(reference.Ref, "#/components/parameters/")
		if definition, ok := parameters[name]; ok && definition.In == "header" {
			headers = append(headers, definition)
		}
	}
	headers = append(headers, parameter{Name: hmac.Name, In: hmac.In, Required: true, Schema: &schema{Type: "string"}})
	var output strings.Builder
	output.WriteString("const (\n")
	for _, header := range headers {
		fmt.Fprintf(&output, "\t%s = %q\n", headerConstant(header.Name), header.Name)
	}
	output.WriteString(")\n\n")
	output.WriteString("type ExchangeHeaders struct {\n")
	for _, header := range headers {
		fmt.Fprintf(&output, "\t%s string\n", headerField(header.Name))
	}
	output.WriteString("}\n\nfunc ParseExchangeHeaders(values http.Header) (ExchangeHeaders, error) {\n\tvar result ExchangeHeaders\n")
	for _, header := range headers {
		field, constant := headerField(header.Name), headerConstant(header.Name)
		fmt.Fprintf(&output, "\tresult.%s = values.Get(%s)\n", field, constant)
		if header.Required {
			fmt.Fprintf(&output, "\tif result.%s == \"\" { return ExchangeHeaders{}, fmt.Errorf(%q) }\n", field, header.Name+" is required")
		}
		if header.Schema != nil && header.Schema.MinLength != nil {
			fmt.Fprintf(&output, "\tif len(result.%s) < %d { return ExchangeHeaders{}, fmt.Errorf(%q) }\n", field, *header.Schema.MinLength, header.Name+" is too short")
		}
		if header.Schema != nil && header.Schema.MaxLength != nil {
			fmt.Fprintf(&output, "\tif len(result.%s) > %d { return ExchangeHeaders{}, fmt.Errorf(%q) }\n", field, *header.Schema.MaxLength, header.Name+" is too long")
		}
	}
	output.WriteString("\treturn result, nil\n}\n")
	return output.String()
}

func headerField(name string) string {
	name = strings.TrimPrefix(name, "X-")
	return goName(strings.ReplaceAll(name, "-", "_"))
}

func headerConstant(name string) string { return headerField(name) + "Header" }

func renderSuccessEnvelope(value *schema) string {
	if value.Properties["data"] == nil || value.Properties["request_id"] == nil {
		fail(fmt.Errorf("SuccessEnvelope must define data and request_id"))
	}
	return "type SuccessEnvelope[T any] struct {\n\tData T `json:\"data\"`\n\tRequestID string `json:\"request_id\"`\n}\n"
}

func responseDataSchema(root *schema) *schema {
	if root == nil {
		return nil
	}
	for _, member := range root.AllOf {
		if data := member.Properties["data"]; data != nil {
			return data
		}
	}
	return root.Properties["data"]
}

func resolveSchema(value *schema, schemas map[string]*schema) *schema {
	if value == nil || value.Ref == "" {
		return value
	}
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(value.Ref, prefix) {
		fail(fmt.Errorf("unsupported schema reference %s", value.Ref))
	}
	resolved := schemas[strings.TrimPrefix(value.Ref, prefix)]
	if resolved == nil {
		fail(fmt.Errorf("schema reference %s is missing", value.Ref))
	}
	return resolved
}

func renderStruct(name string, value *schema) string {
	if value == nil {
		fail(fmt.Errorf("cannot generate %s from an empty schema", name))
	}
	fields := append([]string(nil), value.Required...)
	required := make(map[string]bool, len(fields))
	for _, field := range fields {
		required[field] = true
	}
	var optional []string
	for field := range value.Properties {
		if !required[field] {
			optional = append(optional, field)
		}
	}
	sort.Strings(optional)
	fields = append(fields, optional...)
	var output strings.Builder
	fmt.Fprintf(&output, "type %s struct {\n", name)
	for _, field := range fields {
		property := value.Properties[field]
		if property == nil {
			fail(fmt.Errorf("%s requires missing property %s", name, field))
		}
		goType := schemaType(property)
		jsonName := field
		if property.Nullable {
			goType = "*" + goType
			if !required[field] {
				jsonName += ",omitempty"
			}
		} else if !required[field] {
			goType = "*" + goType
			jsonName += ",omitempty"
		}
		fmt.Fprintf(&output, "\t%s %s `json:\"%s\"`\n", goName(field), goType, jsonName)
	}
	output.WriteString("}\n")
	return output.String()
}

func schemaType(value *schema) string {
	if value.Ref != "" {
		parts := strings.Split(value.Ref, "/")
		name := parts[len(parts)-1]
		if name == "RequestId" {
			return "string"
		}
		return name
	}
	if value.Type == nil {
		return "any"
	}
	if types, ok := value.Type.([]any); ok {
		for _, candidate := range types {
			if candidate == "object" {
				return "map[string]any"
			}
		}
	}
	typeName, _ := value.Type.(string)
	if typeName == "string" && value.Format == "date-time" {
		return "time.Time"
	}
	switch typeName {
	case "string":
		return "string"
	case "boolean":
		return "bool"
	case "integer":
		if value.Format == "int64" {
			return "int64"
		}
		return "int"
	case "object":
		return "map[string]any"
	default:
		fail(fmt.Errorf("unsupported generated schema type %v", value.Type))
		return ""
	}
}

func goName(value string) string {
	parts := strings.Split(value, "_")
	for index, part := range parts {
		switch strings.ToLower(part) {
		case "id":
			parts[index] = "ID"
			continue
		case "uri":
			parts[index] = "URI"
			continue
		case "url":
			parts[index] = "URL"
			continue
		}
		runes := []rune(part)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
		}
		parts[index] = string(runes)
	}
	return strings.Join(parts, "")
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
