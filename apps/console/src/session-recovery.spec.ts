import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import App from "./App.vue";
import { consoleLoginHref, fetchLibraryReadiness, fetchNoticeSnapshot, resolveAccountOperation, resolveFoodOperation } from "./lib/console-gateway";
import { beginConsolePrincipal, isCurrentConsolePrincipal, onConsoleSessionExpired } from "./lib/console-session";
import { writePendingAccountTicketOperation, writePendingFoodOperation, writePendingNoticeOperation } from "./lib/pending-operations";

const session = {
  data: {
    user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
    access_context: { permissions: ["notice.read"], scopes: [{ kind: "product", product_code: "notice" }], verified_at: "2026-11-07T12:00:00Z" },
    expires_at: "2026-11-08T12:00:00Z",
  },
  request_id: "req_session_recovery",
};

const privateReads = [
  { name: "Notice", request: fetchNoticeSnapshot, data: { items: [], generated_at: "2026-11-07T12:00:00Z" } },
  { name: "Library readiness", request: fetchLibraryReadiness, data: { management: "not_ready", reason: "management_migration_pending", capabilities: { workspace: false, commands: false, operation_results: false }, checked_at: "2026-11-07T12:00:00Z" } },
  { name: "Account original operation", request: () => resolveAccountOperation("ticket_reply", "idem_ticket_reply_11111111-1111-4111-8111-111111111111"), data: { operation: "ticket_reply", status: "unknown" } },
  { name: "Food original operation", request: () => resolveFoodOperation("post_edit", "idem_food_post_edit_11111111-1111-4111-8111-111111111111"), data: { operation: "post_edit", state: "unknown" } },
  { name: "Food stored failure", request: () => resolveFoodOperation("post_edit", "idem_food_post_edit_11111111-1111-4111-8111-111111111111"), data: { operation: "post_edit", state: "failed", resource_id: "11111111-1111-4111-8111-111111111111", error_code: "FOOD_CONFLICT" } },
];

let wrapper: VueWrapper | undefined;
beforeEach(() => { window.sessionStorage.clear(); beginConsolePrincipal(); });
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals(); });

