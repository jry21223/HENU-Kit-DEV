"use client";

import Link from "next/link";
import { useRef } from "react";
import { gsap, useGSAP } from "@/lib/gsap";
import { cn } from "@/lib/cn";

/**
 * 磁吸按钮：hover 时按钮向光标轻微吸附，离开后弹性回正；
 * 橙色填充自下而上滑入。
 */
export default function MagneticButton({
  href,
  children,
  dark = false,
  className,
}: {
  href: string;
  children: React.ReactNode;
  dark?: boolean;
  className?: string;
}) {
  const ref = useRef<HTMLAnchorElement>(null);

  // 卸载时清理未完成的补间
  useGSAP(
    () => () => {
      if (ref.current) gsap.killTweensOf(ref.current);
    },
    { scope: ref }
  );

  const onMove = (e: React.MouseEvent) => {
    const el = ref.current;
    if (!el || window.matchMedia("(prefers-reduced-motion: reduce)").matches)
      return;
    const rect = el.getBoundingClientRect();
    const relX = e.clientX - rect.left - rect.width / 2;
    const relY = e.clientY - rect.top - rect.height / 2;
    gsap.to(el, { x: relX * 0.25, y: relY * 0.35, duration: 0.4, ease: "power2.out" });
  };

  const onLeave = () => {
    if (!ref.current) return;
    gsap.to(ref.current, {
      x: 0,
      y: 0,
      duration: 0.7,
      ease: "elastic.out(1, 0.4)",
    });
  };

  return (
    <Link
      ref={ref}
      href={href}
      onMouseMove={onMove}
      onMouseLeave={onLeave}
      className={cn(
        "group relative inline-flex w-fit items-center gap-3 overflow-hidden border px-7 py-3.5 font-mono text-sm tracking-widest",
        dark
          ? "border-line-dark text-paper"
          : "border-ink/30 text-ink",
        className
      )}
    >
      <span
        aria-hidden
        className="absolute inset-0 translate-y-full bg-accent transition-transform duration-300 ease-out group-hover:translate-y-0"
      />
      <span className="relative z-10 transition-colors duration-300 group-hover:text-paper">
        {children}
      </span>
      <span
        aria-hidden
        className="relative z-10 transition-all duration-300 group-hover:translate-x-1 group-hover:text-paper"
      >
        →
      </span>
    </Link>
  );
}
