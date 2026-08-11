import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const envelope = {
  request_id: "req_learning_state",
  data: {
    items: [
      {
        bank_id: "33333333-3333-4333-8333-333333333333",
        question_id: "55555555-5555-4555-8555-555555555555",
        question_version_id: "66666666-6666-4666-8666-666666666666",
        wrong: true,
        attempt_count: 3,
        correct_count: 1,
        updated_at: "2026-08-06T08:00:00Z",
      },
    ],
    pagination: { page: 1, page_size: 20, total: 1, total_pages: 1 },
  },
};

describe("Portal learning-state client", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("reads only the Gateway contract with the Portal Session and no cache", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(envelope), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchLearningState } = await import("./client");
    await expect(fetchLearningState(1, 20, true)).resolves.toEqual(envelope);
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/learning-state?page=1&page_size=20&wrong=true",
      expect.objectContaining({ credentials: "include", cache: "no-store" })
    );
  });

  it("forwards the documented page and maximum page-size boundary", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({
        ...envelope,
        data: { items: envelope.data.items, pagination: { page: 2, page_size: 100, total: 101, total_pages: 2 } },
      }), { status: 200, headers: { "Content-Type": "application/json" } })
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchLearningState } = await import("./client");
    await fetchLearningState(2, 100, true);
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/learning-state?page=2&page_size=100&wrong=true",
      expect.objectContaining({ credentials: "include", cache: "no-store" })
    );
  });

  it.each([
    [0, 20],
    [1, 0],
    [1, 101],
    [Number.MAX_SAFE_INTEGER + 1, 20],
  ])("rejects invalid pagination page=%s page_size=%s before fetch", async (page, pageSize) => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const { fetchLearningState, PortalApiError } = await import("./client");

    await expect(fetchLearningState(page, pageSize)).rejects.toMatchObject({
      name: PortalApiError.name,
      code: "PORTAL_INVALID_LEARNING_STATE_PAGINATION",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("keeps a signed-out response explicit", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "not authenticated", request_id: "req_signed_out" }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        })
      )
    );

    const { fetchLearningState, PortalUnauthorizedError } = await import("./client");
    await expect(fetchLearningState()).rejects.toBeInstanceOf(PortalUnauthorizedError);
  });

  it("does not turn an unavailable owner into empty learning state", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "learning state unavailable", request_id: "req_down" }), {
          status: 503,
          headers: { "Content-Type": "application/json" },
        })
      )
    );

    const { fetchLearningState, PortalHttpError } = await import("./client");
    await expect(fetchLearningState()).rejects.toBeInstanceOf(PortalHttpError);
  });
});
