import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import AccountTicketOperationsView from "./components/AccountTicketOperationsView.vue";

const operatorID = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
const first = "88888888-8888-4888-8888-888888888888";
const second = "77777777-7777-4777-8777-777777777777";
const permissions = ["account.tickets.read", "account.tickets.reply", "account.tickets.transition"];
function reference(id = first, status: "open" | "resolved" = "open") { return { id, reference: `HKT-${id}`, title: status === "open" ? "待处理事项" : "已解决事项", status, version: 3, updated_at: "2026-11-07T10:00:00Z" }; }
function facts(filter = "unresolved", items = [reference()], total = items.length) {
  return { scope: "global", as_of: "2026-11-07T10:01:00Z", stale: false, counts: { open: filter === "resolved" ? 0 : total, in_progress: 0, resolved: filter === "resolved" ? total : 0, unresolved: filter === "resolved" ? 0 : total }, window: { filter, limit: 100, matched_total: total, returned: items.length, has_more: total > items.length, items } };
}
function response(data: unknown, status = 200) { return new Response(JSON.stringify({ data, request_id: "req_ticket_facts_view" }), { status }); }
let wrapper: VueWrapper | undefined;
beforeEach(() => { sessionStorage.clear(); window.history.replaceState({}, "", "/account"); });
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals(); });
async function open() { wrapper = mount(AccountTicketOperationsView, { props: { authState: "authenticated", operatorID, permissions } }); await flushPromises(); return wrapper; }

it("opens the explicit unresolved global window instead of a restored resolved filter without N+1 detail reads", async () => {
  sessionStorage.setItem("henukit.console.account-ticket-view.v1", JSON.stringify({ version: 1, operator_id: operatorID, status: "resolved", selected_id: second, scroll_top: 40 }));
  window.history.replaceState({}, "", "/account?status=unresolved");
  const items = Array.from({ length: 100 }, (_, index) => reference(`${String(index).padStart(8, "0")}-8888-4888-8888-888888888888`));
  const fetcher = vi.fn((input: RequestInfo | URL) => Promise.resolve(String(input).endsWith("/facts?status=unresolved") ? response(facts("unresolved", items, 135)) : response({}, 503)));
  vi.stubGlobal("fetch", fetcher);
  await open();

  expect(fetcher.mock.calls.map(([input]) => String(input))).toEqual(["/api/v1/account/tickets/facts?status=unresolved"]);
  expect(wrapper!.get("select").element).toHaveProperty("value", "unresolved");
  expect(wrapper!.text()).toContain("全局匹配 135 条");
  expect(wrapper!.text()).toContain("本次载入 100 条");
  expect(wrapper!.text()).toContain("还有更多匹配工单未载入");
  expect(wrapper!.text()).toContain("最多 100 条窗口内");
  expect(wrapper!.get("time").attributes("datetime")).toBe("2026-11-07T10:01:00Z");
  expect(wrapper!.text()).not.toMatch(/未设置姓名|邮箱不可用/);
  await wrapper!.get(`[data-ticket-id='${items[0]!.id}']`).trigger("click");
  await flushPromises();
  expect(fetcher.mock.calls[fetcher.mock.calls.length - 1]![0]).toBe(`/api/v1/account/tickets/${items[0]!.id}`);
});

it("requests each status on the server and ignores the earlier filter's late response", async () => {
  let finishOld!: (response: Response) => void;
  const fetcher = vi.fn((input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/facts?status=all")) return Promise.resolve(response(facts("all")));
    if (path.endsWith("/facts?status=open")) return new Promise<Response>((resolve) => { finishOld = resolve; });
    return Promise.resolve(response(facts("resolved", [reference(second, "resolved")])));
  });
  vi.stubGlobal("fetch", fetcher);
  await open();
  await wrapper!.get("select").setValue("open");
  await wrapper!.get("select").setValue("resolved");
  await flushPromises();
  finishOld(response(facts("open")));
  await flushPromises();

  expect(fetcher.mock.calls.map(([input]) => String(input))).toEqual(["/api/v1/account/tickets/facts?status=all", "/api/v1/account/tickets/facts?status=open", "/api/v1/account/tickets/facts?status=resolved"]);
  expect(wrapper!.find(`[data-ticket-id='${first}']`).exists()).toBe(false);
  expect(wrapper!.get(`[data-ticket-id='${second}']`).text()).toContain("已解决事项");
  expect(new URLSearchParams(window.location.search).get("status")).toBe("resolved");
});

it.each([403, 503])("does not turn facts HTTP %s into a zero matching total", async (status) => {
  vi.stubGlobal("fetch", vi.fn(async () => response({}, status)));
  await open();
  expect(wrapper!.text()).toContain(status === 403 ? "当前账户没有工单查看权限" : "工单服务暂时不可用");
  expect(wrapper!.text()).not.toContain("全局匹配 0 条");
});

it("keeps an unconfirmed loading window distinct from a confirmed zero match", async () => {
  let finish!: (result: Response) => void;
  vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((resolve) => { finish = resolve; })));
  await open();
  expect(wrapper!.text()).toContain("正在读取工单队列");
  expect(wrapper!.text()).not.toMatch(/当前筛选下没有工单|本次队列中没有符合条件的工单|全局匹配 0 条/);
  finish(response(facts("all", [], 0))); await flushPromises();
  expect(wrapper!.text()).toContain("全局匹配 0 条");
  expect(wrapper!.text()).toContain("当前筛选下没有工单");
});

it("renders an owner-valid160-codepoint emoji title through the generated SDK instead of reporting a failed queue", async () => {
  const title = "🙂".repeat(160);
  vi.stubGlobal("fetch", vi.fn(async () => response(facts("all", [{ ...reference(), title }]))));
  await open();
  expect(wrapper!.get(`[data-ticket-id='${first}']`).text()).toContain(title);
  expect(wrapper!.text()).not.toContain("工单服务暂时不可用");
});
