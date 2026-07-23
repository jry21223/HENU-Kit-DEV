"use client";

import { useEffect, useRef, useState } from "react";
import { USER_STATS, ABILITY_SERIES } from "@/lib/practice/mock";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import LineChart from "@/components/practice/charts/line-chart";
import RadarChart from "@/components/practice/charts/radar-chart";
import Heatmap from "@/components/practice/charts/heatmap";
import { gsap, FINE_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

const RANGES = [
  { key: "d30", label: "30 天" },
  { key: "d180", label: "180 天" },
  { key: "d520", label: "520 天" },
] as const;

type Range = (typeof RANGES)[number]["key"];

const CARDS = [
  { label: "总刷题", value: String(USER_STATS.totalQuestions), unit: "题" },
  { label: "正确率", value: `${USER_STATS.accuracy}`, unit: "%" },
  { label: "连续打卡", value: String(USER_STATS.streakDays), unit: "天" },
  { label: "击败用户", value: `${USER_STATS.beatPercent}`, unit: "%" },
];

export default function StatsPage() {
  usePageEnter(null);
  const [range, setRange] = useState<Range>("d30");
  const lineWrapRef = useRef<HTMLDivElement>(null);

  // 页签切换：折线重绘过渡
  useEffect(() => {
    const mm = gsap.matchMedia();
    mm.add(FINE_MOTION, () => {
      gsap.from(lineWrapRef.current?.querySelectorAll("[data-line-path]") ?? [], {
        autoAlpha: 0,
        x: -16,
        duration: 0.45,
        ease: "power2.out",
        clearProps: "all",
      });
    });
    return () => mm.revert();
  }, [range]);

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <div data-block data-enter>
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">STATS</span>
          <span className="mx-2">/</span>
          MY DATA
        </p>
        <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">
          数据面板
        </h1>
      </div>

      {/* 统计卡 */}
      <div data-block data-enter className="mt-10 grid grid-cols-2 gap-4 md:grid-cols-4">
        {CARDS.map((c, i) => (
          <div key={c.label} className="border border-ink/25 p-5">
            <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
              {String(i + 1).padStart(2, "0")} / {c.label}
            </p>
            <p className="mt-3 font-display text-4xl font-bold">
              {c.value}
              <span className="ml-1 font-mono text-xs font-normal text-ink/50">{c.unit}</span>
            </p>
          </div>
        ))}
      </div>

      {/* 能力分折线 */}
      <section data-block data-enter className="mt-12 border border-ink/25 p-5 md:p-7">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
            ABILITY / 能力分走势
          </p>
          <div className="flex gap-2">
            {RANGES.map((r) => (
              <button
                key={r.key}
                type="button"
                onClick={() => setRange(r.key)}
                className={cn(
                  "border px-3 py-1.5 font-mono text-xs transition-colors",
                  range === r.key
                    ? "border-ink bg-ink text-paper"
                    : "border-line text-ink/60 hover:border-ink/40"
                )}
              >
                {r.label}
              </button>
            ))}
          </div>
        </div>
        <div ref={lineWrapRef} className="mt-6">
          <LineChart series={ABILITY_SERIES[range]} />
        </div>
      </section>

      <div className="mt-12 grid gap-12 lg:grid-cols-2">
        {/* 雷达图 */}
        <section data-block data-enter className="border border-ink/25 p-5 md:p-7">
          <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
            RADAR / 科目能力（软件工程）
          </p>
          <div className="mt-6">
            <RadarChart />
          </div>
        </section>

        {/* 掌握度 + 薄弱点 */}
        <section data-block data-enter className="flex flex-col gap-10">
          <div className="border border-ink/25 p-5 md:p-7">
            <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
              MASTERY / 知识点掌握度
            </p>
            <div className="mt-5 space-y-4">
              {USER_STATS.mastery.map((m) => (
                <div key={m.label}>
                  <div className="mb-1.5 flex justify-between font-mono text-xs">
                    <span>{m.label}</span>
                    <span className={cn(m.value < 60 ? "text-accent" : "text-ink/60")}>
                      {m.value}%{m.value < 60 ? " / 薄弱" : ""}
                    </span>
                  </div>
                  <div className="h-1.5 w-full bg-ink/10">
                    <div
                      data-mastery-bar
                      className={cn("h-full", m.value < 60 ? "bg-accent" : "bg-ink/70")}
                      style={{ width: `${m.value}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className="border border-ink/25 p-5 md:p-7">
            <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
              WEAK TOP5 / 薄弱点
            </p>
            <ul className="mt-4">
              {USER_STATS.weakTop5.map((w, i) => (
                <li key={w.topic} className="flex items-baseline gap-4 border-b border-line py-2.5 last:border-b-0">
                  <span className="font-mono text-xs text-accent">{String(i + 1).padStart(2, "0")}</span>
                  <span className="flex-1 text-sm">{w.topic}</span>
                  <span className="font-mono text-[10px] text-ink/50">{w.subject}</span>
                  <span className="font-mono text-xs text-ink/60">错 {w.wrong}</span>
                </li>
              ))}
            </ul>
          </div>
        </section>
      </div>

      {/* 热力图 */}
      <section data-block data-enter className="mt-12 border border-ink/25 p-5 md:p-7">
        <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
          HEATMAP / 近 26 周刷题热力
        </p>
        <div className="mt-6">
          <Heatmap />
        </div>
      </section>
    </main>
  );
}
