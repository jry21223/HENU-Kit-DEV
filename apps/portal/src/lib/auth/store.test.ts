import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("authStore", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("logout clears the cached user in mock mode", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "1");

    const { authStore } = await import("./store");
    authStore.login("小河同学");
    expect(authStore.get().user?.name).toBe("小河同学");

    await authStore.logout();
    expect(authStore.get().user).toBeNull();
    expect(authStore.get().ready).toBe(true);
  });

  it("restoring the session reads only the session, never module data", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    const fetch = vi.fn().mockImplementation(
      async () =>
        new Response("{}", { status: 401, headers: { "Content-Type": "application/json" } })
    );
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("window", globalThis);

    const { authStore } = await import("./store");
    const unsubscribe = authStore.subscribe(() => {});
    await vi.waitFor(() => expect(authStore.get().ready).toBe(true));
    unsubscribe();

    expect(fetch.mock.calls.map(([input]) => String(input))).toEqual(["/api/v1/session"]);
  });

  it("clear drops the cached user without any network call", async () => {
    vi.stubEnv("NEXT_PUBLIC_PORTAL_ALLOW_MOCK", "1");
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    const { authStore } = await import("./store");
    authStore.login("小河同学");
    authStore.clear();
    expect(authStore.get().user).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });
});
