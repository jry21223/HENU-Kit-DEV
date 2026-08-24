import type { Metadata } from "next";

import CareerHistoryPageClient from "./page-client";

// 历史页是登录后的账户内页面，不进 sitemap；但仍要有自己的标题，
// 否则会继承 career/layout 的「求职雷达」，两个页面标签页同名。
export const metadata: Metadata = {
  title: "扫描历史 — henukit 求职雷达",
};

export default function CareerHistoryPage() {
  return <CareerHistoryPageClient />;
}
