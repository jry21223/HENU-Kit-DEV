// Code generated from console-gateway.yaml (SHA256 a12ddee828547aaf5e044f5adda30a2427fcc38fe9d5d2cd78b256683cd98b1f); DO NOT EDIT.
export interface ConsoleAccessContext {
  permissions: Array<string>;
  scopes: Array<ConsoleScope>;
  verified_at: string;
}

export interface ConsoleAccountMembership {
  lifetime: boolean;
  plan: "free" | "lifetime";
  version: number;
}

export interface ConsoleAccountMembershipEnvelope {
  membership: ConsoleAccountMembership;
}

export interface ConsoleAccountPointAdjustmentResult {
  balance: number;
  entry: ConsolePointLedgerEntry;
}

export interface ConsoleAccountTicket {
  category: string;
  created_at: string;
  id: string;
  reference: string;
  status: "open" | "in_progress" | "resolved";
  title: string;
  updated_at: string;
  version: number;
}

export interface ConsoleAccountTicketCommandResult {
  ticket: ConsoleAccountTicket;
}

export interface ConsoleAccountTicketDetail {
  events: Array<ConsoleAccountTicketEvent>;
  messages: Array<ConsoleAccountTicketMessage>;
  ticket: ConsoleAccountTicket;
}

export interface ConsoleAccountTicketEvent {
  created_at: string;
  from_status: "open" | "in_progress" | "resolved";
  id: string;
  kind: "operator_reply" | "status_transition" | "reopened";
  to_status: "open" | "in_progress" | "resolved";
}

export interface ConsoleAccountTicketMessage {
  author_kind: "user" | "operator";
  body: string;
  created_at: string;
  id: string;
}

export interface ConsoleAccountTicketQueue {
  tickets: Array<ConsoleAccountTicket>;
}

export interface ConsoleMembershipMutationRequest {
  expected_version: number;
  reason: string;
}

export interface ConsoleMembershipOrder {
  amount_cents: number;
  created_at: string;
  id: string;
  plan: "lifetime";
  status: "created" | "pending_payment" | "paid" | "closed" | "failed" | "refunded";
  updated_at: string;
  version: number;
}

export interface ConsoleMembershipOrderCommandRequest {
  expected_version: number;
  reason: string;
}

export interface ConsoleMembershipOrderRefund {
  amount_cents: number;
  id: string;
  status: "processing" | "succeeded" | "closed" | "abnormal";
}

export interface ConsoleMembershipOrderRefundResult {
  order: ConsoleMembershipOrder;
  refund: ConsoleMembershipOrderRefund;
}

