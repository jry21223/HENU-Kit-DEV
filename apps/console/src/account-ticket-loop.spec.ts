import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import AccountTicketOperationsView from "./components/AccountTicketOperationsView.vue";

const operator = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
const otherOperator = "271f1c6f-7b10-4c92-91a2-b39bf5af5302";
const a = "88888888-8888-4888-8888-888888888888";
const b = "77777777-7777-4777-8777-777777777777";
const permissions = ["account.tickets.read", "account.tickets.reply", "account.tickets.transition"];
function ticket(id: string, version = 1, status = "open") {
  return { id, reference: `HKT-${id}`, title: id === a ? "工单 A" : "工单 B", category: "account", status, version, created_at: "2026-07-28T00:00:00Z", updated_at: "2026-07-28T00:01:00Z" };
}
function response(data: unknown, status = 200) { return new Response(JSON.stringify({ data, request_id: "req_ticket_fixture" }), { status, headers: { "Content-Type": "application/json" } }); }
function detail(id: string, version = 1) { return response({ ticket: ticket(id, version), messages: [], events: [] }); }
function queueResponse(items: ReturnType<typeof ticket>[], path: string) {
  const filter = new URL(path, "https://console.test").searchParams.get("status") ?? "all";
  const counts = { open: items.filter((item) => item.status === "open").length, in_progress: items.filter((item) => item.status === "in_progress").length, resolved: items.filter((item) => item.status === "resolved").length, unresolved: items.filter((item) => item.status !== "resolved").length };
  const window = items.filter((item) => filter === "all" || (filter === "unresolved" ? item.status !== "resolved" : item.status === filter)).map(({ id, reference, title, status, version, updated_at }) => ({ id, reference, title, status, version, updated_at }));
  return response({ scope: "global", as_of: "2026-07-28T00:02:00Z", stale: false, counts, window: { filter, limit: 100, matched_total: window.length, returned: window.length, has_more: false, items: window } });
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; }
let wrapper: VueWrapper | undefined;
const requests: Array<{ path: string; method: string; body: unknown; key: string | null }> = [];
let route: (path: string, init?: RequestInit) => Promise<Response>;
beforeEach(() => {
  window.history.replaceState({}, "", "/account");
  sessionStorage.clear(); requests.length = 0;
  route = async (path) => path.includes("/tickets/facts?") ? queueResponse([ticket(a), ticket(b)], path) : detail(path.split("/").pop()!);
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    requests.push({ path, method: init?.method ?? "GET", body: init?.body ? JSON.parse(String(init.body)) : undefined, key: new Headers(init?.headers).get("Idempotency-Key") });
    return route(path, init);
  }));
});
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals(); });
async function open() {
  wrapper = mount(AccountTicketOperationsView, { props: { authState: "authenticated", permissions, operatorID: operator } });
  await flushPromises(); return wrapper;
}
function button(text: string) { const found = wrapper!.findAll("button").find((item) => item.text().includes(text)); if (!found) throw new Error(`Missing button ${text}`); return found; }

it.each([403, 503])("does not claim a zero queue before initial HTTP %s is confirmed", async (status) => {
  const initialQueue = deferred<Response>();
  route = async () => initialQueue.promise;
  await open();
  expect(wrapper!.text()).toContain("正在读取工单队列");
  expect(wrapper!.text()).not.toContain("本次载入 0 条");
  initialQueue.resolve(response({}, status)); await flushPromises();
  expect(wrapper!.text()).toContain(status === 403 ? "当前账户没有工单查看权限" : "工单服务暂时不可用");
  expect(wrapper!.text()).not.toContain("本次载入 0 条");
});

it("keeps B selected when an earlier A detail arrives and replies only to B's version", async () => {
  const slowA = deferred<Response>();
  route = async (path, init) => path.includes("/tickets/facts?") ? queueResponse([ticket(a), ticket(b, 4)], path)
    : init?.method === "POST" ? response({ ticket: ticket(b, 5, "in_progress") })
    : path.endsWith(a) ? slowA.promise : detail(b, 4);
  await open();
  await button("工单 A").trigger("click");
  await button("工单 B").trigger("click"); await flushPromises();
  slowA.resolve(detail(a)); await flushPromises();
  expect(wrapper!.find('[data-account-ticket-detail-state="ready"]').text()).toContain("工单 B");
  await wrapper!.find("textarea").setValue("Reply to B");
  await wrapper!.find("form").trigger("submit"); await flushPromises();
  expect(requests.find((item) => item.method === "POST")).toMatchObject({ path: `/api/v1/account/tickets/${b}/replies`, body: { body: "Reply to B", expected_version: 4 } });
});

