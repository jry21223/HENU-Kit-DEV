"use client";

import { useEffect } from "react";
import { usePathname } from "next/navigation";
import {
  SCROLL_REAPPLY_WINDOW_MS,
  SCROLL_RESTORE_WINDOW_MS,
  clearStaleScrollRequest,
  isAncestorPath,
  readScrollOffset,
  reapplyScrollOffset,
  requestScrollRestore,
  requestScrollTop,
  scrollLandingFor,
  writeScrollOffset,
} from "@/lib/navigation/scroll-memory";

/** 滚动写入不需要每帧都碰 sessionStorage。 */
const RECORD_INTERVAL_MS = 120;

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

    // 离开这一页时先落一次盘：这是权威值，之后的写入都不该覆盖它。
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
        // 路由换页会把文档拉回顶部。等这条位移落定时路径已经换了，说明它属于上一页，
        // 既不该写进新页的记录，也不该覆盖上一页离开时的位置。
        //
        // 判断依据是「路径变了没有」，不是「位置是不是 0」：后者会让一次没走成的点击
        // （被别的处理函数拦下、下载链接等）把读者真实滚回顶部的位移永久丢掉。
        if (window.location.pathname !== pathname) return;
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

      flush();
      // 只有向上回退才落回原处；横向切换标签、走进更深的页面都从顶部开始。
      if (isAncestorPath(destination, pathname)) requestScrollRestore(destination);
      else if (destination !== pathname) requestScrollTop(destination);
    };

    window.addEventListener("scroll", record, { passive: true });
    document.addEventListener("click", onClick, true);
    window.addEventListener("popstate", flush);
    return () => {
      window.removeEventListener("scroll", record);
      document.removeEventListener("click", onClick, true);
      window.removeEventListener("popstate", flush);
      if (timer) window.clearTimeout(timer);
    };
  }, [pathname]);

  // 落地：向上回退放回原处，其余从顶部开始。
  useEffect(() => {
    clearStaleScrollRequest(pathname);
    const landing = scrollLandingFor(pathname);
    if (!landing) return;
    const target = landing === "top" ? 0 : readScrollOffset(pathname);
    if (target <= 0 && landing === "restore") return;

    const stopReapplying = reapplyScrollOffset(
      target,
      landing === "top" ? SCROLL_REAPPLY_WINDOW_MS : SCROLL_RESTORE_WINDOW_MS
    );

    // 读者一动手就把位置交还给他。pointerdown 覆盖那些"只发 scroll、不发输入事件"的
    // 操作：拖动滚动条、中键自动滚动；锚点跳转本身也是一次点击。我们自己的 scrollTo
    // 不产生 pointerdown，所以不会自我取消。
    const yieldToReader = () => stopReapplying();

    const readerInputs = ["wheel", "touchstart", "keydown", "pointerdown"] as const;
    for (const type of readerInputs) {
      window.addEventListener(type, yieldToReader, { passive: true, once: true });
    }
    return () => {
      stopReapplying();
      for (const type of readerInputs) {
        window.removeEventListener(type, yieldToReader);
      }
    };
  }, [pathname]);

  return null;
}
