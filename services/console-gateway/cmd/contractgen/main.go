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
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type schema struct {
	Ref                  string            `yaml:"$ref"`
	Type                 string            `yaml:"type"`
	Format               string            `yaml:"format"`
	Const                any               `yaml:"const"`
	Enum                 []string          `yaml:"enum"`
	MinItems             int               `yaml:"minItems"`
	MaxItems             int               `yaml:"maxItems"`
	Minimum              *float64          `yaml:"minimum"`
	Maximum              *float64          `yaml:"maximum"`
	MinLength            int               `yaml:"minLength"`
	MaxLength            int               `yaml:"maxLength"`
	Pattern              string            `yaml:"pattern"`
	Required             []string          `yaml:"required"`
	Properties           map[string]schema `yaml:"properties"`
	Items                *schema           `yaml:"items"`
	AdditionalProperties *bool             `yaml:"additionalProperties"`
	Example              any               `yaml:"example"`
	InvalidExample       any               `yaml:"x-invalid-example"`
	AllOf                []schema          `yaml:"allOf"`
	AnyOf                []schema          `yaml:"anyOf"`
	OneOf                []schema          `yaml:"oneOf"`
	If                   *schema           `yaml:"if"`
	Then                 *schema           `yaml:"then"`
	Contains             *schema           `yaml:"contains"`
	MinContains          int               `yaml:"minContains"`
	MaxContains          int               `yaml:"maxContains"`
}

type document struct {
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
	Paths map[string]map[string]struct {
		OperationID string `yaml:"operationId"`
	} `yaml:"paths"`
	Components struct {
		Schemas map[string]schema `yaml:"schemas"`
	} `yaml:"components"`
}

func main() {
	contractPath := flag.String("contract", "../../packages/api-contracts/openapi/console-gateway.yaml", "OpenAPI contract")
	goOutput := flag.String("go-output", "internal/contract/generated.go", "generated Go output")
	tsOutput := flag.String("ts-output", "../../apps/console/src/lib/console-gateway.ts", "generated TypeScript output")
	flag.Parse()
	source, err := os.ReadFile(*contractPath)
	if err != nil {
		fail(err)
	}
	var spec document
	if err := yaml.Unmarshal(source, &spec); err != nil {
		fail(err)
	}
	if len(spec.Servers) == 0 || !strings.HasSuffix(spec.Servers[0].URL, "/api/v1") {
		fail(errors.New("console gateway server must end with /api/v1"))
	}
	routes := operationRoutes(spec)
	for _, operationID := range []string{"getConsoleGatewayHealth", "beginConsoleLogin", "completeConsoleLogin", "getConsoleSession", "getConsoleOverview", "getConsolePlatformOperations", "revokeConsolePlatformSession", "updateConsolePlatformAccess", "getConsolePlatformOperationStatus", "lookupConsoleAccount", "getConsoleNotices", "createConsoleNoticeSource", "createConsoleNoticeVersion", "reviewConsoleNoticeVersion", "distributeConsoleNoticeVersion", "getConsoleNoticeOperationStatus", "getConsoleLibraryWorkspace", "executeConsoleLibraryCommand", "getConsoleLibraryOperationStatus", "getConsoleFoodWorkspace", "executeConsoleFoodCommand", "getConsoleFoodOperationStatus", "searchConsoleAccountMemberships", "getConsoleAccountMembership", "grantConsoleAccountMembership", "revokeConsoleAccountMembership", "adjustConsoleAccountPoints", "getConsoleAccountTickets", "getConsoleAccountTicket", "replyConsoleAccountTicket", "transitionConsoleAccountTicket", "closeConsoleMembershipOrder", "refundConsoleMembershipOrder", "getConsoleMembershipOrderRefund", "logoutConsoleSession"} {
		if routes[operationID] == "" {
			fail(fmt.Errorf("required operation %s is missing", operationID))
		}
	}
	sessionSchema, ok := spec.Components.Schemas["ConsoleSession"]
	if !ok || sessionSchema.Example == nil || sessionSchema.InvalidExample == nil {
		fail(errors.New("ConsoleSession requires example and x-invalid-example"))
	}
	if err := validateSchema(sessionSchema, sessionSchema.Example, spec.Components.Schemas); err != nil {
		fail(fmt.Errorf("ConsoleSession example is invalid: %w", err))
	}
	if err := validateSchema(sessionSchema, sessionSchema.InvalidExample, spec.Components.Schemas); err == nil {
		fail(errors.New("ConsoleSession x-invalid-example unexpectedly satisfies the schema"))
	}
	overviewSchema, ok := spec.Components.Schemas["ConsoleOverview"]
	if !ok || overviewSchema.Example == nil || overviewSchema.InvalidExample == nil {
		fail(errors.New("ConsoleOverview requires example and x-invalid-example"))
	}
	if err := validateSchema(overviewSchema, overviewSchema.Example, spec.Components.Schemas); err != nil {
		fail(fmt.Errorf("ConsoleOverview example is invalid: %w", err))
	}
	if err := validateSchema(overviewSchema, overviewSchema.InvalidExample, spec.Components.Schemas); err == nil {
		fail(errors.New("ConsoleOverview x-invalid-example unexpectedly satisfies the schema"))
	}
	moduleSummarySchema, ok := spec.Components.Schemas["ConsoleModuleSummary"]
	if !ok {
		fail(errors.New("ConsoleModuleSummary is missing"))
	}
	invalidStaleSummary := map[string]any{
		"id": "library", "status": "stale", "metrics": []any{},
		"status_message": "missing stale timestamps", "request_id": "req_invalid_stale",
	}
	if err := validateSchema(moduleSummarySchema, invalidStaleSummary, spec.Components.Schemas); err == nil {
		fail(errors.New("ConsoleModuleSummary stale value without timestamps unexpectedly satisfies the schema"))
	}
	invalidRequestIDSummary := map[string]any{
		"id": "library", "status": "ok", "metrics": []any{}, "as_of": "2026-07-19T01:00:00Z",
		"status_message": "invalid request id", "request_id": "req_invalid value!",
	}
	if err := validateSchema(moduleSummarySchema, invalidRequestIDSummary, spec.Components.Schemas); err == nil {
		fail(errors.New("ConsoleModuleSummary request_id outside its pattern unexpectedly satisfies the schema"))
	}
	distributionSchema, ok := spec.Components.Schemas["NoticeDistributionRequest"]
	if !ok || distributionSchema.Example == nil || distributionSchema.InvalidExample == nil {
		fail(errors.New("notice distribution request requires example and x-invalid-example"))
	}
	if err := validateSchema(distributionSchema, distributionSchema.Example, spec.Components.Schemas); err != nil {
		fail(fmt.Errorf("notice distribution request example is invalid: %w", err))
	}
	if err := validateSchema(distributionSchema, distributionSchema.InvalidExample, spec.Components.Schemas); err == nil {
		fail(errors.New("notice distribution request x-invalid-example unexpectedly satisfies the schema"))
	}
	libraryCommandSchema, ok := spec.Components.Schemas["LibraryCommand"]
	if !ok || libraryCommandSchema.Example == nil || libraryCommandSchema.InvalidExample == nil {
		fail(errors.New("library command requires example and x-invalid-example"))
	}
	if err := validateSchema(libraryCommandSchema, libraryCommandSchema.Example, spec.Components.Schemas); err != nil {
		fail(fmt.Errorf("library command example is invalid: %w", err))
	}
	if err := validateSchema(libraryCommandSchema, libraryCommandSchema.InvalidExample, spec.Components.Schemas); err == nil {
		fail(errors.New("library command x-invalid-example unexpectedly satisfies the schema"))
	}
	foodCommandSchema, ok := spec.Components.Schemas["FoodCommand"]
	if !ok || foodCommandSchema.Example == nil || foodCommandSchema.InvalidExample == nil {
		fail(errors.New("food command requires example and x-invalid-example"))
	}
	if err := validateSchema(foodCommandSchema, foodCommandSchema.Example, spec.Components.Schemas); err != nil {
		fail(fmt.Errorf("food command example is invalid: %w", err))
	}
	if err := validateSchema(foodCommandSchema, foodCommandSchema.InvalidExample, spec.Components.Schemas); err == nil {
		fail(errors.New("food command x-invalid-example unexpectedly satisfies the schema"))
	}

	digest := fmt.Sprintf("%x", sha256.Sum256(source))
	goSource := renderGo(spec, routes, digest)
	formatted, err := format.Source([]byte(goSource))
	if err != nil {
		fail(fmt.Errorf("format generated Go: %w\n%s", err, goSource))
	}
	write(*goOutput, formatted)
	write(*tsOutput, []byte(renderTypeScript(spec, routes, digest)))
}

