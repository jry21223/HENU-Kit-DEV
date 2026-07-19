// Code generated from console-gateway.yaml (SHA256 b5a09cf14075494a97fa8a854dc95baf9c822fb2abd900fec6b71fb92da3c74d); DO NOT EDIT.
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
  kind: "platform" | "product";
  product_code?: string;
}

export interface ConsoleSession {
  access_context: ConsoleAccessContext;
  expires_at: string;
  user: {
  id: string;
};
}

export interface CreateNoticeSourceRequest {
  canonical_url: string;
  code: string;
  name: string;
}

export interface CreateNoticeVersionRequest {
  body: string;
  source_published_at?: string;
  source_url: string;
  title: string;
}

export interface ErrorEnvelope {
  error: ErrorObject;
  request_id: string;
}

export interface ErrorObject {
  code: string;
  message: string;
}

export interface NoticeAudience {
  kind?: "all_students" | "college" | "role";
  value?: string;
}

export interface NoticeDistributionRequest {
  audience: NoticeAudience;
  channel: "in_app" | "email";
  expected_revision: number;
}

export interface NoticeReviewRequest {
  decision: "approved" | "rejected";
  expected_revision: number;
  note?: string;
}

export interface NoticeSnapshot {
  generated_at: string;
  items: Array<NoticeVersion>;
}

export interface NoticeSource {
  code: string;
  id: string;
  name: string;
}

export interface NoticeVersion {
  body: string;
  content_hash: string;
  created_at: string;
  distribution_count: number;
  distribution_status?: "queued" | "processing" | "delivered" | "failed";
  id: string;
  revision: number;
  source: NoticeSource;
  source_published_at?: string;
  source_url: string;
  state: "pending_review" | "approved" | "rejected" | "distributed";
  title: string;
  version: number;
}

export interface PlatformAccessGrantInput {
  role_code: string;
  scope: PlatformScope;
}

export interface PlatformOperationResult {
  operation: "session_revoke" | "access_update";
  resource_id?: string;
  resource_version?: number;
  status: "succeeded" | "unknown";
}

export interface PlatformOperationsAccount {
  authorization_revision: number;
  created_at: string;
  email_verified: boolean;
  grants: Array<PlatformAccessGrantInput>;
  id: string;
  status: "active" | "suspended" | "deleted";
}

export interface PlatformOperationsAuditEvent {
  actor_user_id: string;
  created_at: string;
  decision: "allowed" | "denied";
  permission_code: string;
  reason_code: string;
  request_id: string;
  target_kind: "platform" | "product" | "resource";
  target_product_code?: string;
  target_resource_id?: string;
  target_resource_type?: string;
}

export interface PlatformOperationsDependencies {
  postgres: "ready" | "unavailable";
  redis: "ready" | "unavailable";
}

export interface PlatformOperationsInboxItem {
  created_at: string;
  id: string;
  owner_user_id?: string;
  priority: "low" | "normal" | "high" | "urgent";
  sla_due_at?: string;
  source_product_code: string;
  source_resource_id: string;
  source_resource_type: string;
  source_resource_url?: string;
  status: "open" | "in_progress" | "blocked" | "resolved" | "archived";
  updated_at: string;
  version: number;
}

export interface PlatformOperationsMailStatus {
  accepted: number;
  dead_letters: number;
  delivered: number;
  failed: number;
  pending: number;
  processing: number;
  retry_due: number;
}

export interface PlatformOperationsSession {
  client_id?: string;
  expires_at: string;
  id: string;
  kind: "core" | "client_exchange";
  last_seen_at: string;
  revoked_at?: string;
  user_id: string;
}

export interface PlatformOperationsSnapshot {
  access_context: ConsoleAccessContext;
  accounts: Array<PlatformOperationsAccount>;
  audit: Array<PlatformOperationsAuditEvent>;
  dependencies: PlatformOperationsDependencies;
  generated_at: string;
  inbox_items: Array<PlatformOperationsInboxItem>;
  mail: PlatformOperationsMailStatus;
  sessions: Array<PlatformOperationsSession>;
}

export interface PlatformScope {
  kind: "platform" | "product" | "resource";
  product_code?: string;
  resource_id?: string;
  resource_type?: string;
}

export interface RevokePlatformSessionRequest {
  expected_active: boolean;
}

export interface SuccessEnvelope {
  data: unknown;
  request_id: string;
}