it.each(["write response", "original-key recovery"])("keeps the changed filter after A's delayed %s succeeds", async (outcome) => {
  const slowWrite = deferred<Response>();
  let committed = false;
  route = async (path, init) => {
    if (path.includes("/tickets/facts?")) return queueResponse([ticket(a, committed ? 2 : 1, committed ? "in_progress" : "open"), ticket(b, 3, "resolved")], path);
    if (init?.method === "POST") return slowWrite.promise;
    if (path.includes("/operations/")) return response({ operation: "ticket_reply", status: "succeeded", result: { ticket: ticket(a, 2, "in_progress") } });
    return detail(a, committed ? 2 : 1);
  };
  await open(); await button("工单 A").trigger("click"); await flushPromises();
  await wrapper!.find("textarea").setValue("Reply while changing the queue filter");
  await wrapper!.find("form").trigger("submit");
  await wrapper!.find('select[aria-label="工单状态"]').setValue("resolved");
  committed = true;
  slowWrite.resolve(outcome === "write response" ? response({ ticket: ticket(a, 2, "in_progress") }) : response({}, 503));
  await flushPromises();
  expect(wrapper!.text()).toContain("回复已写入工单");
  expect(wrapper!.find('select[aria-label="工单状态"]').element).toHaveProperty("value", "resolved");
  expect(wrapper!.find('[data-account-ticket-detail-state="idle"]').exists()).toBe(true);
  expect(wrapper!.findAll("button").some((item) => item.text().includes("工单 A"))).toBe(false);
  expect(button("工单 B").exists()).toBe(true);
  expect(requests.filter((item) => item.path === `/api/v1/account/tickets/${a}`)).toHaveLength(1);
  expect(requests.filter((item) => item.path.includes("/tickets/facts?"))).toHaveLength(3);
  expect(sessionStorage.getItem("henukit.console.pending-account-ticket-operation.v1")).toBeNull();
});

it("recovers a lost reply only by the original read-only key and stores no reply body", async () => {
  let queries = 0;
  route = async (path, init) => {
    if (path.includes("/tickets/facts?")) return queueResponse([ticket(a)], path);
    if (init?.method === "POST") throw new Error("write response lost after commit");
    if (path.includes("/operations/ticket_reply/")) {
      queries++;
      return queries < 3 ? response({ operation: "ticket_reply", status: "unknown" }) : response({ operation: "ticket_reply", status: "succeeded", result: { ticket: ticket(a, 2, "in_progress") } });
    }
    return detail(a, queries >= 3 ? 2 : 1);
  };
  await open(); await button("工单 A").trigger("click"); await flushPromises();
  await wrapper!.find("textarea").setValue("Private reply that must stay out of storage");
  await wrapper!.find("form").trigger("submit"); await flushPromises();
  const firstPost = requests.find((item) => item.method === "POST")!;
  expect(queries).toBe(1);
  const stored = sessionStorage.getItem("henukit.console.pending-account-ticket-operation.v1")!;
  expect(JSON.parse(stored)).toEqual({ version: 1, operator_id: operator, operation: "ticket_reply", idempotency_key: firstPost.key, resource_id: a, expected_version: 1 });
  expect(stored).not.toContain("Private reply");
  expect(wrapper!.text()).toContain("结果仍未确认");
  await button("查询原操作结果").trigger("click"); await flushPromises();
  expect(requests.filter((item) => item.method === "POST")).toHaveLength(1);
  wrapper!.unmount(); wrapper = undefined;
  await open(); await flushPromises();
  expect(queries).toBe(3);
  expect(requests.filter((item) => item.method === "POST")).toHaveLength(1);
  expect(requests.filter((item) => item.path.includes("/operations/")).every((item) => item.path.endsWith(firstPost.key!))).toBe(true);
  expect(sessionStorage.getItem("henukit.console.pending-account-ticket-operation.v1")).toBeNull();
  expect(wrapper!.text()).toContain("回复已写入工单");
});

