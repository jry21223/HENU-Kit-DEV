import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import FoodOperationsView from "./FoodOperationsView.vue";
import type { FoodWorkspace } from "@/lib/console-gateway";

const operatorID = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
enableAutoUnmount(afterEach);
const asOf = "2026-11-07T09:30:00Z";
function workspace(): FoodWorkspace {
  return {
    status: "ok", status_message: "数据正常", stale: false, as_of: asOf,
    submissions: [], anomaly_tickets: [], tier_adjustments: [],
    posts: [false, true].map((hidden, index) => ({
      id: `${index + 1}1111111-1111-4111-8111-111111111111`, venue_name: `受控点评${index + 1}`,
      campus: "minglun", tier: "hang", review_text: "受控虚构点评", price_reference: "", hours_reference: "",
      author_display_name: "测试账号", hidden, version: 1, created_at: asOf, updated_at: asOf,
    })),
  };
}
function response(data: unknown) {
  return new Response(JSON.stringify({ data, request_id: "req_food_component" }), { status: 200 });
}
beforeEach(() => { sessionStorage.clear(); history.replaceState({}, "", "/food"); });
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

it("labels bounded returned posts separately from visible and hidden posts", async () => {
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async () => response(workspace())));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read"] } });
  await flushPromises();
  expect(view.text()).toContain("本次返回点评");
  expect(view.text()).toContain("公开可见 1 · 已隐藏 1");
  expect(view.text()).toContain("每类最多返回 200 条");
  expect(view.text()).toContain("异常与调档保留全局队列");
  expect(view.find("time").attributes("datetime")).toBe(asOf);
  view.unmount();
});

it("keeps the newest campus after an earlier campus responds late", async () => {
  let resolveMinglun!: (value: Response) => void;
  const lateMinglun = new Promise<Response>((resolve) => { resolveMinglun = resolve; });
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
    const campus = new URL(String(input), "https://console.henukit.test").searchParams.get("campus");
    if (campus === "minglun") return lateMinglun;
    const data = workspace();
    data.posts = data.posts.slice(0, 1).map((post) => ({ ...post, campus: campus === "jinming" ? "jinming" : "minglun", venue_name: campus === "jinming" ? "金明当前记录" : "初始记录" }));
    return response(data);
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read"] } });
  await flushPromises();
  await view.find("select[aria-label='校区筛选']").setValue("minglun");
  await view.find("select[aria-label='校区筛选']").setValue("jinming");
  await flushPromises();
  expect(view.text()).toContain("金明当前记录");
  const oldData = workspace();
  oldData.posts[0]!.venue_name = "明伦迟到记录";
  resolveMinglun(response(oldData));
  await flushPromises();
  expect(view.text()).toContain("金明当前记录");
  expect(view.text()).not.toContain("明伦迟到记录");
  view.unmount();
});

it("stores only actor-bound recovery metadata and queries unknown writes without another POST", async () => {
  const data = workspace();
  const resourceID = "31111111-1111-4111-8111-111111111111";
  data.submissions = [{ id: resourceID, venue_name: "受控待审核", item_name: "早餐", description: "虚构审核项", campus: "minglun", status: "pending", version: 1, submitted_at: asOf, updated_at: asOf }];
  const writes: string[] = [];
  const reads: string[] = [];
  let resolved = false;
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "https://console.henukit.test").pathname;
    const key = new Headers(init?.headers).get("Idempotency-Key") ?? "";
    if (path.endsWith("/commands")) { writes.push(key); throw new TypeError("response lost"); }
    if (path.includes("/operations/")) {
      reads.push(key);
      return response({ operation: "submission_approve", state: resolved ? "succeeded" : "unknown", ...(resolved ? { resource_id: resourceID, version: 2 } : {}) });
    }
    return response(data);
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read", "food.review"] } });
  await flushPromises();
  await view.findAll("button").find((button) => button.text() === "批准投稿")!.trigger("click");
  await view.find("textarea").setValue("隔离审核理由");
  await view.findAll("button").find((button) => button.text() === "确认批准投稿")!.trigger("click");
  await vi.waitFor(() => {
    const button = view.findAll("button").find((button) => button.text() === "查询操作结果");
    expect(button).toBeDefined();
    expect(button!.attributes("disabled")).toBeUndefined();
  }, { timeout: 1500 });
  const stored = JSON.parse(sessionStorage.getItem("henukit.console.pending-food-operation.v1") ?? "null");
  expect(stored).toEqual({ version: 1, operator_id: operatorID, operation: "submission_approve", idempotency_key: writes[0], resource_id: resourceID });
  expect(view.findAll("button").find((button) => button.text() === "批准投稿")!.attributes("disabled")).toBeDefined();
  resolved = true;
  await view.findAll("button").find((button) => button.text() === "查询操作结果")!.trigger("click");
  await flushPromises();
  expect(view.text()).toContain("投稿已批准。");
  expect(writes).toHaveLength(1);
  expect(reads.length).toBeGreaterThan(0);
  expect(reads.every((key) => key === writes[0])).toBe(true);
  expect(sessionStorage.getItem("henukit.console.pending-food-operation.v1")).toBeNull();
  view.unmount();
});