export interface UpdatePlatformAccessRequest {
  expected_revision: number;
  grants: Array<PlatformAccessGrantInput>;
  status: "active" | "suspended" | "deleted";
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
  return isRecord(value) && (!("as_of" in value) || isDateTime(value["as_of"])) && "id" in value && typeof value["id"] === "string" && ["portal","platform","notice","library","quizcraft","food"].includes(value["id"]) && (!("last_success_at" in value) || isDateTime(value["last_success_at"])) && "metrics" in value && Array.isArray(value["metrics"]) && value["metrics"].length <= 8 && value["metrics"].every((item) => isConsoleModuleMetric(item)) && "request_id" in value && typeof value["request_id"] === "string" && value["request_id"].length <= 120 && new RegExp("^req_[A-Za-z0-9_-]+$").test(value["request_id"]) && "status" in value && typeof value["status"] === "string" && ["ok","empty","partial","stale","unavailable"].includes(value["status"]) && "status_message" in value && typeof value["status_message"] === "string" && value["status_message"].length <= 240 && Object.keys(value).every((key) => ["as_of","id","last_success_at","metrics","request_id","status","status_message"].includes(key)) && ((isRecord(value) && "as_of" in value && true && "last_success_at" in value && true) || (isRecord(value) && "status" in value && typeof value["status"] === "string" && ["ok","empty","partial","unavailable"].includes(value["status"])));
}

function isConsoleOverview(value: unknown): value is ConsoleOverview {
  return isRecord(value) && "generated_at" in value && isDateTime(value["generated_at"]) && "modules" in value && Array.isArray(value["modules"]) && value["modules"].length >= 6 && value["modules"].length <= 6 && value["modules"].every((item) => isConsoleModuleSummary(item)) && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "portal").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "portal").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "platform").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "platform").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "notice").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "notice").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "library").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "library").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "quizcraft").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "quizcraft").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "food").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "food").length <= 1 && Object.keys(value).every((key) => ["generated_at","modules"].includes(key));
}

function isConsoleScope(value: unknown): value is ConsoleScope {
  return isRecord(value) && "kind" in value && typeof value["kind"] === "string" && ["platform","product"].includes(value["kind"]) && (!("product_code" in value) || typeof value["product_code"] === "string" && value["product_code"].length <= 80) && Object.keys(value).every((key) => ["kind","product_code"].includes(key)) && true && (!(isRecord(value) && "kind" in value && value["kind"] === "product") || isRecord(value) && "product_code" in value && typeof value["product_code"] === "string" && value["product_code"].length <= 80);
}

function isConsoleSession(value: unknown): value is ConsoleSession {
  return isRecord(value) && "access_context" in value && isConsoleAccessContext(value["access_context"]) && "expires_at" in value && isDateTime(value["expires_at"]) && "user" in value && isRecord(value["user"]) && "id" in value["user"] && isUUID(value["user"]["id"]) && Object.keys(value["user"]).every((key) => ["id"].includes(key)) && Object.keys(value).every((key) => ["access_context","expires_at","user"].includes(key));
}

function isCreateNoticeSourceRequest(value: unknown): value is CreateNoticeSourceRequest {
  return isRecord(value) && "canonical_url" in value && typeof value["canonical_url"] === "string" && new RegExp("^https://").test(value["canonical_url"]) && "code" in value && typeof value["code"] === "string" && new RegExp("^[a-z0-9][a-z0-9-]{1,62}$").test(value["code"]) && "name" in value && typeof value["name"] === "string" && value["name"].length <= 120 && Object.keys(value).every((key) => ["canonical_url","code","name"].includes(key));
}

function isCreateNoticeVersionRequest(value: unknown): value is CreateNoticeVersionRequest {
  return isRecord(value) && "body" in value && typeof value["body"] === "string" && value["body"].length <= 100000 && (!("source_published_at" in value) || isDateTime(value["source_published_at"])) && "source_url" in value && typeof value["source_url"] === "string" && new RegExp("^https://").test(value["source_url"]) && "title" in value && typeof value["title"] === "string" && value["title"].length <= 200 && Object.keys(value).every((key) => ["body","source_published_at","source_url","title"].includes(key));
}