export interface ConsoleMembershipOrderResult {
  order: ConsoleMembershipOrder;
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

export interface ConsoleOperatorReplyRequest {
  body: string;
  expected_version: number;
}

export interface ConsoleOverview {
  generated_at: string;
  modules: Array<ConsoleModuleSummary>;
}

export interface ConsolePointAdjustmentRequest {
  amount: number;
  reason: string;
  user_id: string;
}

export interface ConsolePointLedgerEntry {
  amount: number;
  created_at: string;
  id: string;
  reason: string;
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

export interface ConsoleTicketTransitionRequest {
  expected_version: number;
  status: "in_progress" | "resolved";
}

export interface CorrectionReviewCommand {
  expected_version: string;
  kind: "correction_resolve" | "correction_reject";
  payload: ReviewPayload;
  resource_id: string;
}

export interface CourseArchiveCommand {
  expected_version: string;
  kind: "course_archive";
  payload: EmptyPayload;
  resource_id: string;
}

export interface CourseCreateCommand {
  kind: "course_create";
  payload: CourseCreatePayload;
}

export interface CourseCreatePayload {
  collegeId: string;
  description?: string;
  examScope?: string;
  grade: string;
  majorId: string;
  name: string;
  schoolId: string;
  slug: string;
  status?: "draft" | "published" | "archived";
}

export interface CourseMutationPayload {
  collegeId?: string;
  description?: string;
  examScope?: string;
  grade?: string;
  majorId?: string;
  name?: string;
  schoolId?: string;
  slug?: string;
  status?: "draft" | "published" | "archived";
}

export interface CourseUpdateCommand {
  expected_version: string;
  kind: "course_update";
  payload: CourseMutationPayload;
  resource_id: string;
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

export interface EmptyPayload {
}

export interface ErrorEnvelope {
  error: ErrorObject;
  request_id: string;
}

export interface ErrorObject {
  code: string;
  message: string;
}

export interface FoodAnomalyTicket {
  created_at: string;
  details: string;
  id: string;
  kind: "duplicate" | "spam" | "quality" | "location";
  severity: "low" | "medium" | "high";
  status: "open" | "resolved" | "dismissed";
  updated_at: string;
  venue_name: string;
  version: number;
}

export interface FoodCommand {
  expected_version: number;
  kind: FoodCommandKind;
  payload: {
  note: string;
};
  resource_id: string;
}

export type FoodCommandKind = "submission_approve" | "submission_reject" | "anomaly_resolve" | "anomaly_dismiss" | "tier_adjustment_confirm" | "tier_adjustment_reject";

export interface FoodOperationResult {
  operation: FoodCommandKind;
  resource_id?: string;
  state: "succeeded" | "unknown";
  version?: number;
}

export interface FoodSubmission {
  description: string;
  id: string;
  item_name: string;
  status: "pending" | "approved" | "rejected";
  submitted_at: string;
  updated_at: string;
  venue_name: string;
  version: number;
}

export interface FoodTierAdjustment {
  created_at: string;
  current_tier: "featured" | "recommended" | "standard" | "watch";
  id: string;
  proposed_tier: "featured" | "recommended" | "standard" | "watch";
  reason: string;
  status: "pending" | "confirmed" | "rejected";
  updated_at: string;
  venue_name: string;
  version: number;
}

export interface FoodWorkspace {
  anomaly_tickets: Array<FoodAnomalyTicket>;
  as_of: string;
  stale: boolean;
  status: "ok" | "empty" | "stale";
  status_message: string;
  submissions: Array<FoodSubmission>;
  tier_adjustments: Array<FoodTierAdjustment>;
}

export type LibraryCommand = CourseCreateCommand | CourseUpdateCommand | CourseArchiveCommand | MaterialCreateCommand | MaterialUpdateCommand | MaterialArchiveCommand | SubmissionReviewCommand | CorrectionReviewCommand;

export type LibraryCommandKind = "course_create" | "course_update" | "course_archive" | "material_create" | "material_update" | "material_archive" | "submission_approve" | "submission_reject" | "correction_resolve" | "correction_reject";

export interface LibraryCorrection {
  description: string;
  id: string;
  reason: string;
  status: "pending" | "approved" | "rejected";
  target_id: string;
  target_type: "course" | "material";
  updated_at: string;
}

export interface LibraryCourse {
  grade: string;
  id: string;
  name: string;
  slug: string;
  status: "draft" | "published" | "archived";
  updated_at: string;
}

export interface LibraryDownload {
  access_level: "public" | "authenticated" | "restricted";
  downloaded_at: string;
  id: string;
  material_id: string;
  material_title: string;
}

export interface LibraryMaterial {
  access_level: "public" | "authenticated" | "restricted";
  course_id: string;
  file_name: string;
  file_size: number;
  id: string;
  status: "draft" | "pending" | "published" | "rejected" | "archived";
  title: string;
  type: "knowledge_note" | "mock_paper" | "answer" | "quick_review" | "past_exam" | "other";
  updated_at: string;
}

export interface LibraryOperationResult {
  operation: LibraryCommandKind;
  resource_id?: string;
  state: "succeeded" | "unknown";
}

export interface LibraryWorkspace {
  corrections: Array<LibraryCorrection>;
  courses: Array<LibraryCourse>;
  degraded: boolean;
  downloads: Array<LibraryDownload>;
  generated_at: string;
  materials: Array<LibraryMaterial>;
  status: "ok" | "partial" | "unavailable";
  status_message: string;
  submissions: Array<LibraryMaterial>;
}

export interface MaterialArchiveCommand {
  expected_version: string;
  kind: "material_archive";
  payload: EmptyPayload;
  resource_id: string;
}

export interface MaterialCreateCommand {
  kind: "material_create";
  payload: MaterialCreatePayload;
}

export interface MaterialCreatePayload {
  accessLevel?: "free" | "login_required";
  courseId: string;
  description?: string;
  fileName?: string;
  fileSize?: number;
  previewContent?: string;
  status?: "draft" | "pending" | "published" | "rejected" | "archived";
  storageKey: string;
  title: string;
  type?: "knowledge_note" | "mock_paper" | "answer" | "quick_review" | "past_exam" | "other";
}

export interface MaterialUpdateCommand {
  expected_version: string;
  kind: "material_update";
  payload: MaterialUpdatePayload;
  resource_id: string;
}

export interface MaterialUpdatePayload {
  accessLevel?: "free" | "login_required";
  courseId?: string;
  description?: string;
  previewContent?: string;
  status?: "draft" | "pending" | "published" | "rejected" | "archived";
  title?: string;
  type?: "knowledge_note" | "mock_paper" | "answer" | "quick_review" | "past_exam" | "other";
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

export interface ReviewPayload {
  reviewReason?: string;
}

export interface RevokePlatformSessionRequest {
  expected_active: boolean;
}

export interface SubmissionReviewCommand {
  expected_version: string;
  kind: "submission_approve" | "submission_reject";
  payload: ReviewPayload;
  resource_id: string;
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

function isConsoleAccountMembership(value: unknown): value is ConsoleAccountMembership {
  return isRecord(value) && "lifetime" in value && typeof value["lifetime"] === "boolean" && "plan" in value && typeof value["plan"] === "string" && ["free","lifetime"].includes(value["plan"]) && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && value["version"] >= 1 && Object.keys(value).every((key) => ["lifetime","plan","version"].includes(key)) && true && (!(isRecord(value) && "plan" in value && value["plan"] === "free") || isRecord(value) && "lifetime" in value && value["lifetime"] === false) && true && (!(isRecord(value) && "plan" in value && value["plan"] === "lifetime") || isRecord(value) && "lifetime" in value && value["lifetime"] === true);
}

function isConsoleAccountMembershipEnvelope(value: unknown): value is ConsoleAccountMembershipEnvelope {
  return isRecord(value) && "membership" in value && isConsoleAccountMembership(value["membership"]) && Object.keys(value).every((key) => ["membership"].includes(key));
}

function isConsoleAccountPointAdjustmentResult(value: unknown): value is ConsoleAccountPointAdjustmentResult {
  return isRecord(value) && "balance" in value && typeof value["balance"] === "number" && Number.isSafeInteger(value["balance"]) && value["balance"] >= 0 && value["balance"] <= 9.007199254740991e+15 && "entry" in value && isConsolePointLedgerEntry(value["entry"]) && Object.keys(value).every((key) => ["balance","entry"].includes(key));
}

function isConsoleAccountTicket(value: unknown): value is ConsoleAccountTicket {
  return isRecord(value) && "category" in value && typeof value["category"] === "string" && value["category"].length <= 80 && new RegExp("^[a-z][a-z0-9_-]*$").test(value["category"]) && "created_at" in value && isDateTime(value["created_at"]) && "id" in value && isUUID(value["id"]) && "reference" in value && typeof value["reference"] === "string" && new RegExp("^HKT-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$").test(value["reference"]) && "status" in value && typeof value["status"] === "string" && ["open","in_progress","resolved"].includes(value["status"]) && "title" in value && typeof value["title"] === "string" && value["title"].length <= 160 && "updated_at" in value && isDateTime(value["updated_at"]) && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && value["version"] >= 1 && Object.keys(value).every((key) => ["category","created_at","id","reference","status","title","updated_at","version"].includes(key));
}

function isConsoleAccountTicketCommandResult(value: unknown): value is ConsoleAccountTicketCommandResult {
  return isRecord(value) && "ticket" in value && isConsoleAccountTicket(value["ticket"]) && Object.keys(value).every((key) => ["ticket"].includes(key));
}

function isConsoleAccountTicketDetail(value: unknown): value is ConsoleAccountTicketDetail {
  return isRecord(value) && "events" in value && Array.isArray(value["events"]) && value["events"].length <= 1000 && value["events"].every((item) => isConsoleAccountTicketEvent(item)) && "messages" in value && Array.isArray(value["messages"]) && value["messages"].length <= 1000 && value["messages"].every((item) => isConsoleAccountTicketMessage(item)) && "ticket" in value && isConsoleAccountTicket(value["ticket"]) && Object.keys(value).every((key) => ["events","messages","ticket"].includes(key));
}

function isConsoleAccountTicketEvent(value: unknown): value is ConsoleAccountTicketEvent {
  return isRecord(value) && "created_at" in value && isDateTime(value["created_at"]) && "from_status" in value && typeof value["from_status"] === "string" && ["open","in_progress","resolved"].includes(value["from_status"]) && "id" in value && isUUID(value["id"]) && "kind" in value && typeof value["kind"] === "string" && ["operator_reply","status_transition","reopened"].includes(value["kind"]) && "to_status" in value && typeof value["to_status"] === "string" && ["open","in_progress","resolved"].includes(value["to_status"]) && Object.keys(value).every((key) => ["created_at","from_status","id","kind","to_status"].includes(key));
}

function isConsoleAccountTicketMessage(value: unknown): value is ConsoleAccountTicketMessage {
  return isRecord(value) && "author_kind" in value && typeof value["author_kind"] === "string" && ["user","operator"].includes(value["author_kind"]) && "body" in value && typeof value["body"] === "string" && value["body"].length <= 5000 && "created_at" in value && isDateTime(value["created_at"]) && "id" in value && isUUID(value["id"]) && Object.keys(value).every((key) => ["author_kind","body","created_at","id"].includes(key));
}

function isConsoleAccountTicketQueue(value: unknown): value is ConsoleAccountTicketQueue {
  return isRecord(value) && "tickets" in value && Array.isArray(value["tickets"]) && value["tickets"].length <= 500 && value["tickets"].every((item) => isConsoleAccountTicket(item)) && Object.keys(value).every((key) => ["tickets"].includes(key));
}

function isConsoleMembershipMutationRequest(value: unknown): value is ConsoleMembershipMutationRequest {
  return isRecord(value) && "expected_version" in value && typeof value["expected_version"] === "number" && Number.isSafeInteger(value["expected_version"]) && value["expected_version"] >= 1 && "reason" in value && typeof value["reason"] === "string" && value["reason"].length >= 1 && value["reason"].length <= 1000 && Object.keys(value).every((key) => ["expected_version","reason"].includes(key));
}

function isConsoleMembershipOrder(value: unknown): value is ConsoleMembershipOrder {
  return isRecord(value) && "amount_cents" in value && typeof value["amount_cents"] === "number" && Number.isSafeInteger(value["amount_cents"]) && "created_at" in value && isDateTime(value["created_at"]) && "id" in value && isUUID(value["id"]) && "plan" in value && typeof value["plan"] === "string" && ["lifetime"].includes(value["plan"]) && "status" in value && typeof value["status"] === "string" && ["created","pending_payment","paid","closed","failed","refunded"].includes(value["status"]) && "updated_at" in value && isDateTime(value["updated_at"]) && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && value["version"] >= 1 && Object.keys(value).every((key) => ["amount_cents","created_at","id","plan","status","updated_at","version"].includes(key));
}

function isConsoleMembershipOrderCommandRequest(value: unknown): value is ConsoleMembershipOrderCommandRequest {
  return isRecord(value) && "expected_version" in value && typeof value["expected_version"] === "number" && Number.isSafeInteger(value["expected_version"]) && value["expected_version"] >= 1 && "reason" in value && typeof value["reason"] === "string" && value["reason"].length >= 1 && value["reason"].length <= 1000 && Object.keys(value).every((key) => ["expected_version","reason"].includes(key));
}

function isConsoleMembershipOrderRefund(value: unknown): value is ConsoleMembershipOrderRefund {
  return isRecord(value) && "amount_cents" in value && typeof value["amount_cents"] === "number" && Number.isSafeInteger(value["amount_cents"]) && "id" in value && typeof value["id"] === "string" && new RegExp("^HNR[A-Z2-7]{29}$").test(value["id"]) && "status" in value && typeof value["status"] === "string" && ["processing","succeeded","closed","abnormal"].includes(value["status"]) && Object.keys(value).every((key) => ["amount_cents","id","status"].includes(key));
}

function isConsoleMembershipOrderRefundResult(value: unknown): value is ConsoleMembershipOrderRefundResult {
  return isRecord(value) && "order" in value && isConsoleMembershipOrder(value["order"]) && "refund" in value && isConsoleMembershipOrderRefund(value["refund"]) && Object.keys(value).every((key) => ["order","refund"].includes(key));
}

function isConsoleMembershipOrderResult(value: unknown): value is ConsoleMembershipOrderResult {
  return isRecord(value) && "order" in value && isConsoleMembershipOrder(value["order"]) && Object.keys(value).every((key) => ["order"].includes(key));
}

function isConsoleModuleMetric(value: unknown): value is ConsoleModuleMetric {
  return isRecord(value) && (!("hint" in value) || typeof value["hint"] === "string" && value["hint"].length <= 120) && "label" in value && typeof value["label"] === "string" && value["label"].length <= 40 && "value" in value && typeof value["value"] === "string" && value["value"].length <= 80 && Object.keys(value).every((key) => ["hint","label","value"].includes(key));
}

function isConsoleModuleSummary(value: unknown): value is ConsoleModuleSummary {
  return isRecord(value) && (!("as_of" in value) || isDateTime(value["as_of"])) && "id" in value && typeof value["id"] === "string" && ["portal","platform","notice","library","quizcraft","food"].includes(value["id"]) && (!("last_success_at" in value) || isDateTime(value["last_success_at"])) && "metrics" in value && Array.isArray(value["metrics"]) && value["metrics"].length <= 8 && value["metrics"].every((item) => isConsoleModuleMetric(item)) && "request_id" in value && typeof value["request_id"] === "string" && value["request_id"].length <= 120 && new RegExp("^req_[A-Za-z0-9_-]+$").test(value["request_id"]) && "status" in value && typeof value["status"] === "string" && ["ok","empty","partial","stale","unavailable"].includes(value["status"]) && "status_message" in value && typeof value["status_message"] === "string" && value["status_message"].length <= 240 && Object.keys(value).every((key) => ["as_of","id","last_success_at","metrics","request_id","status","status_message"].includes(key)) && ((isRecord(value) && "as_of" in value && true && "last_success_at" in value && true) || (isRecord(value) && "status" in value && typeof value["status"] === "string" && ["ok","empty","partial","unavailable"].includes(value["status"])));
}

function isConsoleOperatorReplyRequest(value: unknown): value is ConsoleOperatorReplyRequest {
  return isRecord(value) && "body" in value && typeof value["body"] === "string" && value["body"].length >= 1 && value["body"].length <= 5000 && "expected_version" in value && typeof value["expected_version"] === "number" && Number.isSafeInteger(value["expected_version"]) && value["expected_version"] >= 1 && Object.keys(value).every((key) => ["body","expected_version"].includes(key));
}

function isConsoleOverview(value: unknown): value is ConsoleOverview {
  return isRecord(value) && "generated_at" in value && isDateTime(value["generated_at"]) && "modules" in value && Array.isArray(value["modules"]) && value["modules"].length >= 6 && value["modules"].length <= 6 && value["modules"].every((item) => isConsoleModuleSummary(item)) && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "portal").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "portal").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "platform").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "platform").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "notice").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "notice").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "library").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "library").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "quizcraft").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "quizcraft").length <= 1 && Array.isArray(value["modules"]) && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "food").length >= 1 && value["modules"].filter((item) => isRecord(item) && "id" in item && item["id"] === "food").length <= 1 && Object.keys(value).every((key) => ["generated_at","modules"].includes(key));
}