it("discards previous actor data and pending recovery when the operator changes", async () => {
  const delayed = deferred<Response>();
  route = async (path) => path.includes("/tickets/facts?") ? queueResponse([ticket(a)], path) : delayed.promise;
  await open(); await button("工单 A").trigger("click");
  sessionStorage.setItem("henukit.console.pending-account-ticket-operation.v1", JSON.stringify({ version: 1, operator_id: operator, operation: "ticket_reply", idempotency_key: "idem_account_reply_11111111-1111-4111-8111-111111111111", resource_id: a, expected_version: 1 }));
  route = async (path) => queueResponse([], path);
  await wrapper!.setProps({ operatorID: otherOperator }); await flushPromises();
  delayed.resolve(detail(a)); await flushPromises();
  expect(wrapper!.text()).not.toContain("工单 A");
  expect(sessionStorage.getItem("henukit.console.pending-account-ticket-operation.v1")).toBeNull();
  expect(requests.some((item) => item.path.includes("/operations/") || item.method === "POST")).toBe(false);
});

it.each([403, 404, 503])("keeps an unknown reply blocked after result query HTTP %s", async (status) => {
  route = async (path, init) => path.includes("/tickets/facts?") ? queueResponse([ticket(a)], path)
    : init?.method === "POST" ? Promise.reject(new Error("lost response"))
    : path.includes("/operations/") ? response({}, status) : detail(a);
  await open(); await button("工单 A").trigger("click"); await flushPromises();
  await wrapper!.find("textarea").setValue("My reply"); await wrapper!.find("form").trigger("submit"); await flushPromises();
  expect(button("发送回复").attributes("disabled")).toBeDefined();
  await wrapper!.find("textarea").setValue("Edited reply cannot cancel unknown outcome");
  await wrapper!.find("form").trigger("submit"); await button("查询原操作结果").trigger("click"); await flushPromises();
  expect(requests.filter((item) => item.method === "POST")).toHaveLength(1);
  expect(sessionStorage.getItem("henukit.console.pending-account-ticket-operation.v1")).not.toBeNull();
});

it("filters the bounded queue and restores the same status and selected ticket after reload", async () => {
  route = async (path) => path.includes("/tickets/facts?") ? queueResponse([ticket(a), ticket(b, 3, "resolved")], path) : detail(b, 3);
  await open();
  expect(wrapper!.text()).toContain("全局工单按服务端所选状态筛选");
  await wrapper!.find('select[aria-label="工单状态"]').setValue("resolved");
  await flushPromises();
  expect(wrapper!.findAll("button").some((item) => item.text().includes("工单 A"))).toBe(false);
  await button("工单 B").trigger("click"); await flushPromises();
  await button("返回队列").trigger("click");
  expect(wrapper!.find('select[aria-label="工单状态"]').element).toHaveProperty("value", "resolved");
  wrapper!.unmount(); wrapper = undefined;
  await open(); await flushPromises();
  expect(wrapper!.find('select[aria-label="工单状态"]').element).toHaveProperty("value", "resolved");
  expect(wrapper!.find('[data-account-ticket-detail-state="ready"]').text()).toContain("工单 B");
  expect(new URLSearchParams(window.location.search).get("status")).toBe("resolved");
});

it("queries even a rejected write response because identity enrichment may fail after the owner committed", async () => {
  route = async (path, init) => path.includes("/tickets/facts?") ? queueResponse([ticket(a)], path)
    : init?.method === "POST" ? response({}, 403)
    : path.includes("/operations/") ? response({ operation: "ticket_reply", status: "succeeded", result: { ticket: ticket(a, 2, "in_progress") } }) : detail(a);
  await open(); await button("工单 A").trigger("click"); await flushPromises();
  await wrapper!.find("textarea").setValue("Committed before enrichment rejection");
  await wrapper!.find("form").trigger("submit"); await flushPromises();
  expect(requests.filter((item) => item.path.includes("/operations/"))).toHaveLength(1);
  expect(wrapper!.text()).toContain("回复已写入工单");
});