it.each([403, 409, 404, 503])("retains an unresolved operation when its result query returns HTTP %i", async (status) => {
  const pending = { version: 1, operator_id: operatorID, operation: "post_edit", idempotency_key: "idem_food_post_edit_11111111-1111-4111-8111-111111111111", resource_id: "01234567-89ab-cdef-0123-456789abcdef" };
  const storageKey = "henukit.console.pending-food-operation.v1";
  sessionStorage.setItem(storageKey, JSON.stringify(pending));
  const writes: unknown[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") writes.push(input);
    return String(input).includes("/operations/") ? new Response(null, { status }) : response(workspace());
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read", "food.review"] } });
  await flushPromises();
  await vi.waitFor(() => expect(view.text()).toContain("查询操作结果"), { timeout: 1500 });
  expect(JSON.parse(sessionStorage.getItem(storageKey) ?? "null")).toEqual(pending);
  if (status === 403) expect(view.text()).toContain("无法查询原操作结果");
  expect(writes).toHaveLength(0);
  view.unmount();
});

it("also retains a lost write when its first automatic result query is denied", async () => {
  const writes: string[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { writes.push(new Headers(init.headers).get("Idempotency-Key") ?? ""); throw new TypeError("response lost"); }
    return String(input).includes("/operations/") ? new Response(null, { status: 403 }) : response(workspace());
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read", "food.review"] } });
  await flushPromises();
  await view.findAll("button").find((button) => button.text() === "隐藏投稿")!.trigger("click");
  await view.find("textarea").setValue("隔离隐藏理由");
  await view.findAll("button").find((button) => button.text() === "确认隐藏投稿")!.trigger("click");
  await flushPromises();
  const stored = JSON.parse(sessionStorage.getItem("henukit.console.pending-food-operation.v1") ?? "null");
  expect(stored?.idempotency_key).toBe(writes[0]);
  expect(view.text()).toContain("无法查询原操作结果");
  expect(writes).toHaveLength(1);
  view.unmount();
});

it("restores the non-sensitive campus filter after leaving and reopening the workspace", async () => {
  const paths: string[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL) => { paths.push(String(input)); return response(workspace()); }));
  const props = { authState: "authenticated" as const, operatorID, permissions: ["food.read"] };
  const first = mount(FoodOperationsView, { props });
  await flushPromises();
  await first.find("select[aria-label='校区筛选']").setValue("jinming");
  await flushPromises();
  first.unmount();
  const reopened = mount(FoodOperationsView, { props });
  await flushPromises();
  expect((reopened.find("select[aria-label='校区筛选']").element as HTMLSelectElement).value).toBe("jinming");
  expect(paths[paths.length - 1]).toBe("/api/v1/food?campus=jinming");
  expect(new URL(window.location.href).searchParams.get("campus")).toBe("jinming");
  reopened.unmount();
});

