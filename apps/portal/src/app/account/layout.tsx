import type { Metadata } from "next";
import { moduleLayoutTitle } from "@/lib/seo";

// 账户中心的两个壳（登录与控制台）都是自己的布局，控制台的还是客户端组件；
// 模块名放在这一层，概览页和没有自己标题的页面都落到「账户中心」。
export const metadata: Metadata = { title: moduleLayoutTitle("account") };

export default function AccountLayout({ children }: { children: React.ReactNode }) {
  return children;
}
