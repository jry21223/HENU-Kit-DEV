"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

export interface HeroCounter {
  label: string;
  value: number | null;
  busy?: boolean;
}

function formatNum(n: number) {
  return String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/**
 * 子站首页 hero 统一骨架（library/food/campus）：
 * 左 = mono 编号 + 大字 + 标语 + 动态计数；右 = 图纸画板 + 站点特色 SVG 场景。
 * 不接 WebGL；入场为标题 reveal + 网格线 scaleX 生长的克制版，用 globals.css 的 enter-*
 * keyframes，首帧即开始播放、不等水合（#537）；animationDelay 按原 GSAP 时间轴错峰。
 */
export default function SubHero({
  index,
  en,
  title,
  slogan,
  counters,
  fig,
  scene,
  compactOnMobile = false,
}: {
  index: string;
  en: string;
  title: string;
  slogan: string;
  counters: HeroCounter[];
  fig: string;
  scene: React.ReactNode;
  compactOnMobile?: boolean;
}) {
  const sectionRef = useRef<HTMLElement>(null);
  const counterRefs = useRef<Array<HTMLSpanElement | null>>([]);
  const counterSignature = counters
    .map((counter) => `${counter.label}:${counter.value ?? "unknown"}`)
    .join("|");

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        counters.forEach((counter, index) => {
          const element = counterRefs.current[index];
          if (!element) return;
          if (counter.value === null) {
            element.textContent = "—";
            return;
          }
          const previous = Number(element.textContent?.replaceAll(",", ""));
          const animated = { value: Number.isFinite(previous) ? previous : 0 };
          gsap.to(animated, {
            value: counter.value,
            duration: 1.8,
            delay: 0.4 + index * 0.2,
            ease: "power2.out",
            onUpdate: () => {
              element.textContent = formatNum(animated.value);
            },
          });
        });
      });
      mm.add("(prefers-reduced-motion: reduce)", () => {
        counters.forEach((counter, index) => {
          const element = counterRefs.current[index];
          if (element) {
            element.textContent = counter.value === null ? "—" : formatNum(counter.value);
          }
        });
      });
      return () => mm.revert();
    },
    { scope: sectionRef, dependencies: [counterSignature], revertOnUpdate: true }
  );

  return (
    <section ref={sectionRef} className="relative overflow-hidden border-b border-line">
      <div className={cn("mx-auto grid max-w-site lg:grid-cols-2", compactOnMobile ? "lg:min-h-[52vh]" : "min-h-[52vh]")}>
        {/* 左：文案 + 计数 */}
        <div className={cn("flex flex-col justify-center px-5 md:px-8", compactOnMobile ? "py-6 lg:py-14" : "py-14")}>
          <p data-hero-title className="enter-rise font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">{index}</span>
            <span className="mx-2">/</span>
            {en}
          </p>
          <h1
            data-hero-title
            className={cn("enter-rise font-display font-bold tracking-tight md:text-7xl", compactOnMobile ? "mt-3 text-4xl lg:mt-4" : "mt-4 text-6xl")}
            style={{ animationDelay: "0.1s" }}
          >
            {title}
          </h1>
          <div
            data-hero-line
            className={cn("enter-grow-x h-px w-24 bg-accent", compactOnMobile ? "mt-3 lg:mt-6" : "mt-6")}
            style={{ animationDelay: "0.6s", animationDuration: "0.6s" }}
          />
          <p
            data-hero-title
            className={cn("enter-rise max-w-md text-sm leading-7 text-ink/70", compactOnMobile ? "mt-3 lg:mt-5" : "mt-5")}
            style={{ animationDelay: "0.2s" }}
          >
            {slogan}
          </p>
          <div
            data-hero-title
            className={cn("enter-rise flex flex-wrap gap-x-10 gap-y-4", compactOnMobile ? "mt-5 lg:mt-8" : "mt-8")}
            style={{ animationDelay: "0.3s" }}
          >
            {counters.map((c, i) => (
              <div key={c.label} aria-busy={c.busy ?? false}>
                <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">{c.label}</p>
                <p className="mt-1 font-display text-3xl font-bold tabular-nums">
                  <span ref={(el) => { counterRefs.current[i] = el; }} aria-hidden="true">
                    {c.value === null ? "—" : formatNum(c.value)}
                  </span>
                  <span className="sr-only" aria-live="polite">
                    {c.value === null ? (c.busy ? "加载中" : "暂不可用") : formatNum(c.value)}
                  </span>
                </p>
              </div>
            ))}
          </div>
        </div>

        {/* 右：图纸画板 + 场景 */}
        <div className={cn("bg-blueprint relative items-center justify-center border-t border-line p-10 lg:border-l lg:border-t-0", compactOnMobile ? "hidden lg:flex" : "flex")}>
          <span aria-hidden className="absolute left-4 top-4 font-mono text-[10px] tracking-[0.3em] text-ink/40">
            {fig}
          </span>
          <span aria-hidden className="absolute bottom-4 right-4 font-mono text-accent">+</span>
          <div
            className="enter-fade w-full max-w-sm"
            style={{ animationDelay: "0.9s", animationDuration: "0.7s" }}
          >
            {scene}
          </div>
        </div>
      </div>
    </section>
  );
}