function isErrorEnvelope(value: unknown): value is ErrorEnvelope {
  return isRecord(value) && "error" in value && isErrorObject(value["error"]) && "request_id" in value && typeof value["request_id"] === "string" && value["request_id"].length <= 100 && new RegExp("^req_[A-Za-z0-9_-]+$").test(value["request_id"]);
}

function isErrorObject(value: unknown): value is ErrorObject {
  return isRecord(value) && "code" in value && typeof value["code"] === "string" && "message" in value && typeof value["message"] === "string";
}

function isNoticeAudience(value: unknown): value is NoticeAudience {
  return isRecord(value) && (!("kind" in value) || typeof value["kind"] === "string" && ["all_students","college","role"].includes(value["kind"])) && (!("value" in value) || typeof value["value"] === "string" && value["value"].length <= 120) && Object.keys(value).every((key) => ["kind","value"].includes(key)) && ((isRecord(value) && "kind" in value && typeof value["kind"] === "string" && ["all_students"].includes(value["kind"]) && Object.keys(value).every((key) => ["kind"].includes(key))) || (isRecord(value) && "kind" in value && typeof value["kind"] === "string" && ["college","role"].includes(value["kind"]) && "value" in value && typeof value["value"] === "string" && value["value"].length <= 120 && Object.keys(value).every((key) => ["kind","value"].includes(key))));
}

function isNoticeDistributionRequest(value: unknown): value is NoticeDistributionRequest {
  return isRecord(value) && "audience" in value && isNoticeAudience(value["audience"]) && "channel" in value && typeof value["channel"] === "string" && ["in_app","email"].includes(value["channel"]) && "expected_revision" in value && typeof value["expected_revision"] === "number" && Number.isSafeInteger(value["expected_revision"]) && Object.keys(value).every((key) => ["audience","channel","expected_revision"].includes(key));
}

function isNoticeReviewRequest(value: unknown): value is NoticeReviewRequest {
  return isRecord(value) && "decision" in value && typeof value["decision"] === "string" && ["approved","rejected"].includes(value["decision"]) && "expected_revision" in value && typeof value["expected_revision"] === "number" && Number.isSafeInteger(value["expected_revision"]) && (!("note" in value) || typeof value["note"] === "string" && value["note"].length <= 1000) && Object.keys(value).every((key) => ["decision","expected_revision","note"].includes(key));
}

function isNoticeSnapshot(value: unknown): value is NoticeSnapshot {
  return isRecord(value) && "generated_at" in value && isDateTime(value["generated_at"]) && "items" in value && Array.isArray(value["items"]) && value["items"].length <= 50 && value["items"].every((item) => isNoticeVersion(item)) && Object.keys(value).every((key) => ["generated_at","items"].includes(key));
}

function isNoticeSource(value: unknown): value is NoticeSource {
  return isRecord(value) && "code" in value && typeof value["code"] === "string" && "id" in value && isUUID(value["id"]) && "name" in value && typeof value["name"] === "string" && Object.keys(value).every((key) => ["code","id","name"].includes(key));
}

function isNoticeVersion(value: unknown): value is NoticeVersion {
  return isRecord(value) && "body" in value && typeof value["body"] === "string" && "content_hash" in value && typeof value["content_hash"] === "string" && new RegExp("^[a-f0-9]{64}$").test(value["content_hash"]) && "created_at" in value && isDateTime(value["created_at"]) && "distribution_count" in value && typeof value["distribution_count"] === "number" && Number.isSafeInteger(value["distribution_count"]) && (!("distribution_status" in value) || typeof value["distribution_status"] === "string" && ["queued","processing","delivered","failed"].includes(value["distribution_status"])) && "id" in value && isUUID(value["id"]) && "revision" in value && typeof value["revision"] === "number" && Number.isSafeInteger(value["revision"]) && "source" in value && isNoticeSource(value["source"]) && (!("source_published_at" in value) || isDateTime(value["source_published_at"])) && "source_url" in value && typeof value["source_url"] === "string" && "state" in value && typeof value["state"] === "string" && ["pending_review","approved","rejected","distributed"].includes(value["state"]) && "title" in value && typeof value["title"] === "string" && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && Object.keys(value).every((key) => ["body","content_hash","created_at","distribution_count","distribution_status","id","revision","source","source_published_at","source_url","state","title","version"].includes(key));
}

