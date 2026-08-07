/**
 * Mint one browser-side idempotency key for a practice write. The prefix
 * names the logical write (e.g. "ranking-profile"); the suffix is a random
 * UUID when the runtime provides one, with a timestamp fallback otherwise.
 *
 * quiz/page.tsx on main carries an equivalent createBrowserKey; unify both
 * call sites onto this helper once that code is part of this branch's diff.
 */
export function createIdempotencyKey(prefix: string): string {
  const random =
    globalThis.crypto?.randomUUID?.() ??
    `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  return `${prefix}:${random}`;
}
