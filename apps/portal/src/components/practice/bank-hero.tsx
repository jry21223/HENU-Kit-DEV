"use client";

import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import dynamic from "next/dynamic";
import { gsap, useGSAP, FINE_MOTION, REDUCED_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";
import {
  deriveMasteryVisuals,
  EMPTY_MASTERY,
  masteryPercent,
  type MasterySnapshot,
} from "@/components/practice/bank-hero-mastery";

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

/** Ellipse circumference approx for stroke-dash progress (rx, ry). */
function ellipsePerim(rx: number, ry: number) {
  return Math.PI * (3 * (rx + ry) - Math.sqrt((3 * rx + ry) * (rx + 3 * ry)));
}

const SPARK_PATH = "M0 34 L28 30 L56 33 L84 22 L112 26 L140 14 L168 18 L196 6";
const SPARK_LEN = 240;

function formatNum(n: number) {
  return String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/** reduced-motion：晶体轮廓 + 进度弧（与 3D 同一编码） */
function StaticKnowledgeMesh({ mastery }: { mastery: MasterySnapshot }) {
  const {
    ringSubjects: rings,
    coverage,
    cubeCount,
    orbitRadius,
  } = deriveMasteryVisuals(mastery);
  const acc = masteryPercent(mastery.accuracy);
  const staticOrbitRadius = 56 + ((orbitRadius - 2.2) / 0.35) * 12;
  return (
    <svg
      viewBox="0 0 200 240"
      aria-hidden
      className="h-full w-full text-ink/40"
      fill="none"
      stroke="currentColor"
    >
      <polygon
        points="100,28 168,78 148,168 52,168 32,78"
        strokeWidth="1.2"
        opacity={0.2 + coverage * 0.55}
      />
      <polygon
        points="100,58 140,88 128,142 72,142 60,88"
        strokeWidth="1"
        opacity={0.12 + coverage * 0.4}
      />
      <circle
        cx="100"
        cy="110"
        r={5 + acc * 12}
        className="stroke-accent"
        strokeWidth="1.5"
        opacity={0.45 + acc * 0.5}
      />
      {rings.map((s, i) => {
        const t = masteryPercent(s.value);
        const weak = s.value < 60;
        const rx = 48 + i * 16;
        const ry = 18 + i * 6;
        const perim = ellipsePerim(rx, ry);
        const filled = Math.max(perim * 0.04, perim * t);
        return (
          <g key={s.label}>
            <ellipse
              cx="100"
              cy="118"
              rx={rx}
              ry={ry}
              strokeWidth="0.8"
              opacity={0.14}
            />
            <ellipse
              cx="100"
              cy="118"
              rx={rx}
              ry={ry}
              strokeWidth={1.4 + t * 1.2}
              opacity={0.35 + t * 0.55}
              className={weak ? "stroke-accent" : undefined}
              strokeDasharray={`${filled} ${perim}`}
              strokeLinecap="butt"
              transform="rotate(-90 100 118)"
            />
            <path
              d={`M${100 + rx - 1} 118 h6`}
              strokeWidth="1.2"
              className={weak ? "stroke-accent" : undefined}
              opacity={0.55}
            />
          </g>
        );
      })}
      {Array.from({ length: cubeCount }, (_, index) => {
        const angle = (Math.PI * 2 * index) / cubeCount;
        const x = 100 + Math.cos(angle) * staticOrbitRadius;
        const y = 110 + Math.sin(angle) * staticOrbitRadius * 0.42;
        return (
          <rect
            key={index}
            x={x - 2.5}
            y={y - 2.5}
            width="5"
            height="5"
            className="stroke-accent"
            opacity="0.55"
          />
        );
      })}
      <path d="M100 18 v-12 M94 12 h12" className="stroke-accent" />
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
  const mastery = EMPTY_MASTERY;
  const { ringSubjects, cubeCount } = useMemo(
    () => deriveMasteryVisuals(mastery),
    [mastery]
  );

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

  return (
    <section
      ref={sectionRef}
      data-block
      className="relative flex min-h-[68vh] flex-col overflow-hidden"
    >
      <div className="mx-auto grid w-full max-w-[1440px] flex-1 lg:grid-cols-2">
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

        {/* 右：掌握度驱动的知识体 3D */}
        <div className="bg-blueprint relative min-h-72 border-t border-line lg:border-l lg:border-t-0">
          <span
            aria-hidden
            className="absolute left-4 top-4 z-10 max-w-[70%] font-mono text-[10px] tracking-[0.25em] text-ink/40"
          >
            FIG.01 知识点结构 / KNOWLEDGE MESH
          </span>
          <span
            aria-hidden
            className="absolute right-4 top-4 z-10 font-mono text-accent"
          >
            +
          </span>

          <div className="pointer-events-none absolute bottom-4 left-4 right-4 z-20">
            <ul className="border border-line bg-paper/85 px-2.5 py-1.5 backdrop-blur-sm">
              <li className="py-1 font-mono text-[10px] tracking-wide text-ink/50">
                掌握度数据尚未接入
              </li>
              {ringSubjects.map((s, i) => {
                const weak = s.value < 60;
                const t = Math.min(100, Math.max(0, s.value));
                return (
                  <li
                    key={s.label}
                    className="flex items-center gap-2 py-0.5 font-mono text-[10px] tracking-wide"
                  >
                    <span className="w-5 shrink-0 text-ink/35">R{i + 1}</span>
                    <span className="w-16 shrink-0 truncate text-ink/50">
                      {s.label}
                    </span>
                    <span
                      aria-hidden
                      className="relative h-[3px] min-w-0 flex-1 bg-ink/10"
                    >
                      <span
                        className={cn(
                          "absolute inset-y-0 left-0",
                          weak ? "bg-accent" : "bg-ink/55"
                        )}
                        style={{ width: `${t}%` }}
                      />
                    </span>
                    <span
                      className={cn(
                        "w-9 shrink-0 text-right tabular-nums",
                        weak ? "text-accent" : "text-ink/65"
                      )}
                    >
                      {t}%
                    </span>
                  </li>
                );
              })}
              {ringSubjects.length > 0 ? (
                <li className="mt-1 flex justify-between border-t border-line pt-1 font-mono text-[10px] text-ink/40">
                  <span>
                    核 {mastery.accuracy}% · 连打 {mastery.streakDays}d · 块{" "}
                    {cubeCount}
                  </span>
                  <span>{mastery.totalQuestions} 题</span>
                </li>
              ) : null}
            </ul>
          </div>

          {reduced ? (
            <div className="absolute inset-0 p-14 opacity-70">
              <StaticKnowledgeMesh mastery={mastery} />
            </div>
          ) : (
            <BankHero3D active={active} mastery={mastery} />
          )}
        </div>
      </div>

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
