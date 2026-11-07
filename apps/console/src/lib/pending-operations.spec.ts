import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { clearPendingAccountTicketOperation, clearPendingFoodOperation, clearPendingNoticeOperation, clearPendingPlatformOperation, readPendingAccountTicketOperation, readPendingFoodOperation, readPendingNoticeOperation, readPendingPlatformOperation, writePendingAccountTicketOperation, writePendingFoodOperation, writePendingNoticeOperation, writePendingPlatformOperation } from "./pending-operations";

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
    const setItem = vi.spyOn(Object.getPrototypeOf(window.sessionStorage), "setItem").mockImplementation(() => { throw new DOMException("blocked", "SecurityError"); });
    expect(writePendingPlatformOperation({ version: 1, operator_id: operatorID, operation: "session_revoke", idempotency_key: "idem_console_revoke_11111111-1111-4111-8111-111111111111", resource_id: "371f1c6f-7b10-4c92-91a2-b39bf5af5302" })).toBe(false);
    expect(writePendingNoticeOperation({ version: 1, operator_id: operatorID, operation: "review", idempotency_key: "idem_notice_review_11111111-1111-4111-8111-111111111111", resource_id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302", target_label: "暑期安排" })).toBe(false);
    expect(writePendingAccountTicketOperation({ version: 1, operator_id: operatorID, operation: "ticket_reply", idempotency_key: "idem_ticket_reply_11111111-1111-4111-8111-111111111111", resource_id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302", expected_version: 1 })).toBe(false);
    expect(writePendingFoodOperation({ version: 1, operator_id: operatorID, operation: "post_edit", idempotency_key: "idem_food_edit_11111111-1111-4111-8111-111111111111", resource_id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302" })).toBe(false);
    expect(setItem).toHaveBeenCalledTimes(4);
  });

  it.each(["unreadable", "malformed"])("does not throw when %s recovery cannot be cleaned up", (kind) => {
    const realm = Object.getPrototypeOf(window.sessionStorage);
    vi.spyOn(realm, "getItem").mockImplementation(() => {
      if (kind === "unreadable") throw new window.DOMException("blocked read", "SecurityError");
      return "{malformed";
    });
    vi.spyOn(realm, "removeItem").mockImplementation(() => { throw new window.DOMException("blocked cleanup", "SecurityError"); });
    for (const read of [readPendingPlatformOperation, readPendingNoticeOperation, readPendingAccountTicketOperation, readPendingFoodOperation]) {
      expect(() => read(operatorID)).not.toThrow();
      expect(read(operatorID)).toBeUndefined();
    }
  });

  it("preserves Food recovery when Account recovery is cleared and vice versa", () => {
    const common = { version: 1 as const, operator_id: operatorID, idempotency_key: "idem_original_11111111-1111-4111-8111-111111111111", resource_id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302" };
    const account = { ...common, operation: "ticket_reply" as const, expected_version: 2 };
    const food = { ...common, operation: "post_edit" as const };
    expect(writePendingAccountTicketOperation(account)).toBe(true);
    expect(writePendingFoodOperation(food)).toBe(true);
    expect(readPendingAccountTicketOperation(operatorID)).toEqual(account);
    expect(readPendingFoodOperation(operatorID)).toEqual(food);
    clearPendingAccountTicketOperation();
    expect(readPendingAccountTicketOperation(operatorID)).toBeUndefined();
    expect(readPendingFoodOperation(operatorID)).toEqual(food);
    expect(writePendingAccountTicketOperation(account)).toBe(true);
    clearPendingFoodOperation();
    expect(readPendingFoodOperation(operatorID)).toBeUndefined();
    expect(readPendingAccountTicketOperation(operatorID)).toEqual(account);
  });

  it("rejects a different actor's Account recovery without removing Food recovery", () => {
    const common = { version: 1 as const, operator_id: operatorID, idempotency_key: "idem_original_11111111-1111-4111-8111-111111111111", resource_id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302" };
    expect(writePendingAccountTicketOperation({ ...common, operation: "ticket_transition", expected_version: 2 })).toBe(true);
    expect(writePendingFoodOperation({ ...common, operation: "post_edit" })).toBe(true);
    expect(readPendingAccountTicketOperation(otherOperatorID)).toBeUndefined();
    expect(readPendingFoodOperation(operatorID)?.operation).toBe("post_edit");
    expect(readPendingAccountTicketOperation(operatorID)).toBeUndefined();
  });
});