function isPlatformAccessGrantInput(value: unknown): value is PlatformAccessGrantInput {
  return isRecord(value) && "role_code" in value && typeof value["role_code"] === "string" && new RegExp("^[a-z][a-z0-9-]{1,63}$").test(value["role_code"]) && "scope" in value && isPlatformScope(value["scope"]) && Object.keys(value).every((key) => ["role_code","scope"].includes(key));
}

function isPlatformOperationResult(value: unknown): value is PlatformOperationResult {
  return isRecord(value) && "operation" in value && typeof value["operation"] === "string" && ["session_revoke","access_update"].includes(value["operation"]) && (!("resource_id" in value) || isUUID(value["resource_id"])) && (!("resource_version" in value) || typeof value["resource_version"] === "number" && Number.isSafeInteger(value["resource_version"])) && "status" in value && typeof value["status"] === "string" && ["succeeded","unknown"].includes(value["status"]) && Object.keys(value).every((key) => ["operation","resource_id","resource_version","status"].includes(key));
}

function isPlatformOperationsAccount(value: unknown): value is PlatformOperationsAccount {
  return isRecord(value) && "authorization_revision" in value && typeof value["authorization_revision"] === "number" && Number.isSafeInteger(value["authorization_revision"]) && "created_at" in value && isDateTime(value["created_at"]) && "email_verified" in value && typeof value["email_verified"] === "boolean" && "grants" in value && Array.isArray(value["grants"]) && value["grants"].length <= 50 && value["grants"].every((item) => isPlatformAccessGrantInput(item)) && "id" in value && isUUID(value["id"]) && "status" in value && typeof value["status"] === "string" && ["active","suspended","deleted"].includes(value["status"]) && Object.keys(value).every((key) => ["authorization_revision","created_at","email_verified","grants","id","status"].includes(key));
}

function isPlatformOperationsAuditEvent(value: unknown): value is PlatformOperationsAuditEvent {
  return isRecord(value) && "actor_user_id" in value && isUUID(value["actor_user_id"]) && "created_at" in value && isDateTime(value["created_at"]) && "decision" in value && typeof value["decision"] === "string" && ["allowed","denied"].includes(value["decision"]) && "permission_code" in value && typeof value["permission_code"] === "string" && "reason_code" in value && typeof value["reason_code"] === "string" && "request_id" in value && typeof value["request_id"] === "string" && "target_kind" in value && typeof value["target_kind"] === "string" && ["platform","product","resource"].includes(value["target_kind"]) && (!("target_product_code" in value) || typeof value["target_product_code"] === "string") && (!("target_resource_id" in value) || typeof value["target_resource_id"] === "string") && (!("target_resource_type" in value) || typeof value["target_resource_type"] === "string") && Object.keys(value).every((key) => ["actor_user_id","created_at","decision","permission_code","reason_code","request_id","target_kind","target_product_code","target_resource_id","target_resource_type"].includes(key));
}

function isPlatformOperationsDependencies(value: unknown): value is PlatformOperationsDependencies {
  return isRecord(value) && "postgres" in value && typeof value["postgres"] === "string" && ["ready","unavailable"].includes(value["postgres"]) && "redis" in value && typeof value["redis"] === "string" && ["ready","unavailable"].includes(value["redis"]) && Object.keys(value).every((key) => ["postgres","redis"].includes(key));
}

function isPlatformOperationsInboxItem(value: unknown): value is PlatformOperationsInboxItem {
  return isRecord(value) && "created_at" in value && isDateTime(value["created_at"]) && "id" in value && isUUID(value["id"]) && (!("owner_user_id" in value) || isUUID(value["owner_user_id"])) && "priority" in value && typeof value["priority"] === "string" && ["low","normal","high","urgent"].includes(value["priority"]) && (!("sla_due_at" in value) || isDateTime(value["sla_due_at"])) && "source_product_code" in value && typeof value["source_product_code"] === "string" && "source_resource_id" in value && typeof value["source_resource_id"] === "string" && "source_resource_type" in value && typeof value["source_resource_type"] === "string" && (!("source_resource_url" in value) || typeof value["source_resource_url"] === "string") && "status" in value && typeof value["status"] === "string" && ["open","in_progress","blocked","resolved","archived"].includes(value["status"]) && "updated_at" in value && isDateTime(value["updated_at"]) && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && Object.keys(value).every((key) => ["created_at","id","owner_user_id","priority","sla_due_at","source_product_code","source_resource_id","source_resource_type","source_resource_url","status","updated_at","version"].includes(key));
}

