import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, expect, it, vi } from "vitest";

import LibraryOperationsView from "./components/LibraryOperationsView.vue";

const readiness = { management: "not_ready", reason: "management_migration_pending", capabilities: { workspace: false, commands: false, operation_results: false }, checked_at: "2026-11-07T18:00:00Z" };
const legacyWorkspace = { status: "partial", status_message: "legacy Study API adapter removed (ADR-0037); catalog migration (T1) pending", degraded: true, courses: [], materials: [], downloads: [], submissions: [], corrections: [], generated_at: "2026-11-07T18:00:00Z" };

afterEach(() => { vi.unstubAllGlobals(); sessionStorage.clear(); });

it("keeps the retry focus and announces loading and failure without a duplicate request", async () => {
  let resolveRetry: (response: Response) => void = () => {};
  const fetcher = vi.fn()
    .mockResolvedValueOnce(new Response("{}", { status: 503 }))
    .mockImplementationOnce(() => new Promise<Response>((resolve) => { resolveRetry = resolve; }));
  vi.stubGlobal("fetch", fetcher);
  const wrapper = mount(LibraryOperationsView, { attachTo: document.body, props: { authState: "authenticated", permissions: ["library.read"] } });
  try {
    await flushPromises();
    const retry = wrapper.get("button");
    retry.element.focus();
    await retry.trigger("click");
    expect(document.activeElement).toBe(retry.element);
    expect(retry.attributes("aria-disabled")).toBe("true");
    expect(retry.attributes("aria-busy")).toBe("true");
    expect(wrapper.get('[role="status"]').text()).toContain("正在确认资料管理能力");
    await retry.trigger("click");
    expect(fetcher).toHaveBeenCalledTimes(2);
    resolveRetry(new Response("{}", { status: 503 }));
    await flushPromises();
    expect(document.activeElement).toBe(retry.element);
    expect(retry.attributes("aria-disabled")).toBe("false");
    expect(wrapper.get('[role="status"]').text()).toContain("暂时无法确认资料管理能力");
  } finally { wrapper.unmount(); }
});

it("moves a focused successful retry to the available public directory instead of the page root", async () => {
  vi.stubGlobal("fetch", vi.fn()
    .mockResolvedValueOnce(new Response("{}", { status: 503 }))
    .mockResolvedValueOnce(new Response(JSON.stringify({ data: readiness, request_id: "req_retry_confirmed" }), { status: 200 })));
  const wrapper = mount(LibraryOperationsView, { attachTo: document.body, props: { authState: "authenticated", permissions: ["library.read"] } });
  try {
    await flushPromises();
    const retry = wrapper.get("button");
    retry.element.focus();
    await retry.trigger("click");
    await flushPromises();
    const link = wrapper.get("a[href='https://henukit.cn/library']");
    expect(document.activeElement).toBe(link.element);
    expect(wrapper.get('[role="status"]').text()).toContain("资料管理尚未就绪");
    expect(wrapper.findAll("button")).toHaveLength(0);
  } finally { wrapper.unmount(); }
});

it.each(["2026-11-08T02:00:00+08:00", "2026-11-07T18:00:00Z"])("shows readiness time %s as the same Beijing instant while preserving its source", async (checkedAt) => {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response(JSON.stringify({ data: { ...readiness, checked_at: checkedAt }, request_id: "req_readiness_time" }), { status: 200 }))));
  const wrapper = mount(LibraryOperationsView, { props: { authState: "authenticated", permissions: ["library.read"] } });
  await flushPromises();
  const time = wrapper.get("time");
  expect(time.text()).toBe("2026年11月8日 02:00:00");
  expect(time.element.parentElement?.textContent).toContain("北京时间");
  expect(time.attributes("datetime")).toBe(checkedAt);
  expect(time.attributes("title")).toBe(`来源时间：${checkedAt}`);
  wrapper.unmount();
});

it("does not present an invalid owner timestamp as confirmed readiness", async () => {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response(JSON.stringify({ data: { ...readiness, checked_at: "not-a-time" }, request_id: "req_invalid_readiness_time" }), { status: 200 }))));
  const wrapper = mount(LibraryOperationsView, { props: { authState: "authenticated", permissions: ["library.read"] } });
  await flushPromises();
  expect(wrapper.text()).toContain("暂时无法确认资料管理能力");
  expect(wrapper.text()).not.toContain("资料管理尚未就绪");
  expect(wrapper.find("time").exists()).toBe(false);
  wrapper.unmount();
});

