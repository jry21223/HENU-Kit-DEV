"use client";

import { useCallback, useRef } from "react";
import { createIdempotencyKey } from "@/lib/practice/session-handoff";

/**
 * One live idempotency key per logical write, keyed by scope. A successful
 * write consumes the key via clear(); a failed one keeps it so a retry
 * replays the same Core command instead of minting a second one.
 */
export function useIdempotencyKey(prefix: string) {
  const keys = useRef<Record<string, string>>({});
  const obtain = useCallback(
    (scope: string): string => {
      const remembered = keys.current[scope];
      if (remembered) return remembered;
      const created = createIdempotencyKey(prefix);
      keys.current[scope] = created;
      return created;
    },
    [prefix]
  );
  const clear = useCallback((scope: string): void => {
    delete keys.current[scope];
  }, []);
  return { obtain, clear };
}
