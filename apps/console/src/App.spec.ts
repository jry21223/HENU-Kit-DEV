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
      { id: "portal", status: "ok", metrics: [{ label: "部署版本", value: "2026.07.19" }], status_message: "摘要可用", as_of: "2026-07-19T00:00:00Z", request_id: "req_portal" },
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
  it("renders six modules only after the server verifies the access context", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(new Response(JSON.stringify(String(input).endsWith("/overview") ? overview : authenticated), { status: 200 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App, { attachTo: document.body });
    await flushPromises();

    expect(wrapper.findAll("[data-module-card]")).toHaveLength(6);
    for (const name of ["Portal", "Platform Operations", "Notice", "Library", "QuizCraft", "Food"]) expect(wrapper.text()).toContain(name);
    for (const state of ["ok", "empty", "partial", "stale", "unavailable"]) expect(wrapper.find(`[data-state="${state}"]`).exists()).toBe(true);
    expect(wrapper.text()).toContain("req_quizcraft");
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
    expect(wrapper.findAll("[data-state='denied']")).toHaveLength(6);
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
