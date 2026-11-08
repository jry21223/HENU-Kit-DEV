import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import App from "./App.vue";
import FoodOperationsView from "./components/FoodOperationsView.vue";

const actor = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
const asOf = "2026-11-07T10:01:00Z";
function id(index: number) { return `${String(index).padStart(8, "0")}-8888-4888-8888-888888888888`; }
const permissions = ["console.overview.read", "food.read", "account.tickets.read", "platform.operations.read", "notice.read", "library.read", "account.membership.write", "account.points.adjust"];
function session(grants = permissions, products = ["food", "account-portfolio", "notice", "library"]) { return { user: { id: actor }, expires_at: "2026-11-07T11:00:00Z", access_context: { permissions: grants, scopes: [{ kind: "platform" }, ...products.map((product_code) => ({ kind: "product", product_code }))], verified_at: asOf } }; }
function foodFacts(total = 205) { return { as_of: asOf, scope: "global", stale: false, counts: { pending_submissions: total, open_anomalies: 0, pending_tier_adjustments: 0 }, windows: { submissions: { filter: "pending", limit: 200, returned: Math.min(total, 200), has_more: total > 200, items: Array.from({ length: Math.min(total, 200) }, (_, i) => ({ id: id(i), status: "pending", version: 1, updated_at: asOf })) }, anomalies: { filter: "open", limit: 200, returned: 0, has_more: false, items: [] }, tier_adjustments: { filter: "pending", limit: 200, returned: 0, has_more: false, items: [] } } }; }
function ticketFacts(total = 135) { return { as_of: asOf, scope: "global", stale: false, counts: { open: total, in_progress: 0, resolved: 0, unresolved: total }, window: { filter: "unresolved", limit: 100, returned: Math.min(total, 100), has_more: total > 100, matched_total: total, items: Array.from({ length: Math.min(total, 100) }, (_, i) => ({ id: id(i), reference: `HKT-${id(i)}`, title: "仅队列可见的工单标题", status: "open", version: 1, updated_at: asOf })) } }; }
function platform() { return { access_context: { permissions: ["platform.operations.read"], scopes: [{ kind: "platform" }], verified_at: asOf }, accounts: [{ id: actor, email: "private-fixture@example.test", display_name: "私有账户标记", email_verified: true, status: "active", authorization_revision: 1, created_at: asOf, grants: [] }], sessions: [], audit: [], dependencies: { postgres: "ready", redis: "ready" }, generated_at: asOf, inbox_items: [{ id: id(901), source_product_code: "food", source_resource_type: "submission", source_resource_id: id(902), priority: "normal", status: "open", version: 1, created_at: asOf, updated_at: asOf }], mail: { pending: 1, processing: 0, retry_due: 0, accepted: 0, delivered: 2, failed: 8, dead_letters: 3 }, pagination: Object.fromEntries(["accounts", "sessions", "inbox_items", "audit"].map((key) => [key, { page: 1, next_page: null, next_cursor: null }])) }; }
function notices() { return { generated_at: asOf, items: [{ id: id(903), source: { id: id(904), code: "fixture", name: "隔离来源" }, version: 1, title: "不应出现在总览的标题", body: "不应出现在总览的私有正文", source_url: "https://example.test/notice", content_hash: "a".repeat(64), state: "distributed", revision: 2, created_at: asOf, distribution_count: 1, distribution_status: "queued" }] }; }
function response(data: unknown, status = 200) { return new Response(JSON.stringify({ data, request_id: "req_overview_business" }), { status }); }
let wrapper: VueWrapper | undefined;
let handler: (path: string, init?: RequestInit) => Promise<Response>;
const calls: Array<{ path: string; init?: RequestInit }> = [];
beforeEach(() => {
  calls.length = 0; sessionStorage.clear(); window.history.replaceState({}, "", "/");
  handler = async (path) => path.endsWith("/session") ? response(session()) : path.endsWith("/food/facts") ? response(foodFacts()) : path.includes("/tickets/facts") ? response(ticketFacts()) : path.includes("/operations?") ? response(platform()) : path.endsWith("/notices") ? response(notices()) : path.endsWith("/readiness") ? response({ management: "not_ready", reason: "management_migration_pending", capabilities: { workspace: false, commands: false, operation_results: false }, checked_at: asOf }) : response({}, 503);
  vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => { calls.push({ path: String(input), init }); return handler(String(input), init); }));
});
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers(); vi.unstubAllGlobals(); });
async function open() { wrapper = mount(App); await flushPromises(); return wrapper; }

