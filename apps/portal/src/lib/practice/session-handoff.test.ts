import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  PRACTICE_SESSION_HANDOFF_PREFIX,
  createIdempotencyKey,
  practiceSessionHandoffKey,
  readPracticeSessionHandoff,
  writePracticeSessionHandoff,
  type PracticeSessionHandoff,
} from "./session-handoff";
import {
  MAX_QUESTION_COUNT,
  MIN_QUESTION_COUNT,
  isValidQuestionCount,
} from "./question-count";

function handoff(sessionID: string): PracticeSessionHandoff {
  return {
    session_id: sessionID,
    questions: [{ id: "q-1" }],
  } as unknown as PracticeSessionHandoff;
}

// The Vitest environment is node, so sessionStorage has to be supplied.
function stubSessionStorage() {
  const store = new Map<string, string>();
  vi.stubGlobal("window", {
    sessionStorage: {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => void store.set(key, value),
      removeItem: (key: string) => void store.delete(key),
    },
  });
  return store;
}

describe("practice session handoff", () => {
  beforeEach(() => {
    stubSessionStorage();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("namespaces the key so two sessions cannot collide", () => {
    expect(practiceSessionHandoffKey("s-1")).toBe(
      `${PRACTICE_SESSION_HANDOFF_PREFIX}:s-1`
    );
    expect(practiceSessionHandoffKey("s-1")).not.toBe(
      practiceSessionHandoffKey("s-2")
    );
  });

  it("round-trips a session between the two pages", () => {
    writePracticeSessionHandoff(handoff("s-1"));
    expect(readPracticeSessionHandoff("s-1")).toMatchObject({ session_id: "s-1" });
  });

  it("returns null rather than another session's payload", () => {
    writePracticeSessionHandoff(handoff("s-1"));
    expect(readPracticeSessionHandoff("s-2")).toBeNull();
  });

  it("returns null for absent, unparsable, or malformed handoffs", () => {
    expect(readPracticeSessionHandoff("missing")).toBeNull();

    const store = stubSessionStorage();
    store.set(practiceSessionHandoffKey("s-1"), "{not json");
    expect(readPracticeSessionHandoff("s-1")).toBeNull();

    // A payload whose session_id disagrees with the key it was stored under
    // must not be trusted.
    store.set(practiceSessionHandoffKey("s-1"), JSON.stringify({ session_id: "s-9", questions: [] }));
    expect(readPracticeSessionHandoff("s-1")).toBeNull();

    // Questions must be an array; anything else is a shape mismatch.
    store.set(practiceSessionHandoffKey("s-1"), JSON.stringify({ session_id: "s-1", questions: null }));
    expect(readPracticeSessionHandoff("s-1")).toBeNull();

    store.set(practiceSessionHandoffKey("s-1"), "null");
    expect(readPracticeSessionHandoff("s-1")).toBeNull();
  });

  it("creates a distinct idempotency key per logical write", () => {
    const first = createIdempotencyKey("practice:start");
    const second = createIdempotencyKey("practice:start");

    expect(first.startsWith("practice:start:")).toBe(true);
    expect(first).not.toBe(second);
  });

  it("still creates a key when crypto.randomUUID is unavailable", () => {
    vi.stubGlobal("crypto", {});
    const key = createIdempotencyKey("practice:start");
    expect(key.startsWith("practice:start:")).toBe(true);
    expect(key.length).toBeGreaterThan("practice:start:".length);
  });
});

describe("question count bounds", () => {
  it("accepts the inclusive bounds the session contract allows", () => {
    expect(isValidQuestionCount(MIN_QUESTION_COUNT)).toBe(true);
    expect(isValidQuestionCount(MAX_QUESTION_COUNT)).toBe(true);
    expect(isValidQuestionCount(20)).toBe(true);
  });

  it("rejects anything that cannot cross the command boundary unchanged", () => {
    for (const count of [
      MIN_QUESTION_COUNT - 1,
      MAX_QUESTION_COUNT + 1,
      0,
      -1,
      1.5,
      NaN,
      Infinity,
    ]) {
      expect(isValidQuestionCount(count)).toBe(false);
    }
  });
});