it("ignores an old component's late write response after another actor opens the workspace", async () => {
  const lateWrite = deferred<Response>();
  route = async (path, init) => path.includes("/tickets/facts?") ? queueResponse([ticket(a)], path)
    : init?.method === "POST" ? lateWrite.promise : path.includes("/operations/") ? response({ operation: "ticket_reply", status: "unknown" }) : detail(a);
  await open(); await button("工单 A").trigger("click"); await flushPromises();
  await wrapper!.find("textarea").setValue("Old actor reply"); await wrapper!.find("form").trigger("submit");
  wrapper!.unmount(); wrapper = undefined;
  const newPending = { version: 1, operator_id: otherOperator, operation: "ticket_reply", idempotency_key: "idem_account_reply_11111111-1111-4111-8111-111111111111", resource_id: b, expected_version: 1 };
  sessionStorage.setItem("henukit.console.pending-account-ticket-operation.v1", JSON.stringify(newPending));
  wrapper = mount(AccountTicketOperationsView, { props: { authState: "authenticated", permissions, operatorID: otherOperator } }); await flushPromises();
  lateWrite.resolve(response({ ticket: ticket(a, 2, "in_progress") })); await flushPromises();
  expect(JSON.parse(sessionStorage.getItem("henukit.console.pending-account-ticket-operation.v1")!)).toEqual(newPending);
  expect(wrapper!.text()).not.toContain("回复已写入工单");
});

it("retains a reply draft after a revision conflict and requires an explicit submit against the new version", async () => {
  let version = 1;
  route = async (path, init) => {
    if (path.includes("/tickets/facts?")) return queueResponse([ticket(a, version)], path);
    if (init?.method === "POST") { version = 2; return response({}, 409); }
    return detail(a, version);
  };
  await open(); await button("工单 A").trigger("click"); await flushPromises();
  await wrapper!.find("textarea").setValue("Keep this draft after conflict");
  await wrapper!.find("form").trigger("submit"); await flushPromises();
  expect(wrapper!.find("textarea").element).toHaveProperty("value", "Keep this draft after conflict");
  expect(wrapper!.text()).toContain("版本 2");
  expect(wrapper!.text()).toContain("回复草稿已保留");
  expect(requests.filter((item) => item.method === "POST")).toHaveLength(1);
  expect(sessionStorage.getItem("henukit.console.pending-account-ticket-operation.v1")).toBeNull();
  await wrapper!.find("form").trigger("submit"); await flushPromises();
  expect(requests.filter((item) => item.method === "POST")[1]?.body).toMatchObject({ expected_version: 2 });
});

it.each([503, 404])("keeps the draft without claiming refreshed records after conflict then GET %s", async (status) => {
  let conflicted = false;
  route = async (path, init) => {
    if (init?.method === "POST") { conflicted = true; return response({}, 409); }
    if (conflicted) return response({}, status);
    return path.includes("/tickets/facts?") ? queueResponse([ticket(a)], path) : detail(a);
  };
  await open(); await button("工单 A").trigger("click"); await flushPromises();
  await wrapper!.find("textarea").setValue("Draft survives failed conflict refresh");
  await wrapper!.find("form").trigger("submit"); await flushPromises();
  expect(wrapper!.text()).not.toContain("已刷新最新记录");
  expect(wrapper!.text()).toContain("回复草稿已保留");
  expect(wrapper!.text()).toContain("工单服务暂时不可用");
  expect(requests.filter((item) => item.method === "POST")).toHaveLength(1);
  expect(sessionStorage.getItem("henukit.console.pending-account-ticket-operation.v1")).toBeNull();
  route = async (path) => path.includes("/tickets/facts?") ? queueResponse([ticket(a, 2)], path) : detail(a, 2);
  await button("重新加载").trigger("click"); await flushPromises();
  await button("工单 A").trigger("click"); await flushPromises();
  expect(wrapper!.find("textarea").element).toHaveProperty("value", "Draft survives failed conflict refresh");
  expect(wrapper!.text()).toContain("版本 2");
  expect(requests.filter((item) => item.method === "POST")).toHaveLength(1);
});

it("does not offer a reply or transition on an already resolved ticket", async () => {
  route = async (path) => path.includes("/tickets/facts?") ? queueResponse([ticket(a, 3, "resolved")], path) : response({ ticket: ticket(a, 3, "resolved"), messages: [], events: [] });
  await open(); await button("工单 A").trigger("click"); await flushPromises();
  expect(wrapper!.find("textarea").exists()).toBe(false);
  expect(wrapper!.findAll("button").some((item) => item.text().includes("标记已解决") || item.text().includes("开始处理"))).toBe(false);
});