it("shows owner-wide counts with exact entries despite summary503 and keeps a single diagnostic disclosure", async () => {
  await open();
  expect(wrapper!.get("[data-overview-task='food']").text()).toContain("205 条投稿待审核");
  expect(wrapper!.get("[data-overview-task='tickets']").text()).toContain("135 条未解决");
  expect(wrapper!.get("[data-overview-task='food'] a").attributes("href")).toBe("/food");
  expect(wrapper!.get("[data-overview-task='tickets'] a").attributes("href")).toBe("/account?status=unresolved");
  expect(wrapper!.text()).toContain("资料管理尚未就绪");
  expect(wrapper!.get("a[href='https://henukit.cn/library']").text()).toContain("公开目录");
  expect(wrapper!.findAll("details")).toHaveLength(1);
  expect(wrapper!.get("summary").text()).toBe("连接与诊断");
  expect(wrapper!.findAll("h1")).toHaveLength(1);
  expect(wrapper!.findAll("time").every((time) => time.attributes("datetime") === asOf)).toBe(true);
  expect(wrapper!.text()).not.toMatch(/模块证据对照|摘要可读取|6 个模块|已授予概览查看权限|仅队列可见的工单标题/);
  expect(calls.every((call) => !call.init?.method || call.init.method === "GET")).toBe(true);
});

it("extracts actual bounded Platform and Notice facts without displaying private records or claiming queued delivery", async () => {
  await open();
  const platformRow = wrapper!.get("[data-overview-platform]");
  expect(platformRow.text()).toContain("本页未结项 1 条");
  expect(platformRow.text()).toContain("本页载入 1 条，最多 20 条");
  expect(platformRow.text()).toContain("未重新排队死信 3 条");
  expect(platformRow.text()).toContain("列表创建时间上限");
  expect(platformRow.text()).toContain("邮件本次返回状态");
  expect(wrapper!.get("[data-overview-notice]").text()).toContain("最近 50 个版本内：待审核 0");
  expect(wrapper!.get("[data-overview-notice]").text()).toContain("排队 1");
  expect(wrapper!.get("[data-overview-notice]").text()).not.toContain("已送达 1");
  expect(wrapper!.text()).not.toMatch(/private-fixture|私有账户标记|私有正文|不应出现在总览的标题|全站用户/);
  expect(calls.some((call) => call.path.includes("/operations?accounts_page=1"))).toBe(true);
  expect(calls.some((call) => call.path.endsWith("/notices"))).toBe(true);
});

it("uses exact product scopes and permissions instead of the overview grant for native private facts", async () => {
  const normal = handler;
  handler = (path, init) => path.endsWith("/session") ? Promise.resolve(response(session(["console.overview.read", "food.read", "account.tickets.read"], ["food"]))) : normal(path, init);
  await open();
  expect(calls.some((call) => call.path.includes("tickets/facts"))).toBe(false);
  expect(calls.some((call) => call.path.includes("operations?"))).toBe(false);
  expect(calls.some((call) => call.path.endsWith("/notices") || call.path.endsWith("/readiness"))).toBe(false);
  expect(wrapper!.get("[data-overview-task='tickets']").text()).toContain("没有该模块的查看权限");
  expect(wrapper!.get("[data-overview-task='food']").text()).toContain("205");
});

it("reports true owner zero only after both reads succeed and keeps real entries", async () => {
  const normal = handler;
  handler = (path, init) => path.endsWith("/food/facts") ? Promise.resolve(response(foodFacts(0))) : path.includes("/tickets/facts") ? Promise.resolve(response(ticketFacts(0))) : normal(path, init);
  await open();
  expect(wrapper!.text()).toContain("美食与工单当前没有待处理事项");
  expect(wrapper!.get("[data-overview-platform]").text()).toContain("本页未结项 1 条");
  expect(wrapper!.find("a[href='/food']").exists()).toBe(true);
  expect(wrapper!.find("a[href='/account?status=unresolved']").exists()).toBe(true);
});

it("describes the point destination as adjustment rather than promising a complete ledger", async () => {
  await open();
  expect(wrapper!.get("a[href='/account/points']").text()).toBe("积分调整");
  expect(wrapper!.text()).not.toContain("积分账本运营");
});

it("distinguishes a missing Notice connection from an outage or a zero count", async () => {
  const normal = handler;
  handler = (path, init) => path.endsWith("/notices") ? Promise.resolve(new Response(JSON.stringify({ error: { code: "NOTICE_NOT_CONFIGURED", message: "未接入" }, request_id: "req_notice_not_configured" }), { status: 503 })) : normal(path, init);
  await open();
  expect(wrapper!.get("[data-overview-notice]").text()).toContain("通知工作台尚未接入");
  expect(wrapper!.get("[data-overview-notice]").text()).not.toMatch(/产品停服|待审核 0|通知服务暂时不可用/);
  expect(wrapper!.get("[data-overview-task='food']").text()).toContain("205");
});

