"use client";

import { useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import {
  SCROLL_REAPPLY_WINDOW_MS,
  claimHistoryReturn,
  readScrollOffset,
  reapplyScrollOffset,
  rememberHistoryReturn,
  scrollLandingFor,
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
 * hook adds what a client-rendered list needs: the return intent as trigger
 * and `ready` as the gate, even when a parent return's short window expires.
 *
 * History and explicit parent navigation restore. Fresh entry and sideways
 * navigation keep the normal top-of-page start.
 */
export function useScrollRestoration(ready: boolean) {
  // 新页面可先于地址栏提交渲染；此时 window.location 仍是详情页，须读路由上下文。
  const pathname = usePathname();
  const restored = useRef(false);
  const stopRestoring = useRef<(() => void) | null>(null);

  // Claim the stored offset during the first render of this page, before any
  // effect runs. The router emits its own scroll events while swapping pages,
  // and reading the offset later would hand back whatever those events had
  // already recorded instead of where the reader actually left off.
  const [restoreTarget] = useState(() => {
    if (typeof window === "undefined") return 0;
    const historyReturn = claimHistoryReturn(pathname);
    if (!historyReturn && scrollLandingFor(pathname) !== "restore") return 0;
    return readScrollOffset(pathname);
  });

  useEffect(bindPopstate, []);

  // 数据未到时读者已开始操作，也要放弃之后的补位，避免慢请求把人拉回旧位置。
  useEffect(() => {
    if (restoreTarget <= 0) return;
    const yieldToReader = () => {
      restored.current = true;
      stopRestoring.current?.();
    };
    const inputs = ["wheel", "touchstart", "keydown", "pointerdown"] as const;
    for (const input of inputs) window.addEventListener(input, yieldToReader, { passive: true });
    return () => {
      for (const input of inputs) window.removeEventListener(input, yieldToReader);
    };
  }, [restoreTarget]);

  useEffect(() => {
    if (!ready || restored.current || restoreTarget <= 0) return;
    restored.current = true;
    const stop = reapplyScrollOffset(restoreTarget, SCROLL_REAPPLY_WINDOW_MS);
    stopRestoring.current = stop;
    return stop;
  }, [ready, restoreTarget]);
}