function isPlatformOperationsMailStatus(value: unknown): value is PlatformOperationsMailStatus {
  return isRecord(value) && "accepted" in value && typeof value["accepted"] === "number" && Number.isSafeInteger(value["accepted"]) && "dead_letters" in value && typeof value["dead_letters"] === "number" && Number.isSafeInteger(value["dead_letters"]) && "delivered" in value && typeof value["delivered"] === "number" && Number.isSafeInteger(value["delivered"]) && "failed" in value && typeof value["failed"] === "number" && Number.isSafeInteger(value["failed"]) && "pending" in value && typeof value["pending"] === "number" && Number.isSafeInteger(value["pending"]) && "processing" in value && typeof value["processing"] === "number" && Number.isSafeInteger(value["processing"]) && "retry_due" in value && typeof value["retry_due"] === "number" && Number.isSafeInteger(value["retry_due"]) && Object.keys(value).every((key) => ["accepted","dead_letters","delivered","failed","pending","processing","retry_due"].includes(key));
}

function isPlatformOperationsSession(value: unknown): value is PlatformOperationsSession {
  return isRecord(value) && (!("client_id" in value) || typeof value["client_id"] === "string") && "expires_at" in value && isDateTime(value["expires_at"]) && "id" in value && isUUID(value["id"]) && "kind" in value && typeof value["kind"] === "string" && ["core","client_exchange"].includes(value["kind"]) && "last_seen_at" in value && isDateTime(value["last_seen_at"]) && (!("revoked_at" in value) || isDateTime(value["revoked_at"])) && "user_id" in value && isUUID(value["user_id"]) && Object.keys(value).every((key) => ["client_id","expires_at","id","kind","last_seen_at","revoked_at","user_id"].includes(key));
}

function isPlatformOperationsSnapshot(value: unknown): value is PlatformOperationsSnapshot {
  return isRecord(value) && "access_context" in value && isConsoleAccessContext(value["access_context"]) && "accounts" in value && Array.isArray(value["accounts"]) && value["accounts"].length <= 20 && value["accounts"].every((item) => isPlatformOperationsAccount(item)) && "audit" in value && Array.isArray(value["audit"]) && value["audit"].length <= 20 && value["audit"].every((item) => isPlatformOperationsAuditEvent(item)) && "dependencies" in value && isPlatformOperationsDependencies(value["dependencies"]) && "generated_at" in value && isDateTime(value["generated_at"]) && "inbox_items" in value && Array.isArray(value["inbox_items"]) && value["inbox_items"].length <= 20 && value["inbox_items"].every((item) => isPlatformOperationsInboxItem(item)) && "mail" in value && isPlatformOperationsMailStatus(value["mail"]) && "sessions" in value && Array.isArray(value["sessions"]) && value["sessions"].length <= 20 && value["sessions"].every((item) => isPlatformOperationsSession(item)) && Object.keys(value).every((key) => ["access_context","accounts","audit","dependencies","generated_at","inbox_items","mail","sessions"].includes(key));
}

function isPlatformScope(value: unknown): value is PlatformScope {
  return isRecord(value) && "kind" in value && typeof value["kind"] === "string" && ["platform","product","resource"].includes(value["kind"]) && (!("product_code" in value) || typeof value["product_code"] === "string") && (!("resource_id" in value) || typeof value["resource_id"] === "string") && (!("resource_type" in value) || typeof value["resource_type"] === "string") && Object.keys(value).every((key) => ["kind","product_code","resource_id","resource_type"].includes(key));
}

function isRevokePlatformSessionRequest(value: unknown): value is RevokePlatformSessionRequest {
  return isRecord(value) && "expected_active" in value && value["expected_active"] === true && Object.keys(value).every((key) => ["expected_active"].includes(key));
}

function isSuccessEnvelope(value: unknown): value is SuccessEnvelope {
  return isRecord(value) && "data" in value && true && "request_id" in value && typeof value["request_id"] === "string" && value["request_id"].length <= 100 && new RegExp("^req_[A-Za-z0-9_-]+$").test(value["request_id"]);
}

