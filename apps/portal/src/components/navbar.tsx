"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { gsap, useGSAP, ScrollTrigger, FINE_MOTION } from "@/lib/gsap";
import AccountEntry from "@/components/account/account-entry";
import { cn } from "@/lib/cn";
import { INSET_FOCUS_RING, revealKeyboardFocus } from "@/lib/navigation/scroller-focus";

const LINKS = [
  { index: "01", label: "资料库", href: "/library" },
  { index: "02", label: "智能刷题", href: "/practice" },
  { index: "03", label: "美食榜", href: "/food" },
  { index: "04", label: "互助平台", href: "/campus" },
  { index: "05", label: "求职雷达", href: "/career" },
];

const MOBILE_MENU_ID = "mobile-menu";

export default function Navbar() {
  const navRef = useRef<HTMLElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLElement>(null);
  const [scrolled, setScrolled] = useState(false);
  const [open, setOpen] = useState(false);

  /** Esc 与点遮罩关闭菜单：焦点回到菜单按钮，键盘读者从原处接着走。 */
  const closeMenu = useCallback(() => {
    setOpen(false);
    toggleRef.current?.focus();
  }, []);

  // 打开期间的键盘：Esc 关闭；Tab 只在菜单按钮（此时是关闭按钮）和面板之间循环，
  // 不落到遮罩下面的页面。
  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closeMenu();
        return;
      }
      if (event.key !== "Tab") return;
      const toggle = toggleRef.current;
      const menu = menuRef.current;
      if (!toggle || !menu) return;
      const items = menu.querySelectorAll<HTMLElement>("a[href], button");
      if (!items.length) return;
      const last = items[items.length - 1];
      const active = document.activeElement;
      const inMenu = active === toggle || menu.contains(active);
      const edge = event.shiftKey ? active === toggle : active === last;
      if (inMenu && !edge) return;
      event.preventDefault();
      (event.shiftKey ? last : toggle).focus();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open, closeMenu]);

  // 打开期间锁住遮罩下面的页面：不滚动；正文和页脚设为 inert，读屏软件的滑动浏览
  // （VoiceOver / TalkBack）同 Tab 一样进不去。关闭或卸载（点链接离开首页）时原样还回去。
  useEffect(() => {
    if (!open) return;
    const { style } = document.body;
    const previous = style.overflow;
    style.overflow = "hidden";
    const behind = Array.from(document.querySelectorAll<HTMLElement>("main, footer")).filter((el) => !el.inert);
    for (const el of behind) el.inert = true;
    return () => {
      style.overflow = previous;
      for (const el of behind) el.inert = false;
    };
  }, [open]);

  // 窗口拉宽到 md 起（与 Tailwind 的 md 断点同为 48rem），菜单按钮就不在了：
  // 收起菜单，滚动锁跟着释放。
  useEffect(() => {
    if (!open) return;
    const desktop = window.matchMedia("(min-width: 48rem)");
    const onChange = (event: MediaQueryListEvent) => {
      if (event.matches) setOpen(false);
    };
    desktop.addEventListener("change", onChange);
    return () => desktop.removeEventListener("change", onChange);
  }, [open]);

  useGSAP(
    () => {
      const nav = navRef.current!;
      const mm = gsap.matchMedia();

      mm.add(FINE_MOTION, () => {
        // 向下滚动隐藏、向上滚动显示
        ScrollTrigger.create({
          start: 0,
          end: "max",
          onUpdate(self) {
            const y = self.scroll();
            setScrolled(y > 40);
            if (y < 80) {
              gsap.to(nav, { yPercent: 0, duration: 0.35, ease: "power2.out", overwrite: "auto" });
              return;
            }
            gsap.to(nav, {
              yPercent: self.direction === 1 ? -100 : 0,
              duration: 0.35,
              ease: "power2.out",
              overwrite: "auto",
            });
          },
        });
      });

      mm.add("(prefers-reduced-motion: reduce)", () => {
        ScrollTrigger.create({
          start: 0,
          end: "max",
          onUpdate(self) {
            setScrolled(self.scroll() > 40);
          },
        });
      });

      // 整屏吸附布局就绪后校准所有触发位置
      ScrollTrigger.refresh();
    },
    { scope: navRef }
  );

  return (
    <>
      <header
        ref={navRef}
        className={cn(
          "fixed inset-x-0 top-0 z-50 bg-paper/95 backdrop-blur-none transition-colors",
          scrolled && "border-b border-line"
        )}
      >
        <div className="mx-auto flex h-16 max-w-site items-center justify-between px-5 md:px-8">
          {/* 字标一行只有 28px 高：上下各借 8px 撑满 44px 点击区，页头布局不变。 */}
          <Link href="/" className="-my-2 flex items-baseline gap-3 py-2">
            <span className="font-display text-xl font-bold tracking-tight">
              henukit<span className="text-accent">®</span>
            </span>
            {/* 页头是 95% 纸白，压在墨色的刷题模块上会变深，灰字至少 ink/65。 */}
            <span className="hidden font-mono text-xs tracking-[0.3em] text-ink/65 sm:inline">
              KEEP IN TOUCH
            </span>
          </Link>

          {/* 桌面导航：平板上同样靠手指点，点击区撑到 44px 高；下划线挂在里层 span 上，仍贴着文字。 */}
          <nav className="hidden items-center gap-7 md:flex">
            {LINKS.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                className="group inline-flex min-h-11 items-center font-mono text-xs text-ink/80 transition-colors hover:text-ink"
              >
                {/* 宽字距只加在编号上，中文模块名不拉开（DESIGN_SYSTEM.md 第 4 节）。 */}
                <span className="relative py-1">
                  <span className="mr-1.5 tracking-widest text-accent-text">{link.index}</span>
                  {link.label}
                  <span
                    aria-hidden
                    className="absolute inset-x-0 -bottom-0.5 h-px origin-left scale-x-0 bg-accent transition-transform duration-300 group-hover:scale-x-100"
                  />
                </span>
              </Link>
            ))}
            <span aria-hidden className="h-4 w-px bg-ink/20" />
            <AccountEntry />
          </nav>

          {/* 移动端汉堡 */}
          <button
            ref={toggleRef}
            type="button"
            aria-label={open ? "关闭菜单" : "打开菜单"}
            aria-expanded={open}
            aria-controls={MOBILE_MENU_ID}
            onClick={() => setOpen((v) => !v)}
            className="flex h-11 w-11 flex-col items-center justify-center gap-1.5 md:hidden"
          >
            <span
              className={cn(
                "h-px w-6 bg-ink transition-transform",
                open && "translate-y-[3.5px] rotate-45"
              )}
            />
            <span
              className={cn(
                "h-px w-6 bg-ink transition-transform",
                open && "-translate-y-[3.5px] -rotate-45"
              )}
            />
          </button>
        </div>

        {/* 移动端下拉面板：常驻 DOM，aria-controls 才总有指向。页面锁着滚动，
            矮屏（横屏手机）放不下时面板自己滚。每一行都贴着面板的边，面板又会裁掉
            画在行外面的东西：焦点框画在行里面，键盘聚焦的那一行整个滑进来。 */}
        <nav
          ref={menuRef}
          id={MOBILE_MENU_ID}
          hidden={!open}
          onFocus={revealKeyboardFocus}
          className="max-h-[calc(100dvh-4rem)] overflow-y-auto overscroll-contain border-t border-line bg-paper md:hidden"
        >
          {LINKS.map((link) => (
            <Link
              key={link.href}
              href={link.href}
              onClick={() => setOpen(false)}
              className={cn("flex items-center gap-3 border-b border-line px-5 py-4 font-mono text-sm", INSET_FOCUS_RING)}
            >
              <span className="text-accent-text">{link.index}</span>
              {link.label}
            </Link>
          ))}
          <AccountEntry
            onClick={() => setOpen(false)}
            className={cn("flex w-full items-center gap-3 px-5 py-4 font-mono text-sm text-ink", INSET_FOCUS_RING)}
            nameClassName="max-w-none text-sm text-ink"
          />
        </nav>
      </header>

      {/* 手机菜单遮罩：盖住面板下方露出的页面，点一下关闭菜单。放在 header 外面：
          header 滚动时带 transform，fixed 子元素会被它框住。 */}
      {open && (
        <div aria-hidden onClick={closeMenu} className="fixed inset-0 z-40 bg-ink/50 md:hidden" />
      )}
    </>
  );
}
