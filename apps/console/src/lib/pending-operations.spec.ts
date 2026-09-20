import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { clearPendingNoticeOperation, clearPendingPlatformOperation, readPendingNoticeOperation, readPendingPlatformOperation, writePendingNoticeOperation, writePendingPlatformOperation } from "./pending-operations";

const operatorID = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
const otherOperatorID = "271f1c6f-7b10-4c92-91a2-b39bf5af5302";

describe("pending Console operations", () => {
  beforeEach(() => sessionStorage.clear());
  afterEach(() => vi.restoreAllMocks());

  it("restores only the Platform command owned by the current operator", () => {
    writePendingPlatformOperation({ version: 1, operator_id: operatorID, operation: "access_update", idempotency_key: "idem_console_access_11111111-1111-4111-8111-111111111111", resource_id: "371f1c6f-7b10-4c92-91a2-b39bf5af5302" });
    expect(readPendingPlatformOperation(operatorID)?.operation).toBe("access_update");
    expect(readPendingPlatformOperation(otherOperatorID)).toBeUndefined();
    expect(readPendingPlatformOperation(operatorID)).toBeUndefined();
  });

  it("preserves the exact Notice distribution command and rejects malformed storage", () => {
    writePendingNoticeOperation({ version: 1, operator_id: operatorID, operation: "distribution", idempotency_key: "idem_notice_distribution_11111111-1111-4111-8111-111111111111", resource_id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302", target_label: "暑期安排", distribution: { channel: "email", audience: { kind: "college", value: "software-college" }, expected_revision: 2 } });
    expect(readPendingNoticeOperation(operatorID)?.distribution).toEqual({ channel: "email", audience: { kind: "college", value: "software-college" }, expected_revision: 2 });
    sessionStorage.setItem("henukit.console.pending-notice-operation.v1", "{not-json");
    expect(readPendingNoticeOperation(operatorID)).toBeUndefined();
    expect(sessionStorage.getItem("henukit.console.pending-notice-operation.v1")).toBeNull();
  });

  it("clear helpers remove both operation families", () => {
    sessionStorage.setItem("henukit.console.pending-platform-operation.v1", "value");
    sessionStorage.setItem("henukit.console.pending-notice-operation.v1", "value");
    clearPendingPlatformOperation();
    clearPendingNoticeOperation();
    expect(sessionStorage.length).toBe(0);
  });

  it("reports storage failure so callers can fail closed before writing", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new DOMException("blocked", "SecurityError"); });
    expect(writePendingPlatformOperation({ version: 1, operator_id: operatorID, operation: "session_revoke", idempotency_key: "idem_console_revoke_11111111-1111-4111-8111-111111111111", resource_id: "371f1c6f-7b10-4c92-91a2-b39bf5af5302" })).toBe(false);
    expect(writePendingNoticeOperation({ version: 1, operator_id: operatorID, operation: "review", idempotency_key: "idem_notice_review_11111111-1111-4111-8111-111111111111", resource_id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302", target_label: "暑期安排" })).toBe(false);
  });
});
