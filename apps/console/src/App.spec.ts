import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import App from "./App.vue";
import { fetchLibraryWorkspace, type ConsoleOverview } from "./lib/console-gateway";

const authenticated = {
  data: {
    user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
    access_context: { permissions: ["console.overview.read"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" },
    expires_at: "2026-07-19T00:05:00Z",
  },
  request_id: "req_console_test",
};

const overview: { data: ConsoleOverview; request_id: string } = {
  data: {
    modules: [
      {
        id: "portal", status: "ok", status_message: "Portal 部署与只读探测正常", as_of: "2026-07-19T00:00:00Z", request_id: "req_portal",
        metrics: [
          { label: "部署版本", value: "2026.07.19" }, { label: "Commit", value: "0123456789ab" },
          { label: "部署时间", value: "07-19 08:00" }, { label: "Readiness", value: "ready" },
          { label: "关键探测", value: "2/2" }, { label: "入口健康", value: "2/2" },
          { label: "反馈摘要", value: "0 待处理" }, { label: "当前异常", value: "0" },
        ],
      },
      { id: "platform", status: "partial", metrics: [], status_message: "部分来源可用", as_of: "2026-07-19T00:00:00Z", request_id: "req_platform" },
      { id: "notice", status: "empty", metrics: [], status_message: "当前无待办", as_of: "2026-07-19T00:00:00Z", request_id: "req_notice" },
      { id: "library", status: "stale", metrics: [], status_message: "展示最近成功摘要", as_of: "2026-07-19T00:00:00Z", last_success_at: "2026-07-19T00:00:01Z", request_id: "req_library" },
      { id: "quizcraft", status: "unavailable", metrics: [], status_message: "摘要暂不可用", request_id: "req_quizcraft" },
      { id: "food", status: "ok", metrics: [], status_message: "摘要可用", as_of: "2026-07-19T00:00:00Z", request_id: "req_food" },
    ],
    generated_at: "2026-07-19T00:00:01Z",
  },
  request_id: "req_overview_test",
};

afterEach(() => { vi.unstubAllGlobals(); vi.unstubAllEnvs(); });

describe("Console Overview", () => {
  it("keeps partial and stale source explanations once inside diagnostics without promoting summary metrics to business counts", async () => {
    const degraded = structuredClone(overview);
    degraded.data.modules[2] = { ...degraded.data.modules[2]!, status: "partial", metrics: [{ label: "待审核", value: "6" }], status_message: "邮件渠道暂不可用，站内通知摘要仍可读取" };
    degraded.data.modules[5] = { ...degraded.data.modules[5]!, status: "stale", metrics: [{ label: "异常票", value: "2" }], last_success_at: "2026-07-19T00:00:00Z", status_message: "当前展示上一次成功读取的美食摘要，请核对最新对象版本" };
    const granted = structuredClone(authenticated);
    granted.data.access_context.permissions.push("notice.read", "food.read");
    const context = { ...granted, data: { ...granted.data, access_context: { ...granted.data.access_context, scopes: [{ kind: "platform" }, { kind: "product", product_code: "notice" }, { kind: "product", product_code: "food" }] } } };
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? degraded : context), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();
    for (const explanation of ["邮件渠道暂不可用，站内通知摘要仍可读取", "当前展示上一次成功读取的美食摘要，请核对最新对象版本"]) {
      expect(wrapper.get("details").text()).toContain(explanation);
      expect(wrapper.text().split(explanation)).toHaveLength(2);
    }
    expect(wrapper.get("[data-overview-task='food']").text()).not.toContain("异常票 2");
    wrapper.unmount();
  });

  it("keeps authorized product entry without claiming Overview access from a product-only session", async () => {
    const productSession = { ...authenticated, data: { ...authenticated.data, access_context: { permissions: ["notice.read"], scopes: [{ kind: "product", product_code: "notice" }], verified_at: "2026-07-19T00:00:00Z" } } };
    const fetcher = vi.fn(async () => new Response(JSON.stringify(productSession), { status: 200 }));
    vi.stubGlobal("fetch", fetcher);
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.text()).toContain("当前账户没有总览查看权限");
    expect(wrapper.find("a[href='/notices']").exists()).toBe(true);
    expect(fetcher).toHaveBeenCalledOnce();
    expect(wrapper.text()).not.toContain("已授予概览查看权限");
    wrapper.unmount();
  });

  it("keeps navigation and diagnostic disclosure in the same reading order on the mobile layout", async () => {
    const scoped = { ...authenticated, data: { ...authenticated.data, access_context: { permissions: ["console.overview.read", "notice.read"], scopes: [{ kind: "platform" }, { kind: "product", product_code: "notice" }], verified_at: "2026-07-19T00:00:00Z" } } };
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : scoped), { status: 200 })));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.findAll("details")).toHaveLength(1);
    expect(wrapper.get("summary").text()).toBe("连接与诊断");
    expect(wrapper.findAll("[data-module-evidence]")).toHaveLength(0);
    expect(wrapper.find("a[href='/notices']").exists()).toBe(true);
    expect(wrapper.find("a[href='/food']").exists()).toBe(false);
    wrapper.unmount();
  });

  it("shows owner-confirmed Library readiness while retaining the old Workspace consumer", async () => {
    const librarySession = { ...authenticated, data: { ...authenticated.data, access_context: { permissions: ["library.read", "library.manage", "library.review"], scopes: [{ kind: "product", product_code: "library" }], verified_at: "2026-07-19T00:00:00Z" } } };
    const workspace = { data: { status: "partial", status_message: "legacy Study API adapter removed (ADR-0037); catalog migration (T1) pending", degraded: true, courses: [], materials: [], downloads: [], submissions: [], corrections: [], generated_at: "2026-07-19T00:00:00Z" }, request_id: "req_library_workspace" };
    const readiness = { data: { management: "not_ready", reason: "management_migration_pending", capabilities: { workspace: false, commands: false, operation_results: false }, checked_at: "2026-11-07T18:00:00Z" }, request_id: "req_library_readiness" };
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => {
      const path = String(input);
      return Promise.resolve(new Response(JSON.stringify(path.endsWith("/session") ? librarySession : path.endsWith("/readiness") ? readiness : workspace), { status: 200 }));
    }));
    expect(await fetchLibraryWorkspace()).toMatchObject({ state: "authenticated", workspace: workspace.data });
    window.history.replaceState({}, "", "/library");
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.text()).toContain("资料管理尚未就绪");
    expect(wrapper.get("a[href='https://henukit.cn/library']").text()).toContain("打开公开资料目录");
    expect(wrapper.find("form").exists()).toBe(false);
    expect(wrapper.text()).not.toMatch(/ADR-0037|同步生效|暂无课程|暂无资料/);
    wrapper.unmount();
  });

  it("reviews an immutable Notice version through the scoped workflow", async () => {
    const noticeSession = { ...authenticated, data: { ...authenticated.data, access_context: { permissions: ["notice.read", "notice.review", "notice.distribute"], scopes: [{ kind: "product", product_code: "notice" }], verified_at: "2026-07-19T00:00:00Z" } } };
    let approved = false;
    const noticeEnvelope = () => ({ data: { items: [{ id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302", source: { id: "571f1c6f-7b10-4c92-91a2-b39bf5af5302", code: "henu-office", name: "学校办公室" }, version: 1, title: "暑期安排", body: "不可变正文", source_url: "https://example.edu/notices/1", content_hash: "a".repeat(64), state: approved ? "approved" : "pending_review", revision: approved ? 2 : 1, created_at: "2026-07-19T00:00:00Z", distribution_count: 0 }], generated_at: "2026-07-19T00:00:00Z" }, request_id: "req_notice_snapshot" });
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path.endsWith("/api/v1/session")) return Promise.resolve(new Response(JSON.stringify(noticeSession), { status: 200 }));
      if (path.endsWith("/reviews") && init?.method === "POST") { approved = true; return Promise.resolve(new Response(JSON.stringify({ data: { version_id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302", state: "approved", revision: 2 }, request_id: "req_notice_review" }), { status: 200 })); }
      return Promise.resolve(new Response(JSON.stringify(noticeEnvelope()), { status: 200 }));
    }));
    window.history.replaceState({}, "", "/notices");
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.text()).toContain("校园通知审核与分发");
    expect(wrapper.text()).toContain("不可变正文");
    await wrapper.findAll("button").find((button) => button.text() === "批准")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("审核已批准");
    expect(wrapper.text()).toContain("已通过 · 修订 r2");
    wrapper.unmount();
  });

  it("renders the Platform Operations workspace without sensitive fields", async () => {
    const operations = {
      data: {
        access_context: { permissions: ["platform.operations.read", "platform.operations.write"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" },
        accounts: [{ id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", email: "operator@henu.edu.cn", email_verified: true, status: "active", authorization_revision: 1, created_at: "2026-07-19T00:00:00Z", grants: [{ role_code: "operations-operator", scope: { kind: "platform" } }] }],
        sessions: [{ id: "271f1c6f-7b10-4c92-91a2-b39bf5af5302", user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", email: "operator@henu.edu.cn", kind: "core", last_seen_at: "2026-07-19T00:00:00Z", expires_at: "2026-07-19T01:00:00Z" }],
        mail: { pending: 1, processing: 0, retry_due: 0, accepted: 0, delivered: 2, failed: 0, dead_letters: 0 },
        inbox_items: [{ id: "371f1c6f-7b10-4c92-91a2-b39bf5af5302", source_product_code: "quizcraft", source_resource_type: "submission", source_resource_id: "submission-7", priority: "normal", status: "open", version: 1, created_at: "2026-07-19T00:00:00Z", updated_at: "2026-07-19T00:00:00Z" }],
        audit: [{ request_id: "req_operations_test", actor_user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", email: "operator@henu.edu.cn", permission_code: "platform.operations.read", target_kind: "platform", decision: "allowed", reason_code: "permission_granted", created_at: "2026-07-19T00:00:00Z" }],
        pagination: { accounts: { page: 1, next_page: null, next_cursor: null }, sessions: { page: 1, next_page: null, next_cursor: null }, inbox_items: { page: 1, next_page: null, next_cursor: null }, audit: { page: 1, next_page: null, next_cursor: null } },
        dependencies: { postgres: "ready", redis: "ready" }, generated_at: "2026-07-19T00:00:00Z",
      }, request_id: "req_operations_envelope",
    };
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(new URL(String(input), "https://console.henukit.test").pathname.endsWith("/operations") ? operations : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/operations");
    const wrapper = mount(App);
    await flushPromises();
    for (const heading of ["平台运营工作台", "账户、角色与权限", "登录会话", "邮件基础设施", "运营收件箱", "授权审计"]) expect(wrapper.text()).toContain(heading);
    expect((wrapper.get("input[pattern]").element as HTMLInputElement).value).toBe("operations-operator");
    expect(wrapper.text()).toContain("撤销登录");
    expect(wrapper.findAll("button").some((button) => button.text().includes("保存访问设置"))).toBe(true);
    for (const secret of ["token_hash", "recipient_ciphertext", "provider_message_id"]) expect(wrapper.text()).not.toContain(secret);
    wrapper.unmount();

    operations.data.access_context.permissions = ["platform.operations.read"];
    const readOnly = mount(App);
    await flushPromises();
    expect(readOnly.text()).toContain("只读权限");
    expect(readOnly.findAll("button").some((button) => button.text().includes("保存访问设置") || button.text().includes("撤销登录"))).toBe(false);
    readOnly.unmount();
  });
  it("keeps technical module coverage in one disclosure after the server verifies the access context", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.findAll("[data-module-card]")).toHaveLength(0);
    for (const name of ["Portal", "Platform Operations", "Notice", "Library", "练习服务", "Food"]) expect(wrapper.get("details").text()).toContain(name);
    for (const retiredCopy of ["QuizCraft", "题库工坊", "刷题产品", "模块证据对照"]) expect(wrapper.text()).not.toContain(retiredCopy);
    for (const fact of ["0123456789ab", "Readiness", "关键探测", "入口健康", "反馈摘要", "当前异常"]) expect(wrapper.get("details").text()).toContain(fact);
    for (const control of ["编辑内容", "重新部署", "回滚版本", "切换版本"]) expect(wrapper.text()).not.toContain(control);
    expect(wrapper.text()).toContain("权限已验证");
    expect(wrapper.find("a[href='/account/points']").exists()).toBe(false);
    expect(wrapper.find("a[href='/account/memberships']").exists()).toBe(false);
    wrapper.unmount();
  });

  it("labels an operator-disabled owner distinctly from an outage", async () => {
    const disabled = structuredClone(overview);
    disabled.data.modules[2] = {
      id: "notice", status: "unavailable", unavailable_reason: "operator_disabled", metrics: [],
      status_message: "校园通知摘要已由运营配置停用；如需恢复，请重新配置摘要地址", request_id: "req_notice_disabled",
    };
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? disabled : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.get("details").text()).toContain("Notice · 摘要已停用");
    expect(wrapper.text()).not.toContain("通知产品已停服");
    wrapper.unmount();
  });

  it("shows a signed-out state without exposing mock metrics", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));
    window.history.replaceState({}, "", "/operations?tab=inbox");
    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.get("a[href^='/api/v1/auth/login']").text()).toContain("登录 Console");
    expect(wrapper.text()).toContain("请登录后继续");
    expect(wrapper.findAll("[data-module-card]")).toHaveLength(0);
    expect(wrapper.findAll(".metric-tile")).toHaveLength(0);
    expect(wrapper.get("a[href^='/api/v1/auth/login']").attributes("href")).toContain(encodeURIComponent("/operations?tab=inbox"));
  });

  it("drops the verified UI when the overview recheck observes revocation", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(String(input).endsWith("/overview") ? new Response("", { status: 401 }) : new Response(JSON.stringify(authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).not.toContain("权限已验证");
    expect(wrapper.get("a[href^='/api/v1/auth/login']").text()).toContain("登录 Console");
    expect(wrapper.findAll("[data-module-card]")).toHaveLength(0);
    expect(wrapper.text()).toContain("登录已过期，请重新登录后继续");
    wrapper.unmount();
  });

  it("exposes a loading state without inventing metrics", () => {
    vi.stubGlobal("fetch", vi.fn(() => new Promise(() => undefined)));
    window.history.replaceState({}, "", "/?scenario=loading");
    const wrapper = mount(App);

    expect(wrapper.find("[role='status'][aria-busy='true']").text()).toContain("正在验证登录状态");
    expect(wrapper.findAll("[data-module-card]")).toHaveLength(0);
    expect(wrapper.findAll(".metric-tile")).toHaveLength(0);
    wrapper.unmount();
  });

  it("keeps a summary failure inside diagnostics without inventing six business outages or a coverage count", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(String(input).endsWith("/overview") ? new Response("", { status: 500 }) : new Response(JSON.stringify(authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();
    expect(wrapper.get("details").text()).toContain("本次摘要诊断未能完成");
    expect(wrapper.findAll("[data-module-card]")).toHaveLength(0);
    expect(wrapper.text()).not.toContain("0/6");
    expect(wrapper.text()).toContain("权限已验证");
    wrapper.unmount();
  });

  it("renders each degraded status message exactly once", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();

    // A degraded card's explanation must appear in the state panel or the
    // footer, never both (the footer only echoes it for metric-bearing cards).
    expect(wrapper.text()).not.toContain("部分来源可用");
    expect(wrapper.text().match(/练习服务摘要暂不可用/g)).toHaveLength(1);
    expect(wrapper.text().match(/Portal 部署与只读探测正常/g)).toHaveLength(1);
    wrapper.unmount();
  });

  it("keeps task rows informational and places actions on their actual destination links", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();
    for (const row of wrapper.findAll("[data-overview-task]")) {
      expect(row.element.tagName).toBe("ARTICLE");
      expect(row.attributes("href")).toBeUndefined();
    }
    expect(wrapper.find("a[href='https://henukit.cn/library']").exists()).toBe(true);
    wrapper.unmount();
  });

  it("formats overview timestamps in local time instead of raw UTC", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.get("details").text()).toContain("来源时间");
    expect(wrapper.get("details").text()).not.toContain("2026-07-19T00:00:01Z");
    expect(wrapper.text()).not.toContain("2026-07-19T00:00:00Z");
    expect(wrapper.text()).not.toContain("2026-07-19T00:00:01Z");
    wrapper.unmount();
  });
});
