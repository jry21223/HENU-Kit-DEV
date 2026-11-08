import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import NoticeOperationsView from "./components/NoticeOperationsView.vue";
import type { NoticeVersion } from "./lib/console-gateway";

const operatorID = "171f1c6f-7b10-4c92-91a2-b39bf5af5302";
const permissions = ["notice.read", "notice.manage", "notice.review", "notice.distribute"];
const firstID = "471f1c6f-7b10-4c92-91a2-b39bf5af5302";
const secondID = "671f1c6f-7b10-4c92-91a2-b39bf5af5302";
function notice(id: string, state: "pending_review" | "approved" | "distributed" = "pending_review") {
  return { id, source: { id: "571f1c6f-7b10-4c92-91a2-b39bf5af5302", code: "henu-office", name: "学校办公室" }, version: 1, title: id === firstID ? "暑期安排" : "开学安排", body: id === firstID ? "不可变正文一" : "不可变正文二", source_url: "https://example.edu/notices/1", content_hash: "a".repeat(64), state, revision: state === "pending_review" ? 1 : 2, created_at: "2026-11-07T12:00:00Z", distribution_count: state === "distributed" ? 1 : 0 };
}
function response(items: NoticeVersion[] = [notice(firstID), notice(secondID, "approved")]) { return new Response(JSON.stringify({ data: { items, generated_at: "2026-11-07T12:01:00Z" }, request_id: "req_notice_snapshot" }), { status: 200 }); }
let wrapper: VueWrapper | undefined;
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals(); sessionStorage.clear(); window.history.replaceState({}, "", "/notices"); });

