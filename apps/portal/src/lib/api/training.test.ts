import { afterEach, beforeEach, expect, it, vi } from "vitest";

beforeEach(() => { vi.resetModules(); vi.stubEnv("NEXT_PUBLIC_PORTAL_REQUIRE_GATEWAY", "1"); });
afterEach(() => { vi.unstubAllGlobals(); vi.unstubAllEnvs(); });

it("uses the exact training command and preserves the answer and retry key", async () => {
  const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ request_id: "r1", data: { saved_count: 1 } }), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  const { submitTrainingAnswer } = await import("./training");
  const answer = { unit_version_id: "77777777-7777-4777-8777-777777777777", question_id: "55555555-5555-4555-8555-555555555555", question_version_id: "66666666-6666-4666-8666-666666666666", answer: 0 };
  await submitTrainingAnswer("22222222-2222-4222-8222-222222222222", answer, "training-answer-retry-0001");
  expect(fetch).toHaveBeenCalledWith("/api/v1/practice/training/sessions/22222222-2222-4222-8222-222222222222/answers", expect.objectContaining({ method: "POST", credentials: "same-origin", cache: "no-store", body: JSON.stringify(answer), headers: expect.objectContaining({ "Idempotency-Key": "training-answer-retry-0001" }) }));
});

it("fails visibly on owner errors and never replaces recovery with local progress", async () => {
  const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ request_id: "r-failed", error: { code: "TRAINING_DISABLED", message: "disabled" } }), { status: 503 }));
  vi.stubGlobal("fetch", fetch);
  const { fetchTrainingSession } = await import("./training");
  const { PortalHttpError } = await import("./client");
  await expect(fetchTrainingSession("22222222-2222-4222-8222-222222222222")).rejects.toBeInstanceOf(PortalHttpError);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch).toHaveBeenCalledWith("/api/v1/practice/training/sessions/22222222-2222-4222-8222-222222222222", expect.objectContaining({ credentials: "same-origin", cache: "no-store" }));
});

it("rejects invalid route IDs and keys before sending commands", async () => {
  const fetch = vi.fn(); vi.stubGlobal("fetch", fetch);
  const { createTrainingSession, fetchTrainingSession } = await import("./training");
  const { PortalApiError } = await import("./client");
  expect(() => fetchTrainingSession("../other-owner")).toThrow(PortalApiError);
  expect(() => createTrainingSession({ unit_id: "u", unit_version_id: "v" }, "short")).toThrow(PortalApiError);
  expect(fetch).not.toHaveBeenCalled();
});