func operationRoutes(spec document) map[string]string {
	routes := map[string]string{}
	for path, methods := range spec.Paths {
		for _, operation := range methods {
			routes[operation.OperationID] = "/api/v1" + path
		}
	}
	return routes
}

func schemaNames(spec document) []string {
	names := make([]string, 0, len(spec.Components.Schemas))
	for name := range spec.Components.Schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func renderGo(spec document, routes map[string]string, digest string) string {
	var output strings.Builder
	fmt.Fprintf(&output, `// Code generated by cmd/contractgen from console-gateway.yaml; DO NOT EDIT.
package contract

import "time"

const (
	HealthRoute = %q
	LoginRoute = %q
	CallbackRoute = %q
	SessionRoute = %q
	OverviewRoute = %q
	OperationsRoute = %q
	RevokeSessionRoute = %q
	UpdateAccessRoute = %q
	OperationStatusRoute = %q
	AccountLookupRoute = %q
	NoticeSnapshotRoute = %q
	NoticeSourceRoute = %q
	NoticeVersionRoute = %q
	NoticeReviewRoute = %q
	NoticeDistributionRoute = %q
	NoticeOperationRoute = %q
	LibraryWorkspaceRoute = %q
	LibraryCommandRoute = %q
	LibraryOperationRoute = %q
		FoodWorkspaceRoute = %q
		FoodCommandRoute = %q
		FoodOperationRoute = %q
		AccountMembershipRoute = %q
		SearchAccountMembershipsRoute = %q
		AccountMembershipGrantsRoute = %q
		AccountMembershipRevocationsRoute = %q
		AccountPointAdjustmentsRoute = %q
		AccountTicketsRoute = %q
		AccountTicketRoute = %q
		AccountTicketRepliesRoute = %q
		AccountTicketTransitionsRoute = %q
		AccountMembershipOrderClosuresRoute = %q
		AccountMembershipOrderRefundsRoute = %q
		AccountMembershipOrderRefundRoute = %q
		LogoutRoute = %q
	SourceSHA256 = %q
)

	`, routes["getConsoleGatewayHealth"], routes["beginConsoleLogin"], routes["completeConsoleLogin"], routes["getConsoleSession"], routes["getConsoleOverview"], routes["getConsolePlatformOperations"], routes["revokeConsolePlatformSession"], routes["updateConsolePlatformAccess"], routes["getConsolePlatformOperationStatus"], routes["lookupConsoleAccount"], routes["getConsoleNotices"], routes["createConsoleNoticeSource"], routes["createConsoleNoticeVersion"], routes["reviewConsoleNoticeVersion"], routes["distributeConsoleNoticeVersion"], routes["getConsoleNoticeOperationStatus"], routes["getConsoleLibraryWorkspace"], routes["executeConsoleLibraryCommand"], routes["getConsoleLibraryOperationStatus"], routes["getConsoleFoodWorkspace"], routes["executeConsoleFoodCommand"], routes["getConsoleFoodOperationStatus"], routes["getConsoleAccountMembership"], routes["searchConsoleAccountMemberships"], routes["grantConsoleAccountMembership"], routes["revokeConsoleAccountMembership"], routes["adjustConsoleAccountPoints"], routes["getConsoleAccountTickets"], routes["getConsoleAccountTicket"], routes["replyConsoleAccountTicket"], routes["transitionConsoleAccountTicket"], routes["closeConsoleMembershipOrder"], routes["refundConsoleMembershipOrder"], routes["getConsoleMembershipOrderRefund"], routes["logoutConsoleSession"], digest)
	for _, name := range schemaNames(spec) {
		fmt.Fprintf(&output, "type %s %s\n\n", name, goType(spec.Components.Schemas[name], 0))
	}
	return output.String()
}

func goType(value schema, indent int) string {
	if value.Ref != "" {
		return refName(value.Ref)
	}
	if len(value.OneOf) > 0 {
		var shared string
		for _, item := range value.OneOf {
			candidate := goType(item, indent)
			if shared == "" {
				shared = candidate
				continue
			}
			if shared != candidate {
				return "any"
			}
		}
		if shared != "" {
			return shared
		}
	}
	switch value.Type {
	case "string":
		if value.Format == "date-time" {
			return "time.Time"
		}
		return "string"
	case "boolean":
		return "bool"
	case "integer":
		return "int64"
	case "array":
		if value.Items == nil {
			return "[]any"
		}
		return "[]" + goType(*value.Items, indent)
	case "object":
		var output strings.Builder
		output.WriteString("struct {\n")
		required := stringSet(value.Required)
		for _, property := range sortedProperties(value.Properties) {
			fieldType := goType(value.Properties[property], indent+1)
			tag := property
			if !required[property] {
				fieldType = "*" + fieldType
				tag += ",omitempty"
			}
			fmt.Fprintf(&output, "%s%s %s `json:\"%s\"`\n", strings.Repeat("\t", indent+1), goName(property), fieldType, tag)
		}
		output.WriteString(strings.Repeat("\t", indent) + "}")
		return output.String()
	default:
		return "any"
	}
}

func renderTypeScript(spec document, routes map[string]string, digest string) string {
	var schemas strings.Builder
	var validators strings.Builder
	for _, name := range schemaNames(spec) {
		value := spec.Components.Schemas[name]
		if value.Type == "object" || schemaType(value) == "object" {
			fmt.Fprintf(&schemas, "export interface %s %s\n\n", name, tsObject(value))
		} else {
			fmt.Fprintf(&schemas, "export type %s = %s;\n\n", name, tsType(value))
		}
		fmt.Fprintf(&validators, "function is%s(value: unknown): value is %s {\n  return %s;\n}\n\n", name, name, tsCheck("value", value))
	}
	template := `// Code generated from console-gateway.yaml (SHA256 {{SHA}}); DO NOT EDIT.
{{SCHEMAS}}function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isUUID(value: unknown): value is string {
  // PostgreSQL's uuid type and google/uuid.Parse accept the canonical
  // 8-4-4-4-12 hexadecimal shape even when deterministic imported values do
  // not carry RFC version/variant marker bits.
  return typeof value === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);
}

function isDateTime(value: unknown): value is string {
  return typeof value === "string" && !Number.isNaN(Date.parse(value));
}

{{VALIDATORS}}export type ConsoleSessionResult =
  | { state: "authenticated"; session: ConsoleSession }
  | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchConsoleSession(): Promise<ConsoleSessionResult> {
  try {
    const response = await fetch("{{SESSION_ROUTE}}", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleSession(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", session: envelope.data };
  } catch {
    return { state: "unavailable" };
  }
}

export type ConsoleOverviewResult =
  | { state: "authenticated"; overview: ConsoleOverview }
  | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchConsoleOverview(): Promise<ConsoleOverviewResult> {
  try {
    const response = await fetch("{{OVERVIEW_ROUTE}}", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleOverview(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", overview: envelope.data };
  } catch {
    return { state: "unavailable" };
  }
}

export type PlatformOperationsResult =
  | { state: "authenticated"; operations: PlatformOperationsSnapshot }
  | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchPlatformOperations(): Promise<PlatformOperationsResult> {
  try {
    const response = await fetch("{{OPERATIONS_ROUTE}}", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isPlatformOperationsSnapshot(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", operations: envelope.data };
  } catch {
    return { state: "unavailable" };
  }
}

export type PlatformOperationWriteResult =
  | { state: "succeeded" | "unknown"; result: PlatformOperationResult }
  | { state: "signed_out" | "denied" | "conflict" | "invalid" | "not_found" | "unavailable" };

async function writePlatformOperation(path: string, body: unknown, idempotencyKey: string): Promise<PlatformOperationWriteResult> {
  try {
    const response = await fetch(path, { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(body) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (response.status === 409) return { state: "conflict" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unknown" as const, result: { operation: path.includes("access-updates") ? "access_update" : "session_revoke", status: "unknown" } };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isPlatformOperationResult(envelope.data)) return { state: "unknown" as const, result: { operation: path.includes("access-updates") ? "access_update" : "session_revoke", status: "unknown" } };
    return { state: envelope.data.status, result: envelope.data };
  } catch {
    return { state: "unknown" as const, result: { operation: path.includes("access-updates") ? "access_update" : "session_revoke", status: "unknown" } };
  }
}

export function revokePlatformSession(sessionID: string, idempotencyKey: string): Promise<PlatformOperationWriteResult> {
  return writePlatformOperation("{{REVOKE_SESSION_ROUTE}}".replace("{session_id}", encodeURIComponent(sessionID)), { expected_active: true }, idempotencyKey);
}

export function updatePlatformAccess(userID: string, input: UpdatePlatformAccessRequest, idempotencyKey: string): Promise<PlatformOperationWriteResult> {
  return writePlatformOperation("{{UPDATE_ACCESS_ROUTE}}".replace("{user_id}", encodeURIComponent(userID)), input, idempotencyKey);
}

export async function resolvePlatformOperation(operation: "session_revoke" | "access_update", idempotencyKey: string): Promise<PlatformOperationWriteResult> {
  try {
    const path = "{{OPERATION_STATUS_ROUTE}}".replace("{operation}", operation);
    const response = await fetch(path, { credentials: "same-origin", headers: { Accept: "application/json", "Idempotency-Key": idempotencyKey } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isPlatformOperationResult(envelope.data)) return { state: "unavailable" };
    return { state: envelope.data.status, result: envelope.data };
  } catch {
    return { state: "unavailable" };
  }
}

export type AccountLookupResult =
  | { state: "authenticated"; account: ConsoleLookedUpAccount | null }
  | { state: "signed_out" | "denied" | "invalid" | "unavailable" };

export async function lookupConsoleAccount(email: string): Promise<AccountLookupResult> {
  try {
    const response = await fetch("{{ACCOUNT_LOOKUP_ROUTE}}", { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify({ email }) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleAccountLookupResult(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", account: envelope.data.account };
  } catch { return { state: "unavailable" }; }
}

export type NoticeSnapshotResult =
  | { state: "authenticated"; snapshot: NoticeSnapshot }
  | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchNoticeSnapshot(): Promise<NoticeSnapshotResult> {
  try {
    const response = await fetch("{{NOTICE_ROUTE}}", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isNoticeSnapshot(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", snapshot: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export type NoticeWriteResult = { state: "succeeded"; result: Record<string, unknown> } | { state: "signed_out" | "denied" | "conflict" | "invalid" | "not_found" | "unknown" | "unavailable" };

async function writeNotice(path: string, input: unknown, idempotencyKey: string): Promise<NoticeWriteResult> {
  try {
    const response = await fetch(path, { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (response.status === 409) return { state: "conflict" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unknown" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isRecord(envelope.data)) return { state: "unknown" };
    return { state: "succeeded", result: envelope.data };
  } catch { return { state: "unknown" }; }
}

export function createNoticeSource(input: CreateNoticeSourceRequest, idempotencyKey: string): Promise<NoticeWriteResult> { return writeNotice("{{NOTICE_SOURCE_ROUTE}}", input, idempotencyKey); }
export function createNoticeVersion(sourceID: string, input: CreateNoticeVersionRequest, idempotencyKey: string): Promise<NoticeWriteResult> { return writeNotice("{{NOTICE_VERSION_ROUTE}}".replace("{source_id}", encodeURIComponent(sourceID)), input, idempotencyKey); }
export function reviewNoticeVersion(versionID: string, input: NoticeReviewRequest, idempotencyKey: string): Promise<NoticeWriteResult> { return writeNotice("{{NOTICE_REVIEW_ROUTE}}".replace("{version_id}", encodeURIComponent(versionID)), input, idempotencyKey); }
export function distributeNoticeVersion(versionID: string, input: NoticeDistributionRequest, idempotencyKey: string): Promise<NoticeWriteResult> { return writeNotice("{{NOTICE_DISTRIBUTION_ROUTE}}".replace("{version_id}", encodeURIComponent(versionID)), input, idempotencyKey); }

export async function resolveNoticeOperation(operation: "source_create" | "version_create" | "review" | "distribution", idempotencyKey: string): Promise<NoticeWriteResult> {
  try {
    const response = await fetch("{{NOTICE_OPERATION_ROUTE}}".replace("{operation}", operation), { credentials: "same-origin", headers: { Accept: "application/json", "Idempotency-Key": idempotencyKey } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 409) return { state: "conflict" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isRecord(envelope.data)) return { state: "unavailable" };
    return envelope.data.status === "unknown" ? { state: "unknown" } : { state: "succeeded", result: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export type LibraryWorkspaceResult = { state: "authenticated"; workspace: LibraryWorkspace } | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchLibraryWorkspace(): Promise<LibraryWorkspaceResult> {
  try {
    const response = await fetch("{{LIBRARY_ROUTE}}", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isLibraryWorkspace(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", workspace: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export type LibraryWriteResult = { state: "succeeded" | "unknown"; result?: LibraryOperationResult } | { state: "signed_out" | "denied" | "conflict" | "invalid" | "unavailable" };

export async function executeLibraryCommand(input: LibraryCommand, idempotencyKey: string): Promise<LibraryWriteResult> {
  try {
    const response = await fetch("{{LIBRARY_COMMAND_ROUTE}}", { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 409) return { state: "conflict" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unknown" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isLibraryOperationResult(envelope.data)) return { state: "unknown" };
    return { state: envelope.data.state, result: envelope.data };
  } catch { return { state: "unknown" }; }
}

export async function resolveLibraryOperation(operation: LibraryCommandKind, idempotencyKey: string): Promise<LibraryWriteResult> {
  try {
    const response = await fetch("{{LIBRARY_OPERATION_ROUTE}}".replace("{operation}", operation), { credentials: "same-origin", headers: { Accept: "application/json", "Idempotency-Key": idempotencyKey } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 409) return { state: "conflict" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isLibraryOperationResult(envelope.data)) return { state: "unavailable" };
    return { state: envelope.data.state, result: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export type FoodWorkspaceResult = { state: "authenticated"; workspace: FoodWorkspace } | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchFoodWorkspace(campus?: FoodCampus): Promise<FoodWorkspaceResult> {
  try {
    const route = "{{FOOD_ROUTE}}" + (campus ? "?campus=" + encodeURIComponent(campus) : "");
    const response = await fetch(route, { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isFoodWorkspace(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", workspace: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export type FoodWriteResult = { state: "succeeded" | "unknown"; result?: FoodOperationResult } | { state: "signed_out" | "denied" | "conflict" | "invalid" | "unavailable" };

export async function executeFoodCommand(input: FoodCommand, idempotencyKey: string): Promise<FoodWriteResult> {
  try {
    const response = await fetch("{{FOOD_COMMAND_ROUTE}}", { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 409) return { state: "conflict" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unknown" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isFoodOperationResult(envelope.data)) return { state: "unknown" };
    return { state: envelope.data.state, result: envelope.data };
  } catch { return { state: "unknown" }; }
}

export async function resolveFoodOperation(operation: FoodCommandKind, idempotencyKey: string): Promise<FoodWriteResult> {
  try {
    const response = await fetch("{{FOOD_OPERATION_ROUTE}}".replace("{operation}", operation), { credentials: "same-origin", headers: { Accept: "application/json", "Idempotency-Key": idempotencyKey } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 409) return { state: "conflict" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isFoodOperationResult(envelope.data)) return { state: "unavailable" };
    return { state: envelope.data.state, result: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export type AccountMembershipReadResult =
  | { state: "authenticated"; membership: ConsoleAccountMembership }
  | { state: "signed_out" | "denied" | "not_found" | "invalid" | "unavailable" };

export type AccountMembershipPageResult =
  | { state: "authenticated"; page: ConsoleMembershipAccountPage }
  | { state: "signed_out" | "denied" | "invalid" | "unavailable" };

export async function searchAccountMemberships(input: ConsoleMembershipAccountSearchRequest): Promise<AccountMembershipPageResult> {
  try {
    const response = await fetch("{{ACCOUNT_MEMBERSHIP_SEARCH_ROUTE}}", { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify(input) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleMembershipAccountPage(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", page: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export async function fetchAccountMembership(userID: string): Promise<AccountMembershipReadResult> {
  try {
    const response = await fetch("{{ACCOUNT_MEMBERSHIP_ROUTE}}".replace("{user_id}", encodeURIComponent(userID)), { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleAccountMembershipEnvelope(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", membership: envelope.data.membership };
  } catch { return { state: "unavailable" }; }
}

export type AccountMembershipWriteResult =
  | { state: "succeeded"; membership: ConsoleAccountMembership }
  | { state: "signed_out" | "denied" | "not_found" | "conflict" | "invalid" | "unavailable" };

async function writeAccountMembership(path: string, input: ConsoleMembershipMutationRequest, idempotencyKey: string): Promise<AccountMembershipWriteResult> {
  try {
    const response = await fetch(path, { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (response.status === 409) return { state: "conflict" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleAccountMembershipEnvelope(envelope.data)) return { state: "unavailable" };
    return { state: "succeeded", membership: envelope.data.membership };
  } catch { return { state: "unavailable" }; }
}

export function grantAccountMembership(userID: string, input: ConsoleMembershipMutationRequest, idempotencyKey: string): Promise<AccountMembershipWriteResult> {
  return writeAccountMembership("{{ACCOUNT_MEMBERSHIP_GRANTS_ROUTE}}".replace("{user_id}", encodeURIComponent(userID)), input, idempotencyKey);
}

export function revokeAccountMembership(userID: string, input: ConsoleMembershipMutationRequest, idempotencyKey: string): Promise<AccountMembershipWriteResult> {
  return writeAccountMembership("{{ACCOUNT_MEMBERSHIP_REVOCATIONS_ROUTE}}".replace("{user_id}", encodeURIComponent(userID)), input, idempotencyKey);
}

export type AccountPointAdjustmentWriteResult =
  | { state: "succeeded"; result: ConsoleAccountPointAdjustmentResult }
  | { state: "signed_out" | "denied" | "conflict" | "invalid" | "unavailable" };

export async function adjustAccountPoints(input: ConsolePointAdjustmentRequest, idempotencyKey: string): Promise<AccountPointAdjustmentWriteResult> {
  try {
    const response = await fetch("{{ACCOUNT_POINT_ADJUSTMENTS_ROUTE}}", { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 409) return { state: "conflict" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleAccountPointAdjustmentResult(envelope.data)) return { state: "unavailable" };
    return { state: "succeeded", result: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export type AccountTicketQueueResult = { state: "authenticated"; queue: ConsoleAccountTicketQueue } | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchAccountTicketQueue(): Promise<AccountTicketQueueResult> {
  try {
    const response = await fetch("{{ACCOUNT_TICKETS_ROUTE}}", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleAccountTicketQueue(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", queue: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export type AccountTicketDetailResult = { state: "authenticated"; ticket: ConsoleAccountTicketDetail } | { state: "signed_out" | "denied" | "not_found" | "invalid" | "unavailable" };

export async function fetchAccountTicket(ticketID: string): Promise<AccountTicketDetailResult> {
  try {
    const response = await fetch("{{ACCOUNT_TICKET_ROUTE}}".replace("{ticket_id}", encodeURIComponent(ticketID)), { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleAccountTicketDetail(envelope.data)) return { state: "unavailable" };
    return { state: "authenticated", ticket: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export type AccountTicketWriteResult = { state: "succeeded"; ticket: ConsoleAccountTicket } | { state: "signed_out" | "denied" | "not_found" | "conflict" | "invalid" | "unavailable" };

async function writeAccountTicket(path: string, input: ConsoleOperatorReplyRequest | ConsoleTicketTransitionRequest, idempotencyKey: string): Promise<AccountTicketWriteResult> {
  try {
    const response = await fetch(path, { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input) });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 404) return { state: "not_found" };
    if (response.status === 409) return { state: "conflict" };
    if (response.status === 400) return { state: "invalid" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isConsoleAccountTicketCommandResult(envelope.data)) return { state: "unavailable" };
    return { state: "succeeded", ticket: envelope.data.ticket };
  } catch { return { state: "unavailable" }; }
}

export function replyToAccountTicket(ticketID: string, input: ConsoleOperatorReplyRequest, idempotencyKey: string): Promise<AccountTicketWriteResult> {
  return writeAccountTicket("{{ACCOUNT_TICKET_REPLIES_ROUTE}}".replace("{ticket_id}", encodeURIComponent(ticketID)), input, idempotencyKey);
}

export function transitionAccountTicket(ticketID: string, input: ConsoleTicketTransitionRequest, idempotencyKey: string): Promise<AccountTicketWriteResult> {
  return writeAccountTicket("{{ACCOUNT_TICKET_TRANSITIONS_ROUTE}}".replace("{ticket_id}", encodeURIComponent(ticketID)), input, idempotencyKey);
}

export async function logoutConsoleSession(): Promise<void> {
  const response = await fetch("{{LOGOUT_ROUTE}}", { method: "POST", credentials: "same-origin" });
  if (!response.ok) throw new Error("Console logout failed");
}

export function consoleLoginHref(): string {
  const returnTo = window.location.pathname + window.location.search + window.location.hash;
  return "{{LOGIN_ROUTE}}?return_to=" + encodeURIComponent(returnTo);
}
`
	replacements := map[string]string{
		"{{SHA}}": digest, "{{SCHEMAS}}": schemas.String(), "{{VALIDATORS}}": validators.String(),
		"{{SESSION_ROUTE}}": routes["getConsoleSession"], "{{OVERVIEW_ROUTE}}": routes["getConsoleOverview"], "{{OPERATIONS_ROUTE}}": routes["getConsolePlatformOperations"], "{{REVOKE_SESSION_ROUTE}}": routes["revokeConsolePlatformSession"], "{{UPDATE_ACCESS_ROUTE}}": routes["updateConsolePlatformAccess"], "{{OPERATION_STATUS_ROUTE}}": routes["getConsolePlatformOperationStatus"], "{{ACCOUNT_LOOKUP_ROUTE}}": routes["lookupConsoleAccount"], "{{LOGOUT_ROUTE}}": routes["logoutConsoleSession"], "{{LOGIN_ROUTE}}": routes["beginConsoleLogin"],
		"{{NOTICE_ROUTE}}": routes["getConsoleNotices"], "{{NOTICE_SOURCE_ROUTE}}": routes["createConsoleNoticeSource"], "{{NOTICE_VERSION_ROUTE}}": routes["createConsoleNoticeVersion"], "{{NOTICE_REVIEW_ROUTE}}": routes["reviewConsoleNoticeVersion"], "{{NOTICE_DISTRIBUTION_ROUTE}}": routes["distributeConsoleNoticeVersion"], "{{NOTICE_OPERATION_ROUTE}}": routes["getConsoleNoticeOperationStatus"],
		"{{LIBRARY_ROUTE}}": routes["getConsoleLibraryWorkspace"], "{{LIBRARY_COMMAND_ROUTE}}": routes["executeConsoleLibraryCommand"], "{{LIBRARY_OPERATION_ROUTE}}": routes["getConsoleLibraryOperationStatus"],
		"{{FOOD_ROUTE}}": routes["getConsoleFoodWorkspace"], "{{FOOD_COMMAND_ROUTE}}": routes["executeConsoleFoodCommand"], "{{FOOD_OPERATION_ROUTE}}": routes["getConsoleFoodOperationStatus"],
		"{{ACCOUNT_MEMBERSHIP_ROUTE}}": routes["getConsoleAccountMembership"], "{{ACCOUNT_MEMBERSHIP_SEARCH_ROUTE}}": routes["searchConsoleAccountMemberships"], "{{ACCOUNT_MEMBERSHIP_GRANTS_ROUTE}}": routes["grantConsoleAccountMembership"], "{{ACCOUNT_MEMBERSHIP_REVOCATIONS_ROUTE}}": routes["revokeConsoleAccountMembership"],
		"{{ACCOUNT_POINT_ADJUSTMENTS_ROUTE}}": routes["adjustConsoleAccountPoints"],
		"{{ACCOUNT_TICKETS_ROUTE}}":           routes["getConsoleAccountTickets"], "{{ACCOUNT_TICKET_ROUTE}}": routes["getConsoleAccountTicket"], "{{ACCOUNT_TICKET_REPLIES_ROUTE}}": routes["replyConsoleAccountTicket"], "{{ACCOUNT_TICKET_TRANSITIONS_ROUTE}}": routes["transitionConsoleAccountTicket"],
	}
	for old, replacement := range replacements {
		template = strings.ReplaceAll(template, old, replacement)
	}
	return template
}

func tsObject(value schema) string {
	if value.Type != "object" {
		return tsType(value)
	}
	required := stringSet(value.Required)
	var output strings.Builder
	output.WriteString("{\n")
	for _, property := range sortedProperties(value.Properties) {
		optional := ""
		if !required[property] {
			optional = "?"
		}
		fmt.Fprintf(&output, "  %s%s: %s;\n", property, optional, tsType(value.Properties[property]))
	}
	output.WriteString("}")
	return output.String()
}

func tsType(value schema) string {
	if value.Ref != "" {
		return refName(value.Ref)
	}
	if len(value.OneOf) > 0 {
		alternatives := make([]string, 0, len(value.OneOf))
		seen := make(map[string]struct{}, len(value.OneOf))
		for _, item := range value.OneOf {
			alternative := tsType(item)
			if _, exists := seen[alternative]; !exists {
				alternatives = append(alternatives, alternative)
				seen[alternative] = struct{}{}
			}
		}
		return strings.Join(alternatives, " | ")
	}
	switch value.Type {
	case "string":
		if len(value.Enum) > 0 {
			encoded := make([]string, 0, len(value.Enum))
			for _, item := range value.Enum {
				quoted, _ := json.Marshal(item)
				encoded = append(encoded, string(quoted))
			}
			return strings.Join(encoded, " | ")
		}
		if literal, ok := value.Const.(string); ok {
			encoded, _ := json.Marshal(literal)
			return string(encoded)
		}
		return "string"
	case "boolean":
		return "boolean"
	case "integer":
		return "number"
	case "array":
		if value.Items == nil {
			return "unknown[]"
		}
		return "Array<" + tsType(*value.Items) + ">"
	case "object":
		return tsObject(value)
	case "null":
		return "null"
	default:
		return "unknown"
	}
}

func tsCheck(expression string, value schema) string {
	checks := []string{tsCheckBase(expression, value)}
	for _, item := range value.AllOf {
		checks = append(checks, tsCheck(expression, item))
	}
	if len(value.AnyOf) > 0 {
		alternatives := make([]string, 0, len(value.AnyOf))
		for _, item := range value.AnyOf {
			alternatives = append(alternatives, "("+tsCheck(expression, item)+")")
		}
		checks = append(checks, "("+strings.Join(alternatives, " || ")+")")
	}
	if len(value.OneOf) > 0 {
		alternatives := make([]string, 0, len(value.OneOf))
		for _, item := range value.OneOf {
			alternatives = append(alternatives, "("+tsCheck(expression, item)+")")
		}
		checks = append(checks, "(["+strings.Join(alternatives, ", ")+"].filter(Boolean).length === 1)")
	}
	if value.If != nil && value.Then != nil {
		checks = append(checks, "(!("+tsCheck(expression, *value.If)+") || "+tsCheck(expression, *value.Then)+")")
	}
	if value.Contains != nil {
		count := expression + ".filter((item) => " + tsCheck("item", *value.Contains) + ").length"
		if value.MinContains > 0 {
			checks = append(checks, fmt.Sprintf("%s >= %d", count, value.MinContains))
		}
		if value.MaxContains > 0 {
			checks = append(checks, fmt.Sprintf("%s <= %d", count, value.MaxContains))
		}
	}
	return strings.Join(checks, " && ")
}

func tsCheckBase(expression string, value schema) string {
	if value.Ref != "" {
		return "is" + refName(value.Ref) + "(" + expression + ")"
	}
	if value.Const != nil {
		encoded, _ := json.Marshal(value.Const)
		return expression + " === " + string(encoded)
	}
	switch schemaType(value) {
	case "string":
		if len(value.Enum) > 0 {
			encoded, _ := json.Marshal(value.Enum)
			return "typeof " + expression + ` === "string" && ` + string(encoded) + ".includes(" + expression + ")"
		}
		if literal, ok := value.Const.(string); ok {
			encoded, _ := json.Marshal(literal)
			return expression + " === " + string(encoded)
		}
		if value.Format == "uuid" {
			return "isUUID(" + expression + ")"
		}
		if value.Format == "date-time" {
			return "isDateTime(" + expression + ")"
		}
		check := "typeof " + expression + ` === "string"`
		if value.MinLength > 0 {
			check += fmt.Sprintf(" && %s.length >= %d", expression, value.MinLength)
		}
		if value.MaxLength > 0 {
			check += fmt.Sprintf(" && %s.length <= %d", expression, value.MaxLength)
		}
		if value.Pattern != "" {
			encoded, _ := json.Marshal(value.Pattern)
			check += " && new RegExp(" + string(encoded) + ").test(" + expression + ")"
		}
		return check
	case "boolean":
		return "typeof " + expression + ` === "boolean"`
	case "integer":
		check := "typeof " + expression + ` === "number" && Number.isSafeInteger(` + expression + ")"
		if value.Minimum != nil {
			check += fmt.Sprintf(" && %s >= %g", expression, *value.Minimum)
		}
		if value.Maximum != nil {
			check += fmt.Sprintf(" && %s <= %g", expression, *value.Maximum)
		}
		return check
	case "array":
		if value.Items == nil {
			return "Array.isArray(" + expression + ")"
		}
		checks := []string{"Array.isArray(" + expression + ")"}
		if value.MinItems > 0 {
			checks = append(checks, fmt.Sprintf("%s.length >= %d", expression, value.MinItems))
		}
		if value.MaxItems > 0 {
			checks = append(checks, fmt.Sprintf("%s.length <= %d", expression, value.MaxItems))
		}
		checks = append(checks, expression+".every((item) => "+tsCheck("item", *value.Items)+")")
		return strings.Join(checks, " && ")
	case "object":
		checks := []string{"isRecord(" + expression + ")"}
		required := stringSet(value.Required)
		for _, property := range sortedProperties(value.Properties) {
			propertyExpression := expression + "[" + strconvQuote(property) + "]"
			check := tsCheck(propertyExpression, value.Properties[property])
			if required[property] {
				checks = append(checks, strconvQuote(property)+" in "+expression+" && "+check)
			} else {
				checks = append(checks, "(!("+strconvQuote(property)+" in "+expression+") || "+check+")")
			}
		}
		if value.AdditionalProperties != nil && !*value.AdditionalProperties {
			properties := sortedProperties(value.Properties)
			if len(properties) == 0 {
				checks = append(checks, "Object.keys("+expression+").length === 0")
			} else {
				allowed, _ := json.Marshal(properties)
				checks = append(checks, "Object.keys("+expression+").every((key) => "+string(allowed)+".includes(key))")
			}
		}
		return strings.Join(checks, " && ")
	case "null":
		return expression + " === null"
	default:
		return "true"
	}
}

func validateSchema(value schema, candidate any, schemas map[string]schema) error {
	if err := validateSchemaBase(value, candidate, schemas); err != nil {
		return err
	}
	for _, item := range value.AllOf {
		if err := validateSchema(item, candidate, schemas); err != nil {
			return err
		}
	}
	if len(value.AnyOf) > 0 {
		matched := false
		for _, item := range value.AnyOf {
			if validateSchema(item, candidate, schemas) == nil {
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("value does not satisfy anyOf")
		}
	}
	if len(value.OneOf) > 0 {
		matches := 0
		for _, item := range value.OneOf {
			if validateSchema(item, candidate, schemas) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("value satisfies %d oneOf alternatives", matches)
		}
	}
	if value.If != nil && value.Then != nil && validateSchema(*value.If, candidate, schemas) == nil {
		if err := validateSchema(*value.Then, candidate, schemas); err != nil {
			return err
		}
	}
	if value.Contains != nil {
		items, ok := candidate.([]any)
		if !ok {
			return errors.New("contains requires array")
		}
		count := 0
		for _, item := range items {
			if validateSchema(*value.Contains, item, schemas) == nil {
				count++
			}
		}
		if value.MinContains > 0 && count < value.MinContains {
			return fmt.Errorf("contains matched %d, want at least %d", count, value.MinContains)
		}
		if value.MaxContains > 0 && count > value.MaxContains {
			return fmt.Errorf("contains matched %d, want at most %d", count, value.MaxContains)
		}
	}
	return nil
}

func validateSchemaBase(value schema, candidate any, schemas map[string]schema) error {
	if value.Ref != "" {
		referenced, ok := schemas[refName(value.Ref)]
		if !ok {
			return fmt.Errorf("unknown schema reference %s", value.Ref)
		}
		return validateSchema(referenced, candidate, schemas)
	}
	if value.Const != nil && !reflect.DeepEqual(value.Const, candidate) {
		return fmt.Errorf("expected constant %v", value.Const)
	}
	switch schemaType(value) {
	case "string":
		text, ok := candidate.(string)
		if !ok {
			return errors.New("expected string")
		}
		if literal, ok := value.Const.(string); ok && text != literal {
			return fmt.Errorf("expected constant %q", literal)
		}
		if len(value.Enum) > 0 && !stringSet(value.Enum)[text] {
			return fmt.Errorf("value %q is outside enum", text)
		}
		if value.Format == "uuid" {
			_, err := uuid.Parse(text)
			return err
		}
		if value.Format == "date-time" {
			_, err := time.Parse(time.RFC3339, text)
			return err
		}
		if value.MaxLength > 0 && len([]rune(text)) > value.MaxLength {
			return fmt.Errorf("string exceeds %d characters", value.MaxLength)
		}
		if value.MinLength > 0 && len([]rune(text)) < value.MinLength {
			return fmt.Errorf("string is shorter than %d characters", value.MinLength)
		}
		if value.Pattern != "" {
			matched, err := regexp.MatchString(value.Pattern, text)
			if err != nil {
				return fmt.Errorf("invalid pattern %q: %w", value.Pattern, err)
			}
			if !matched {
				return fmt.Errorf("string does not match %s", value.Pattern)
			}
		}
	case "boolean":
		if _, ok := candidate.(bool); !ok {
			return errors.New("expected boolean")
		}
	case "integer":
		if _, ok := candidate.(int); !ok {
			return errors.New("expected integer")
		}
	case "array":
		items, ok := candidate.([]any)
		if !ok {
			return errors.New("expected array")
		}
		if value.MinItems > 0 && len(items) < value.MinItems {
			return fmt.Errorf("expected at least %d items", value.MinItems)
		}
		if value.MaxItems > 0 && len(items) > value.MaxItems {
			return fmt.Errorf("expected at most %d items", value.MaxItems)
		}
		if value.Items != nil {
			for _, item := range items {
				if err := validateSchema(*value.Items, item, schemas); err != nil {
					return err
				}
			}
		}
	case "object":
		object, ok := candidate.(map[string]any)
		if !ok {
			return errors.New("expected object")
		}
		for _, required := range value.Required {
			if _, exists := object[required]; !exists {
				return fmt.Errorf("missing required property %s", required)
			}
		}
		for property, child := range object {
			propertySchema, exists := value.Properties[property]
			if !exists {
				if value.AdditionalProperties != nil && !*value.AdditionalProperties {
					return fmt.Errorf("unexpected property %s", property)
				}
				continue
			}
			if err := validateSchema(propertySchema, child, schemas); err != nil {
				return fmt.Errorf("property %s: %w", property, err)
			}
		}
	}
	return nil
}

func schemaType(value schema) string {
	if value.Type != "" {
		return value.Type
	}
	if len(value.Properties) > 0 || len(value.Required) > 0 || value.AdditionalProperties != nil {
		return "object"
	}
	if value.Items != nil || value.Contains != nil || value.MinItems > 0 || value.MaxItems > 0 {
		return "array"
	}
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

func refName(ref string) string { return ref[strings.LastIndex(ref, "/")+1:] }

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
	if string(output) == "Id" {
		return "ID"
	}
	return strings.ReplaceAll(string(output), "Id", "ID")
}

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func write(path string, content []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
