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
  } catch { target.removeItem(key); }
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
