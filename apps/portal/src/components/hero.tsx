"use client";

import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import dynamic from "next/dynamic";
import { gsap, useGSAP, REDUCED_MOTION } from "@/lib/gsap";
import Marquee from "@/components/marquee";
import AmbientSvg from "@/components/ui/ambient-svg";

const Hero3D = dynamic(() => import("@/components/hero-3d"), { ssr: false });

const MARQUEE_ITEMS = [
  "往年试卷",
  "AI 刷题",
  "美食榜",
  "互助接单",
  "学长笔记",
  "KEEP IN TOUCH",
];

/**
 * 是否是够宽的屏幕（SSR 时按 false 处理，即先不上 3D）。
 *
 * 3D 场景在手机上是绝对定位铺满整个 hero 的，标题和正文压在它上面，密集的
 * 线框把字彻底盖住；桌面端因为只占右侧 55% 才没暴露。加上它整套 three.js /
 * fiber / drei 恰恰跑在最扛不住的设备上，所以窄屏直接不渲染，而不是靠 CSS
 * 隐藏——那样画布照样会创建并逐帧渲染。
 */
const WIDE_VIEWPORT = "(min-width: 768px)";

function useWideViewport() {
  return useSyncExternalStore(
    (onChange) => {
      const mq = window.matchMedia(WIDE_VIEWPORT);
      mq.addEventListener("change", onChange);
      return () => mq.removeEventListener("change", onChange);
    },
    () => window.matchMedia(WIDE_VIEWPORT).matches,
    () => false
  );
}

/** 订阅 prefers-reduced-motion（SSR 时按 false 处理） */
function useReducedMotion() {
  return useSyncExternalStore(
    (onChange) => {
      const mq = window.matchMedia(REDUCED_MOTION);
      mq.addEventListener("change", onChange);
      return () => mq.removeEventListener("change", onChange);
    },
    () => window.matchMedia(REDUCED_MOTION).matches,
    () => false
  );
}

/** reduced-motion 时替代 3D 场景的静态 SVG 图纸网格 */
function StaticBlueprint() {
  return (
    <svg
      viewBox="0 0 400 400"
      aria-hidden
      className="h-full w-full text-ink/40"
      fill="none"
      stroke="currentColor"
    >
      <circle cx="200" cy="200" r="120" strokeDasharray="4 6" />
      <circle cx="200" cy="200" r="70" />
      <rect x="130" y="130" width="140" height="140" strokeDasharray="4 6" />
      <path d="M200 40v320M40 200h320" strokeDasharray="2 6" />
      <circle cx="200" cy="80" r="6" className="fill-accent stroke-none" />
      <circle cx="320" cy="200" r="6" className="fill-accent stroke-none" />
      <circle cx="140" cy="290" r="6" className="fill-accent stroke-none" />
    </svg>
  );
}

