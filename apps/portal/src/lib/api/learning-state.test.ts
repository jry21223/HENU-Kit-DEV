import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const envelope = {
  request_id: "req_learning_state",
  data: [
    {
      bank_id: "33333333-3333-4333-8333-333333333333",
      question_id: "55555555-5555-4555-8555-555555555555",
      question_version_id: "66666666-6666-4666-8666-666666666666",
      wrong: true,
      attempt_count: 3,
      correct_count: 1,
      updated_at: "2026-08-06T08:00:00Z",
    },
    {
      bank_id: "33333333-3333-4333-8333-333333333333",
      question_id: "77777777-7777-4777-8777-777777777777",
      question_version_id: "88888888-8888-4888-8888-888888888888",
      wrong: false,
      attempt_count: 1,
      correct_count: 1,
      updated_at: "2026-08-05T08:00:00Z",
    },
  ],
};

describe("QuizCraft learning-state read", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("fetches the signed-in user's server learning state with no mock fallback", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(envelope), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchLearningState } = await import("./client");
    const result = await fetchLearningState();

    expect(result).toEqual(envelope);
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/learning-state",
      expect.objectContaining({ credentials: "include" })
    );
  });

  it("turns an unauthenticated read into PortalUnauthorizedError instead of a success", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ error: "not authenticated", request_id: "req_x" }),
        { status: 401, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchLearningState, PortalUnauthorizedError } = await import("./client");
    await expect(fetchLearningState()).rejects.toBeInstanceOf(PortalUnauthorizedError);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("keeps a failed read an honest error with no fabricated data", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: "learning state is temporarily unavailable",
          request_id: "req_x",
        }),
        { status: 503, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchLearningState, PortalHttpError } = await import("./client");
    await expect(fetchLearningState()).rejects.toBeInstanceOf(PortalHttpError);
  });
});
