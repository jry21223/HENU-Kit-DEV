import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { readFile } from "node:fs/promises";

const bankID = "33333333-3333-4333-8333-333333333333";
const bankVersionID = "44444444-4444-4444-8444-444444444444";
const sessionID = "22222222-2222-4222-8222-222222222222";
const questionID = "55555555-5555-4555-8555-555555555555";
const questionVersionID = "66666666-6666-4666-8666-666666666666";

describe("QuizCraft Practice commands", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1");
    vi.stubEnv("NODE_ENV", "test");
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it("creates a real server-selected session with no browser answer key", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          request_id: "req_session",
          data: {
            session_id: sessionID,
            bank_id: bankID,
            bank_version_id: bankVersionID,
            mode: "random",
            excluded_unavailable_count: 0,
            questions: [
              {
                question_id: questionID,
                question_version_id: questionVersionID,
                type: "single",
                chapter_id: "chapter-1",
                chapter: "基础",
                content: "服务端题干",
                options: ["甲", "乙"],
              },
            ],
          },
        }),
        { status: 201, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { createPracticeSession } = await import("./client");
    const result = await createPracticeSession(
      { bank_id: bankID, bank_version_id: bankVersionID, mode: "random", question_count: 1 },
      "practice-create-retry-0001"
    );

    expect(result.data.questions[0]).not.toHaveProperty("answer");
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/practice/sessions",
      expect.objectContaining({
        method: "POST",
        cache: "no-store",
        credentials: "same-origin",
        headers: expect.objectContaining({
          "Content-Type": "application/json",
          "Idempotency-Key": "practice-create-retry-0001",
        }),
      })
    );
  });

  it("submits the selected answer to Gateway and does not manufacture a score on failure", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ error: "practice_commands_unavailable", request_id: "req_core_down" }),
        { status: 503, headers: { "Content-Type": "application/json" } }
      )
    );
    vi.stubGlobal("fetch", fetch);

    const { PortalHttpError, submitPracticeAnswer } = await import("./client");
    await expect(
      submitPracticeAnswer(
        sessionID,
        { question_id: questionID, question_version_id: questionVersionID, answer: 0 },
        "practice-answer-retry-0001"
      )
    ).rejects.toBeInstanceOf(PortalHttpError);
    expect(fetch).toHaveBeenCalledWith(
      `/api/v1/practice/sessions/${sessionID}/answers`,
      expect.objectContaining({
        method: "POST",
        credentials: "same-origin",
        headers: expect.objectContaining({ "Idempotency-Key": "practice-answer-retry-0001" }),
        body: JSON.stringify({ question_id: questionID, question_version_id: questionVersionID, answer: 0 }),
      })
    );
  });

  it("rejects a malformed idempotency key before it can become a second attempt", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const { createPracticeSession, PortalApiError } = await import("./client");
    await expect(
      createPracticeSession({ bank_id: bankID, bank_version_id: bankVersionID, mode: "random" }, "short")
    ).rejects.toBeInstanceOf(PortalApiError);
    expect(fetch).not.toHaveBeenCalled();
  });

  it("keeps the live quiz route free of local question fixtures and local answer checks", async () => {
    const source = await readFile(new URL("../../app/practice/quiz/page.tsx", import.meta.url), "utf8");
    expect(source).not.toMatch(/@\/lib\/practice\/mock|\bQUIZ_SET\b/);
    expect(source).not.toMatch(/\b(?:q|question)\.answer\b/);
    expect(source).toMatch(/createPracticeSession/);
    expect(source).toMatch(/submitPracticeAnswer/);
  });
});