function isUpdatePlatformAccessRequest(value: unknown): value is UpdatePlatformAccessRequest {
  return isRecord(value) && "expected_revision" in value && typeof value["expected_revision"] === "number" && Number.isSafeInteger(value["expected_revision"]) && "grants" in value && Array.isArray(value["grants"]) && value["grants"].length <= 50 && value["grants"].every((item) => isPlatformAccessGrantInput(item)) && "status" in value && typeof value["status"] === "string" && ["active","suspended","deleted"].includes(value["status"]) && Object.keys(value).every((key) => ["expected_revision","grants","status"].includes(key));
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

export type ConsoleOverviewResult =
  | { state: "authenticated"; overview: ConsoleOverview }
  | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchConsoleOverview(): Promise<ConsoleOverviewResult> {
  try {
    const response = await fetch("/api/v1/overview", { credentials: "same-origin", headers: { Accept: "application/json" } });
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
    const response = await fetch("/api/v1/operations", { credentials: "same-origin", headers: { Accept: "application/json" } });
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
  return writePlatformOperation("/api/v1/operations/sessions/{session_id}/revocations".replace("{session_id}", encodeURIComponent(sessionID)), { expected_active: true }, idempotencyKey);
}

export function updatePlatformAccess(userID: string, input: UpdatePlatformAccessRequest, idempotencyKey: string): Promise<PlatformOperationWriteResult> {
  return writePlatformOperation("/api/v1/operations/users/{user_id}/access-updates".replace("{user_id}", encodeURIComponent(userID)), input, idempotencyKey);
}

export async function resolvePlatformOperation(operation: "session_revoke" | "access_update", idempotencyKey: string): Promise<PlatformOperationWriteResult> {
  try {
    const path = "/api/v1/operations/results/{operation}".replace("{operation}", operation);
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

export type NoticeSnapshotResult =
  | { state: "authenticated"; snapshot: NoticeSnapshot }
  | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchNoticeSnapshot(): Promise<NoticeSnapshotResult> {
  try {
    const response = await fetch("/api/v1/notices", { credentials: "same-origin", headers: { Accept: "application/json" } });
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

export function createNoticeSource(input: CreateNoticeSourceRequest, idempotencyKey: string): Promise<NoticeWriteResult> { return writeNotice("/api/v1/notices/sources", input, idempotencyKey); }
export function createNoticeVersion(sourceID: string, input: CreateNoticeVersionRequest, idempotencyKey: string): Promise<NoticeWriteResult> { return writeNotice("/api/v1/notices/sources/{source_id}/versions".replace("{source_id}", encodeURIComponent(sourceID)), input, idempotencyKey); }
export function reviewNoticeVersion(versionID: string, input: NoticeReviewRequest, idempotencyKey: string): Promise<NoticeWriteResult> { return writeNotice("/api/v1/notices/versions/{version_id}/reviews".replace("{version_id}", encodeURIComponent(versionID)), input, idempotencyKey); }
export function distributeNoticeVersion(versionID: string, input: NoticeDistributionRequest, idempotencyKey: string): Promise<NoticeWriteResult> { return writeNotice("/api/v1/notices/versions/{version_id}/distributions".replace("{version_id}", encodeURIComponent(versionID)), input, idempotencyKey); }

export async function resolveNoticeOperation(operation: "source_create" | "version_create" | "review" | "distribution", idempotencyKey: string): Promise<NoticeWriteResult> {
  try {
    const response = await fetch("/api/v1/notices/operations/{operation}".replace("{operation}", operation), { credentials: "same-origin", headers: { Accept: "application/json", "Idempotency-Key": idempotencyKey } });
    if (response.status === 401) return { state: "signed_out" };
    if (response.status === 403) return { state: "denied" };
    if (response.status === 409) return { state: "conflict" };
    if (!response.ok) return { state: "unavailable" };
    const envelope: unknown = await response.json();
    if (!isSuccessEnvelope(envelope) || !isRecord(envelope.data)) return { state: "unavailable" };
    return envelope.data.status === "unknown" ? { state: "unknown" } : { state: "succeeded", result: envelope.data };
  } catch { return { state: "unavailable" }; }
}

export async function logoutConsoleSession(): Promise<void> {
  const response = await fetch("/api/v1/session/logout", { method: "POST", credentials: "same-origin" });
  if (!response.ok) throw new Error("Console logout failed");
}

export function consoleLoginHref(): string {
  const returnTo = window.location.pathname + window.location.search + window.location.hash;
  return "/api/v1/auth/login?return_to=" + encodeURIComponent(returnTo);
}