it("distinguishes an owner-ready Library from this Console version's unimplemented management UI", async () => {
  const normal = handler;
  handler = (path, init) => path.endsWith("/readiness") ? Promise.resolve(response({ management: "ready", reason: "none", capabilities: { workspace: true, commands: true, operation_results: true }, checked_at: asOf })) : normal(path, init);
  await open();
  const row = wrapper!.get("[data-overview-library]");
  expect(row.text()).toContain("服务已报告管理能力可用");
  expect(row.text()).toContain("当前后台版本尚未接入管理操作");
  expect(row.text()).not.toContain("资料管理尚未就绪");
  expect(row.get("a[href='https://henukit.cn/library']").text()).toBe("打开公开目录");
  expect(row.findAll("form, input, textarea")).toHaveLength(0);
});

it("counts versions by their latest distribution state instead of counting all distribution records", async () => {
  const normal = handler;
  let latest: "delivered" | "queued" = "delivered";
  handler = (path, init) => path.endsWith("/notices") ? Promise.resolve(response({ ...notices(), items: [{ ...notices().items[0]!, distribution_count: latest === "delivered" ? 2 : 3, distribution_status: latest }] })) : normal(path, init);
  await open();
  expect(wrapper!.get("[data-overview-notice]").text()).toContain("标记送达 1 个版本");
  latest = "queued";
  await wrapper!.get("button[aria-label='刷新总览']").trigger("click"); await flushPromises();
  const row = wrapper!.get("[data-overview-notice]");
  expect(row.text()).toContain("各版本最近一次分发状态：排队 1 个版本 · 标记送达 0 个版本");
  expect(row.text()).toContain("排队不表示送达");
  expect(row.text()).not.toMatch(/窗口内分发记录|排队 3|标记送达 2/);
  expect(calls.filter((call) => call.path.endsWith("/notices"))).toHaveLength(2);
});

it("settles a hanging native module at2s and summary at3s without replacing source time or accepting late data", async () => {
  vi.useFakeTimers();
  let finishFood!: (response: Response) => void;
  const normal = handler;
  handler = (path, init) => path.endsWith("/food/facts") ? new Promise((resolve) => { finishFood = resolve; }) : path.endsWith("/overview") ? new Promise(() => undefined) : normal(path, init);
  await open();
  expect(wrapper!.get("[data-overview-task='tickets']").text()).toContain("135");
  await vi.advanceTimersByTimeAsync(1999); await flushPromises();
  expect(wrapper!.get("[data-overview-task='food']").text()).toContain("正在确认");
  await vi.advanceTimersByTimeAsync(1); await flushPromises();
  expect(wrapper!.get("[data-overview-task='food']").text()).toContain("本次未能确认");
  expect(calls.find((call) => call.path.endsWith("/food/facts"))!.init?.signal?.aborted).toBe(true);
  expect(wrapper!.text()).not.toContain("当前没有待处理事项");
  await vi.advanceTimersByTimeAsync(1000); await flushPromises();
  expect(wrapper!.get("details").text()).toContain("本次摘要诊断未能完成");
  finishFood(response(foodFacts())); await flushPromises();
  expect(wrapper!.get("[data-overview-task='food']").text()).not.toContain("205");
  expect(wrapper!.get("[data-overview-task='tickets'] time").attributes("datetime")).toBe(asOf);
});

it.each([
  { endpoint: "/account/tickets/facts?status=unresolved", selector: "[data-overview-task='tickets']", loading: "正在确认", failed: "本次未能确认", ready: "135 条未解决" },
  { endpoint: "/operations?", selector: "[data-overview-platform]", loading: "正在读取本页运营事实", failed: "本次未能读取平台运营事实", ready: "本页未结项 1 条" },
  { endpoint: "/notices", selector: "[data-overview-notice]", loading: "正在读取通知版本窗口", failed: "本次未能读取通知版本", ready: "最近 50 个版本内" },
  { endpoint: "/library/readiness", selector: "[data-overview-library]", loading: "正在确认资料管理状态", failed: "本次未能确认资料管理状态", ready: "资料管理尚未就绪" },
])("bounds the independent $endpoint read at2s and rejects its late successful payload", async ({ endpoint, selector, loading, failed, ready }) => {
  vi.useFakeTimers();
  let finish!: (response: Response) => void;
  let delayedPath = "";
  const normal = handler;
  handler = (path, init) => {
    if (!path.includes(endpoint)) return normal(path, init);
    delayedPath = path;
    return new Promise((resolve) => { finish = resolve; });
  };
  await open();
  await vi.advanceTimersByTimeAsync(1999); await flushPromises();
  expect(wrapper!.get(selector).text()).toContain(loading);
  await vi.advanceTimersByTimeAsync(1); await flushPromises();
  expect(wrapper!.get(selector).text()).toContain(failed);
  expect(calls.find((call) => call.path === delayedPath)!.init?.signal?.aborted).toBe(true);
  expect(wrapper!.get("[data-overview-task='food']").text()).toContain("205 条投稿待审核");
  finish(await normal(delayedPath)); await flushPromises();
  expect(wrapper!.get(selector).text()).toContain(failed);
  expect(wrapper!.get(selector).text()).not.toContain(ready);
  expect(wrapper!.get("[data-overview-task='food'] time").attributes("datetime")).toBe(asOf);
  expect(calls.filter((call) => call.path === delayedPath)).toHaveLength(1);
});