function isConsolePointAdjustmentRequest(value: unknown): value is ConsolePointAdjustmentRequest {
  return isRecord(value) && "amount" in value && true && ([(typeof value["amount"] === "number" && Number.isSafeInteger(value["amount"]) && value["amount"] >= 1 && value["amount"] <= 9.007199254740991e+15), (typeof value["amount"] === "number" && Number.isSafeInteger(value["amount"]) && value["amount"] >= -9.007199254740991e+15 && value["amount"] <= -1)].filter(Boolean).length === 1) && "reason" in value && typeof value["reason"] === "string" && value["reason"].length >= 1 && value["reason"].length <= 1000 && "user_id" in value && isUUID(value["user_id"]) && Object.keys(value).every((key) => ["amount","reason","user_id"].includes(key));
}

function isConsolePointLedgerEntry(value: unknown): value is ConsolePointLedgerEntry {
  return isRecord(value) && "amount" in value && true && ([(typeof value["amount"] === "number" && Number.isSafeInteger(value["amount"]) && value["amount"] >= -9.007199254740991e+15 && value["amount"] <= -1), (typeof value["amount"] === "number" && Number.isSafeInteger(value["amount"]) && value["amount"] >= 1 && value["amount"] <= 9.007199254740991e+15)].filter(Boolean).length === 1) && "created_at" in value && isDateTime(value["created_at"]) && "id" in value && isUUID(value["id"]) && "reason" in value && typeof value["reason"] === "string" && value["reason"].length >= 1 && value["reason"].length <= 1000 && Object.keys(value).every((key) => ["amount","created_at","id","reason"].includes(key));
}

function isConsoleScope(value: unknown): value is ConsoleScope {
  return isRecord(value) && "kind" in value && typeof value["kind"] === "string" && ["platform","product"].includes(value["kind"]) && (!("product_code" in value) || typeof value["product_code"] === "string" && value["product_code"].length >= 1 && value["product_code"].length <= 80) && Object.keys(value).every((key) => ["kind","product_code"].includes(key)) && true && (!(isRecord(value) && "kind" in value && value["kind"] === "product") || isRecord(value) && "product_code" in value && typeof value["product_code"] === "string" && value["product_code"].length >= 1 && value["product_code"].length <= 80);
}