it("shows owner-confirmed management readiness without fake forms, empty success or command replay", async () => {
  sessionStorage.setItem("henukit.library.pending-operation", JSON.stringify({ kind: "course_create", key: "idem_library_old_pending", input: { kind: "course_create", payload: {} }, success: "课程已创建。" }));
  const fetcher = vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify({ data: String(input).endsWith("/readiness") ? readiness : legacyWorkspace, request_id: "req_library_readiness" }), { status: 200 })));
  vi.stubGlobal("fetch", fetcher);
  const wrapper = mount(LibraryOperationsView, { props: { authState: "authenticated", permissions: ["library.read", "library.manage", "library.review"] } });
  await flushPromises();

  expect(wrapper.text()).toContain("资料管理尚未就绪");
  expect(wrapper.find("form").exists()).toBe(false);
  expect(wrapper.find(".operation-summary-grid").exists()).toBe(false);
  expect(wrapper.text()).not.toMatch(/ADR-0037|\bT1\b|同步生效|暂无课程|暂无资料|按原请求重试/);
  expect(fetcher.mock.calls.map(([path]) => String(path))).toEqual(["/api/v1/library/readiness"]);
  expect(sessionStorage.getItem("henukit.library.pending-operation")).toContain("idem_library_old_pending");
  expect(wrapper.get("a[href='https://henukit.cn/library']").text()).toBe("打开公开资料目录");
  expect(wrapper.findAll("button")).toHaveLength(0);
  wrapper.unmount();
});

it.each([
  [404, "暂时无法确认资料管理能力"],
  [503, "暂时无法确认资料管理能力"],
  [403, "当前账户没有资料库查看权限"],
  [401, "登录状态已过期"],
])("separates HTTP %s from owner-confirmed not-ready state", async (status, message) => {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response("{}", { status }))));
  const wrapper = mount(LibraryOperationsView, { props: { authState: "authenticated", permissions: ["library.read"] } });
  await flushPromises();
  expect(wrapper.text()).toContain(message);
  expect(wrapper.text()).not.toContain("资料管理尚未就绪");
  expect(wrapper.find("form").exists()).toBe(false);
  if (status === 401) expect(wrapper.get("a").attributes("href")).toContain("/api/v1/auth/login?return_to=");
  wrapper.unmount();
});

it.each([legacyWorkspace, { ...readiness, capabilities: {} }, { ...readiness, management: "unknown" }])("does not infer readiness from HTTP success or incomplete contracts", async (data) => {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response(JSON.stringify({ data, request_id: "req_unconfirmed" }), { status: 200 }))));
  const wrapper = mount(LibraryOperationsView, { props: { authState: "authenticated", permissions: ["library.read", "library.manage"] } });
  await flushPromises();
  expect(wrapper.text()).toContain("暂时无法确认资料管理能力");
  expect(wrapper.find("form").exists()).toBe(false);
  wrapper.unmount();
});

it("keeps future owner readiness honest when this Console cannot operate it", async () => {
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response(JSON.stringify({ data: { ...readiness, management: "ready", reason: "none", capabilities: { workspace: true, commands: true, operation_results: true } }, request_id: "req_future_owner" }), { status: 200 }))));
  const wrapper = mount(LibraryOperationsView, { props: { authState: "authenticated", permissions: ["library.read", "library.manage"] } });
  await flushPromises();
  expect(wrapper.text()).toContain("当前版本暂不支持资料管理");
  expect(wrapper.find("form").exists()).toBe(false);
  wrapper.unmount();
});

it("refreshes readiness and ignores a response after access is lost", async () => {
  let resolveLate: (response: Response) => void = () => {};
  let requests = 0;
  vi.stubGlobal("fetch", vi.fn(() => {
    requests += 1;
    if (requests === 1) return Promise.resolve(new Response("{}", { status: 503 }));
    return new Promise<Response>((resolve) => { resolveLate = resolve; });
  }));
  const wrapper = mount(LibraryOperationsView, { props: { authState: "authenticated", permissions: ["library.read"] } });
  await flushPromises();
  await wrapper.get("button").trigger("click");
  await wrapper.setProps({ authState: "signed_out" });
  resolveLate(new Response(JSON.stringify({ data: readiness, request_id: "req_late" }), { status: 200 }));
  await flushPromises();
  expect(wrapper.text()).toContain("登录状态已过期");
  expect(wrapper.text()).not.toContain("资料管理尚未就绪");
  expect(requests).toBe(2);
  wrapper.unmount();
});

it("does not fetch readiness without the separately verified Library read permission", async () => {
  const fetcher = vi.fn();
  vi.stubGlobal("fetch", fetcher);
  const wrapper = mount(LibraryOperationsView, { props: { authState: "authenticated", permissions: ["console.overview.read"] } });
  await flushPromises();
  expect(wrapper.text()).toContain("当前账户没有资料库查看权限");
  expect(fetcher).not.toHaveBeenCalled();
  wrapper.unmount();
});
