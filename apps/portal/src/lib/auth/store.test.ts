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