function isConsoleSession(value: unknown): value is ConsoleSession {
  return isRecord(value) && "access_context" in value && isConsoleAccessContext(value["access_context"]) && "expires_at" in value && isDateTime(value["expires_at"]) && "user" in value && isRecord(value["user"]) && "id" in value["user"] && isUUID(value["user"]["id"]) && Object.keys(value["user"]).every((key) => ["id"].includes(key)) && Object.keys(value).every((key) => ["access_context","expires_at","user"].includes(key));
}

function isConsoleTicketTransitionRequest(value: unknown): value is ConsoleTicketTransitionRequest {
  return isRecord(value) && "expected_version" in value && typeof value["expected_version"] === "number" && Number.isSafeInteger(value["expected_version"]) && value["expected_version"] >= 1 && "status" in value && typeof value["status"] === "string" && ["in_progress","resolved"].includes(value["status"]) && Object.keys(value).every((key) => ["expected_version","status"].includes(key));
}

function isCorrectionReviewCommand(value: unknown): value is CorrectionReviewCommand {
  return isRecord(value) && "expected_version" in value && isDateTime(value["expected_version"]) && "kind" in value && typeof value["kind"] === "string" && ["correction_resolve","correction_reject"].includes(value["kind"]) && "payload" in value && isReviewPayload(value["payload"]) && "resource_id" in value && isUUID(value["resource_id"]) && Object.keys(value).every((key) => ["expected_version","kind","payload","resource_id"].includes(key));
}

function isCourseArchiveCommand(value: unknown): value is CourseArchiveCommand {
  return isRecord(value) && "expected_version" in value && isDateTime(value["expected_version"]) && "kind" in value && value["kind"] === "course_archive" && "payload" in value && isEmptyPayload(value["payload"]) && "resource_id" in value && isUUID(value["resource_id"]) && Object.keys(value).every((key) => ["expected_version","kind","payload","resource_id"].includes(key));
}

function isCourseCreateCommand(value: unknown): value is CourseCreateCommand {
  return isRecord(value) && "kind" in value && value["kind"] === "course_create" && "payload" in value && isCourseCreatePayload(value["payload"]) && Object.keys(value).every((key) => ["kind","payload"].includes(key));
}

function isCourseCreatePayload(value: unknown): value is CourseCreatePayload {
  return isRecord(value) && "collegeId" in value && isUUID(value["collegeId"]) && (!("description" in value) || typeof value["description"] === "string" && value["description"].length <= 4000) && (!("examScope" in value) || typeof value["examScope"] === "string" && value["examScope"].length <= 2000) && "grade" in value && typeof value["grade"] === "string" && value["grade"].length <= 32 && "majorId" in value && isUUID(value["majorId"]) && "name" in value && typeof value["name"] === "string" && value["name"].length <= 160 && "schoolId" in value && isUUID(value["schoolId"]) && "slug" in value && typeof value["slug"] === "string" && value["slug"].length <= 160 && (!("status" in value) || typeof value["status"] === "string" && ["draft","published","archived"].includes(value["status"])) && Object.keys(value).every((key) => ["collegeId","description","examScope","grade","majorId","name","schoolId","slug","status"].includes(key));
}

function isCourseMutationPayload(value: unknown): value is CourseMutationPayload {
  return isRecord(value) && (!("collegeId" in value) || isUUID(value["collegeId"])) && (!("description" in value) || typeof value["description"] === "string" && value["description"].length <= 4000) && (!("examScope" in value) || typeof value["examScope"] === "string" && value["examScope"].length <= 2000) && (!("grade" in value) || typeof value["grade"] === "string" && value["grade"].length <= 32) && (!("majorId" in value) || isUUID(value["majorId"])) && (!("name" in value) || typeof value["name"] === "string" && value["name"].length <= 160) && (!("schoolId" in value) || isUUID(value["schoolId"])) && (!("slug" in value) || typeof value["slug"] === "string" && value["slug"].length <= 160) && (!("status" in value) || typeof value["status"] === "string" && ["draft","published","archived"].includes(value["status"])) && Object.keys(value).every((key) => ["collegeId","description","examScope","grade","majorId","name","schoolId","slug","status"].includes(key));
}

function isCourseUpdateCommand(value: unknown): value is CourseUpdateCommand {
  return isRecord(value) && "expected_version" in value && isDateTime(value["expected_version"]) && "kind" in value && value["kind"] === "course_update" && "payload" in value && isCourseMutationPayload(value["payload"]) && "resource_id" in value && isUUID(value["resource_id"]) && Object.keys(value).every((key) => ["expected_version","kind","payload","resource_id"].includes(key));
}

function isCreateNoticeSourceRequest(value: unknown): value is CreateNoticeSourceRequest {
  return isRecord(value) && "canonical_url" in value && typeof value["canonical_url"] === "string" && new RegExp("^https://").test(value["canonical_url"]) && "code" in value && typeof value["code"] === "string" && new RegExp("^[a-z0-9][a-z0-9-]{1,62}$").test(value["code"]) && "name" in value && typeof value["name"] === "string" && value["name"].length >= 1 && value["name"].length <= 120 && Object.keys(value).every((key) => ["canonical_url","code","name"].includes(key));
}

function isCreateNoticeVersionRequest(value: unknown): value is CreateNoticeVersionRequest {
  return isRecord(value) && "body" in value && typeof value["body"] === "string" && value["body"].length >= 1 && value["body"].length <= 100000 && (!("source_published_at" in value) || isDateTime(value["source_published_at"])) && "source_url" in value && typeof value["source_url"] === "string" && new RegExp("^https://").test(value["source_url"]) && "title" in value && typeof value["title"] === "string" && value["title"].length >= 1 && value["title"].length <= 200 && Object.keys(value).every((key) => ["body","source_published_at","source_url","title"].includes(key));
}

function isEmptyPayload(value: unknown): value is EmptyPayload {
  return isRecord(value) && Object.keys(value).length === 0;
}

function isErrorEnvelope(value: unknown): value is ErrorEnvelope {
  return isRecord(value) && "error" in value && isErrorObject(value["error"]) && "request_id" in value && typeof value["request_id"] === "string" && value["request_id"].length <= 100 && new RegExp("^req_[A-Za-z0-9_-]+$").test(value["request_id"]);
}

function isErrorObject(value: unknown): value is ErrorObject {
  return isRecord(value) && "code" in value && typeof value["code"] === "string" && "message" in value && typeof value["message"] === "string";
}

function isFoodAnomalyTicket(value: unknown): value is FoodAnomalyTicket {
  return isRecord(value) && "created_at" in value && isDateTime(value["created_at"]) && "details" in value && typeof value["details"] === "string" && value["details"].length <= 2000 && "id" in value && isUUID(value["id"]) && "kind" in value && typeof value["kind"] === "string" && ["duplicate","spam","quality","location"].includes(value["kind"]) && "severity" in value && typeof value["severity"] === "string" && ["low","medium","high"].includes(value["severity"]) && "status" in value && typeof value["status"] === "string" && ["open","resolved","dismissed"].includes(value["status"]) && "updated_at" in value && isDateTime(value["updated_at"]) && "venue_name" in value && typeof value["venue_name"] === "string" && value["venue_name"].length <= 160 && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && value["version"] >= 1 && Object.keys(value).every((key) => ["created_at","details","id","kind","severity","status","updated_at","venue_name","version"].includes(key));
}

