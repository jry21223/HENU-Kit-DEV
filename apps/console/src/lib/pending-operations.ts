import type { FoodCommandKind } from "./console-gateway";

export type PendingPlatformOperation = {
  version: 1;
  operator_id: string;
  operation: "session_revoke" | "access_update";
  idempotency_key: string;
  resource_id: string;
};

export type PendingNoticeOperation = {
  version: 1;
  operator_id: string;
  operation: "source_create" | "version_create" | "review" | "distribution";
  idempotency_key: string;
  resource_id: string;
  target_label: string;
  distribution?: {
    channel: "in_app" | "email";
    audience: { kind: "all_students" | "college" | "role"; value?: string };
    expected_revision: number;
  };
};

const platformKey = "henukit.console.pending-platform-operation.v1";
const noticeKey = "henukit.console.pending-notice-operation.v1";
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const idempotencyPattern = /^idem_[a-z0-9_-]+_[0-9a-f]{8}-[0-9a-f-]{27}$/i;

function storage(): Storage | undefined {
  try { return typeof window === "undefined" ? undefined : window.sessionStorage; } catch { return undefined; }
}

function validCommon(value: unknown, operatorID: string): value is {
  version: 1; operator_id: string; idempotency_key: string; resource_id: string;
} {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  return item.version === 1 && item.operator_id === operatorID && uuidPattern.test(operatorID) && typeof item.idempotency_key === "string" && idempotencyPattern.test(item.idempotency_key) && typeof item.resource_id === "string" && item.resource_id.length > 0 && item.resource_id.length <= 400;
}

function read<T>(key: string, operatorID: string, valid: (value: unknown, operatorID: string) => value is T): T | undefined {
  const target = storage();
  if (!target || !operatorID) return undefined;
  try {
    const raw = target.getItem(key);
    if (!raw) return undefined;
    const parsed: unknown = JSON.parse(raw);
    if (valid(parsed, operatorID)) return parsed;
    target.removeItem(key);
  } catch { clear(key); }
  return undefined;
}

function write<T>(key: string, value: T) {
  try {
    const target = storage();
    if (!target) return false;
    target.setItem(key, JSON.stringify(value));
    return true;
  } catch { return false; }
}

function clear(key: string) {
  try { storage()?.removeItem(key); } catch { /* storage may be unavailable */ }
}

function validPlatform(value: unknown, operatorID: string): value is PendingPlatformOperation {
  if (!validCommon(value, operatorID)) return false;
  const item = value as unknown as PendingPlatformOperation;
  return uuidPattern.test(item.resource_id) && (item.operation === "session_revoke" || item.operation === "access_update");
}

function validNotice(value: unknown, operatorID: string): value is PendingNoticeOperation {
  if (!validCommon(value, operatorID)) return false;
  const item = value as unknown as PendingNoticeOperation;
  if (typeof item.target_label !== "string" || item.target_label.length === 0 || item.target_label.length > 400) return false;
  if (!["source_create", "version_create", "review", "distribution"].includes(item.operation)) return false;
  if (item.operation !== "distribution") return item.distribution === undefined;
  const command = item.distribution;
  return Boolean(command && (command.channel === "in_app" || command.channel === "email") && ["all_students", "college", "role"].includes(command.audience.kind) && (command.audience.value === undefined || typeof command.audience.value === "string") && Number.isSafeInteger(command.expected_revision) && command.expected_revision > 0);
}

export function readPendingPlatformOperation(operatorID: string) { return read(platformKey, operatorID, validPlatform); }
export function writePendingPlatformOperation(value: PendingPlatformOperation) { return write(platformKey, value); }
export function clearPendingPlatformOperation() { clear(platformKey); }
export function readPendingNoticeOperation(operatorID: string) { return read(noticeKey, operatorID, validNotice); }
export function writePendingNoticeOperation(value: PendingNoticeOperation) { return write(noticeKey, value); }
export function clearPendingNoticeOperation() { clear(noticeKey); }

export type PendingAccountTicketOperation = {
  version: 1;
  operator_id: string;
  operation: "ticket_reply" | "ticket_transition";
  idempotency_key: string;
  resource_id: string;
  expected_version: number;
};

const accountTicketKey = "henukit.console.pending-account-ticket-operation.v1";
function validAccountTicket(value: unknown, operatorID: string): value is PendingAccountTicketOperation {
  if (!validCommon(value, operatorID)) return false;
  const item = value as PendingAccountTicketOperation;
  return uuidPattern.test(item.resource_id) && ["ticket_reply", "ticket_transition"].includes(item.operation)
    && Number.isSafeInteger(item.expected_version) && item.expected_version > 0
    && Object.keys(item).every((key) => ["version", "operator_id", "operation", "idempotency_key", "resource_id", "expected_version"].includes(key));
}
export function readPendingAccountTicketOperation(operatorID: string) { return read(accountTicketKey, operatorID, validAccountTicket); }
export function writePendingAccountTicketOperation(value: PendingAccountTicketOperation) { return write(accountTicketKey, value); }
export function clearPendingAccountTicketOperation() { clear(accountTicketKey); }
export type PendingFoodOperation = {
  version: 1;
  operator_id: string;
  operation: FoodCommandKind;
  idempotency_key: string;
  resource_id: string;
};

