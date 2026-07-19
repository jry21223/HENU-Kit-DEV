// Code generated from console-gateway.yaml (SHA256 61b9cb7efd9827f5a638d5b027f5696417dbca5223a0eebc3fd21507acdaeb74); DO NOT EDIT.
export interface ConsoleAccessContext {
  permissions: Array<string>;
  scopes: Array<ConsoleScope>;
  verified_at: string;
}

export interface ConsoleModuleMetric {
  hint?: string;
  label: string;
  value: string;
}

export interface ConsoleModuleSummary {
  as_of?: string;
  id: "portal" | "platform" | "notice" | "library" | "quizcraft" | "food";
  last_success_at?: string;
  metrics: Array<ConsoleModuleMetric>;
  request_id: string;
  status: "ok" | "empty" | "partial" | "stale" | "unavailable";
  status_message: string;
}

export interface ConsoleOverview {
  generated_at: string;
  modules: Array<ConsoleModuleSummary>;
}

export interface ConsoleScope {
  kind: "platform";
}

export interface ConsoleSession {
  access_context: ConsoleAccessContext;
  expires_at: string;
  user: {
  id: string;
};
}

export interface ErrorEnvelope {
  error: ErrorObject;
  request_id: string;
}

export interface ErrorObject {
  code: string;
  message: string;
}

export interface SuccessEnvelope {
  data: unknown;
  request_id: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isUUID(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}

function isDateTime(value: unknown): value is string {
  return typeof value === "string" && !Number.isNaN(Date.parse(value));
}

function isConsoleAccessContext(value: unknown): value is ConsoleAccessContext {
  return isRecord(value) && "permissions" in value && Array.isArray(value["permissions"]) && value["permissions"].every((item) => typeof item === "string") && "scopes" in value && Array.isArray(value["scopes"]) && value["scopes"].every((item) => isConsoleScope(item)) && "verified_at" in value && isDateTime(value["verified_at"]) && Object.keys(value).every((key) => ["permissions","scopes","verified_at"].includes(key));
}

function isConsoleModuleMetric(value: unknown): value is ConsoleModuleMetric {
  return isRecord(value) && (!("hint" in value) || typeof value["hint"] === "string" && value["hint"].length <= 120) && "label" in value && typeof value["label"] === "string" && value["label"].length <= 40 && "value" in value && typeof value["value"] === "string" && value["value"].length <= 80 && Object.keys(value).every((key) => ["hint","label","value"].includes(key));
}

function isConsoleModuleSummary(value: unknown): value is ConsoleModuleSummary {
  return isRecord(value) && (!("as_of" in value) || isDateTime(value["as_of"])) && "id" in value && typeof value["id"] === "string" && ["portal","platform","notice","library","quizcraft","food"].includes(value["id"]) && (!("last_success_at" in value) || isDateTime(value["last_success_at"])) && "metrics" in value && Array.isArray(value["metrics"]) && value["metrics"].length <= 8 && value["metrics"].every((item) => isConsoleModuleMetric(item)) && "request_id" in value && typeof value["request_id"] === "string" && "status" in value && typeof value["status"] === "string" && ["ok","empty","partial","stale","unavailable"].includes(value["status"]) && "status_message" in value && typeof value["status_message"] === "string" && value["status_message"].length <= 240 && Object.keys(value).every((key) => ["as_of","id","last_success_at","metrics","request_id","status","status_message"].includes(key));
}

function isConsoleOverview(value: unknown): value is ConsoleOverview {
  return isRecord(value) && "generated_at" in value && isDateTime(value["generated_at"]) && "modules" in value && Array.isArray(value["modules"]) && value["modules"].length >= 6 && value["modules"].length <= 6 && value["modules"].every((item) => isConsoleModuleSummary(item)) && Object.keys(value).every((key) => ["generated_at","modules"].includes(key));
}

function isConsoleScope(value: unknown): value is ConsoleScope {
  return isRecord(value) && "kind" in value && value["kind"] === "platform" && Object.keys(value).every((key) => ["kind"].includes(key));
}

function isConsoleSession(value: unknown): value is ConsoleSession {
  return isRecord(value) && "access_context" in value && isConsoleAccessContext(value["access_context"]) && "expires_at" in value && isDateTime(value["expires_at"]) && "user" in value && isRecord(value["user"]) && "id" in value["user"] && isUUID(value["user"]["id"]) && Object.keys(value["user"]).every((key) => ["id"].includes(key)) && Object.keys(value).every((key) => ["access_context","expires_at","user"].includes(key));
}

function isErrorEnvelope(value: unknown): value is ErrorEnvelope {
  return isRecord(value) && "error" in value && isErrorObject(value["error"]) && "request_id" in value && typeof value["request_id"] === "string";
}

function isErrorObject(value: unknown): value is ErrorObject {
  return isRecord(value) && "code" in value && typeof value["code"] === "string" && "message" in value && typeof value["message"] === "string";
}

function isSuccessEnvelope(value: unknown): value is SuccessEnvelope {
  return isRecord(value) && "data" in value && true && "request_id" in value && typeof value["request_id"] === "string";
}

export type ConsoleSessionResult =
  | { state: "authenticated"; session: ConsoleSession }
  | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchConsoleSession(): Promise<ConsoleSessionResult> {
  try {
    const response = await fetch("/api/v1/session", { credentials: "same-origin", headers: { Accept: "application/json" } });
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

export async function fetchConsoleOverview(): Promise<ConsoleOverview> {
  const response = await fetch("/api/v1/overview", { credentials: "same-origin", headers: { Accept: "application/json" } });
  if (!response.ok) throw new Error("Console overview failed");
  const envelope: unknown = await response.json();
  if (!isSuccessEnvelope(envelope) || !isConsoleOverview(envelope.data)) throw new Error("Console overview contract mismatch");
  return envelope.data;
}

export async function logoutConsoleSession(): Promise<void> {
  const response = await fetch("/api/v1/session/logout", { method: "POST", credentials: "same-origin" });
  if (!response.ok) throw new Error("Console logout failed");
}

export function consoleLoginHref(): string {
  const returnTo = window.location.pathname + window.location.search + window.location.hash;
  return "/api/v1/auth/login?return_to=" + encodeURIComponent(returnTo);
}
