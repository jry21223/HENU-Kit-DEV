"use client";

import { useEffect } from "react";
import { documentTitle, type TitleModule } from "@/lib/seo";

/**
 * 详情页的内容在客户端才到达：服务端 metadata 先给出这一类页面的静态标题，内容到了
 * 再把内容名补进标签页标题（格式同 pageTitle）。`page` 为空时不动标题。
 *
 * Next 的 metadata 是流式到达的，站内跳转时它的 <title> 可能比内容还晚挂上，把这里写的
 * 标题盖回静态标题；所以盯着 <head>，标题元素换了就再写一次。只在还停在这一页时写：
 * 离开后换上的是下一页自己的标题，不能被这里覆盖。
 */
export function useDocumentTitle(page: string | null | undefined, module: TitleModule) {
  useEffect(() => {
    if (!page) return;
    const title = documentTitle(page, module);
    const pathname = window.location.pathname;
    const apply = () => {
      if (window.location.pathname === pathname && document.title !== title) {
        document.title = title;
      }
    };
    apply();
    const observer = new MutationObserver(apply);
    observer.observe(document.head, { childList: true });
    return () => observer.disconnect();
  }, [page, module]);
}