it("discards old workspace and operation responses after the operator changes", async () => {
  let oldWorkspace!: (value: Response) => void;
  let oldOperation!: (value: Response) => void;
  const slowWorkspace = new Promise<Response>((resolve) => { oldWorkspace = resolve; });
  const slowOperation = new Promise<Response>((resolve) => { oldOperation = resolve; });
  const storageKey = "henukit.console.pending-food-operation.v1";
  sessionStorage.setItem(storageKey, JSON.stringify({ version: 1, operator_id: operatorID, operation: "post_edit", idempotency_key: "idem_food_post_edit_11111111-1111-4111-8111-111111111111", resource_id: workspace().posts[0]!.id }));
  let workspaceRequests = 0;
  const writes: unknown[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") writes.push(input);
    if (String(input).includes("/operations/")) return slowOperation;
    if (++workspaceRequests === 1) return slowWorkspace;
    const data = workspace();
    data.posts = [{ ...data.posts[0]!, venue_name: "新运营者当前数据" }];
    return response(data);
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read", "food.review"] } });
  await flushPromises();
  await view.setProps({ operatorID: "271f1c6f-7b10-4c92-91a2-b39bf5af5302" });
  await flushPromises();
  const data = workspace();
  data.posts[0]!.venue_name = "旧运营者迟到数据";
  oldWorkspace(response(data));
  oldOperation(response({ operation: "post_edit", resource_id: data.posts[0]!.id, state: "succeeded", version: 2 }));
  await flushPromises();
  expect(view.text()).toContain("新运营者当前数据");
  expect(view.text()).not.toContain("旧运营者迟到数据");
  expect(view.text()).not.toContain("点评变更已完成");
  expect(sessionStorage.getItem(storageKey)).toBeNull();
  expect(writes).toHaveLength(0);
  view.unmount();
});

it("ends recovery only after the owner explicitly confirms the stored operation failed", async () => {
  const storageKey = "henukit.console.pending-food-operation.v1";
  const pending = { version: 1, operator_id: operatorID, operation: "post_edit", idempotency_key: "idem_food_post_edit_11111111-1111-4111-8111-111111111111", resource_id: workspace().posts[0]!.id };
  sessionStorage.setItem(storageKey, JSON.stringify(pending));
  const reads: string[] = [];
  const writes: unknown[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") writes.push(input);
    if (String(input).includes("/operations/")) {
      reads.push(String(input));
      return response({ operation: "post_edit", resource_id: pending.resource_id, state: "failed", error_code: "FOOD_CONFLICT" });
    }
    return response(workspace());
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read", "food.review"] } });
  await flushPromises();
  expect(sessionStorage.getItem(storageKey)).toBeNull();
  expect(view.text()).toContain("原操作已确认未完成");
  expect(reads[0]).toBe("/api/v1/food/operations/post_edit/outcome");
  expect(writes).toHaveLength(0);
  expect(view.findAll("button").find((button) => button.text() === "隐藏投稿")!.attributes("disabled")).toBeUndefined();
  view.unmount();
});

it.each(["QuotaExceededError", "SecurityError"])("does not submit when recovery metadata cannot be saved: %s", async (errorName) => {
  const writes: unknown[] = [];
  const operationReads: unknown[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") {
      writes.push(input);
      return response({ operation: "post_edit", resource_id: workspace().posts[0]!.id, state: "succeeded", version: 2 });
    }
    if (String(input).includes("/operations/")) operationReads.push(input);
    return response(workspace());
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read", "food.review"] } });
  await flushPromises();
  vi.spyOn(Object.getPrototypeOf(window.sessionStorage), "setItem").mockImplementation(() => { throw new DOMException("blocked", errorName); });
  await view.findAll("button").find((button) => button.text() === "隐藏投稿")!.trigger("click");
  await view.find("textarea").setValue("保存失败隔离理由");
  await view.findAll("button").find((button) => button.text() === "确认隐藏投稿")!.trigger("click");
  await flushPromises();
  expect(writes).toHaveLength(0);
  expect(operationReads).toHaveLength(0);
  expect(view.text()).toContain("尚未提交");
  expect((view.find("textarea").element as HTMLTextAreaElement).value).toBe("保存失败隔离理由");
  expect(view.findAll("button").find((button) => button.text() === "确认隐藏投稿")!.attributes("disabled")).toBeUndefined();
  expect(sessionStorage.getItem("henukit.console.pending-food-operation.v1")).toBeNull();
  view.unmount();
});

it.each(["initial", "after-success"])("shows the current unavailable workspace state: %s", async (mode) => {
  let available = mode === "after-success";
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async () => available ? response(workspace()) : new Response(null, { status: 503 })));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read"] } });
  await flushPromises();
  if (available) {
    expect(view.find(".access-context strong").text()).toBe("正常");
    available = false;
    await view.find("select[aria-label='校区筛选']").setValue("jinming");
    await flushPromises();
  }
  expect(view.find(".access-context strong").text()).toBe("暂不可用");
  expect(view.text()).toContain("美食服务暂时不可用");
  expect(view.text()).not.toContain("本次返回点评");
  view.unmount();
});

