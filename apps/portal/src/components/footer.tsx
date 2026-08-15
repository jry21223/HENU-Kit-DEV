"use client";

import Link from "next/link";
import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import AmbientSvg from "@/components/ui/ambient-svg";

const LINKS = [
  { index: "01", label: "资料库", href: "/library" },
  { index: "02", label: "智能刷题", href: "/practice" },
  { index: "03", label: "美食榜", href: "/food" },
  { index: "04", label: "互助平台", href: "/campus" },
  { index: "05", label: "求职雷达", href: "/career" },
];

export default function Footer() {
  const sectionRef = useRef<HTMLElement>(null);
  const giantRef = useRef<HTMLParagraphElement>(null);

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        // 巨型字进入揭示（接近视窗中部时开始，可重播）
        gsap.from(giantRef.current, {
          y: 80,
          opacity: 0,
          duration: 1,
          ease: "power3.out",
          scrollTrigger: {
            trigger: sectionRef.current,
            start: "top 60%",
            toggleActions: "play none none reverse",
          },
        });
        // 巨型字水平视差（位置联动，贯穿 Footer 过场全程）
        gsap.fromTo(
          giantRef.current,
          { x: "4%" },
          {
            x: "-4%",
            ease: "none",
            scrollTrigger: {
              trigger: sectionRef.current,
              start: "top bottom",
              end: "bottom top",
              scrub: 1,
            },
          }
        );
        gsap.from("[data-footer-bottom]", {
          y: 24,
          opacity: 0,
          duration: 0.7,
          ease: "power3.out",
          scrollTrigger: {
            trigger: sectionRef.current,
            start: "top 55%",
            toggleActions: "play none none reverse",
          },
        });
      });
      return () => mm.revert();
    },
    { scope: sectionRef }
  );

  return (
    <footer
      ref={sectionRef}
      className="snap-screen relative overflow-hidden border-t border-line bg-paper"
    >
      {/* 环境装饰：背景刻度盘 */}
      <AmbientSvg
        variant="dial"
        className="absolute -right-10 top-1/2 hidden -translate-y-1/2 text-ink/20 md:block"
      />

      <div className="mx-auto flex min-h-svh max-w-7xl flex-col px-5 pt-24 md:px-10">
        <div className="flex flex-1 items-center">
          <p
            ref={giantRef}
            aria-hidden
            className="text-outline font-display text-[clamp(3rem,11vw,10rem)] leading-none font-bold whitespace-nowrap select-none"
          >
            KEEP IN TOUCH
          </p>
        </div>

        <AmbientSvg variant="flow" className="mb-8 text-ink/30" />

        <div
          data-footer-bottom
          className="flex flex-col justify-between gap-8 border-t border-line py-10 md:flex-row md:items-center"
        >
          <p className="font-display text-xl font-bold">
            henukit<span className="text-accent">®</span>
          </p>
          <nav className="flex flex-wrap gap-x-8 gap-y-3">
            {LINKS.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                className="group relative font-mono text-xs tracking-widest text-ink/70 transition-colors hover:text-ink"
              >
                <span className="mr-1.5 text-accent">{link.index}</span>
                {link.label}
                <span
                  aria-hidden
                  className="absolute inset-x-0 -bottom-1 h-px origin-left scale-x-0 bg-accent transition-transform duration-300 group-hover:scale-x-100"
                />
              </Link>
            ))}
          </nav>
          <p className="font-mono text-xs tracking-widest text-ink/50">
            © {new Date().getFullYear()} henukit
          </p>
          <p className="mt-2 max-w-xs font-mono text-xs leading-5 tracking-wider text-ink/50">
            学生自主运营 · 非河南大学官方项目
          </p>
        </div>
      </div>
    </footer>
  );
}
