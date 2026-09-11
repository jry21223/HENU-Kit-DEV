"use client";

import { useEffect, useRef, useState } from "react";
import {
  SCROLL_REAPPLY_WINDOW_MS,
  claimHistoryReturn,
  readScrollOffset,
  reapplyScrollOffset,
  rememberHistoryReturn,
} from "@/lib/navigation/scroll-memory";

let popstateBound = false;

function bindPopstate() {
  if (popstateBound || typeof window === "undefined") return;
  popstateBound = true;
  window.addEventListener("popstate", () => {
    rememberHistoryReturn(window.location.pathname);
  });
}

/**
 * Restores the reader's place when a list page is reopened through browser
 * back/forward.
 *
 * These pages fetch their rows on the client, so at the moment the browser
 * would normally restore the offset the document is still empty and there is
 * nothing to scroll to — the reader is dropped at the top of the board instead
 * of where they left off. Pass `ready` as true only once the rows are in
 * state; the offset is then reapplied across frames until the document is
 * actually tall enough to hold it.
 *
 * Recording lives in the global ScrollMemory, which remembers every path's
 * offset and also covers the sub-site "back to the level above" control. This
 * hook adds only what a client-rendered list needs on top: history traversal
 * as the trigger, and `ready` as the gate.
 *
 * Only history navigation restores. Arriving fresh from another page keeps the
 * normal top-of-page start.
 */
export function useScrollRestoration(ready: boolean) {
  const restored = useRef(false);

  // Claim the stored offset during the first render of this page, before any
  // effect runs. The router emits its own scroll events while swapping pages,
  // and reading the offset later would hand back whatever those events had
  // already recorded instead of where the reader actually left off.
  const [restoreTarget] = useState(() => {
    if (typeof window === "undefined") return 0;
    const pathname = window.location.pathname;
    if (!claimHistoryReturn(pathname)) return 0;
    return readScrollOffset(pathname);
  });

  useEffect(bindPopstate, []);

  useEffect(() => {
    if (!ready || restored.current || restoreTarget <= 0) return;
    restored.current = true;
    return reapplyScrollOffset(restoreTarget, SCROLL_REAPPLY_WINDOW_MS);
  }, [ready, restoreTarget]);
}