describe("Console Session recovery", () => {
  it.each(["unresolved", "all", "open", "in_progress", "resolved"])("preserves the explicit %s ticket filter through App and the central facts401 login boundary", async (status) => {
    window.history.replaceState({}, "", `/account?status=${status}&search=private-search-body&title=private-title&email=private-address%40example.test&return_to=https%3A%2F%2Fexample.test`);
    const ticketSession = structuredClone(session);
    ticketSession.data.access_context.permissions = ["account.tickets.read"];
    ticketSession.data.access_context.scopes = [{ kind: "product", product_code: "account-portfolio" }];
    const fetcher = vi.fn(async (input: RequestInfo | URL) => String(input).endsWith("/session")
      ? new Response(JSON.stringify(ticketSession), { status: 200 })
      : new Response("", { status: 401 }));
    vi.stubGlobal("fetch", fetcher);
    wrapper = mount(App);
    await flushPromises();

    expect(fetcher.mock.calls.map(([input]) => String(input))).toEqual(["/api/v1/session", `/api/v1/account/tickets/facts?status=${status}`]);
    expect(wrapper.text()).toContain("登录已过期，请重新登录后继续");
    expect(wrapper.text()).not.toContain("权限已验证");
    expect(wrapper.findAll("[data-ticket-id]")).toHaveLength(0);
    const login = wrapper.get("a[href^='/api/v1/auth/login']");
    expect(new URL(login.attributes("href")!, window.location.origin).searchParams.get("return_to")).toBe(`/account?status=${status}`);
    expect(login.attributes("href")).not.toMatch(/private-search|private-title|private-address|example\.test/);
    expect(Object.keys(sessionStorage).map((key) => sessionStorage.getItem(key)).join(" ")).not.toMatch(/private-search|private-title|private-address/);
  });

  it("removes the verified Shell when a product request observes expiry and returns to the same safe deep page", async () => {
    window.history.replaceState({}, "", "/notices?state=pending_review&page=2#queue");
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => String(input).endsWith("/session")
      ? new Response(JSON.stringify(session), { status: 200 })
      : new Response("", { status: 401 })));
    wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).not.toContain("权限已验证");
    const login = wrapper.get("a[href^='/api/v1/auth/login']");
    expect(new URL(login.attributes("href")!, window.location.origin).searchParams.get("return_to")).toBe("/notices?state=pending_review&page=2#queue");
    expect(wrapper.text()).toContain("登录已过期，请重新登录后继续");
  });

  it("shows an unverified retry state when the root Session check is unavailable", async () => {
    window.history.replaceState({}, "", "/");
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 503 })));
    wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).toContain("暂时无法验证访问权限，请重试连接");
    expect(wrapper.get("h1").text()).toBe("全局总览");
    expect(wrapper.findAll("h1")).toHaveLength(1);
    expect(wrapper.findAll("[data-module-card]")).toHaveLength(0);
    expect(wrapper.text()).not.toMatch(/无权读取|缺少相应权限|权限已验证/);
    expect(wrapper.find("[aria-label='服务端验证的访问上下文']").exists()).toBe(false);
    expect(wrapper.findAll("button").some((button) => button.text() === "重试连接")).toBe(true);
  });

  it.each([
    [401, "请登录后继续"],
    [403, "当前账户没有此工作台的访问权限"],
  ])("keeps root HTTP %s distinct from dependency failure", async (status, message) => {
    window.history.replaceState({}, "", "/");
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status })));
    wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).toContain(message);
    expect(wrapper.findAll("h1")).toHaveLength(1);
    expect(wrapper.findAll("[data-module-card]")).toHaveLength(0);
    expect(wrapper.text()).not.toContain("无权读取");
    expect(wrapper.text()).not.toContain("暂时无法验证访问权限");
    expect(wrapper.text()).not.toContain("权限已验证");
  });

  it("removes the verified Shell when Library readiness observes Session expiry", async () => {
    window.history.replaceState({}, "", "/library");
    const librarySession = structuredClone(session);
    librarySession.data.access_context.permissions = ["library.read"];
    librarySession.data.access_context.scopes = [{ kind: "product", product_code: "library" }];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => String(input).endsWith("/session")
      ? new Response(JSON.stringify(librarySession), { status: 200 })
      : new Response("", { status: 401 })));
    wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).not.toContain("权限已验证");
    expect(wrapper.text()).toContain("登录已过期，请重新登录后继续");
    const login = wrapper.get("a[href^='/api/v1/auth/login']");
    expect(new URL(login.attributes("href")!, window.location.origin).searchParams.get("return_to")).toBe("/library");
  });

  it.each([
    { path: "/food", product: "food", permissions: ["food.read", "food.review"] },
    { path: "/account", product: "account-portfolio", permissions: ["account.tickets.read", "account.tickets.reply"] },
    { path: "/notices?state=approved&selected=11111111-1111-4111-8111-111111111111", product: "notice", permissions: ["notice.read", "notice.review"] },
  ])("removes the verified Shell when restored $path operation lookup observes expiry", async ({ path, product, permissions }) => {
    window.history.replaceState({}, "", path);
    const productSession = structuredClone(session);
    productSession.data.access_context.permissions = permissions;
    productSession.data.access_context.scopes = [{ kind: "product", product_code: product }];
    const recovery = { version: 1 as const, operator_id: productSession.data.user.id, idempotency_key: "idem_original_11111111-1111-4111-8111-111111111111", resource_id: "11111111-1111-4111-8111-111111111111" };
    if (path === "/food") expect(writePendingFoodOperation({ ...recovery, operation: "post_edit" })).toBe(true);
    else if (path === "/account") expect(writePendingAccountTicketOperation({ ...recovery, operation: "ticket_reply", expected_version: 1 })).toBe(true);
    else expect(writePendingNoticeOperation({ ...recovery, operation: "review", target_label: "通知审核" })).toBe(true);
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/session")) return new Response(JSON.stringify(productSession), { status: 200 });
      if (url.includes("/operations/")) return new Response("", { status: 401 });
      const data = path === "/food"
        ? { status: "ok", status_message: "数据正常", stale: false, as_of: "2026-11-07T09:30:00Z", submissions: [], anomaly_tickets: [], tier_adjustments: [], posts: [] }
        : path === "/account" ? { tickets: [] } : { items: [], generated_at: "2026-11-07T09:30:00Z" };
      return new Response(JSON.stringify({ data, request_id: "req_restored_operation" }), { status: 200 });
    });
    vi.stubGlobal("fetch", fetch);
    wrapper = mount(App);
    await flushPromises();

    expect(fetch.mock.calls.some(([input]) => String(input).includes("/operations/"))).toBe(true);
    expect(wrapper.text()).not.toContain("权限已验证");
    expect(wrapper.text()).toContain("登录已过期，请重新登录后继续");
    const login = wrapper.get("a[href^='/api/v1/auth/login']");
    expect(new URL(login.attributes("href")!, window.location.origin).searchParams.get("return_to")).toBe(path);
  });

  it.each(privateReads)("notifies the Shell when the current $name request returns 401", async ({ request }) => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 401 })));
    const expired = vi.fn();
    const stop = onConsoleSessionExpired(expired);
    try {
      expect(await request()).toEqual({ state: "signed_out" });
      expect(expired).toHaveBeenCalledOnce();
    } finally { stop(); }
  });

  it.each([
    ["/food", "food.read", "food"],
    ["/account", "account.tickets.read", "account-portfolio"],
  ])("keeps the operator identifier out of the %s workspace DOM", async (path, permission, product) => {
    window.history.replaceState({}, "", path);
    const productSession = structuredClone(session);
    productSession.data.access_context.permissions = [permission];
    productSession.data.access_context.scopes = [{ kind: "product", product_code: product }];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => String(input).endsWith("/session")
      ? new Response(JSON.stringify(productSession), { status: 200 })
      : new Response("", { status: 503 })));
    wrapper = mount(App);
    await flushPromises();

    expect(wrapper.find("[operator-i-d]").exists()).toBe(false);
    expect(wrapper.html()).not.toContain(productSession.data.user.id);
  });

  it.each(privateReads)("does not expire a new principal when an old $name request returns 401 late", async ({ request }) => {
    beginConsolePrincipal();
    let complete!: (response: Response) => void;
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((resolve) => { complete = resolve; })));
    const expired = vi.fn();
    const stop = onConsoleSessionExpired(expired);
    try {
      const oldRequest = request();
      const newPrincipal = beginConsolePrincipal();
      complete(new Response("", { status: 401 }));
      expect(await oldRequest).toEqual({ state: "unavailable" });
      expect(expired).not.toHaveBeenCalled();
      expect(isCurrentConsolePrincipal(newPrincipal)).toBe(true);
    } finally { stop(); }
  });

  it.each(privateReads)("discards $name JSON which finishes after the principal changed", async ({ request, data }) => {
    beginConsolePrincipal();
    let completeJSON!: (body: unknown) => void;
    const response = new Response("", { status: 200 });
    response.json = () => new Promise((resolve) => { completeJSON = resolve; });
    vi.stubGlobal("fetch", vi.fn(async () => response));
    const oldRequest = request();
    await flushPromises();
    beginConsolePrincipal();
    completeJSON({ data, request_id: "req_old_principal" });
    expect(await oldRequest).toEqual({ state: "unavailable" });
  });

  it("discards an old principal's Notice not-configured error body instead of presenting it as the current connection state", async () => {
    let completeJSON!: (body: unknown) => void;
    const response = new Response("", { status: 503 });
    response.json = () => new Promise((resolve) => { completeJSON = resolve; });
    vi.stubGlobal("fetch", vi.fn(async () => response));
    const oldRequest = fetchNoticeSnapshot();
    await flushPromises();
    beginConsolePrincipal();
    completeJSON({ error: { code: "NOTICE_NOT_CONFIGURED", message: "通知工作台尚未接入" }, request_id: "req_old_notice_connection" });
    expect(await oldRequest).toEqual({ state: "unavailable" });
  });

  it.each(privateReads.flatMap((read) => [403, 503].map((status) => ({ ...read, status }))))("keeps $name HTTP $status distinct from Session expiry", async ({ request, status }) => {
    beginConsolePrincipal();
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status })));
    const expired = vi.fn();
    const stop = onConsoleSessionExpired(expired);
    try {
      expect(await request()).toEqual({ state: status === 403 ? "denied" : "unavailable" });
      expect(expired).not.toHaveBeenCalled();
    } finally { stop(); }
  });

  it("keeps non-sensitive page context and drops personal fields and redirect inputs from the login URL", () => {
    window.history.replaceState({}, "", "/operations?tab=inbox&inbox_page=3&email=student%40henu.edu.cn&name=Alice&return_to=https%3A%2F%2Fexample.org#inbox");
    const login = new URL(consoleLoginHref(), window.location.origin);
    expect(login.searchParams.get("return_to")).toBe("/operations?tab=inbox&inbox_page=3#inbox");
    expect(login.href).not.toContain("student");
    expect(login.href).not.toContain("Alice");
    expect(login.href).not.toContain("example.org");
  });
});
