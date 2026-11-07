import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import App from "./App.vue";

function session(permissions = ["console.overview.read", "food.read"]) {
  return { data: { user: { id: "171f1c6f-7b10-4c92-91a2-b39bf5af5302" }, access_context: { permissions, scopes: [{ kind: "platform" }, { kind: "product", product_code: "food" }], verified_at: "2026-11-07T20:00:00Z" }, expires_at: "2026-11-07T21:00:00Z" }, request_id: "req_overview_authorization" };
}

afterEach(() => { vi.unstubAllGlobals(); });

describe("Overview authorization is distinct from the Console Session", () => {
  it("keeps the verified Session and authorized entries when the Overview recheck denies access", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => Promise.resolve(String(input).endsWith("/session")
      ? new Response(JSON.stringify(session()), { status: 200 })
      : new Response("", { status: 403 }))));
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).toContain("权限已验证");
    expect(wrapper.text()).toContain("当前账户没有总览查看权限");
    expect(wrapper.find("a[href='/food']").exists()).toBe(true);
    expect(wrapper.find("a[href^='/api/v1/auth/login']").exists()).toBe(false);
    expect(wrapper.findAll("h1")).toHaveLength(1);
    expect(wrapper.findAll("[data-module-card]")).toHaveLength(0);
    wrapper.unmount();
  });

  it("does not request combined private facts without the exact Overview permission", async () => {
    const fetcher = vi.fn((input: RequestInfo | URL) => Promise.resolve(String(input).endsWith("/session")
      ? new Response(JSON.stringify(session(["food.read"])), { status: 200 })
      : new Response("", { status: 503 })));
    vi.stubGlobal("fetch", fetcher);
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();

    expect(fetcher.mock.calls.map(([input]) => String(input))).toEqual(["/api/v1/session"]);
    expect(wrapper.text()).toContain("当前账户没有总览查看权限");
    expect(wrapper.find("a[href='/food']").exists()).toBe(true);
    expect(wrapper.findAll("[data-module-card]")).toHaveLength(0);
    wrapper.unmount();
  });

  it("does not treat a product scope as the platform scope required for Overview", async () => {
    const productOnly = session();
    productOnly.data.access_context.scopes = productOnly.data.access_context.scopes.filter((scope) => scope.kind !== "platform");
    const fetcher = vi.fn(async () => new Response(JSON.stringify(productOnly), { status: 200 }));
    vi.stubGlobal("fetch", fetcher);
    window.history.replaceState({}, "", "/");
    const wrapper = mount(App);
    await flushPromises();
    expect(fetcher.mock.calls).toHaveLength(1);
    expect(wrapper.text()).toContain("当前账户没有总览查看权限");
    expect(wrapper.find("a[href='/food']").exists()).toBe(true);
    wrapper.unmount();
  });
});
