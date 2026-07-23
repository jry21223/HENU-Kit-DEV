"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";

export interface HeroCounter {
  label: string;
  value: number;
}

function formatNum(n: number) {
  return String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/**
 * 子站首页 hero 统一骨架（library/food/campus）：
 * 左 = mono 编号 + 大字 + 标语 + 动态计数；右 = 图纸画板 + 站点特色 SVG 场景。
 * 不接 WebGL；入场为标题 reveal + 网格线 scaleX 生长的克制版。
 */
export default function SubHero({
  index,
  en,
  title,
  slogan,
  counters,
  fig,
  scene,
}: {
  index: string;
  en: string;
  title: string;
  slogan: string;
  counters: HeroCounter[];
  fig: string;
  scene: React.ReactNode;
}) {
  const sectionRef = useRef<HTMLElement>(null);
  const counterRefs = useRef<Array<HTMLSpanElement | null>>([]);

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        const tl = gsap.timeline({ defaults: { ease: "power3.out" } });
        tl.from("[data-hero-title]", { y: 36, opacity: 0, duration: 0.7, stagger: 0.1 })
          .from(
            "[data-hero-line]",
            { scaleX: 0, transformOrigin: "left center", duration: 0.6 },
            "-=0.4"
          )
          .from("[data-hero-scene]", { opacity: 0, duration: 0.7 }, "-=0.3");

        counters.forEach((c, i) => {
          const el = counterRefs.current[i];
          if (!el) return;
          const obj = { v: 0 };
          gsap.to(obj, {
            v: c.value,
            duration: 1.8,
            delay: 0.4 + i * 0.2,
            ease: "power2.out",
            onUpdate: () => {
              el.textContent = formatNum(obj.v);
            },
          });
        });
      });
      mm.add("(prefers-reduced-motion: reduce)", () => {
        counters.forEach((c, i) => {
          const el = counterRefs.current[i];
          if (el) el.textContent = formatNum(c.value);
        });
      });
      return () => mm.revert();
    },
    { scope: sectionRef }
  );

  return (
    <section ref={sectionRef} className="relative overflow-hidden border-b border-line">
      <div className="mx-auto grid min-h-[52vh] max-w-[1440px] lg:grid-cols-2">
        {/* 左：文案 + 计数 */}
        <div className="flex flex-col justify-center px-5 py-14 md:px-8">
          <p data-hero-title className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">{index}</span>
            <span className="mx-2">/</span>
            {en}
          </p>
          <h1 data-hero-title className="mt-4 font-display text-6xl font-bold tracking-tight md:text-7xl">
            {title}
          </h1>
          <div data-hero-line className="mt-6 h-px w-24 bg-accent" />
          <p data-hero-title className="mt-5 max-w-md text-sm leading-7 text-ink/70">
            {slogan}
          </p>
          <div data-hero-title className="mt-8 flex flex-wrap gap-x-10 gap-y-4">
            {counters.map((c, i) => (
              <div key={c.label}>
                <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">{c.label}</p>
                <p className="mt-1 font-display text-3xl font-bold tabular-nums">
                  <span ref={(el) => { counterRefs.current[i] = el; }}>0</span>
                </p>
              </div>
            ))}
          </div>
        </div>

        {/* 右：图纸画板 + 场景 */}
        <div className="bg-blueprint relative flex items-center justify-center border-t border-line p-10 lg:border-l lg:border-t-0">
          <span aria-hidden className="absolute left-4 top-4 font-mono text-[10px] tracking-[0.3em] text-ink/40">
            {fig}
          </span>
          <span aria-hidden className="absolute bottom-4 right-4 font-mono text-accent">+</span>
          <div data-hero-scene className="w-full max-w-sm">
            {scene}
          </div>
        </div>
      </div>
    </section>
  );
}
