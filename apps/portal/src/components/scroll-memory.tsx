"use client";

import { useEffect } from "react";
import { usePathname } from "next/navigation";
import {
  SCROLL_RESTORE_WINDOW_MS,
  clearStaleScrollRestore,
  isAncestorPath,
  readScrollOffset,
  requestScrollRestore,
  scrollRestoreRequested,
  writeScrollOffset,
} from "@/lib/navigation/scroll-memory";

/** 滚动写入不需要每帧都碰 sessionStorage。 */
const RECORD_INTERVAL_MS = 120;

/**
 * 路由换页时会把文档拉回顶部，那个 0 不是读者的位置。标记必须跨页面存活到读者
 * 真的滚动为止，所以放在模块作用域，而不是某一个 effect 的闭包里。
 */
let leftThroughNavigation = false;

function sameOriginPathname(href: string): string | null {
  try {
    const url = new URL(href, window.location.href);
    return url.origin === window.location.origin ? url.pathname : null;
  } catch {
    return null;
  }
}

/**
 * 记录读者在每个页面滚到哪，并在「返回上一级」落地后把位置放回去。
 *
 * 浏览器只在历史前进/后退时自己恢复滚动，而左上角的返回箭头是一次新的客户端
 * 导航——它会把文档拉到顶部。这里补上那一份：只要是向上导航（落点是当前路径的
 * 上一层，含回到平台首页），落点页面就用它自己上次离开时的 offset 落位。
 */
export default function ScrollMemory() {
  const pathname = usePathname();

  // 客户端外壳就绪标记：浏览器验收用它等待水合完成，而不是靠猜时间。
  useEffect(() => {
    document.documentElement.dataset.scrollMemory = "ready";
  }, []);

  // 记录当前页面的浏览进度。
  useEffect(() => {
    let timer = 0;

    const flush = () => {
      if (timer) {
        window.clearTimeout(timer);
        timer = 0;
      }
      writeScrollOffset(pathname, window.scrollY);
    };

    const record = () => {
      if (timer) return;
      timer = window.setTimeout(() => {
        timer = 0;
        // 换页时路由自己把文档拉回 0，那不是读者滚上去的。
        if (leftThroughNavigation && window.scrollY === 0) return;
        leftThroughNavigation = false;
        writeScrollOffset(pathname, window.scrollY);
      }, RECORD_INTERVAL_MS);
    };

    const onClick = (event: MouseEvent) => {
      if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const anchor = (event.target as Element | null)?.closest?.("a[href]") as HTMLAnchorElement | null;
      if (!anchor || (anchor.target && anchor.target !== "_self")) return;
      if ((anchor.getAttribute("href") ?? "").startsWith("#")) return;
      const destination = sameOriginPathname(anchor.href);
      if (!destination) return;

      leftThroughNavigation = true;
      flush();
      // 横向切换标签、走进更深的页面都该从顶部开始，只有向上回退才落回原处。
      if (isAncestorPath(destination, pathname)) requestScrollRestore(destination);
    };

    const onPopState = () => {
      leftThroughNavigation = true;
      flush();
    };

    window.addEventListener("scroll", record, { passive: true });
    document.addEventListener("click", onClick, true);
    window.addEventListener("popstate", onPopState);
    return () => {
      window.removeEventListener("scroll", record);
      document.removeEventListener("click", onClick, true);
      window.removeEventListener("popstate", onPopState);
      if (timer) window.clearTimeout(timer);
    };
  }, [pathname]);

  // 返回上一级落地：把读者放回上次离开的位置。
  useEffect(() => {
    clearStaleScrollRestore(pathname);
    if (!scrollRestoreRequested(pathname)) return;
    const target = readScrollOffset(pathname);
    if (target <= 0) return;

    let cancelled = false;
    let frame = 0;
    const deadline = performance.now() + SCROLL_RESTORE_WINDOW_MS;

    // 列表页的行是客户端拉取的，文档要过几帧才够高，所以反复落到同一位置，
    // 直到文档撑得下、窗口过期，或者读者自己动手（那就把位置交还给他）。
    const apply = () => {
      if (cancelled) return;
      window.scrollTo(0, target);
      const roomy = document.documentElement.scrollHeight - window.innerHeight >= target;
      if (!roomy && performance.now() < deadline) frame = window.requestAnimationFrame(apply);
    };

    const yieldToReader = () => {
      cancelled = true;
      if (frame) window.cancelAnimationFrame(frame);
    };

    window.addEventListener("wheel", yieldToReader, { passive: true, once: true });
    window.addEventListener("touchstart", yieldToReader, { passive: true, once: true });
    window.addEventListener("keydown", yieldToReader, { passive: true, once: true });
    frame = window.requestAnimationFrame(apply);

    return () => {
      cancelled = true;
      if (frame) window.cancelAnimationFrame(frame);
      window.removeEventListener("wheel", yieldToReader);
      window.removeEventListener("touchstart", yieldToReader);
      window.removeEventListener("keydown", yieldToReader);
    };
  }, [pathname]);

  return null;
}
