"use client";

import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import dynamic from "next/dynamic";
import { gsap, useGSAP, FINE_MOTION, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";
import type { BankHeroVariant } from "@/components/practice/bank-hero-3d";

const BankHero3D = dynamic(() => import("@/components/practice/bank-hero-3d"), {
  ssr: false,
});

const COUNTERS = [
  { label: "全站总刷题", value: 128436 },
  { label: "今日答题", value: 1024 },
];

const TICKER = [
  "卷王本王 刚答对了 数据结构 · Q-07",
  "早八不迟到 完成了 高等数学A · 极限与连续",
  "考研上岸ing 连对 12 题",
  "图书馆常驻人口 刚答错了 线性代数 · Q-06",
  "代码炼丹师 收藏了 数据结构 · 树与图专题",
  "你 今天的刷题数超过了 83% 的同学",
];

const VARIANT_META: Record<
  BankHeroVariant,
  { fig: string; blurb: string }
> = {
  A: {
    fig: "FIG.01 知识点结构 / KNOWLEDGE MESH",
    blurb: "线框晶体 + 掌握度环",
  },
  B: {
    fig: "FIG.01 答题卡叠层 / ANSWER SHEETS",
    blurb: "题册剖面 + 勾选",
  },
};

// 迷你折线（固定路径，循环重绘）
const SPARK_PATH = "M0 34 L28 30 L56 33 L84 22 L112 26 L140 14 L168 18 L196 6";
const SPARK_LEN = 240;

function formatNum(n: number) {
  return String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/** reduced-motion：A 晶体轮廓 */
function StaticMeshA() {
  return (
    <svg
      viewBox="0 0 200 240"
      aria-hidden
      className="h-full w-full text-ink/40"
      fill="none"
      stroke="currentColor"
    >
      <polygon points="100,28 168,78 148,168 52,168 32,78" strokeWidth="1.2" />
      <polygon
        points="100,58 140,88 128,142 72,142 60,88"
        strokeWidth="1"
        opacity="0.55"
      />
      <circle cx="100" cy="110" r="10" className="stroke-accent" strokeWidth="1.4" />
      <ellipse
        cx="100"
        cy="118"
        rx="62"
        ry="22"
        strokeDasharray="4 6"
        opacity="0.45"
      />
      <ellipse
        cx="100"
        cy="118"
        rx="78"
        ry="30"
        strokeDasharray="2 8"
        opacity="0.3"
      />
      <path d="M100 18 v-12 M94 12 h12" className="stroke-accent" />
    </svg>
  );
}

/** reduced-motion：B 答题卡轮廓 */
function StaticMeshB() {
  return (
    <svg
      viewBox="0 0 200 240"
      aria-hidden
      className="h-full w-full text-ink/40"
      fill="none"
      stroke="currentColor"
    >
      <rect x="48" y="36" width="96" height="128" rx="2" opacity="0.35" />
      <rect x="56" y="48" width="96" height="128" rx="2" opacity="0.55" />
      <rect x="64" y="60" width="96" height="128" rx="2" strokeWidth="1.2" />
      <path d="M78 78 h68" className="stroke-accent" strokeWidth="1.2" />
      {[0, 1, 2, 3].map((i) => (
        <g key={i}>
          <rect x="78" y={98 + i * 20} width="10" height="10" />
          {i < 2 ? (
            <path
              d={`M80 ${104 + i * 20} l3 3 l6 -7`}
              className="stroke-accent"
              strokeWidth="1.4"
            />
          ) : null}
          <path d={`M96 ${103 + i * 20} h40`} opacity="0.45" />
        </g>
      ))}
      {/* pencil */}
      <g transform="translate(150 150) rotate(-35)">
        <rect x="0" y="0" width="8" height="54" />
        <path d="M0 54 L4 66 L8 54" className="stroke-accent" />
      </g>
      <path d="M100 28 v-14 M94 20 h12" className="stroke-accent" />
    </svg>
  );
}

export default function BankHero({
  query,
  onQueryChange,
}: {
  query: string;
  onQueryChange: (v: string) => void;
}) {
  const sectionRef = useRef<HTMLElement>(null);
  const counterRefs = useRef<Array<HTMLSpanElement | null>>([]);
  const tickerRef = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState(true);
  const [variant, setVariant] = useState<BankHeroVariant>("A");

  useEffect(() => {
    const el = sectionRef.current;
    if (!el) return;
    const io = new IntersectionObserver(
      ([entry]) => setActive(entry.isIntersecting),
      { threshold: 0 }
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  const reduced = useSyncExternalStore(
    (onChange) => {
      const mq = window.matchMedia(REDUCED_MOTION);
      mq.addEventListener("change", onChange);
      return () => mq.removeEventListener("change", onChange);
    },
    () => window.matchMedia(REDUCED_MOTION).matches,
    () => false
  );

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        COUNTERS.forEach((c, i) => {
          const el = counterRefs.current[i];
          if (!el) return;
          const obj = { v: 0 };
          gsap.to(obj, {
            v: c.value,
            duration: 2,
            delay: 0.3 + i * 0.2,
            ease: "power2.out",
            onUpdate: () => {
              el.textContent = formatNum(obj.v);
            },
          });
        });
        gsap.fromTo(
          "[data-spark]",
          { strokeDasharray: SPARK_LEN, strokeDashoffset: SPARK_LEN },
          {
            strokeDashoffset: -SPARK_LEN,
            duration: 3.2,
            repeat: -1,
            repeatDelay: 0.8,
            ease: "power1.inOut",
          }
        );
        gsap.to(tickerRef.current, {
          xPercent: -50,
          ease: "none",
          duration: 26,
          repeat: -1,
        });
      });
      mm.add("(prefers-reduced-motion: reduce)", () => {
        COUNTERS.forEach((c, i) => {
          const el = counterRefs.current[i];
          if (el) el.textContent = formatNum(c.value);
        });
      });
      return () => mm.revert();
    },
    { scope: sectionRef }
  );

  const meta = VARIANT_META[variant];

  return (
    <section
      ref={sectionRef}
      data-block
      className="relative flex min-h-[68vh] flex-col overflow-hidden"
    >
      <div className="mx-auto grid w-full max-w-[1440px] flex-1 lg:grid-cols-2">
        {/* 左：文案 + 搜索 + 动态数据 */}
        <div className="flex flex-col justify-center px-5 py-14 md:px-8 lg:pr-12">
          <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">01</span>
            <span className="mx-2">/</span>
            QUESTION BANK
          </p>
          <h1
            data-enter
            className="mt-4 font-display text-6xl font-bold tracking-tight md:text-7xl"
          >
            题库
          </h1>
          <p data-enter className="mt-5 max-w-md text-sm leading-7 text-ink/70">
            按学院、专业、科目逐级定位题单；真题讲解、掌握度追踪、
            排行榜——数据在流动，像工地一样热火朝天。
          </p>

          <div data-enter className="mt-8 w-full max-w-md">
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
              SEARCH / 搜索科目
            </label>
            <input
              value={query}
              onChange={(e) => onQueryChange(e.target.value)}
              placeholder="如：数据结构 / 高等数学"
              className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none transition-colors placeholder:text-ink/30 focus:border-accent"
            />
          </div>

          <div data-enter className="mt-10 flex flex-wrap items-end gap-x-10 gap-y-6">
            {COUNTERS.map((c, i) => (
              <div key={c.label}>
                <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
                  {c.label}
                </p>
                <p className="mt-1 font-display text-3xl font-bold tabular-nums">
                  <span
                    ref={(el) => {
                      counterRefs.current[i] = el;
                    }}
                  >
                    0
                  </span>
                </p>
              </div>
            ))}
            <svg
              viewBox="0 0 196 40"
              aria-hidden
              className="h-10 w-48 text-ink/60"
              fill="none"
            >
              <path d="M0 38 H196" className="stroke-line" />
              <path
                data-spark
                d={SPARK_PATH}
                className="stroke-accent"
                strokeWidth="1.5"
              />
            </svg>
          </div>
        </div>

        {/* 右：3D A/B 预览 */}
        <div className="bg-blueprint relative min-h-72 border-t border-line lg:border-l lg:border-t-0">
          <span
            aria-hidden
            className="absolute left-4 top-4 z-10 max-w-[70%] font-mono text-[10px] tracking-[0.25em] text-ink/40"
          >
            {meta.fig}
          </span>

          {/* A/B switch — preview only; remove after pick */}
          <div className="absolute right-4 top-4 z-20 flex border border-line bg-paper/90 backdrop-blur-sm">
            {(["A", "B"] as const).map((v) => (
              <button
                key={v}
                type="button"
                onClick={() => setVariant(v)}
                className={cn(
                  "px-2.5 py-1 font-mono text-[10px] tracking-widest transition-colors",
                  variant === v
                    ? "bg-ink text-paper"
                    : "text-ink/50 hover:text-ink"
                )}
                aria-pressed={variant === v}
                title={VARIANT_META[v].blurb}
              >
                {v}
              </button>
            ))}
          </div>

          <span
            aria-hidden
            className="absolute bottom-4 left-4 z-10 font-mono text-[10px] tracking-wider text-ink/35"
          >
            PREVIEW · {meta.blurb}
          </span>
          <span
            aria-hidden
            className="absolute bottom-4 right-4 z-10 font-mono text-accent"
          >
            +
          </span>

          {reduced ? (
            <div className="absolute inset-0 p-14 opacity-70">
              {variant === "A" ? <StaticMeshA /> : <StaticMeshB />}
            </div>
          ) : (
            <BankHero3D active={active} variant={variant} />
          )}
        </div>
      </div>

      {/* 底部滚动 ticker */}
      <div className="relative border-t border-line py-2.5">
        <div className="overflow-hidden">
          <div ref={tickerRef} className="flex w-max will-change-transform">
            {[0, 1].map((dup) => (
              <div
                key={dup}
                className="flex shrink-0 items-center"
                aria-hidden={dup === 1}
              >
                {TICKER.map((t, i) => (
                  <span
                    key={`${dup}-${i}`}
                    className="flex items-center whitespace-nowrap"
                  >
                    <span className="px-5 font-mono text-[11px] tracking-wider text-ink/60">
                      {t}
                    </span>
                    <span aria-hidden className="text-[9px] text-accent">
                      +
                    </span>
                  </span>
                ))}
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