function isFoodCommand(value: unknown): value is FoodCommand {
  return isRecord(value) && "expected_version" in value && typeof value["expected_version"] === "number" && Number.isSafeInteger(value["expected_version"]) && value["expected_version"] >= 1 && "kind" in value && isFoodCommandKind(value["kind"]) && "payload" in value && isRecord(value["payload"]) && "note" in value["payload"] && typeof value["payload"]["note"] === "string" && value["payload"]["note"].length >= 2 && value["payload"]["note"].length <= 1000 && Object.keys(value["payload"]).every((key) => ["note"].includes(key)) && "resource_id" in value && isUUID(value["resource_id"]) && Object.keys(value).every((key) => ["expected_version","kind","payload","resource_id"].includes(key));
}

function isFoodCommandKind(value: unknown): value is FoodCommandKind {
  return typeof value === "string" && ["submission_approve","submission_reject","anomaly_resolve","anomaly_dismiss","tier_adjustment_confirm","tier_adjustment_reject"].includes(value);
}

function isFoodOperationResult(value: unknown): value is FoodOperationResult {
  return isRecord(value) && "operation" in value && isFoodCommandKind(value["operation"]) && (!("resource_id" in value) || isUUID(value["resource_id"])) && "state" in value && typeof value["state"] === "string" && ["succeeded","unknown"].includes(value["state"]) && (!("version" in value) || typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && value["version"] >= 1) && Object.keys(value).every((key) => ["operation","resource_id","state","version"].includes(key));
}

function isFoodSubmission(value: unknown): value is FoodSubmission {
  return isRecord(value) && "description" in value && typeof value["description"] === "string" && value["description"].length <= 2000 && "id" in value && isUUID(value["id"]) && "item_name" in value && typeof value["item_name"] === "string" && value["item_name"].length <= 160 && "status" in value && typeof value["status"] === "string" && ["pending","approved","rejected"].includes(value["status"]) && "submitted_at" in value && isDateTime(value["submitted_at"]) && "updated_at" in value && isDateTime(value["updated_at"]) && "venue_name" in value && typeof value["venue_name"] === "string" && value["venue_name"].length <= 160 && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && value["version"] >= 1 && Object.keys(value).every((key) => ["description","id","item_name","status","submitted_at","updated_at","venue_name","version"].includes(key));
}

function isFoodTierAdjustment(value: unknown): value is FoodTierAdjustment {
  return isRecord(value) && "created_at" in value && isDateTime(value["created_at"]) && "current_tier" in value && typeof value["current_tier"] === "string" && ["featured","recommended","standard","watch"].includes(value["current_tier"]) && "id" in value && isUUID(value["id"]) && "proposed_tier" in value && typeof value["proposed_tier"] === "string" && ["featured","recommended","standard","watch"].includes(value["proposed_tier"]) && "reason" in value && typeof value["reason"] === "string" && value["reason"].length <= 2000 && "status" in value && typeof value["status"] === "string" && ["pending","confirmed","rejected"].includes(value["status"]) && "updated_at" in value && isDateTime(value["updated_at"]) && "venue_name" in value && typeof value["venue_name"] === "string" && value["venue_name"].length <= 160 && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && value["version"] >= 1 && Object.keys(value).every((key) => ["created_at","current_tier","id","proposed_tier","reason","status","updated_at","venue_name","version"].includes(key));
}

function isFoodWorkspace(value: unknown): value is FoodWorkspace {
  return isRecord(value) && "anomaly_tickets" in value && Array.isArray(value["anomaly_tickets"]) && value["anomaly_tickets"].length <= 200 && value["anomaly_tickets"].every((item) => isFoodAnomalyTicket(item)) && "as_of" in value && isDateTime(value["as_of"]) && "stale" in value && typeof value["stale"] === "boolean" && "status" in value && typeof value["status"] === "string" && ["ok","empty","stale"].includes(value["status"]) && "status_message" in value && typeof value["status_message"] === "string" && value["status_message"].length <= 240 && "submissions" in value && Array.isArray(value["submissions"]) && value["submissions"].length <= 200 && value["submissions"].every((item) => isFoodSubmission(item)) && "tier_adjustments" in value && Array.isArray(value["tier_adjustments"]) && value["tier_adjustments"].length <= 200 && value["tier_adjustments"].every((item) => isFoodTierAdjustment(item)) && Object.keys(value).every((key) => ["anomaly_tickets","as_of","stale","status","status_message","submissions","tier_adjustments"].includes(key));
}

function isLibraryCommand(value: unknown): value is LibraryCommand {
  return true && ([(isCourseCreateCommand(value)), (isCourseUpdateCommand(value)), (isCourseArchiveCommand(value)), (isMaterialCreateCommand(value)), (isMaterialUpdateCommand(value)), (isMaterialArchiveCommand(value)), (isSubmissionReviewCommand(value)), (isCorrectionReviewCommand(value))].filter(Boolean).length === 1);
}

function isLibraryCommandKind(value: unknown): value is LibraryCommandKind {
  return typeof value === "string" && ["course_create","course_update","course_archive","material_create","material_update","material_archive","submission_approve","submission_reject","correction_resolve","correction_reject"].includes(value);
}

function isLibraryCorrection(value: unknown): value is LibraryCorrection {
  return isRecord(value) && "description" in value && typeof value["description"] === "string" && value["description"].length <= 2000 && "id" in value && isUUID(value["id"]) && "reason" in value && typeof value["reason"] === "string" && value["reason"].length <= 120 && "status" in value && typeof value["status"] === "string" && ["pending","approved","rejected"].includes(value["status"]) && "target_id" in value && isUUID(value["target_id"]) && "target_type" in value && typeof value["target_type"] === "string" && ["course","material"].includes(value["target_type"]) && "updated_at" in value && isDateTime(value["updated_at"]) && Object.keys(value).every((key) => ["description","id","reason","status","target_id","target_type","updated_at"].includes(key));
}

function isLibraryCourse(value: unknown): value is LibraryCourse {
  return isRecord(value) && "grade" in value && typeof value["grade"] === "string" && value["grade"].length <= 32 && "id" in value && isUUID(value["id"]) && "name" in value && typeof value["name"] === "string" && value["name"].length <= 160 && "slug" in value && typeof value["slug"] === "string" && value["slug"].length <= 160 && "status" in value && typeof value["status"] === "string" && ["draft","published","archived"].includes(value["status"]) && "updated_at" in value && isDateTime(value["updated_at"]) && Object.keys(value).every((key) => ["grade","id","name","slug","status","updated_at"].includes(key));
}

function isLibraryDownload(value: unknown): value is LibraryDownload {
  return isRecord(value) && "access_level" in value && typeof value["access_level"] === "string" && ["public","authenticated","restricted"].includes(value["access_level"]) && "downloaded_at" in value && isDateTime(value["downloaded_at"]) && "id" in value && isUUID(value["id"]) && "material_id" in value && isUUID(value["material_id"]) && "material_title" in value && typeof value["material_title"] === "string" && value["material_title"].length <= 200 && Object.keys(value).every((key) => ["access_level","downloaded_at","id","material_id","material_title"].includes(key));
}

