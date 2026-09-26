import type { HTMLAttributes, ReactNode } from "react";
import SiteFooter from "@/components/site-footer";

/**
 * 除首页外所有布局共用的外壳：顶部导航、撑满剩余高度的正文和固定页脚，
 * 这样“每个页面都有主体声明与协议入口”由结构保证，而不是逐个布局记得加。
 * 首页的页脚在整屏吸附的最后一屏里，见 footer.tsx。
 */
export default function SiteShell({
  header,
  children,
  className,
  ...rest
}: { header?: ReactNode; children: ReactNode } & Omit<HTMLAttributes<HTMLDivElement>, "children">) {
  // 直接拼接而不用 cn：tailwind-merge 会把自定义的 bg-blueprint（网格背景图）当成背景色，
  // 进而丢掉 bg-paper；两者在 CSS 里并不冲突。
  return (
    <div className={`flex min-h-svh flex-col bg-paper text-ink ${className ?? ""}`} {...rest}>
      {header}
      <div className="flex-1">{children}</div>
      <SiteFooter />
    </div>
  );
}
