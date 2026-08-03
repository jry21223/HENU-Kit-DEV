"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import SectionHeading from "@/components/ui/section-heading";
import MagneticButton from "@/components/ui/magnetic-button";
import AmbientSvg from "@/components/ui/ambient-svg";
import { redirectToLogin } from "@/lib/api/client";
import { usePersonalPracticeStats } from "@/lib/practice/personal-stats";

const TYPE_TEXT =
  "【题目】求极限 lim(x→0) sin x / x。\n【AI 讲解】这是经典的 0/0 型极限。由重要极限公式直接得 1；也可用洛必达法则，分子分母分别求导得 cos x → 1。\n【易错点】注意 x 需以弧度计，且该公式只在 x→0 时成立。";

const FEATURES = ["按知识点智能推题", "错题自动归因讲解", "掌握度曲线每周更新"];

export default function SectionPractice() {
  const sectionRef = useRef<HTMLElement>(null);
  const textRef = useRef<HTMLParagraphElement>(null);
  const { state, retry } = usePersonalPracticeStats();

  // 进度条只在真实作答事实就绪后渲染；动画在数据到达后重建。
  const masteryBars =
    state.status === "ready" || state.status === "empty"
      ? state.data.mastery.slice(0, 3)
      : [];
  const barsReady =
    state.status === "ready" || state.status === "empty";

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        // 打字机：滚动进入时逐字输出（动画启用时先清空面板）
        if (textRef.current) textRef.current.textContent = "";
        const counter = { value: 0 };
        gsap.to(counter, {
          value: TYPE_TEXT.length,
          duration: 4,
          ease: "none",
          scrollTrigger: {
            trigger: textRef.current,
            start: "top 60%",
            toggleActions: "restart none none restart",
          },
          onUpdate() {
            if (textRef.current)
              textRef.current.textContent = TYPE_TEXT.slice(
                0,
                Math.round(counter.value)
              );
          },
        });

        // 掌握度进度条：从 0 生长。
        // 注意 trigger 用 section 而非 bar 自身——整屏模块中位于底部的元素，
        // 其 "top 60%" 触发线会在模块切换过渡途中被穿过，导致进入/离开动画观感颠倒。
        gsap.utils.toArray<HTMLElement>("[data-bar]").forEach((bar) => {
          gsap.from(bar, {
            scaleX: 0,
            transformOrigin: "left center",
            duration: 1.1,
            ease: "power3.out",
            scrollTrigger: {
              trigger: sectionRef.current,
              start: "top 60%",
              toggleActions: "play none none reverse",
            },
          });
        });

        // 面板整体淡入
        gsap.from("[data-terminal]", {
          y: 40,
          opacity: 0,
          duration: 0.9,
          ease: "power3.out",
          scrollTrigger: {
            trigger: "[data-terminal]",
            start: "top 60%",
            toggleActions: "play none none reverse",
          },
        });
      });
      return () => mm.revert();
    },
    { scope: sectionRef, dependencies: [barsReady] }
  );

  function renderMastery() {
    switch (state.status) {
      case "disabled":
        return (
          <p className="font-mono text-xs leading-5 text-paper/50">
            掌握度即将上线，敬请期待。
          </p>
        );
      case "loading":
        return (
          <div className="space-y-5" aria-busy="true">
            {[0, 1, 2].map((i) => (
              <div key={i}>
                <div className="mb-1.5 h-4 w-24 animate-pulse bg-paper/15" />
                <div className="h-1.5 w-full bg-paper/10" />
              </div>
            ))}
          </div>
        );
      case "unauthenticated":
        return (
          <div>
            <p className="font-mono text-xs leading-5 text-paper/50">
              登录后展示你的掌握度。
            </p>
            <button
              type="button"
              onClick={() => redirectToLogin("/practice")}
              className="mt-4 border border-paper/40 px-4 py-2 font-mono text-xs tracking-widest text-paper transition-colors hover:bg-paper hover:text-ink"
            >
              登录查看
            </button>
          </div>
        );
      case "error":
        return (
          <div role="alert">
            <p className="font-mono text-xs leading-5 text-paper/50">
              掌握度数据暂时不可用，请稍后重试。
            </p>
            <button
              type="button"
              onClick={retry}
              className="mt-4 border border-paper/40 px-4 py-2 font-mono text-xs tracking-widest text-paper transition-colors hover:bg-paper hover:text-ink"
            >
              重试
            </button>
          </div>
        );
      case "empty":
        return (
          <p className="font-mono text-xs leading-5 text-paper/50">
            还没有作答记录，掌握度会随刷题积累。
          </p>
        );
      case "ready":
        if (masteryBars.length === 0) {
          return (
            <p className="font-mono text-xs leading-5 text-paper/50">
              还没有题库掌握度数据，去刷题建立学习图谱。
            </p>
          );
        }
        return (
          <div className="space-y-5">
            {masteryBars.map((bar) => {
              const weak = bar.value < 60;
              return (
                <div key={bar.bank_id}>
                  <div className="mb-1.5 flex justify-between font-mono text-xs">
                    <span>{bar.label}</span>
                    <span className={weak ? "text-accent" : "text-paper/60"}>
                      {bar.value}%{weak ? " / 薄弱" : ""}
                    </span>
                  </div>
                  <div className="h-1.5 w-full bg-paper/10">
                    <div
                      data-bar
                      className={weak ? "h-full bg-accent" : "h-full bg-paper/70"}
                      style={{ width: `${bar.value}%` }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        );
    }
  }

  return (
    <section
      ref={sectionRef}
      className="snap-screen relative border-t border-line-dark bg-ink text-paper"
    >
      <div aria-hidden className="bg-blueprint-dark absolute inset-0 opacity-40" />
      <AmbientSvg
        variant="dial"
        className="absolute right-10 top-24 hidden text-paper/25 lg:block"
      />
      <div className="relative mx-auto grid min-h-svh max-w-7xl items-center gap-12 px-5 py-24 md:grid-cols-2 md:px-10">
        {/* 左侧文案 */}
        <div>
          <SectionHeading index="02" en="PRACTICE" title="AI 智能刷题" dark />
          <p className="mt-6 max-w-sm text-sm leading-7 text-paper/70">
            不是题海，是靶向训练。AI 按你的薄弱知识点推题，
            每道错题都配一段人话讲解。
          </p>
          <ul className="mt-6 space-y-2 font-mono text-xs tracking-wider text-paper/60">
            {FEATURES.map((f) => (
              <li key={f}>
                <span className="mr-2 text-accent">+</span>
                {f}
              </li>
            ))}
          </ul>
          <MagneticButton href="/practice" dark className="mt-8">
            进入模块
          </MagneticButton>

          {/* 掌握度进度条（真实作答事实聚合） */}
          <div className="mt-12 space-y-5">
            <p className="font-mono text-[10px] tracking-[0.3em] text-paper/40">
              MASTERY / 知识点掌握度
            </p>
            {renderMastery()}
          </div>
        </div>

        {/* 右侧 AI 讲解终端面板 */}
        <div data-terminal className="w-full self-center border border-line-dark bg-ink/60">
          <div className="flex items-center justify-between border-b border-line-dark px-4 py-2.5 font-mono text-[10px] tracking-[0.25em] text-paper/50">
            <span>AI-TUTOR / EXPLAIN</span>
            <span className="flex gap-1.5">
              <i className="h-2 w-2 border border-paper/40" />
              <i className="h-2 w-2 border border-paper/40" />
              <i className="h-2 w-2 bg-accent" />
            </span>
          </div>
          <div className="p-5">
            <p className="mb-3 font-mono text-[10px] text-accent">$ henukit explain --course=calculus</p>
            <p
              ref={textRef}
              className="min-h-40 whitespace-pre-line font-mono text-[13px] leading-7 text-paper/85"
            >
              {TYPE_TEXT}
            </p>
            <p className="mt-4 border-t border-line-dark pt-3 font-mono text-[10px] tracking-wider text-paper/40">
              示例讲解 · 已关联同类题
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