function isLibraryMaterial(value: unknown): value is LibraryMaterial {
  return isRecord(value) && "access_level" in value && typeof value["access_level"] === "string" && ["public","authenticated","restricted"].includes(value["access_level"]) && "course_id" in value && isUUID(value["course_id"]) && "file_name" in value && typeof value["file_name"] === "string" && value["file_name"].length <= 255 && "file_size" in value && typeof value["file_size"] === "number" && Number.isSafeInteger(value["file_size"]) && value["file_size"] >= 0 && "id" in value && isUUID(value["id"]) && "status" in value && typeof value["status"] === "string" && ["draft","pending","published","rejected","archived"].includes(value["status"]) && "title" in value && typeof value["title"] === "string" && value["title"].length <= 200 && "type" in value && typeof value["type"] === "string" && ["knowledge_note","mock_paper","answer","quick_review","past_exam","other"].includes(value["type"]) && "updated_at" in value && isDateTime(value["updated_at"]) && Object.keys(value).every((key) => ["access_level","course_id","file_name","file_size","id","status","title","type","updated_at"].includes(key));
}

function isLibraryOperationResult(value: unknown): value is LibraryOperationResult {
  return isRecord(value) && "operation" in value && isLibraryCommandKind(value["operation"]) && (!("resource_id" in value) || isUUID(value["resource_id"])) && "state" in value && typeof value["state"] === "string" && ["succeeded","unknown"].includes(value["state"]) && Object.keys(value).every((key) => ["operation","resource_id","state"].includes(key));
}

function isLibraryWorkspace(value: unknown): value is LibraryWorkspace {
  return isRecord(value) && "corrections" in value && Array.isArray(value["corrections"]) && value["corrections"].length <= 500 && value["corrections"].every((item) => isLibraryCorrection(item)) && "courses" in value && Array.isArray(value["courses"]) && value["courses"].length <= 500 && value["courses"].every((item) => isLibraryCourse(item)) && "degraded" in value && typeof value["degraded"] === "boolean" && "downloads" in value && Array.isArray(value["downloads"]) && value["downloads"].length <= 200 && value["downloads"].every((item) => isLibraryDownload(item)) && "generated_at" in value && isDateTime(value["generated_at"]) && "materials" in value && Array.isArray(value["materials"]) && value["materials"].length <= 500 && value["materials"].every((item) => isLibraryMaterial(item)) && "status" in value && typeof value["status"] === "string" && ["ok","partial","unavailable"].includes(value["status"]) && "status_message" in value && typeof value["status_message"] === "string" && value["status_message"].length <= 240 && "submissions" in value && Array.isArray(value["submissions"]) && value["submissions"].length <= 500 && value["submissions"].every((item) => isLibraryMaterial(item)) && Object.keys(value).every((key) => ["corrections","courses","degraded","downloads","generated_at","materials","status","status_message","submissions"].includes(key));
}

function isMaterialArchiveCommand(value: unknown): value is MaterialArchiveCommand {
  return isRecord(value) && "expected_version" in value && isDateTime(value["expected_version"]) && "kind" in value && value["kind"] === "material_archive" && "payload" in value && isEmptyPayload(value["payload"]) && "resource_id" in value && isUUID(value["resource_id"]) && Object.keys(value).every((key) => ["expected_version","kind","payload","resource_id"].includes(key));
}

function isMaterialCreateCommand(value: unknown): value is MaterialCreateCommand {
  return isRecord(value) && "kind" in value && value["kind"] === "material_create" && "payload" in value && isMaterialCreatePayload(value["payload"]) && Object.keys(value).every((key) => ["kind","payload"].includes(key));
}

function isMaterialCreatePayload(value: unknown): value is MaterialCreatePayload {
  return isRecord(value) && (!("accessLevel" in value) || typeof value["accessLevel"] === "string" && ["free","login_required"].includes(value["accessLevel"])) && "courseId" in value && isUUID(value["courseId"]) && (!("description" in value) || typeof value["description"] === "string" && value["description"].length <= 4000) && (!("fileName" in value) || typeof value["fileName"] === "string" && value["fileName"].length <= 255) && (!("fileSize" in value) || typeof value["fileSize"] === "number" && Number.isSafeInteger(value["fileSize"]) && value["fileSize"] >= 0) && (!("previewContent" in value) || typeof value["previewContent"] === "string" && value["previewContent"].length <= 20000) && (!("status" in value) || typeof value["status"] === "string" && ["draft","pending","published","rejected","archived"].includes(value["status"])) && "storageKey" in value && typeof value["storageKey"] === "string" && value["storageKey"].length <= 512 && "title" in value && typeof value["title"] === "string" && value["title"].length <= 200 && (!("type" in value) || typeof value["type"] === "string" && ["knowledge_note","mock_paper","answer","quick_review","past_exam","other"].includes(value["type"])) && Object.keys(value).every((key) => ["accessLevel","courseId","description","fileName","fileSize","previewContent","status","storageKey","title","type"].includes(key));
}

function isMaterialUpdateCommand(value: unknown): value is MaterialUpdateCommand {
  return isRecord(value) && "expected_version" in value && isDateTime(value["expected_version"]) && "kind" in value && value["kind"] === "material_update" && "payload" in value && isMaterialUpdatePayload(value["payload"]) && "resource_id" in value && isUUID(value["resource_id"]) && Object.keys(value).every((key) => ["expected_version","kind","payload","resource_id"].includes(key));
}

function isMaterialUpdatePayload(value: unknown): value is MaterialUpdatePayload {
  return isRecord(value) && (!("accessLevel" in value) || typeof value["accessLevel"] === "string" && ["free","login_required"].includes(value["accessLevel"])) && (!("courseId" in value) || isUUID(value["courseId"])) && (!("description" in value) || typeof value["description"] === "string" && value["description"].length <= 4000) && (!("previewContent" in value) || typeof value["previewContent"] === "string" && value["previewContent"].length <= 20000) && (!("status" in value) || typeof value["status"] === "string" && ["draft","pending","published","rejected","archived"].includes(value["status"])) && (!("title" in value) || typeof value["title"] === "string" && value["title"].length <= 200) && (!("type" in value) || typeof value["type"] === "string" && ["knowledge_note","mock_paper","answer","quick_review","past_exam","other"].includes(value["type"])) && Object.keys(value).every((key) => ["accessLevel","courseId","description","previewContent","status","title","type"].includes(key));
}

function isNoticeAudience(value: unknown): value is NoticeAudience {
  return isRecord(value) && (!("kind" in value) || typeof value["kind"] === "string" && ["all_students","college","role"].includes(value["kind"])) && (!("value" in value) || typeof value["value"] === "string" && value["value"].length >= 1 && value["value"].length <= 120) && Object.keys(value).every((key) => ["kind","value"].includes(key)) && ((isRecord(value) && "kind" in value && typeof value["kind"] === "string" && ["all_students"].includes(value["kind"]) && Object.keys(value).every((key) => ["kind"].includes(key))) || (isRecord(value) && "kind" in value && typeof value["kind"] === "string" && ["college","role"].includes(value["kind"]) && "value" in value && typeof value["value"] === "string" && value["value"].length >= 1 && value["value"].length <= 120 && Object.keys(value).every((key) => ["kind","value"].includes(key))));
}

