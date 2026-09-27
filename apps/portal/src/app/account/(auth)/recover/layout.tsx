import type { Metadata } from "next";
import { pageTitle } from "@/lib/seo";

// 页面是客户端组件，不能导出 metadata；标题由这一段的布局给出。
export const metadata: Metadata = { title: pageTitle("找回密码") };

export default function RecoverLayout({ children }: { children: React.ReactNode }) {
  return children;
}