const foodKey = "henukit.console.pending-food-operation.v1";
const legacyFoodKey = "henukit.food.pending-operation";
const foodOperations: FoodCommandKind[] = ["submission_approve", "submission_reject", "submission_edit", "post_edit", "anomaly_resolve", "anomaly_dismiss", "tier_adjustment_confirm", "tier_adjustment_reject"];

function validFood(value: unknown, operatorID: string): value is PendingFoodOperation {
  if (!validCommon(value, operatorID)) return false;
  const item = value as unknown as PendingFoodOperation;
  // Food includes frozen canonical UUIDs without RFC version/variant bits.
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(item.resource_id) && foodOperations.includes(item.operation);
}

export function readPendingFoodOperation(operatorID: string) {
  clear(legacyFoodKey); // Old full commands have no trusted actor binding.
  return read(foodKey, operatorID, validFood);
}
export function writePendingFoodOperation(value: PendingFoodOperation) { return write(foodKey, value); }
export function clearPendingFoodOperation() { clear(foodKey); clear(legacyFoodKey); }

export type PendingMembershipOperation = {
  version: 1; operator_id: string; operation: "membership_grant" | "membership_revoke";
  idempotency_key: string; resource_id: string; expected_version: number;
};
export type PendingPointOperation = {
  version: 1; operator_id: string; operation: "point_adjustment";
  idempotency_key: string; resource_id: string; expected_amount: number;
};
const membershipKey = "henukit.console.pending-membership-operation.v1";
const pointKey = "henukit.console.pending-point-operation.v1";
function validMembershipOperation(value: unknown, operatorID: string): value is PendingMembershipOperation {
  if (!validCommon(value, operatorID)) return false;
  const item = value as PendingMembershipOperation;
  return uuidPattern.test(item.resource_id) && ["membership_grant", "membership_revoke"].includes(item.operation)
    && Number.isSafeInteger(item.expected_version) && item.expected_version > 0
    && Object.keys(item).every((key) => ["version", "operator_id", "operation", "idempotency_key", "resource_id", "expected_version"].includes(key));
}
function validPointOperation(value: unknown, operatorID: string): value is PendingPointOperation {
  if (!validCommon(value, operatorID)) return false;
  const item = value as PendingPointOperation;
  return uuidPattern.test(item.resource_id) && item.operation === "point_adjustment" && Number.isSafeInteger(item.expected_amount) && item.expected_amount !== 0
    && Object.keys(item).every((key) => ["version", "operator_id", "operation", "idempotency_key", "resource_id", "expected_amount"].includes(key));
}
function legacyAccountOperation(operatorID: string, kind: "membership" | "points") {
  const legacyKey = kind === "membership" ? "henukit.account-membership.pending-command" : "henukit.account-points.pending-command";
  try {
    const value = JSON.parse(storage()?.getItem(legacyKey) ?? "null");
    if (!value) return;
    const command = kind === "membership"
      ? { version: 1, operator_id: value.operatorID, operation: value.kind === "grant" ? "membership_grant" : value.kind === "revoke" ? "membership_revoke" : "", resource_id: value.userID, idempotency_key: value.key, expected_version: value.input?.expected_version }
      : { version: 1, operator_id: value.operatorID, operation: "point_adjustment", resource_id: value.input?.user_id, idempotency_key: value.key, expected_amount: value.input?.amount };
    const valid = kind === "membership" ? validMembershipOperation(command, operatorID) : validPointOperation(command, operatorID);
    if (!valid) { clear(legacyKey); return; }
    // Recover the original key even if storage cannot be rewritten. The full
    // request is never replayed; only its validated minimum query metadata is used.
    if (write(kind === "membership" ? membershipKey : pointKey, command)) clear(legacyKey);
    return command;
  } catch { clear(legacyKey); return; }
}
export function readPendingMembershipOperation(operatorID: string) {
  const value = read(membershipKey, operatorID, validMembershipOperation) ?? legacyAccountOperation(operatorID, "membership");
  return validMembershipOperation(value, operatorID) ? value : undefined;
}
export function writePendingMembershipOperation(value: PendingMembershipOperation) { return write(membershipKey, value); }
export function clearPendingMembershipOperation() { clear(membershipKey); clear("henukit.account-membership.pending-command"); }
export function readPendingPointOperation(operatorID: string) {
  const value = read(pointKey, operatorID, validPointOperation) ?? legacyAccountOperation(operatorID, "points");
  return validPointOperation(value, operatorID) ? value : undefined;
}
export function writePendingPointOperation(value: PendingPointOperation) { return write(pointKey, value); }
export function clearPendingPointOperation() { clear(pointKey); clear("henukit.account-points.pending-command"); }