function isNoticeDistributionRequest(value: unknown): value is NoticeDistributionRequest {
  return isRecord(value) && "audience" in value && isNoticeAudience(value["audience"]) && "channel" in value && typeof value["channel"] === "string" && ["in_app","email"].includes(value["channel"]) && "expected_revision" in value && typeof value["expected_revision"] === "number" && Number.isSafeInteger(value["expected_revision"]) && value["expected_revision"] >= 1 && Object.keys(value).every((key) => ["audience","channel","expected_revision"].includes(key));
}

function isNoticeReviewRequest(value: unknown): value is NoticeReviewRequest {
  return isRecord(value) && "decision" in value && typeof value["decision"] === "string" && ["approved","rejected"].includes(value["decision"]) && "expected_revision" in value && typeof value["expected_revision"] === "number" && Number.isSafeInteger(value["expected_revision"]) && value["expected_revision"] >= 1 && (!("note" in value) || typeof value["note"] === "string" && value["note"].length <= 1000) && Object.keys(value).every((key) => ["decision","expected_revision","note"].includes(key));
}

function isNoticeSnapshot(value: unknown): value is NoticeSnapshot {
  return isRecord(value) && "generated_at" in value && isDateTime(value["generated_at"]) && "items" in value && Array.isArray(value["items"]) && value["items"].length <= 50 && value["items"].every((item) => isNoticeVersion(item)) && Object.keys(value).every((key) => ["generated_at","items"].includes(key));
}

function isNoticeSource(value: unknown): value is NoticeSource {
  return isRecord(value) && "code" in value && typeof value["code"] === "string" && "id" in value && isUUID(value["id"]) && "name" in value && typeof value["name"] === "string" && Object.keys(value).every((key) => ["code","id","name"].includes(key));
}

function isNoticeVersion(value: unknown): value is NoticeVersion {
  return isRecord(value) && "body" in value && typeof value["body"] === "string" && "content_hash" in value && typeof value["content_hash"] === "string" && new RegExp("^[a-f0-9]{64}$").test(value["content_hash"]) && "created_at" in value && isDateTime(value["created_at"]) && "distribution_count" in value && typeof value["distribution_count"] === "number" && Number.isSafeInteger(value["distribution_count"]) && value["distribution_count"] >= 0 && (!("distribution_status" in value) || typeof value["distribution_status"] === "string" && ["queued","processing","delivered","failed"].includes(value["distribution_status"])) && "id" in value && isUUID(value["id"]) && "revision" in value && typeof value["revision"] === "number" && Number.isSafeInteger(value["revision"]) && value["revision"] >= 1 && "source" in value && isNoticeSource(value["source"]) && (!("source_published_at" in value) || isDateTime(value["source_published_at"])) && "source_url" in value && typeof value["source_url"] === "string" && "state" in value && typeof value["state"] === "string" && ["pending_review","approved","rejected","distributed"].includes(value["state"]) && "title" in value && typeof value["title"] === "string" && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && value["version"] >= 1 && Object.keys(value).every((key) => ["body","content_hash","created_at","distribution_count","distribution_status","id","revision","source","source_published_at","source_url","state","title","version"].includes(key));
}

function isPlatformAccessGrantInput(value: unknown): value is PlatformAccessGrantInput {
  return isRecord(value) && "role_code" in value && typeof value["role_code"] === "string" && new RegExp("^[a-z][a-z0-9-]{1,63}$").test(value["role_code"]) && "scope" in value && isPlatformScope(value["scope"]) && Object.keys(value).every((key) => ["role_code","scope"].includes(key));
}

function isPlatformOperationResult(value: unknown): value is PlatformOperationResult {
  return isRecord(value) && "operation" in value && typeof value["operation"] === "string" && ["session_revoke","access_update"].includes(value["operation"]) && (!("resource_id" in value) || isUUID(value["resource_id"])) && (!("resource_version" in value) || typeof value["resource_version"] === "number" && Number.isSafeInteger(value["resource_version"]) && value["resource_version"] >= 1) && "status" in value && typeof value["status"] === "string" && ["succeeded","unknown"].includes(value["status"]) && Object.keys(value).every((key) => ["operation","resource_id","resource_version","status"].includes(key));
}

function isPlatformOperationsAccount(value: unknown): value is PlatformOperationsAccount {
  return isRecord(value) && "authorization_revision" in value && typeof value["authorization_revision"] === "number" && Number.isSafeInteger(value["authorization_revision"]) && value["authorization_revision"] >= 1 && "created_at" in value && isDateTime(value["created_at"]) && "email_verified" in value && typeof value["email_verified"] === "boolean" && "grants" in value && Array.isArray(value["grants"]) && value["grants"].length <= 50 && value["grants"].every((item) => isPlatformAccessGrantInput(item)) && "id" in value && isUUID(value["id"]) && "status" in value && typeof value["status"] === "string" && ["active","suspended","deleted"].includes(value["status"]) && Object.keys(value).every((key) => ["authorization_revision","created_at","email_verified","grants","id","status"].includes(key));
}

function isPlatformOperationsAuditEvent(value: unknown): value is PlatformOperationsAuditEvent {
  return isRecord(value) && "actor_user_id" in value && isUUID(value["actor_user_id"]) && "created_at" in value && isDateTime(value["created_at"]) && "decision" in value && typeof value["decision"] === "string" && ["allowed","denied"].includes(value["decision"]) && "permission_code" in value && typeof value["permission_code"] === "string" && "reason_code" in value && typeof value["reason_code"] === "string" && "request_id" in value && typeof value["request_id"] === "string" && "target_kind" in value && typeof value["target_kind"] === "string" && ["platform","product","resource"].includes(value["target_kind"]) && (!("target_product_code" in value) || typeof value["target_product_code"] === "string") && (!("target_resource_id" in value) || typeof value["target_resource_id"] === "string") && (!("target_resource_type" in value) || typeof value["target_resource_type"] === "string") && Object.keys(value).every((key) => ["actor_user_id","created_at","decision","permission_code","reason_code","request_id","target_kind","target_product_code","target_resource_id","target_resource_type"].includes(key));
}

function isPlatformOperationsDependencies(value: unknown): value is PlatformOperationsDependencies {
  return isRecord(value) && "postgres" in value && typeof value["postgres"] === "string" && ["ready","unavailable"].includes(value["postgres"]) && "redis" in value && typeof value["redis"] === "string" && ["ready","unavailable"].includes(value["redis"]) && Object.keys(value).every((key) => ["postgres","redis"].includes(key));
}

function isPlatformOperationsInboxItem(value: unknown): value is PlatformOperationsInboxItem {
  return isRecord(value) && "created_at" in value && isDateTime(value["created_at"]) && "id" in value && isUUID(value["id"]) && (!("owner_user_id" in value) || isUUID(value["owner_user_id"])) && "priority" in value && typeof value["priority"] === "string" && ["low","normal","high","urgent"].includes(value["priority"]) && (!("sla_due_at" in value) || isDateTime(value["sla_due_at"])) && "source_product_code" in value && typeof value["source_product_code"] === "string" && "source_resource_id" in value && typeof value["source_resource_id"] === "string" && "source_resource_type" in value && typeof value["source_resource_type"] === "string" && (!("source_resource_url" in value) || typeof value["source_resource_url"] === "string") && "status" in value && typeof value["status"] === "string" && ["open","in_progress","blocked","resolved","archived"].includes(value["status"]) && "updated_at" in value && isDateTime(value["updated_at"]) && "version" in value && typeof value["version"] === "number" && Number.isSafeInteger(value["version"]) && value["version"] >= 1 && Object.keys(value).every((key) => ["created_at","id","owner_user_id","priority","sla_due_at","source_product_code","source_resource_id","source_resource_type","source_resource_url","status","updated_at","version"].includes(key));
}

