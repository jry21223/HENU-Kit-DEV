import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import App from "./App.vue";

const authenticated = {
  data: {
    user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" },
    access_context: { permissions: ["console.overview.read"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" },
    expires_at: "2026-07-19T00:05:00Z",
  },
  request_id: "req_console_test",
};

const overview = {
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

afterEach(() => vi.unstubAllGlobals());

describe("Console Overview", () => {
  it("renders the Platform Operations workspace without sensitive fields", async () => {
    const operations = {
      data: {
        access_context: { permissions: ["platform.operations.read", "platform.operations.write"], scopes: [{ kind: "platform" }], verified_at: "2026-07-19T00:00:00Z" },
        accounts: [{ id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", email_verified: true, status: "active", authorization_revision: 1, created_at: "2026-07-19T00:00:00Z", grants: [{ role_code: "operations-operator", scope: { kind: "platform" } }] }],
        sessions: [{ id: "271f1c6f-7b10-4c92-91a2-b39bf5af5302", user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", kind: "core", last_seen_at: "2026-07-19T00:00:00Z", expires_at: "2026-07-19T01:00:00Z" }],
        mail: { pending: 1, processing: 0, retry_due: 0, accepted: 0, delivered: 2, failed: 0, dead_letters: 0 },
        inbox_items: [{ id: "371f1c6f-7b10-4c92-91a2-b39bf5af5302", source_product_code: "quizcraft", source_resource_type: "submission", source_resource_id: "submission-7", priority: "normal", status: "open", version: 1, created_at: "2026-07-19T00:00:00Z", updated_at: "2026-07-19T00:00:00Z" }],
        audit: [{ request_id: "req_operations_test", actor_user_id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", permission_code: "platform.operations.read", target_kind: "platform", decision: "allowed", reason_code: "permission_granted", created_at: "2026-07-19T00:00:00Z" }],
        dependencies: { postgres: "ready", redis: "ready" }, generated_at: "2026-07-19T00:00:00Z",
      }, request_id: "req_operations_envelope",
    };
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/operations") ? operations : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/operations");
    const wrapper = mount(App);
    await flushPromises();
    for (const heading of ["平台运营工作台", "账户、角色与 Scope", "Session", "邮件基础设施", "Operations Inbox", "授权审计"]) expect(wrapper.text()).toContain(heading);
    expect((wrapper.get("input[pattern]").element as HTMLInputElement).value).toBe("operations-operator");
    expect(wrapper.text()).toContain("撤销 Session");
    expect(wrapper.findAll("button").some((button) => button.text().includes("保存访问设置"))).toBe(true);
    for (const secret of ["token_hash", "recipient_ciphertext", "provider_message_id"]) expect(wrapper.text()).not.toContain(secret);
    wrapper.unmount();

    operations.data.access_context.permissions = ["platform.operations.read"];
    const readOnly = mount(App);
    await flushPromises();
    expect(readOnly.text()).toContain("只读权限");
    expect(readOnly.findAll("button").some((button) => button.text().includes("保存访问设置") || button.text().includes("撤销 Session"))).toBe(false);
    readOnly.unmount();
  });
  it("renders six modules only after the server verifies the access context", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App, { attachTo: document.body });
    await flushPromises();

    expect(wrapper.findAll("[data-module-card]")).toHaveLength(6);
    for (const name of ["Portal", "Platform Operations", "Notice", "Library", "QuizCraft", "Food"]) expect(wrapper.text()).toContain(name);
    for (const state of ["ok", "empty", "partial", "stale", "unavailable"]) expect(wrapper.find(`[data-state="${state}"]`).exists()).toBe(true);
    expect(wrapper.text()).toContain("req_quizcraft");
    for (const portalFact of ["0123456789ab", "Readiness", "关键探测", "入口健康", "反馈摘要", "当前异常"]) expect(wrapper.text()).toContain(portalFact);
    for (const forbiddenControl of ["编辑内容", "重新部署", "回滚版本", "切换版本"]) expect(wrapper.text()).not.toContain(forbiddenControl);
    expect(wrapper.text()).toContain("权限已验证");
    expect(wrapper.text()).toContain("console.overview.read");
    expect(wrapper.text()).not.toContain("积分");
    expect(wrapper.text()).not.toContain("会员");
    wrapper.unmount();
  });

  it("shows a signed-out state without exposing mock metrics", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));
    window.history.replaceState({}, "", "/operations?tab=inbox");
    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.get("a[href^='/api/v1/auth/login']").text()).toContain("登录 Console");
    expect(wrapper.text()).toContain("缺少 platform.operations.read");
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
    expect(wrapper.findAll("[data-state='denied']")).toHaveLength(6);
  });

  it("exposes a loading state without inventing metrics", () => {
    vi.stubGlobal("fetch", vi.fn(() => new Promise(() => undefined)));
    window.history.replaceState({}, "", "/?scenario=loading");
    const wrapper = mount(App);

    expect(wrapper.find("section[aria-busy='true']").exists()).toBe(true);
    expect(wrapper.findAll("[data-state='loading']")).toHaveLength(6);
    expect(wrapper.findAll(".metric-tile")).toHaveLength(0);
  });
});