it("keeps original-key result queries available when the workspace cannot load", async () => {
  const pending = { version: 1, operator_id: operatorID, operation: "post_edit", idempotency_key: "idem_food_post_edit_11111111-1111-4111-8111-111111111111", resource_id: workspace().posts[0]!.id };
  sessionStorage.setItem("henukit.console.pending-food-operation.v1", JSON.stringify(pending));
  const writes: unknown[] = [];
  const keys: string[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") writes.push(input);
    if (String(input).includes("/operations/")) {
      keys.push(new Headers(init?.headers).get("Idempotency-Key") ?? "");
      return response({ operation: "post_edit", state: "unknown" });
    }
    return new Response(null, { status: 503 });
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read", "food.review"] } });
  await vi.waitFor(() => {
    const button = view.findAll("button").find((button) => button.text() === "查询操作结果");
    expect(button).toBeDefined();
    expect(button!.attributes("disabled")).toBeUndefined();
  }, { timeout: 1500 });
  expect(view.text()).toContain("结果尚未确认");
  const before = keys.length;
  const query = view.findAll("button").find((button) => button.text() === "查询操作结果");
  expect(query).toBeDefined();
  await query!.trigger("click");
  await vi.waitFor(() => expect(keys.length).toBeGreaterThan(before), { timeout: 1500 });
  expect(keys.every((key) => key === pending.idempotency_key)).toBe(true);
  expect(writes).toHaveLength(0);
  view.unmount();
});

it("keeps a confirmed write result visible if the following workspace refresh fails", async () => {
  let written = false;
  const writes: unknown[] = [];
  const operationReads: unknown[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") {
      writes.push(input);
      written = true;
      return response({ operation: "post_edit", resource_id: workspace().posts[0]!.id, state: "succeeded", version: 2 });
    }
    if (String(input).includes("/operations/")) operationReads.push(input);
    return written ? new Response(null, { status: 503 }) : response(workspace());
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read", "food.review"] } });
  await flushPromises();
  await view.findAll("button").find((button) => button.text() === "隐藏投稿")!.trigger("click");
  await view.find("textarea").setValue("隔离成功刷新失败");
  await view.findAll("button").find((button) => button.text() === "确认隐藏投稿")!.trigger("click");
  await flushPromises();
  expect(view.find(".access-context strong").text()).toBe("暂不可用");
  expect(view.text()).toContain("投稿已隐藏。");
  expect(view.text()).toContain("美食服务暂时不可用");
  expect(writes).toHaveLength(1);
  expect(operationReads).toHaveLength(0);
  expect(sessionStorage.getItem("henukit.console.pending-food-operation.v1")).toBeNull();
  view.unmount();
});

it("does not query a pending operation without its original operation permission", async () => {
  sessionStorage.setItem("henukit.console.pending-food-operation.v1", JSON.stringify({ version: 1, operator_id: operatorID, operation: "post_edit", idempotency_key: "idem_food_post_edit_11111111-1111-4111-8111-111111111111", resource_id: workspace().posts[0]!.id }));
  const operationReads: unknown[] = [];
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
    if (String(input).includes("/operations/")) operationReads.push(input);
    return new Response(null, { status: 503 });
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read"] } });
  await flushPromises();
  expect(operationReads).toHaveLength(0);
  expect(view.text()).toContain("缺少查询原操作所需权限");
  expect(view.findAll("button").find((button) => button.text() === "查询操作结果")!.attributes("disabled")).toBeDefined();
  view.unmount();
});

it.each(["unavailable", "stale"])("does not claim a fresh read after a conflict followed by %s data", async (readState) => {
  let conflicted = false;
  const writes: unknown[] = [];
  const operationReads: unknown[] = [];
  const oldTime = "2026-11-07T08:00:00Z";
  vi.stubGlobal("fetch", vi.fn().mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") {
      writes.push(input);
      conflicted = true;
      return new Response(null, { status: 409 });
    }
    if (String(input).includes("/operations/")) operationReads.push(input);
    if (!conflicted) return response(workspace());
    if (readState === "unavailable") return new Response(null, { status: 503 });
    return response({ ...workspace(), status: "stale", stale: true, as_of: oldTime, status_message: "受控旧快照" });
  }));
  const view = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID, permissions: ["food.read", "food.review"] } });
  await flushPromises();
  await view.findAll("button").find((button) => button.text() === "隐藏投稿")!.trigger("click");
  await view.find("textarea").setValue("隔离冲突验证");
  await view.findAll("button").find((button) => button.text() === "确认隐藏投稿")!.trigger("click");
  await flushPromises();
  expect(view.text()).not.toContain("已重新读取当前状态");
  expect(view.text()).toContain("操作遇到冲突，请核对最新状态后再操作。");
  if (readState === "unavailable") {
    expect(view.find(".access-context strong").text()).toBe("暂不可用");
  } else {
    expect(view.find(".access-context strong").text()).toBe("数据过期");
    expect(view.find("time").attributes("datetime")).toBe(oldTime);
  }
  expect(writes).toHaveLength(1);
  expect(operationReads).toHaveLength(0);
  view.unmount();
});
