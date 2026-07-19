"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { gsap, useGSAP, ScrollTrigger, FINE_MOTION } from "@/lib/gsap";
import AccountEntry from "@/components/account/account-entry";
import { cn } from "@/lib/cn";

const LINKS = [
  { index: "01", label: "资料库", href: "/library" },
  { index: "02", label: "智能刷题", href: "/practice" },
  { index: "03", label: "美食榜", href: "/food" },
  { index: "04", label: "互助平台", href: "/campus" },
];

export default function Navbar() {
  const navRef = useRef<HTMLElement>(null);
  const [scrolled, setScrolled] = useState(false);
  const [open, setOpen] = useState(false);

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
    <header
      ref={navRef}
      className={cn(
        "fixed inset-x-0 top-0 z-50 bg-paper/95 backdrop-blur-none transition-colors",
        scrolled && "border-b border-line"
      )}
    >
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-5 md:px-10">
        <Link href="/" className="flex items-baseline gap-3">
          <span className="font-display text-xl font-bold tracking-tight">
            henukit<span className="text-accent">®</span>
          </span>
          <span className="hidden font-mono text-[10px] tracking-[0.3em] text-ink/50 sm:inline">
            KEEP IN TOUCH
          </span>
        </Link>

        {/* 桌面导航 */}
        <nav className="hidden items-center gap-8 md:flex">
          {LINKS.map((link) => (
            <Link
              key={link.href}
              href={link.href}
              className="group relative py-1 font-mono text-xs tracking-widest text-ink/80 transition-colors hover:text-ink"
            >
              <span className="mr-1.5 text-accent">{link.index}</span>
              {link.label}
              <span
                aria-hidden
                className="absolute inset-x-0 -bottom-0.5 h-px origin-left scale-x-0 bg-accent transition-transform duration-300 group-hover:scale-x-100"
              />
            </Link>
          ))}
          <span aria-hidden className="h-4 w-px bg-ink/20" />
          <AccountEntry />
        </nav>

        {/* 移动端汉堡 */}
        <button
          type="button"
          aria-label="打开菜单"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          className="flex h-10 w-10 flex-col items-center justify-center gap-1.5 md:hidden"
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

      {/* 移动端下拉面板 */}
      {open && (
        <nav className="border-t border-line bg-paper md:hidden">
          {LINKS.map((link) => (
            <Link
              key={link.href}
              href={link.href}
              onClick={() => setOpen(false)}
              className="flex items-center gap-3 border-b border-line px-5 py-4 font-mono text-sm"
            >
              <span className="text-accent">{link.index}</span>
              {link.label}
            </Link>
          ))}
          <div className="flex items-center gap-3 px-5 py-4">
            <span className="font-mono text-sm text-accent">ACC</span>
            <AccountEntry />
          </div>
        </nav>
      )}
    </header>
  );
}