export default function Hero() {
  const sectionRef = useRef<HTMLElement>(null);
  const sceneRef = useRef<HTMLDivElement>(null);
  const reduced = useReducedMotion();
  const wide = useWideViewport();
  // Hero 有任何一部分在视窗内才驱动 3D 渲染循环
  const [inView, setInView] = useState(true);

  useEffect(() => {
    const el = sectionRef.current;
    if (!el) return;
    const io = new IntersectionObserver(
      ([entry]) => setInView(entry.isIntersecting),
      { threshold: 0 }
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  useGSAP(
    () => {
      const mm = gsap.matchMedia();

      mm.add("(prefers-reduced-motion: no-preference)", () => {
        // 引入动画：标题逐行 3D 翻转 → 网格线生长 → 橙色擦除 → 3D 淡入 → marquee 进入
        const tl = gsap.timeline({ defaults: { ease: "power3.out" } });
        tl.from("[data-hero-line]", {
          rotateX: -90,
          yPercent: 60,
          opacity: 0,
          transformOrigin: "50% 100%",
          duration: 0.9,
          stagger: 0.14,
        })
          .from(
            "[data-hero-gridline]",
            { scaleX: 0, transformOrigin: "left center", duration: 0.7, stagger: 0.06 },
            "-=0.5"
          )
          .fromTo(
            "[data-hero-wipe]",
            { scaleX: 1 },
            { scaleX: 0, transformOrigin: "right center", duration: 0.6, ease: "power2.inOut" },
            "-=0.6"
          )
          .from(sceneRef.current, { opacity: 0, duration: 0.8 }, "-=0.3")
          .from("[data-hero-marquee]", { yPercent: 100, duration: 0.5 }, "-=0.4");
      });

      // 旋转的 ®（所有动效偏好下都允许，纯装饰且极慢）
      gsap.to("[data-hero-reg]", {
        rotate: 360,
        duration: 20,
        repeat: -1,
        ease: "none",
      });

      return () => mm.revert();
    },
    { scope: sectionRef }
  );

  return (
    <section
      ref={sectionRef}
      className="snap-screen relative flex h-svh flex-col overflow-hidden bg-paper"
    >
      {/* 图纸网格背景 */}
      <div aria-hidden className="bg-blueprint absolute inset-0 opacity-70" />

      {/* 环境装饰：旋转刻度盘 + 呼吸十字 */}
      <AmbientSvg variant="dial" className="absolute bottom-[16%] left-[4%] hidden text-ink/25 md:block" />
      <AmbientSvg variant="crosshair" className="absolute top-[22%] left-[42%] hidden text-ink/30 md:block" />

      {/* 十字对位标记 */}
      <span aria-hidden className="absolute left-6 top-24 font-mono text-lg text-ink/40">+</span>
      <span aria-hidden className="absolute right-6 top-24 font-mono text-lg text-ink/40">+</span>
      <span aria-hidden className="absolute bottom-24 left-6 font-mono text-lg text-ink/40">+</span>
      <span aria-hidden className="absolute bottom-24 right-6 font-mono text-lg text-accent">+</span>

      {/* 3D 场景 / 静态替代：常驻 Hero，不做滚动淡出 */}
      <div
        ref={sceneRef}
        className="pointer-events-none absolute inset-y-0 right-0 hidden w-full md:block md:w-[55%]"
      >
        {reduced || !wide ? (
          <div className="h-full w-full p-16 opacity-60">
            <StaticBlueprint />
          </div>
        ) : (
          <Hero3D active={inView} />
        )}
      </div>

      {/* 左侧文案 */}
      <div className="relative z-10 mx-auto flex w-full max-w-7xl flex-1 flex-col justify-center px-5 pt-28 pb-16 md:px-10">
        <div data-hero-gridline className="mb-8 h-px w-24 bg-accent" />

        <p className="mb-4 flex items-center gap-2 font-mono text-xs tracking-[0.35em] text-ink/60">
          HENU — STUDENT PLATFORM
          <span
            data-hero-reg
            className="inline-flex h-8 w-8 items-center justify-center rounded-full border border-ink/30 text-[10px] text-ink/60"
          >
            ®
          </span>
        </p>

        <div className="relative" style={{ perspective: "900px" }}>
          <h1 className="font-display text-[clamp(3.5rem,12vw,9rem)] leading-[0.95] font-bold tracking-tight">
            <span data-hero-line className="block">HENUKIT</span>
            <span data-hero-line className="mt-2 block text-[clamp(1.6rem,4.5vw,3.2rem)] font-medium">
              保持联系，保持热爱
            </span>
          </h1>
          {/* 橙色块擦除 */}
          <span
            data-hero-wipe
            aria-hidden
            className="absolute inset-0 bg-accent"
            style={{ transform: "scaleX(0)" }}
          />
        </div>

        <p data-hero-line className="mt-8 max-w-md text-sm leading-7 text-ink/70 md:text-base">
          henukit 是面向校园的综合性学生平台：资料库、AI 智能刷题、
          美食排行榜、校园互助——一份图纸，四个模块，一个站点全部搞定。
        </p>

        <div data-hero-gridline className="mt-10 h-px w-full max-w-md bg-line" />
        <p className="mt-3 font-mono text-[10px] tracking-[0.3em] text-ink/40">
          SCROLL / 向下滚动查看模块 01—04
        </p>
      </div>

      {/* 底部橙色 marquee */}
      <div data-hero-marquee className="relative z-10">
        <Marquee items={MARQUEE_ITEMS} />
      </div>
    </section>
  );
}
