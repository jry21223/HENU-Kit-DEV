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
  /** 画板左上角的图注：拉丁编号与英文名拉开字距，中文名不拉开（DESIGN_SYSTEM.md 第 4 节）。 */
  fig: { code: string; name: string; en: string };
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
      {/* 移动端紧凑页头保留开放式图纸网格底纹，不另加边框小卡，与桌面右栏画板保持同一建筑语言。 */}
      {compactOnMobile ? (
        <div aria-hidden className="bg-blueprint pointer-events-none absolute inset-0 opacity-60 lg:hidden" />
      ) : null}
      <div className={cn("relative mx-auto grid max-w-site lg:grid-cols-2", compactOnMobile ? "lg:min-h-[52vh]" : "min-h-[52vh]")}>
        {/* 左：文案 + 计数；移动端在标题右侧留白区铺开无边框工程线稿 */}
        <div className={cn("flex flex-col justify-center px-5 md:px-8", compactOnMobile ? "py-5 lg:py-14" : "py-14")}>
          {/* 移动端线稿绝对定位在标题行右上角，不占文档流高度：首屏的搜索和第一条内容不被往下推。 */}
          <div className="relative">
            <div className="min-w-0">
              <p data-hero-title className="enter-rise font-mono text-xs tracking-[0.3em] text-ink/60">
                <span className="text-accent-text">{index}</span>
                <span className="mx-2">/</span>
                {en}
              </p>
              <h1
                data-hero-title
                className={cn("enter-rise font-display font-bold tracking-tight md:text-7xl", compactOnMobile ? "mt-2.5 text-4xl lg:mt-4" : "mt-4 text-6xl")}
                style={{ animationDelay: "0.1s" }}
              >
                {title}
              </h1>
              <div
                data-hero-line
                className={cn("enter-grow-x h-px w-24 bg-accent", compactOnMobile ? "mt-3 lg:mt-6" : "mt-6")}
                style={{ animationDelay: "0.4s", animationDuration: "0.5s" }}
              />
            </div>

            {compactOnMobile ? (
              <div
                aria-hidden
                className="pointer-events-none absolute right-0 top-0 flex w-24 flex-col items-end sm:w-32 lg:hidden"
              >
                <span className="font-mono text-[10px] tracking-[0.25em] text-ink/45">
                  {fig.code}
                </span>
                <div className="mt-0.5 w-full opacity-80">{scene}</div>
                <span className="font-mono text-[10px] leading-none text-accent-text">+</span>
              </div>
            ) : null}
          </div>

          <p
            data-hero-title
            className={cn("enter-rise max-w-md text-sm leading-7 text-ink/70", compactOnMobile ? "mt-3 lg:mt-5" : "mt-5")}
            style={{ animationDelay: "0.18s" }}
          >
            {slogan}
          </p>
          <div
            data-hero-title
            className={cn("enter-rise flex flex-wrap gap-x-10 gap-y-3", compactOnMobile ? "mt-4 lg:mt-8" : "mt-8")}
            style={{ animationDelay: "0.26s" }}
          >
            {counters.map((c, i) => (
              <div key={c.label} aria-busy={c.busy ?? false}>
                <p className="font-mono text-xs text-ink/60">{c.label}</p>
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
          <span aria-hidden className="absolute left-4 top-4 font-mono text-xs text-ink/40">
            <span className="tracking-[0.3em]">{fig.code}</span> {fig.name} /{" "}
            <span className="tracking-[0.3em]">{fig.en}</span>
          </span>
          <span aria-hidden className="absolute bottom-4 right-4 font-mono text-accent-text">+</span>
          <div
            className="enter-fade w-full max-w-sm"
            style={{ animationDelay: "0.25s", animationDuration: "0.6s" }}
          >
            {scene}
          </div>
        </div>
      </div>
    </section>
  );
}
