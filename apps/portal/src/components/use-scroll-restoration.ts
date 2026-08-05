"use client";

import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";

const STORAGE_PREFIX = "henukit.scroll.v1:";

/**
 * Frames to keep reapplying the offset while the list finishes painting.
 * Rows arrive in one render, but fonts and images can still grow the document
 * for a few frames after that; ~30 frames is half a second at 60fps.
 */
const MAX_RESTORE_FRAMES = 30;

let returnedThroughHistory = false;
let popstateBound = false;

function bindPopstate() {
  if (popstateBound || typeof window === "undefined") return;
  popstateBound = true;
  window.addEventListener("popstate", () => {
    returnedThroughHistory = true;
  });
}

function offsetKey(pathname: string): string {
  return STORAGE_PREFIX + pathname;
}

function readOffset(key: string): number {
  try {
    const stored = Number(window.sessionStorage.getItem(key) ?? 0);
    return Number.isFinite(stored) ? stored : 0;
  } catch {
    // Blocked or full sessionStorage only costs scroll restoration.
    return 0;
  }
}

function writeOffset(key: string, offset: number) {
  try {
    window.sessionStorage.setItem(key, String(Math.round(offset)));
  } catch {
    // See readOffset: losing the offset is not worth failing navigation.
  }
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
 * Only history navigation restores. Arriving fresh from another page keeps the
 * normal top-of-page start.
 */
export function useScrollRestoration(ready: boolean) {
  const pathname = usePathname();
  const restored = useRef(false);

  // Claim the stored offset during the first render of this page, before any
  // effect can attach the recorder below. The router emits its own scroll
  // events while swapping pages, and reading the offset later would hand back
  // whatever those events had already recorded instead of where the reader
  // actually left off.
  const [restoreTarget] = useState(() => {
    if (typeof window === "undefined" || !returnedThroughHistory) return 0;
    returnedThroughHistory = false;
    return readOffset(offsetKey(window.location.pathname));
  });

  useEffect(bindPopstate, []);

  // Track the offset while the reader browses, so it is already recorded by
  // the time they follow a link away from the list.
  useEffect(() => {
    const key = offsetKey(pathname);
    let frame = 0;
    let leaving = false;

    const record = () => {
      if (frame) return;
      frame = window.requestAnimationFrame(() => {
        frame = 0;
        // The router returns to the top of the document as it swaps pages,
        // which would otherwise replace the reader's offset with 0 before this
        // effect is torn down. Ignore that one reset; any other movement means
        // the reader is still here (a cancelled click, say) and re-arms
        // recording.
        if (leaving && window.scrollY === 0) return;
        leaving = false;
        writeOffset(key, window.scrollY);
      });
    };

    const freeze = () => {
      leaving = true;
      if (frame) {
        window.cancelAnimationFrame(frame);
        frame = 0;
      }
      writeOffset(key, window.scrollY);
    };

    const onClick = (event: MouseEvent) => {
      const anchor = (event.target as Element | null)?.closest?.("a[href]") as
        | HTMLAnchorElement
        | null;
      if (!anchor) return;
      // An in-page jump (the tier rail) and anything opening in another tab
      // both leave this page mounted, so they must not freeze recording.
      if (anchor.getAttribute("href")?.startsWith("#")) return;
      if (anchor.target && anchor.target !== "_self") return;
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
        return;
      }
      freeze();
    };

    window.addEventListener("scroll", record, { passive: true });
    document.addEventListener("click", onClick, true);
    window.addEventListener("popstate", freeze);
    return () => {
      window.removeEventListener("scroll", record);
      document.removeEventListener("click", onClick, true);
      window.removeEventListener("popstate", freeze);
      if (frame) window.cancelAnimationFrame(frame);
    };
  }, [pathname]);

  useEffect(() => {
    if (!ready || restored.current || restoreTarget <= 0) return;
    restored.current = true;

    const target = restoreTarget;
    let frame = 0;
    let attempts = 0;
    const apply = () => {
      attempts += 1;
      const settled =
        document.documentElement.scrollHeight - window.innerHeight >= target;
      window.scrollTo(0, target);
      if (!settled && attempts < MAX_RESTORE_FRAMES) {
        frame = window.requestAnimationFrame(apply);
      }
    };

    frame = window.requestAnimationFrame(apply);
    return () => {
      if (frame) window.cancelAnimationFrame(frame);
    };
  }, [ready, restoreTarget]);
}
