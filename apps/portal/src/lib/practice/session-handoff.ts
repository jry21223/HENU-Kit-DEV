import type { PortalPracticeSessionResponse } from "@/lib/api/types";

/**
 * The favorites flow creates a practice session on the folder page and hands
 * it to the quiz page through sessionStorage, because no endpoint re-reads a
 * session by id. Both ends must agree on the key and the payload shape, so
 * they live here instead of being maintained in two components.
 */
export const PRACTICE_SESSION_HANDOFF_PREFIX = "henukit.practice.session.v1";

export type PracticeSessionHandoff = PortalPracticeSessionResponse["data"];

export function practiceSessionHandoffKey(sessionID: string): string {
  return `${PRACTICE_SESSION_HANDOFF_PREFIX}:${sessionID}`;
}

export function writePracticeSessionHandoff(payload: PracticeSessionHandoff): void {
  window.sessionStorage.setItem(
    practiceSessionHandoffKey(payload.session_id),
    JSON.stringify(payload)
  );
}

/** Returns null when the handoff is absent, unparsable, or mismatched. */
export function readPracticeSessionHandoff(sessionID: string): PracticeSessionHandoff | null {
  try {
    const raw = window.sessionStorage.getItem(practiceSessionHandoffKey(sessionID));
    if (!raw) return null;
    const parsed = JSON.parse(raw) as PracticeSessionHandoff;
    if (!parsed || parsed.session_id !== sessionID || !Array.isArray(parsed.questions)) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

/** One key per logical write; a retry replays the same Core command. */
export function createIdempotencyKey(prefix: string): string {
  const random =
    globalThis.crypto?.randomUUID?.() ??
    `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  return `${prefix}:${random}`;
}
