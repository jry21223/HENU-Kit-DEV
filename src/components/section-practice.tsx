"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import SectionHeading from "@/components/ui/section-heading";
import MagneticButton from "@/components/ui/magnetic-button";
import AmbientSvg from "@/components/ui/ambient-svg";

const TYPE_TEXT =
  "【题目】求极限 lim(x→0) sin x / x。\n【AI 讲解】这是经典的 0/0 型极限。由重要极限公式直接得 1；也可用洛必达法则，分子分母分别求导得 cos x → 1。\n【易错点】注意 x 需以弧度计，且该公式只在 x→0 时成立。";

const BARS = [
  { label: "高等数学", value: 82, weak: false },
  { label: "线性代数", value: 45, weak: true },
  { label: "概率论", value: 71, weak: false },
];

const FEATURES = ["按知识点智能推题", "错题自动归因讲解", "掌握度曲线每周更新"];

export default function SectionPractice() {
  const sectionRef = useRef<HTMLElement>(null);
  const textRef = useRef<HTMLParagraphElement>(null);

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
    { scope: sectionRef }
  );

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

          {/* 掌握度进度条 */}
          <div className="mt-12 space-y-5">
            <p className="font-mono text-[10px] tracking-[0.3em] text-paper/40">
              MASTERY / 知识点掌握度
            </p>
            {BARS.map((bar) => (
              <div key={bar.label}>
                <div className="mb-1.5 flex justify-between font-mono text-xs">
                  <span>{bar.label}</span>
                  <span className={bar.weak ? "text-accent" : "text-paper/60"}>
                    {bar.value}%{bar.weak ? " / 薄弱" : ""}
                  </span>
                </div>
                <div className="h-1.5 w-full bg-paper/10">
                  <div
                    data-bar
                    className={bar.weak ? "h-full bg-accent" : "h-full bg-paper/70"}
                    style={{ width: `${bar.value}%` }}
                  />
                </div>
              </div>
            ))}
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
              生成耗时 0.8s / 已关联 3 道同类题
            </p>
          </div>
        </div>
      </div>
    </section>
  );
}
