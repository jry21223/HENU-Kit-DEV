"use client";

import { useEffect, useRef, useState } from "react";
import { LEADERBOARD } from "@/lib/practice/mock";
import { hasGateway, fetchLeaderboard } from "@/lib/api/client";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import { gsap, FINE_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

const PERIODS = [
  { key: "week", label: "本周" },
  { key: "month", label: "本月" },
  { key: "all", label: "总榜" },
] as const;

type Period = (typeof PERIODS)[number]["key"];

export default function LeaderboardPage() {
  usePageEnter(null);
  const [period, setPeriod] = useState<Period>("week");
  const [rows, setRows] = useState(LEADERBOARD.week);
  const listRef = useRef<HTMLUListElement>(null);
  const barsRef = useRef<HTMLDivElement>(null);

  // Fetch from gateway when period changes; fall back to mock
  useEffect(() => {
    if (!hasGateway) {
      setRows(LEADERBOARD[period]);
      return;
    }
    let cancelled = false;
    fetchLeaderboard(period).then((resp) => {
      if (!cancelled) {
        setRows(resp?.rows ? (resp.rows as typeof LEADERBOARD.week) : LEADERBOARD[period]);
      }
    });
    return () => { cancelled = true; };
  }, [period]);

  // 页签切换时行入场 stagger + 分布柱生长
  useEffect(() => {
    const mm = gsap.matchMedia();
    mm.add(FINE_MOTION, () => {
      gsap.from(listRef.current?.children ?? [], {
        x: -40,
        autoAlpha: 0,
        duration: 0.5,
        ease: "power3.out",
        stagger: 0.06,
        clearProps: "all",
      });
      gsap.from(barsRef.current?.querySelectorAll("[data-dist-bar]") ?? [], {
        scaleX: 0,
        transformOrigin: "left center",
        duration: 0.7,
        ease: "power3.out",
        stagger: 0.05,
        delay: 0.15,
        clearProps: "all",
      });
    });
    return () => mm.revert();
  }, [period]);

  const maxQ = Math.max(...rows.map((r) => r.questions));

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <div data-block data-enter className="flex flex-wrap items-end justify-between gap-6">
        <div>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">RANK</span>
            <span className="mx-2">/</span>
            LEADERBOARD
          </p>
          <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">
            排行榜
          </h1>
        </div>
        <div className="flex gap-2">
          {PERIODS.map((p) => (
            <button
              key={p.key}
              type="button"
              onClick={() => setPeriod(p.key)}
              className={cn(
                "border px-4 py-2 font-mono text-xs tracking-widest transition-colors",
                period === p.key
                  ? "border-ink bg-ink text-paper"
                  : "border-line text-ink/60 hover:border-ink/40 hover:text-ink"
              )}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      <div data-block className="mt-10">
        <div className="grid grid-cols-[3.5rem_1fr_4.5rem_4rem_3.5rem] items-center gap-3 border-b border-ink/40 pb-2 font-mono text-[10px] tracking-[0.25em] text-ink/40 md:grid-cols-[5rem_1fr_6rem_5rem_5rem]">
          <span>排名</span>
          <span>用户</span>
          <span>刷题数</span>
          <span>正确率</span>
          <span className="text-right">连续</span>
        </div>
        <ul ref={listRef}>
          {rows.map((row, i) => (
            <li
              key={row.name}
              className={cn(
                "grid grid-cols-[3.5rem_1fr_4.5rem_4rem_3.5rem] items-center gap-3 border-b border-line py-4 md:grid-cols-[5rem_1fr_6rem_5rem_5rem]",
                row.isYou && "border border-accent px-3 -mx-3"
              )}
            >
              <span
                className={cn(
                  "font-display text-3xl font-bold md:text-4xl",
                  i < 3 ? "text-accent" : "text-ink/25"
                )}
              >
                {String(i + 1).padStart(2, "0")}
              </span>
              <span className="truncate text-sm md:text-base">
                {row.name}
                {row.isYou && (
                  <span className="ml-2 border border-accent px-1.5 py-0.5 font-mono text-[10px] text-accent">
                    你
                  </span>
                )}
              </span>
              <span className="font-mono text-sm">{row.questions}</span>
              <span className="font-mono text-sm text-ink/70">{row.accuracy}%</span>
              <span className="text-right font-mono text-sm text-ink/70">
                {row.streak}d
              </span>
            </li>
          ))}
        </ul>
        <p className="mt-6 font-mono text-[10px] tracking-[0.3em] text-ink/40">
          每日 04:00 结算 / 数据为 v1 预览示例
        </p>
      </div>

      {/* 刷题数分布条形图 */}
      <div data-block className="mt-14">
        <p className="font-mono text-[10px] tracking-[0.3em] text-ink/40">
          DIST / 刷题数分布
        </p>
        <div ref={barsRef} className="mt-5 space-y-2.5">
          {rows.map((row, i) => (
            <div
              key={row.name}
              className="grid grid-cols-[5.5rem_1fr_3.5rem] items-center gap-3 md:grid-cols-[8rem_1fr_4rem]"
            >
              <span className="truncate font-mono text-xs text-ink/70">
                {String(i + 1).padStart(2, "0")} {row.name}
              </span>
              <div className="h-4 w-full border border-line bg-paper">
                <div
                  data-dist-bar
                  className={cn(
                    "h-full",
                    row.isYou ? "bg-accent" : i < 3 ? "bg-ink" : "bg-ink/35"
                  )}
                  style={{ width: `${Math.round((row.questions / maxQ) * 100)}%` }}
                />
              </div>
              <span className="text-right font-mono text-xs text-ink/70">
                {row.questions}
              </span>
            </div>
          ))}
        </div>
      </div>
    </main>
  );
}
