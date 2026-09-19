import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import NoticeOperationsView from "./components/NoticeOperationsView.vue";
import PlatformOperationsView from "./components/PlatformOperationsView.vue";

const mocks = vi.hoisted(() => ({
  fetchPlatformOperations: vi.fn(),
  resolvePlatformOperation: vi.fn(),
  fetchNoticeSnapshot: vi.fn(),
  resolveNoticeOperation: vi.fn(),
}));

vi.mock("./lib/console-gateway", async (importOriginal) => {
  const original = await importOriginal<typeof import("./lib/console-gateway")>();
  return {
    ...original,
    fetchPlatformOperations: mocks.fetchPlatformOperations,
    resolvePlatformOperation: mocks.resolvePlatformOperation,
    fetchNoticeSnapshot: mocks.fetchNoticeSnapshot,
    resolveNoticeOperation: mocks.resolveNoticeOperation,
  };
});

const operatorA = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
const operatorB = "271f1c6f-7b10-4c92-91a2-b39bf5af5302";
const accountID = "371f1c6f-7b10-4c92-91a2-b39bf5af5302";
const noticeID = "471f1c6f-7b10-4c92-91a2-b39bf5af5302";

describe("operator-bound pending operations", () => {
  beforeEach(() => {
    sessionStorage.clear();
    mocks.resolvePlatformOperation.mockResolvedValue({ state: "unknown", result: { operation: "access_update", status: "unknown" } });
    mocks.resolveNoticeOperation.mockResolvedValue({ state: "unknown" });
    mocks.fetchPlatformOperations.mockResolvedValue({ state: "authenticated", operations: {
      access_context: { permissions: ["platform.operations.read", "platform.operations.write"], scopes: [{ kind: "platform" }], verified_at: "2026-09-20T00:00:00Z" },
      accounts: [{ id: accountID, display_name: "张老师", email: "operator@henu.edu.cn", email_verified: true, status: "active", authorization_revision: 1, created_at: "2026-09-20T00:00:00Z", grants: [] }],
      sessions: [], mail: { pending: 0, processing: 0, retry_due: 0, accepted: 0, delivered: 0, failed: 0, dead_letters: 0 }, inbox_items: [], audit: [], dependencies: { postgres: "ready", redis: "ready" }, generated_at: "2026-09-20T00:00:00Z",
    } });
    mocks.fetchNoticeSnapshot.mockResolvedValue({ state: "authenticated", snapshot: { items: [{ id: noticeID, source: { id: accountID, code: "henu-office", name: "学校办公室" }, version: 1, title: "暑期安排", body: "正文", source_url: "https://example.edu/notice", content_hash: "a".repeat(64), state: "approved", revision: 2, created_at: "2026-09-20T00:00:00Z", distribution_count: 0 }], generated_at: "2026-09-20T00:00:00Z" } });
  });

  afterEach(() => vi.clearAllMocks());

  it("clears Platform memory and storage when the authenticated operator changes", async () => {
    sessionStorage.setItem("henukit.console.pending-platform-operation.v1", JSON.stringify({ version: 1, operator_id: operatorA, operation: "access_update", idempotency_key: "idem_console_access_11111111-1111-4111-8111-111111111111", resource_id: accountID }));
    const wrapper = mount(PlatformOperationsView, { props: { authState: "authenticated", operatorID: operatorA } });
    await flushPromises();
    expect(wrapper.text()).toContain("等待结果");
    await wrapper.setProps({ operatorID: operatorB });
    await flushPromises();
    expect(wrapper.text()).toContain("保存访问设置");
    expect(wrapper.text()).not.toContain("查询结果");
    expect(sessionStorage.getItem("henukit.console.pending-platform-operation.v1")).toBeNull();
    wrapper.unmount();
  });

  it("clears Notice memory and storage when the authenticated operator changes", async () => {
    sessionStorage.setItem("henukit.console.pending-notice-operation.v1", JSON.stringify({ version: 1, operator_id: operatorA, operation: "distribution", idempotency_key: "idem_notice_distribution_11111111-1111-4111-8111-111111111111", resource_id: noticeID, target_label: "暑期安排", distribution: { channel: "email", audience: { kind: "college", value: "software-college" }, expected_revision: 2 } }));
    const wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID: operatorA, permissions: ["notice.read", "notice.distribute"] } });
    await flushPromises();
    expect(wrapper.findAll("button").find((button) => button.text() === "创建分发任务")?.attributes("disabled")).toBeDefined();
    await wrapper.setProps({ operatorID: operatorB });
    await flushPromises();
    expect(wrapper.text()).not.toContain("原请求已保留");
    expect(wrapper.findAll("button").find((button) => button.text() === "创建分发任务")?.attributes("disabled")).toBeUndefined();
    expect(sessionStorage.getItem("henukit.console.pending-notice-operation.v1")).toBeNull();
    wrapper.unmount();
  });
});
