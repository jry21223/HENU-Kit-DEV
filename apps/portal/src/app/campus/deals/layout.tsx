import type { Metadata } from "next";
import { pageTitle } from "@/lib/seo";

// 标题由这一段的布局给出，页面本身只负责正文。
export const metadata: Metadata = { title: pageTitle("我的交易", "campus") };

export default function DealsLayout({ children }: { children: React.ReactNode }) {
  return children;
}