describe("Notice operational loop", () => {
  it("keeps the open management form and its draft after creating a source", async () => {
    vi.stubGlobal("fetch", vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => init?.method === "POST"
      ? new Response(JSON.stringify({ data: { id: secondID, code: "fixture-source", name: "新来源", canonical_url: "https://example.edu/new" }, request_id: "req_created_source" }), { status: 200 })
      : response()));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    const disclosure = wrapper.get("details");
    (disclosure.element as HTMLDetailsElement).open = true;
    await disclosure.trigger("toggle");
    await wrapper.findAll("form")[1]!.findAll("input")[1]!.setValue("下一条通知草稿");
    const sourceForm = wrapper.findAll("form")[0]!;
    await sourceForm.findAll("input")[0]!.setValue("fixture-source");
    await sourceForm.findAll("input")[1]!.setValue("新来源");
    await sourceForm.findAll("input")[2]!.setValue("https://example.edu/new");
    await sourceForm.trigger("submit");
    await flushPromises();
    expect(wrapper.text()).toContain("通知来源已创建");
    expect((wrapper.get("details").element as HTMLDetailsElement).open).toBe(true);
    const versionInputs = wrapper.findAll("form")[1]!.findAll("input");
    expect((versionInputs[0]!.element as HTMLInputElement).value).toBe(secondID);
    expect((versionInputs[1]!.element as HTMLInputElement).value).toBe("下一条通知草稿");
  });
  it("does not resolve a definitively denied write and permits a corrected new command", async () => {
    let allowed = false;
    let approved = false;
    let lookups = 0;
    const keys: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        keys.push(new Headers(init.headers).get("Idempotency-Key")!);
        if (!allowed) return new Response(JSON.stringify({ error: { code: "PERMISSION_DENIED", message: "denied" }, request_id: "req_denied_write" }), { status: 403 });
        approved = true;
        return new Response(JSON.stringify({ data: { version_id: firstID, state: "approved", revision: 2 }, request_id: "req_corrected_write" }), { status: 200 });
      }
      if (String(input).includes("/operations/")) {
        lookups++;
        return new Response(JSON.stringify({ data: { version_id: firstID, state: "approved", revision: 2 }, request_id: "req_unrelated_lookup" }), { status: 200 });
      }
      return response([notice(firstID, approved ? "approved" : "pending_review")]);
    }));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text() === "批准")!.trigger("click");
    await flushPromises();
    expect(lookups).toBe(0);
    expect(wrapper.text()).toContain("操作被拒绝");
    expect(wrapper.text()).not.toContain("审核已批准");
    expect(sessionStorage.getItem("henukit.console.pending-notice-operation.v1")).toBeNull();
    const retry = wrapper.findAll("button").find((button) => button.text() === "批准")!;
    expect(retry.attributes("disabled")).toBeUndefined();
    allowed = true;
    await retry.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("审核已批准");
    expect(keys).toHaveLength(2);
    expect(keys[1]).not.toBe(keys[0]);
    expect(lookups).toBe(0);
  });
  it("distinguishes a missing management connection from an owner outage", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: { code: "NOTICE_NOT_CONFIGURED", message: "通知工作台尚未接入" }, request_id: "req_notice_not_configured" }), { status: 503 })));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    expect(wrapper.text()).toContain("通知工作台尚未接入");
    expect(wrapper.text()).not.toContain("通知服务暂时不可用");
    expect(wrapper.findAll("form")).toHaveLength(0);
  });
  it("filters only the latest fifty versions and restores a selected immutable version from safe URL context", async () => {
    window.history.replaceState({}, "", `/notices?state=approved&selected=${secondID}`);
    vi.stubGlobal("fetch", vi.fn(async () => response()));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    expect(wrapper.text()).toContain("最近 50 个版本");
    expect(wrapper.get("[data-notice-list]").text()).toContain("开学安排");
    expect(wrapper.get("[data-notice-list]").text()).not.toContain("暑期安排");
    expect(wrapper.get("[data-notice-detail]").text()).toContain("不可变正文二");
    expect(wrapper.get("[data-notice-detail]").text()).not.toContain("不可变正文一");
    expect(wrapper.get("[data-notice-detail] a").attributes("href")).toBe("https://example.edu/notices/1");
    await wrapper.get("select[aria-label='版本状态筛选']").setValue("pending_review");
    await wrapper.findAll("button").find((button) => button.text().includes("暑期安排"))!.trigger("click");
    expect(new URLSearchParams(window.location.search).get("state")).toBe("pending_review");
    expect(new URLSearchParams(window.location.search).get("selected")).toBe(firstID);
  });
  it("does not let an old actor's late lookup clear the new actor's unresolved command", async () => {
    const nextActor = "271f1c6f-7b10-4c92-91a2-b39bf5af5302";
    const storageKey = "henukit.console.pending-notice-operation.v1";
    const firstKey = "idem_notice_review_11111111-1111-4111-8111-111111111111";
    const nextKey = "idem_notice_review_22222222-2222-4222-8222-222222222222";
    const pending = (actor: string, key: string) => ({ version: 1, operator_id: actor, operation: "review", idempotency_key: key, resource_id: firstID, target_label: "通知审核" });
    sessionStorage.setItem(storageKey, JSON.stringify(pending(operatorID, firstKey)));
    let completeOld!: (response: Response) => void;
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      if (!String(input).includes("/operations/")) return Promise.resolve(response());
      if (new Headers(init?.headers).get("Idempotency-Key") === firstKey) return new Promise<Response>((resolve) => { completeOld = resolve; });
      return Promise.resolve(new Response(JSON.stringify({ data: { operation: "review", status: "unknown" }, request_id: "req_next_unknown" }), { status: 200 }));
    }));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    sessionStorage.setItem(storageKey, JSON.stringify(pending(nextActor, nextKey)));
    await wrapper.setProps({ operatorID: nextActor });
    await flushPromises();
    completeOld(new Response(JSON.stringify({ data: { version_id: firstID, state: "approved", revision: 2 }, request_id: "req_old_success" }), { status: 200 }));
    await flushPromises();
    expect(JSON.parse(sessionStorage.getItem(storageKey)!)).toMatchObject({ operator_id: nextActor, idempotency_key: nextKey });
    expect(wrapper.text()).toContain("原请求仍已保留");
  });
  it.each(["success", "conflict", "transport"] as const)("keeps command B owned when command A's %s arrives after its successful recovery GET", async (lateResult) => {
    let confirmedA = false;
    let completeA!: (result: Response) => void;
    let failA!: (error: Error) => void;
    let lookupsA = 0;
    let keyA = ""; let keyB = "";
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") {
        if (String(input).includes(firstID)) {
          keyA = new Headers(init.headers).get("Idempotency-Key")!;
          return new Promise<Response>((resolve, reject) => { completeA = resolve; failA = reject; });
        }
        keyB = new Headers(init.headers).get("Idempotency-Key")!;
        return new Promise<Response>(() => undefined);
      }
      if (String(input).includes("/operations/")) {
        lookupsA++;
        expect(new Headers(init?.headers).get("Idempotency-Key")).toBe(keyA);
        confirmedA = true;
        return Promise.resolve(new Response(JSON.stringify({ data: { version_id: firstID, state: "approved", revision: 2 }, request_id: "req_a_resolved" }), { status: 200 }));
      }
      return Promise.resolve(response([notice(firstID, confirmedA ? "approved" : "pending_review"), notice(secondID)]));
    }));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text() === "批准")!.trigger("click");
    await wrapper.findAll("button").find((button) => button.text() === "再次查询结果")!.trigger("click");
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text().includes("开学安排"))!.trigger("click");
    await wrapper.findAll("button").find((button) => button.text() === "批准")!.trigger("click");
    expect(keyB).not.toBe(keyA);
    if (lateResult === "transport") failA(new Error("isolated late transport failure"));
    else completeA(new Response(JSON.stringify({ data: { version_id: firstID, state: "approved", revision: 2 }, request_id: "req_a_late" }), { status: lateResult === "conflict" ? 409 : 200 }));
    await flushPromises();
    expect(JSON.parse(sessionStorage.getItem("henukit.console.pending-notice-operation.v1")!)).toMatchObject({ idempotency_key: keyB, resource_id: secondID });
    expect(wrapper.findAll("button").find((button) => button.text() === "批准")!.attributes("disabled")).toBeDefined();
    expect(wrapper.text()).toContain("正在提交审核");
    expect(lookupsA).toBe(1);
  });
  it("does not replace an explicitly selected unavailable version with another review target", async () => {
    window.history.replaceState({}, "", `/notices?state=pending_review&selected=${firstID}`);
    vi.stubGlobal("fetch", vi.fn(async () => response([notice(secondID)])));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    expect(new URLSearchParams(window.location.search).get("selected")).toBe(firstID);
    expect(wrapper.find("[data-notice-detail]").exists()).toBe(false);
    expect(wrapper.text()).toContain("选中的通知版本不在最近 50 个版本中");
    expect(wrapper.findAll("button").some((button) => button.text() === "批准")).toBe(false);
    await wrapper.findAll("button").find((button) => button.text().includes("开学安排"))!.trigger("click");
    expect(new URLSearchParams(window.location.search).get("selected")).toBe(secondID);
    expect(wrapper.get("[data-notice-detail]").text()).toContain("开学安排");
  });
  it.each([
    { operation: "source_create", result: { id: "not-a-uuid", code: "fixture-source", name: "隔离来源", canonical_url: "https://example.edu/notices" } },
    { operation: "source_create", result: { id: secondID, code: "fixture-source" } },
    { operation: "version_create", result: { id: "not-a-uuid", source_id: firstID, version: 1, state: "pending_review", revision: 1 } },
    { operation: "version_create", result: { id: secondID, source_id: firstID, state: "pending_review", revision: 1 } },
  ])("keeps malformed $operation results unresolved instead of inventing a created target", async ({ operation, result }) => {
    let lookups = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") return new Response(JSON.stringify({ data: result, request_id: "req_malformed_creation" }), { status: 200 });
      if (String(input).includes("/operations/")) { lookups++; return new Response(JSON.stringify({ data: { operation, status: "unknown" }, request_id: "req_creation_unknown" }), { status: 200 }); }
      return response();
    }));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    const form = wrapper.findAll("form")[operation === "source_create" ? 0 : 1]!;
    const inputs = form.findAll("input");
    await inputs[0]!.setValue(operation === "source_create" ? "fixture-source" : firstID);
    await inputs[1]!.setValue(operation === "source_create" ? "隔离来源" : "隔离正文版本");
    await inputs[2]!.setValue("https://example.edu/notices");
    if (operation === "version_create") await form.get("textarea").setValue("隔离正文");
    await form.trigger("submit");
    await flushPromises();
    expect(lookups).toBe(1);
    expect(wrapper.text()).toContain("原请求仍已保留");
    expect(wrapper.text()).not.toContain(operation === "source_create" ? "通知来源已创建" : "通知版本已创建并进入待审核状态");
    expect(sessionStorage.getItem("henukit.console.pending-notice-operation.v1")).not.toBeNull();
    expect((wrapper.get("input[placeholder='来源 ID']").element as HTMLInputElement).value).not.toBe("not-a-uuid");
  });
  it.each([403, 404, 409, 503])("retains the original key when a recovery GET returns %s and never replays the POST", async (status) => {
    const key = "henukit.console.pending-notice-operation.v1";
    const original = { version: 1, operator_id: operatorID, operation: "review", idempotency_key: "idem_notice_review_11111111-1111-4111-8111-111111111111", resource_id: firstID, target_label: "通知审核" };
    sessionStorage.setItem(key, JSON.stringify(original));
    const fetcher = vi.fn((input: RequestInfo | URL) => Promise.resolve(String(input).includes("/operations/") ? new Response("", { status }) : response()));
    vi.stubGlobal("fetch", fetcher);
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    expect(JSON.parse(sessionStorage.getItem(key)!)).toEqual(original);
    expect(wrapper.text()).toContain("原请求仍已保留");
    expect(fetcher.mock.calls.every((args) => !((args as unknown as [unknown, RequestInit?])[1]?.method === "POST"))).toBe(true);
    expect(wrapper.findAll("button").find((button) => button.text() === "批准")?.attributes("disabled")).toBeDefined();
  });
  it("keeps the previous read honest and read-only when a refresh fails", async () => {
    const fetcher = vi.fn().mockResolvedValueOnce(response()).mockResolvedValue(new Response("", { status: 503 }));
    vi.stubGlobal("fetch", fetcher);
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text() === "刷新版本")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("更新失败，展示上次读取");
    expect(wrapper.get("[data-notice-detail]").text()).toContain("不可变正文一");
    expect(wrapper.get("time").attributes("datetime")).toBe("2026-11-07T12:01:00Z");
    expect(wrapper.findAll("button").some((button) => button.text() === "批准")).toBe(false);
  });
  it("does not confirm a malformed 200 result and does not replay its write", async () => {
    let writes = 0; let lookups = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") { writes++; return new Response(JSON.stringify({ data: {}, request_id: "req_malformed" }), { status: 200 }); }
      if (String(input).includes("/operations/")) { lookups++; return new Response(JSON.stringify({ data: { operation: "review", status: "unknown" }, request_id: "req_unknown" }), { status: 200 }); }
      return response();
    }));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    const approve = wrapper.findAll("button").find((button) => button.text() === "批准")!;
    approve.element.click(); approve.element.click();
    await flushPromises();
    expect(writes).toBe(1); expect(lookups).toBe(1);
    expect(wrapper.text()).not.toContain("审核已批准");
    expect(wrapper.text()).toContain("原请求仍已保留");
    expect(sessionStorage.getItem("henukit.console.pending-notice-operation.v1")).not.toBeNull();
  });
  it.each(["queued", "processing", "delivered", "failed"] as const)("labels only the latest owner-confirmed distribution status: %s", async (status) => {
    window.history.replaceState({}, "", `/notices?state=distributed&selected=${firstID}`);
    vi.stubGlobal("fetch", vi.fn(async () => response([{ ...notice(firstID, "distributed"), distribution_status: status }])));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    expect(wrapper.get("[data-notice-detail]").text()).toContain(status === "queued" ? "最近一次分发已排队，尚未确认送达" : status === "processing" ? "最近一次分发正在处理，尚未确认送达" : status === "delivered" ? "最近一次分发已送达" : "最近一次分发失败");
    if (status !== "delivered") expect(wrapper.text()).not.toContain("最近一次分发已送达");
  });
  it("confirms the immutable body version separately from its review revision", async () => {
    window.history.replaceState({}, "", `/notices?state=approved&selected=${firstID}`);
    vi.stubGlobal("fetch", vi.fn(async () => response([notice(firstID, "approved")])));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions } });
    await flushPromises();
    const dialog = wrapper.get("dialog").element as HTMLDialogElement;
    dialog.showModal = () => { dialog.open = true; };
    dialog.close = () => { dialog.open = false; };
    await wrapper.findAll("button").find((button) => button.text() === "创建分发任务")!.trigger("click");
    await flushPromises();
    expect(wrapper.get("dialog").text()).toContain("正文 v1 · 修订 r2");
  });
  it("describes read denial without asking for review permission", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 403 })));
    wrapper = mount(NoticeOperationsView, { props: { authState: "authenticated", operatorID, permissions: [] } });
    await flushPromises();
    expect(wrapper.text()).toContain("当前账户无权查看通知工作台");
    expect(wrapper.text()).not.toContain("没有通知审核权限");
  });
});