function isPlatformOperationsMailStatus(value: unknown): value is PlatformOperationsMailStatus {
  return isRecord(value) && "accepted" in value && typeof value["accepted"] === "number" && Number.isSafeInteger(value["accepted"]) && value["accepted"] >= 0 && "dead_letters" in value && typeof value["dead_letters"] === "number" && Number.isSafeInteger(value["dead_letters"]) && value["dead_letters"] >= 0 && "delivered" in value && typeof value["delivered"] === "number" && Number.isSafeInteger(value["delivered"]) && value["delivered"] >= 0 && "failed" in value && typeof value["failed"] === "number" && Number.isSafeInteger(value["failed"]) && value["failed"] >= 0 && "pending" in value && typeof value["pending"] === "number" && Number.isSafeInteger(value["pending"]) && value["pending"] >= 0 && "processing" in value && typeof value["processing"] === "number" && Number.isSafeInteger(value["processing"]) && value["processing"] >= 0 && "retry_due" in value && typeof value["retry_due"] === "number" && Number.isSafeInteger(value["retry_due"]) && value["retry_due"] >= 0 && Object.keys(value).every((key) => ["accepted","dead_letters","delivered","failed","pending","processing","retry_due"].includes(key));
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

function isReviewPayload(value: unknown): value is ReviewPayload {
  return isRecord(value) && (!("reviewReason" in value) || typeof value["reviewReason"] === "string" && value["reviewReason"].length <= 1000) && Object.keys(value).every((key) => ["reviewReason"].includes(key));
}

function isRevokePlatformSessionRequest(value: unknown): value is RevokePlatformSessionRequest {
  return isRecord(value) && "expected_active" in value && value["expected_active"] === true && Object.keys(value).every((key) => ["expected_active"].includes(key));
}

function isSubmissionReviewCommand(value: unknown): value is SubmissionReviewCommand {
  return isRecord(value) && "expected_version" in value && isDateTime(value["expected_version"]) && "kind" in value && typeof value["kind"] === "string" && ["submission_approve","submission_reject"].includes(value["kind"]) && "payload" in value && isReviewPayload(value["payload"]) && "resource_id" in value && isUUID(value["resource_id"]) && Object.keys(value).every((key) => ["expected_version","kind","payload","resource_id"].includes(key));
}

function isSuccessEnvelope(value: unknown): value is SuccessEnvelope {
  return isRecord(value) && "data" in value && true && "request_id" in value && typeof value["request_id"] === "string" && value["request_id"].length <= 100 && new RegExp("^req_[A-Za-z0-9_-]+$").test(value["request_id"]);
}

function isUpdatePlatformAccessRequest(value: unknown): value is UpdatePlatformAccessRequest {
  return isRecord(value) && "expected_revision" in value && typeof value["expected_revision"] === "number" && Number.isSafeInteger(value["expected_revision"]) && value["expected_revision"] >= 1 && "grants" in value && Array.isArray(value["grants"]) && value["grants"].length <= 50 && value["grants"].every((item) => isPlatformAccessGrantInput(item)) && "status" in value && typeof value["status"] === "string" && ["active","suspended","deleted"].includes(value["status"]) && Object.keys(value).every((key) => ["expected_revision","grants","status"].includes(key));
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

export type LibraryWorkspaceResult = { state: "authenticated"; workspace: LibraryWorkspace } | { state: "signed_out" | "denied" | "unavailable" };

export async function fetchLibraryWorkspace(): Promise<LibraryWorkspaceResult> {
  try {
    const response = await fetch("/api/v1/library", { credentials: "same-origin", headers: { Accept: "application/json" } });
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
    const response = await fetch("/api/v1/library/commands", { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input) });
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
    const response = await fetch("/api/v1/library/operations/{operation}".replace("{operation}", operation), { credentials: "same-origin", headers: { Accept: "application/json", "Idempotency-Key": idempotencyKey } });
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

export async function fetchFoodWorkspace(): Promise<FoodWorkspaceResult> {
  try {
    const response = await fetch("/api/v1/food", { credentials: "same-origin", headers: { Accept: "application/json" } });
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
    const response = await fetch("/api/v1/food/commands", { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input) });
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
    const response = await fetch("/api/v1/food/operations/{operation}".replace("{operation}", operation), { credentials: "same-origin", headers: { Accept: "application/json", "Idempotency-Key": idempotencyKey } });
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

export async function fetchAccountMembership(userID: string): Promise<AccountMembershipReadResult> {
  try {
    const response = await fetch("/api/v1/account/memberships/{user_id}".replace("{user_id}", encodeURIComponent(userID)), { credentials: "same-origin", headers: { Accept: "application/json" } });
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
  return writeAccountMembership("/api/v1/account/memberships/{user_id}/grants".replace("{user_id}", encodeURIComponent(userID)), input, idempotencyKey);
}

export function revokeAccountMembership(userID: string, input: ConsoleMembershipMutationRequest, idempotencyKey: string): Promise<AccountMembershipWriteResult> {
  return writeAccountMembership("/api/v1/account/memberships/{user_id}/revocations".replace("{user_id}", encodeURIComponent(userID)), input, idempotencyKey);
}

export type AccountPointAdjustmentWriteResult =
  | { state: "succeeded"; result: ConsoleAccountPointAdjustmentResult }
  | { state: "signed_out" | "denied" | "conflict" | "invalid" | "unavailable" };

export async function adjustAccountPoints(input: ConsolePointAdjustmentRequest, idempotencyKey: string): Promise<AccountPointAdjustmentWriteResult> {
  try {
    const response = await fetch("/api/v1/account/points/adjustments", { method: "POST", credentials: "same-origin", headers: { Accept: "application/json", "Content-Type": "application/json", "Idempotency-Key": idempotencyKey }, body: JSON.stringify(input) });
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
    const response = await fetch("/api/v1/account/tickets", { credentials: "same-origin", headers: { Accept: "application/json" } });
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
    const response = await fetch("/api/v1/account/tickets/{ticket_id}".replace("{ticket_id}", encodeURIComponent(ticketID)), { credentials: "same-origin", headers: { Accept: "application/json" } });
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
  return writeAccountTicket("/api/v1/account/tickets/{ticket_id}/replies".replace("{ticket_id}", encodeURIComponent(ticketID)), input, idempotencyKey);
}

export function transitionAccountTicket(ticketID: string, input: ConsoleTicketTransitionRequest, idempotencyKey: string): Promise<AccountTicketWriteResult> {
  return writeAccountTicket("/api/v1/account/tickets/{ticket_id}/transitions".replace("{ticket_id}", encodeURIComponent(ticketID)), input, idempotencyKey);
}

export async function logoutConsoleSession(): Promise<void> {
  const response = await fetch("/api/v1/session/logout", { method: "POST", credentials: "same-origin" });
  if (!response.ok) throw new Error("Console logout failed");
}

export function consoleLoginHref(): string {
  const returnTo = window.location.pathname + window.location.search + window.location.hash;
  return "/api/v1/auth/login?return_to=" + encodeURIComponent(returnTo);
}
