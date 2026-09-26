"use client";

import { useEffect } from "react";
import { documentTitle, type TitleModule } from "@/lib/seo";

/**
 * 详情页的内容在客户端才到达：服务端 metadata 先给出这一类页面的静态标题，内容到了
 * 再把内容名补进标签页标题（格式同 pageTitle）。`page` 为空时不动标题。
 *
 * Next 的 metadata 是流式到达的，站内跳转时它的 <title> 可能比内容还晚挂上，把这里写的
 * 标题盖回静态标题；内容名在挂载时就已知（如题库收藏夹的题库名）时，metadata 稍后提交
 * 还会把原标题元素里的文字改回去。所以盯着 <head> 连同标题里的文字，标题元素换了或文字
 * 被改回，就再写一次。只在还停在这一页时写：离开后换上的是下一页自己的标题，不能被这里覆盖。
 *
 * document.title 读回来的是去掉首尾空白、连续空白并成一个空格之后的文字，所以写之前先按同样
 * 规则整理。内容名来自用户投稿，可能带连续空格：不整理的话读回来永远对不上，每次写入又会被
 * 上面的观察当成“被改回”，一直写下去把页面卡死。
 */
function asReadBack(title: string): string {
  return title.replace(/[\t\n\f\r ]+/g, " ").replace(/^ | $/g, "");
}

export function useDocumentTitle(page: string | null | undefined, module: TitleModule) {
  useEffect(() => {
    if (!page) return;
    const title = asReadBack(documentTitle(page, module));
    const pathname = window.location.pathname;
    const apply = () => {
      if (window.location.pathname === pathname && document.title !== title) {
        document.title = title;
      }
    };
    apply();
    const observer = new MutationObserver(apply);
    observer.observe(document.head, { childList: true, subtree: true, characterData: true });
    return () => observer.disconnect();
  }, [page, module]);
}
