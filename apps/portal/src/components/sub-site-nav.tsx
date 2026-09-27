"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, type ComponentType, type ReactNode } from "react";
import AccountEntry from "@/components/account/account-entry";
import BackLink from "@/components/back-link";
import { cn } from "@/lib/cn";
import { INSET_FOCUS_RING, revealInScroller, revealKeyboardFocus } from "@/lib/navigation/scroller-focus";

export type SubSiteTab = {
  href: string;
  index: string;
  label: string;
  match: (pathname: string) => boolean;
  /** 功能还没开放：标签不可点，旁边直接写「未开放」，触屏和键盘用户都看得到。 */
  disabled?: boolean;
};

type TabLink = ComponentType<{ href: string; className?: string; "aria-current"?: "page"; children: ReactNode }>;

/**
 * 五个子站共用的页头：返回上一级 + 品牌字标 + 标签行 + 账户入口。
 *
 * 内容框与正文同为 max-w-site + px-5 md:px-8，返回链接与正文标题左缘对齐。
 * 只有一个标签时不渲染标签行：没有可切换的去处，手机上只会白占一整行。
 */
export default function SubSiteNav({
  brand,
  tabs,
  linkAs: TabLinkComponent = Link,
}: {
  brand: string;
  tabs: SubSiteTab[];
  /** 刷题用带形变过渡的链接，其余子站用普通 Link。 */
  linkAs?: TabLink;
}) {
  const pathname = usePathname();
  const showTabs = tabs.length > 1;
  const activeIndex = tabs.findIndex((tab) => tab.match(pathname));
  const navRef = useRef<HTMLElement>(null);

  // 手机上标签行放不下时横向滑动；当前标签被裁在哪一边，就往哪一边把它滑进来。
  // 页头在同一子站的页面之间一直挂着，上一页滑过的位置会带过来，所以两边都要管。
  useEffect(() => {
    const nav = navRef.current;
    const active = activeIndex >= 0 ? nav?.children[activeIndex] : undefined;
    if (nav && active) revealInScroller(nav, active);
  }, [activeIndex]);

  return (
    <header className="sticky top-0 z-40 border-b border-line bg-paper">
      <div className="mx-auto flex min-h-14 max-w-site flex-wrap items-center px-5 md:flex-nowrap md:justify-between md:px-8">
        <div
          className={cn(
            "flex h-14 w-full items-center justify-between",
            showTabs && "md:h-auto md:w-auto md:justify-start md:gap-4"
          )}
        >
          <div className="flex items-baseline gap-4">
            <BackLink className="inline-flex min-h-11 min-w-11 items-center" />
            <span className="font-display text-base font-bold tracking-tight">
              {brand}<span className="text-accent">®</span>
            </span>
          </div>
          <div className={showTabs ? "md:hidden" : undefined}>
            <AccountEntry compact />
          </div>
        </div>
        {showTabs ? (
          <nav
            ref={navRef}
            // 键盘聚焦的标签同样整个滑进来：浏览器聚焦时只要标签露出一截就不再横滑，焦点框会被裁掉一边。
            onFocus={revealKeyboardFocus}
            className="order-3 -mx-5 flex w-[calc(100%+2.5rem)] min-w-0 items-center gap-5 overflow-x-auto border-t border-line px-5 scrollbar-none md:order-none md:mx-0 md:w-auto md:gap-8 md:overflow-visible md:border-t-0 md:px-0"
          >
            {tabs.map((tab, index) => {
              const active = index === activeIndex;
              // 宽字距只加在编号上，中文标签不拉开（DESIGN_SYSTEM.md 第 4 节）。
              const content = (
                <>
                  <span className={cn("mr-1 tracking-widest", active ? "text-accent-text" : "text-ink/60")}>{tab.index}</span>
                  {tab.label}
                  <span
                    aria-hidden
                    className={cn(
                      "absolute inset-x-0 -bottom-0.5 h-px origin-left bg-accent transition-transform duration-300",
                      active ? "scale-x-100" : "scale-x-0",
                      !tab.disabled && "group-hover:scale-x-100"
                    )}
                  />
                </>
              );
              if (tab.disabled) {
                // 读者已经在这一页（如从收藏夹进入答题）时它就是当前标签，不再标「未开放」。
                return (
                  <span
                    key={tab.href}
                    data-tab-unavailable={active ? undefined : true}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "relative shrink-0 py-1 font-mono text-xs md:shrink",
                      active ? "text-ink" : "cursor-not-allowed text-ink/60"
                    )}
                  >
                    {content}
                    {active ? null : (
                      <span className="ml-2 border border-line px-1 text-ink/70">未开放</span>
                    )}
                  </span>
                );
              }
              // 点击区撑到 44px 高（DESIGN_SYSTEM §13），下划线仍贴着文字：它挂在里层的 span 上。
              // 手机上的标签行横向滑动，会裁掉画在标签外面的东西，而标签又撑满了行高：焦点框画在
              // 标签里面，左右各留 6px（等量负外边距，间距不变），整圈都看得见。
              return (
                <TabLinkComponent
                  key={tab.href}
                  href={tab.href}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    // 只在手机的横向滑动行里不收缩；md 起标签行不滑动，放不下时允许折行，页面不横向溢出。
                    "group -mx-1.5 inline-flex min-h-11 shrink-0 items-center px-1.5 font-mono text-xs transition-colors md:shrink",
                    INSET_FOCUS_RING,
                    active ? "text-ink" : "text-ink/60 hover:text-ink"
                  )}
                >
                  <span className="relative py-1">{content}</span>
                </TabLinkComponent>
              );
            })}
            <span aria-hidden className="hidden h-4 w-px bg-ink/20 md:block" />
            <span className="hidden md:block">
              <AccountEntry compact />
            </span>
          </nav>
        ) : null}
      </div>
    </header>
  );
}
