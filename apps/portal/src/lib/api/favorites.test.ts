import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const bankID = "33333333-3333-4333-8333-333333333333";
const questionID = "55555555-5555-4555-8555-555555555555";

describe("QuizCraft Practice favorites client", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("lists the signed-in user's per-bank favorites overview", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          request_id: "req_overview",
          data: [
            { bank_id: bankID, bank_name: "高等数学", available_count: 2, unavailable_count: 1 },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchFavoritesOverview } = await import("./client");
    const result = await fetchFavoritesOverview();

    expect(result.data[0].bank_name).toBe("高等数学");
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/practice/favorites",
      expect.objectContaining({ credentials: "include" })
    );
  });

  it("lists one bank's favorite references; unavailable items carry no version", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          request_id: "req_list",
          data: [
            {
              bank_id: bankID,
              question_id: questionID,
              available: true,
              question_version_id: "66666666-6666-4666-8666-666666666666",
            },
            { bank_id: bankID, question_id: "77777777-7777-4777-8777-777777777777", available: false },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { fetchBankFavorites } = await import("./client");
    const result = await fetchBankFavorites(bankID);

    expect(result.data).toHaveLength(2);
    expect(result.data[1]).not.toHaveProperty("question_version_id");
    expect(fetch).toHaveBeenCalledWith(
      `/api/v1/practice/banks/${bankID}/favorites`,
      expect.objectContaining({ credentials: "include" })
    );
  });

  it("favorites a stable question reference with a signed PUT command", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          request_id: "req_fav",
          data: {
            operation_id: "op-1",
            state: "succeeded",
            idempotency_key: "practice-favorite-key-0001",
            request_id: "req_fav",
            resource_id: questionID,
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { favoriteQuestion } = await import("./client");
    await favoriteQuestion(bankID, questionID, "practice-favorite-key-0001");

    expect(fetch).toHaveBeenCalledWith(
      `/api/v1/practice/banks/${bankID}/favorites/${questionID}`,
      expect.objectContaining({
        method: "PUT",
        credentials: "same-origin",
        headers: expect.objectContaining({ "Idempotency-Key": "practice-favorite-key-0001" }),
        body: JSON.stringify({}),
      })
    );
  });

  it("unfavorites through a signed DELETE command", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          request_id: "req_unfav",
          data: {
            operation_id: "op-2",
            state: "succeeded",
            idempotency_key: "practice-unfavorite-key-0001",
            request_id: "req_unfav",
            resource_id: questionID,
          },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { unfavoriteQuestion } = await import("./client");
    await unfavoriteQuestion(bankID, questionID, "practice-unfavorite-key-0001");

    expect(fetch).toHaveBeenCalledWith(
      `/api/v1/practice/banks/${bankID}/favorites/${questionID}`,
      expect.objectContaining({
        method: "DELETE",
        credentials: "same-origin",
        headers: expect.objectContaining({ "Idempotency-Key": "practice-unfavorite-key-0001" }),
      })
    );
  });

  it("starts a favorites practice session from one bank", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          request_id: "req_session",
          data: {
            session_id: "22222222-2222-4222-8222-222222222222",
            bank_id: bankID,
            bank_version_id: "44444444-4444-4444-8444-444444444444",
            mode: "favorites",
            excluded_unavailable_count: 1,
            questions: [],
          },
        }),
        { status: 201, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { createFavoritesSession } = await import("./client");
    const result = await createFavoritesSession(bankID, "practice-favorites-session-0001");

    expect(result.data.mode).toBe("favorites");
    expect(result.data.excluded_unavailable_count).toBe(1);
    expect(fetch).toHaveBeenCalledWith(
      `/api/v1/practice/banks/${bankID}/favorites/practice-sessions`,
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({ "Idempotency-Key": "practice-favorites-session-0001" }),
      })
    );
  });

  it("rejects a malformed idempotency key before any favorites request", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);

    const { favoriteQuestion, PortalApiError } = await import("./client");
    await expect(favoriteQuestion(bankID, questionID, "short")).rejects.toBeInstanceOf(
      PortalApiError
    );
    expect(fetch).not.toHaveBeenCalled();
  });
});
