import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import App from "./App.vue";
import { fetchLibraryWorkspace } from "./lib/console-gateway";

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

afterEach(() => { vi.unstubAllGlobals(); vi.unstubAllEnvs(); });

describe("Console Overview", () => {
  it("reviews a Library submission without exposing retired Study capabilities", async () => {
    const librarySession = { ...authenticated, data: { ...authenticated.data, access_context: { permissions: ["library.read", "library.manage", "library.review"], scopes: [{ kind: "product", product_code: "library" }], verified_at: "2026-07-19T00:00:00Z" } } };
    let approved = false;
    const material = { id: "22222222-2222-4222-8222-222222222222", course_id: "11111111-1111-4111-8111-111111111111", title: "期末复习提纲", type: "quick_review", file_name: "review.pdf", file_size: 2048, access_level: "authenticated", status: approved ? "published" : "pending", updated_at: "2026-07-19T00:00:00Z" };
    const workspace = () => ({ data: { status: "partial", status_message: "纠错来源暂不可用", degraded: true, courses: [{ id: "11111111-1111-4111-8111-111111111111", name: "高等数学", slug: "math", grade: "2025", status: "published", updated_at: "2026-07-19T00:00:00Z" }], materials: [material], downloads: [{ id: "33333333-3333-4333-8333-333333333333", material_id: material.id, material_title: material.title, access_level: "authenticated", downloaded_at: "2026-07-19T01:00:00Z" }], submissions: approved ? [] : [material], corrections: [], generated_at: "2026-07-19T00:00:00Z" }, request_id: "req_library_workspace" });
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path.endsWith("/api/v1/session")) return Promise.resolve(new Response(JSON.stringify(librarySession), { status: 200 }));
      if (path.endsWith("/library/commands") && init?.method === "POST") { approved = true; return Promise.resolve(new Response(JSON.stringify({ data: { operation: "submission_approve", resource_id: "22222222-2222-4222-8222-222222222222", state: "succeeded" }, request_id: "req_library_review" }), { status: 200 })); }
      return Promise.resolve(new Response(JSON.stringify(workspace()), { status: 200 }));
    }));
    expect(await fetchLibraryWorkspace()).toMatchObject({ state: "authenticated" });
    window.history.replaceState({}, "", "/library");
    const wrapper = mount(App);
    await flushPromises();
    for (const text of ["资料库运营", "课程", "资料", "下载", "投稿审核", "资料纠错", "纠错来源暂不可用"]) expect(wrapper.text()).toContain(text);
    for (const excluded of ["社区", "支付", "刷题", "积分", "会员"]) expect(wrapper.text()).not.toContain(excluded);
    await wrapper.findAll("button").find((button) => button.text() === "批准投稿")!.trigger("click");
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text() === "确认批准")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("投稿已批准");
    expect(wrapper.text()).not.toContain("批准投稿");
    wrapper.unmount();
  });

  it("reviews an immutable Notice version through the scoped workflow", async () => {
    const noticeSession = { ...authenticated, data: { ...authenticated.data, access_context: { permissions: ["notice.read", "notice.review", "notice.distribute"], scopes: [{ kind: "product", product_code: "notice" }], verified_at: "2026-07-19T00:00:00Z" } } };
    let approved = false;
    const noticeEnvelope = () => ({ data: { items: [{ id: "471f1c6f-7b10-4c92-91a2-b39bf5af5302", source: { id: "571f1c6f-7b10-4c92-91a2-b39bf5af5302", code: "henu-office", name: "学校办公室" }, version: 1, title: "暑期安排", body: "不可变正文", source_url: "https://example.edu/notices/1", content_hash: "a".repeat(64), state: approved ? "approved" : "pending_review", revision: approved ? 2 : 1, created_at: "2026-07-19T00:00:00Z", distribution_count: 0 }], generated_at: "2026-07-19T00:00:00Z" }, request_id: "req_notice_snapshot" });
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path.endsWith("/api/v1/session")) return Promise.resolve(new Response(JSON.stringify(noticeSession), { status: 200 }));
      if (path.endsWith("/reviews") && init?.method === "POST") { approved = true; return Promise.resolve(new Response(JSON.stringify({ data: { state: "approved", revision: 2 }, request_id: "req_notice_review" }), { status: 200 })); }
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
    expect(wrapper.text()).toContain("已通过 · 版本 v2");
    wrapper.unmount();
  });

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
  it("renders six modules only after the server verifies the access context", async () => {
	vi.stubEnv("VITE_QUIZCRAFT_WORKSHOP_URL", "https://quizcraft.staging.example/extract");
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App, { attachTo: document.body });
    await flushPromises();

    expect(wrapper.findAll("[data-module-card]")).toHaveLength(6);
    for (const name of ["Portal", "Platform Operations", "Notice", "Library", "QuizCraft", "Food"]) expect(wrapper.text()).toContain(name);
    expect(wrapper.get("a[href='https://quizcraft.staging.example/extract']").text()).toContain("打开 QuizCraft 题库工坊");
    expect(wrapper.text()).not.toContain("创建草稿版本");
    for (const state of ["ok", "empty", "partial", "stale", "unavailable"]) expect(wrapper.find(`[data-state="${state}"]`).exists()).toBe(true);
    expect(wrapper.text()).toContain("摘要暂不可用");
    for (const portalFact of ["0123456789ab", "Readiness", "关键探测", "入口健康", "反馈摘要", "当前异常"]) expect(wrapper.text()).toContain(portalFact);
    for (const forbiddenControl of ["编辑内容", "重新部署", "回滚版本", "切换版本"]) expect(wrapper.text()).not.toContain(forbiddenControl);
    expect(wrapper.text()).toContain("权限已验证");
    expect(wrapper.text()).toContain("已授予概览查看权限");
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
    expect(wrapper.text()).toContain("没有平台运营权限");
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

  it("counts zero visible modules when the overview feed is unavailable", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(String(input).endsWith("/overview") ? new Response("", { status: 500 }) : new Response(JSON.stringify(authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.findAll("[data-state='unavailable']")).toHaveLength(6);
    expect(wrapper.get(".access-context").text()).toContain("0/6 可见");
    expect(wrapper.text()).toContain("概览数据暂时不可用");
    wrapper.unmount();
  });

  it("renders each degraded status message exactly once", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();

    // A degraded card's explanation must appear in the state panel or the
    // footer, never both (the footer only echoes it for metric-bearing cards).
    expect(wrapper.text().match(/部分来源可用/g)).toHaveLength(1);
    expect(wrapper.text().match(/摘要暂不可用/g)).toHaveLength(1);
    expect(wrapper.text().match(/Portal 部署与只读探测正常/g)).toHaveLength(1);
    wrapper.unmount();
  });

  it("links module cards to their operations pages and keeps Portal plain", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();

    for (const [id, href] of [
      ["platform", "/operations"],
      ["notice", "/notices"],
      ["library", "/library"],
      ["food", "/food"],
    ] as const) {
      const card = wrapper.get(`[data-module-card='${id}']`);
      expect(card.element.tagName).toBe("A");
      expect(card.attributes("href")).toBe(href);
    }
    // Portal has no operations page: its card must stay a non-clickable
    // article rather than a link that silently does nothing.
    const portal = wrapper.get("[data-module-card='portal']");
    expect(portal.element.tagName).toBe("ARTICLE");
    expect(portal.attributes("href")).toBeUndefined();
    expect(wrapper.get("[data-module-card='quizcraft']").element.tagName).toBe("ARTICLE");
    wrapper.unmount();
  });

  it("formats overview timestamps in local time instead of raw UTC", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).toContain("截至");
    expect(wrapper.text()).toContain("最近成功");
    expect(wrapper.text()).not.toContain("2026-07-19T00:00:00Z");
    expect(wrapper.text()).not.toContain("2026-07-19T00:00:01Z");
    wrapper.unmount();
  });
});