it("clears combined facts after summary403 and ignores late private JSON while retaining the Session", async () => {
  let finishSummary!: (response: Response) => void;
  let finishJSON!: (value: unknown) => void;
  const normal = handler;
  handler = (path, init) => {
    if (path.endsWith("/overview")) return new Promise((resolve) => { finishSummary = resolve; });
    if (path.endsWith("/food/facts")) { const result = response(foodFacts()); vi.spyOn(result, "json").mockReturnValue(new Promise((resolve) => { finishJSON = resolve; })); return Promise.resolve(result); }
    return normal(path, init);
  };
  await open();
  expect(wrapper!.text()).toContain("135 条未解决");
  finishSummary(response({}, 403)); await flushPromises();
  finishJSON({ data: foodFacts(), request_id: "req_late_private" }); await flushPromises();
  expect(wrapper!.text()).toContain("权限已验证");
  expect(wrapper!.text()).toContain("当前账户没有总览查看权限");
  expect(wrapper!.text()).not.toMatch(/135 条未解决|205 条投稿/);
});

it("opens the existing Food destination in the global scope without restoring a campus", async () => {
  window.history.replaceState({}, "", "/food");
  handler = async () => response({ status: "ok", status_message: "可读", stale: false, as_of: asOf, submissions: [], anomaly_tickets: [], tier_adjustments: [], posts: [] });
  wrapper = mount(FoodOperationsView, { props: { authState: "authenticated", operatorID: actor, permissions: ["food.read"] } }); await flushPromises();
  expect(calls.map((call) => call.path)).toEqual(["/api/v1/food"]);
  expect(wrapper.get("select[aria-label='校区筛选']").element).toHaveProperty("value", "");
});

it.each(["/food/facts", "/account/tickets/facts?status=unresolved", "/operations?", "/notices", "/library/readiness"])("lets the central boundary remove all verified facts on a real %s Session401", async (endpoint) => {
  const normal = handler;
  handler = (path, init) => path.includes(endpoint) ? Promise.resolve(response({}, 401)) : normal(path, init);
  await open();
  expect(wrapper!.text()).toContain("登录已过期，请重新登录后继续");
  expect(wrapper!.text()).not.toContain("权限已验证");
  expect(wrapper!.findAll("[data-overview-task]")).toHaveLength(0);
  expect(wrapper!.findAll("h1")).toHaveLength(1);
});

it("does not let the previous principal's late JSON replace a newly verified actor's facts", async () => {
  let finishOldJSON!: (data: unknown) => void;
  const normal = handler;
  handler = (path, init) => {
    if (!path.endsWith("/food/facts")) return normal(path, init);
    const result = response(foodFacts());
    vi.spyOn(result, "json").mockReturnValue(new Promise((resolve) => { finishOldJSON = resolve; }));
    return Promise.resolve(result);
  };
  await open();
  wrapper!.unmount(); wrapper = undefined;
  handler = (path, init) => {
    if (path.endsWith("/session")) return Promise.resolve(response({ ...session(), user: { id: id(999) } }));
    if (path.endsWith("/food/facts")) return Promise.resolve(response(foodFacts(0)));
    return normal(path, init);
  };
  await open();
  finishOldJSON({ data: foodFacts(), request_id: "req_old_actor" }); await flushPromises();
  expect(wrapper!.get("[data-overview-task='food']").text()).toContain("0 条投稿待审核");
  expect(wrapper!.get("[data-overview-task='food']").text()).not.toContain("205");
});

it("does not expose an unauthorized owner's legacy summary content through diagnostic text", async () => {
  const normal = handler;
  handler = (path, init) => path.endsWith("/session") ? Promise.resolve(response(session(["console.overview.read"])))
    : path.endsWith("/overview") ? Promise.resolve(response({ generated_at: asOf, modules: ["portal", "platform", "notice", "library", "quizcraft", "food"].map((id) => ({ id, status: "ok", status_message: id === "food" ? "private-owner-count-205" : "摘要连接可读", metrics: [], as_of: asOf, request_id: `req_${id}` })) })) : normal(path, init);
  await open();
  expect(wrapper!.text()).not.toContain("private-owner-count-205");
  expect(wrapper!.get("details").text()).toContain("当前账户未获该模块查看权限");
  expect(calls.filter((call) => /facts|notices|operations\?|readiness/.test(call.path))).toHaveLength(0);
});
