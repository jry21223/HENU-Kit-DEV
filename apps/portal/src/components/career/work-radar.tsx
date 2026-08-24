"use client";

import { useId, useMemo, useRef } from "react";

import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import { careerSearchStatusLabel } from "@/lib/career/career-scan-state";
import type { CareerSearchStatus } from "@/lib/api/types";
import { cn } from "@/lib/cn";

/** 表盘状态 = 服务端任务状态，外加一个「还没有任务」的待机态。 */
export type WorkRadarStatus = "idle" | CareerSearchStatus;

type WorkRadarProps = {
  /** 真实任务状态；schematic 模式下忽略。 */
  status?: WorkRadarStatus;
  /**
   * 示意模式：首页 05 区块与未登录介绍页的装饰用法。表盘持续扫动、表头标
   * SCHEMATIC，并对辅助技术隐藏——装饰图不该被读成一次真实扫描。
   */
  schematic?: boolean;
  /**
   * 服务端确认的推荐岗位数，决定点亮几个目标点。未知传 null（例如任务仍在
   * 进行时后端只给 stage、不给计数），此时一个都不点亮，不按进度估算。
   */
  matched?: number | null;
  compact?: boolean;
  className?: string;
};

const CENTER = 280;
const DIAL_RADIUS = 250;

/**
 * 扫描楔形：从正北 0° 张开到 SWEEP_LEAD_DEG。顺时针旋转时后者是前缘，
 * 目标点的「被扫到」时刻按前缘扫过它的角度算。楔形路径与前缘角必须同源，
 * 否则改了路径就会让所有命中呼吸悄悄失步。
 */
const SWEEP_LEAD_DEG = 55;

const TARGETS = [
  { x: 394, y: 170 },
  { x: 430, y: 224 },
  { x: 350, y: 246 },
  { x: 418, y: 348 },
  { x: 360, y: 398 },
  { x: 210, y: 382 },
  { x: 152, y: 334 },
  { x: 198, y: 176 },
];

const RINGS = [58, 108, 158, 208, 250];
const ANGLES = Array.from({ length: 12 }, (_, index) => index * 30);
const TICKS = Array.from({ length: 72 }, (_, index) => index * 5);

/** 一次命中呼吸的时长；光束停转时的呼吸周期。 */
const PING_SECONDS = 0.9;
const IDLE_BREATH_SECONDS = 2.4;

const STATUS_COPY: Record<WorkRadarStatus, string> = {
  idle: "STANDBY",
  queued: "QUEUED",
  running: "SCANNING",
  completed: "COMPLETE",
  failed: "FAULT",
};

function polarPoint(angle: number, radius: number) {
  const rad = ((angle - 90) * Math.PI) / 180;
  return {
    x: CENTER + Math.cos(rad) * radius,
    y: CENTER + Math.sin(rad) * radius,
  };
}

const SWEEP_START = polarPoint(0, DIAL_RADIUS);
const SWEEP_END = polarPoint(SWEEP_LEAD_DEG, DIAL_RADIUS);
const SWEEP_PATH = [
  `M${CENTER} ${CENTER}`,
  `L${SWEEP_START.x} ${SWEEP_START.y}`,
  `A${DIAL_RADIUS} ${DIAL_RADIUS} 0 0 1 ${SWEEP_END.x} ${SWEEP_END.y}`,
  "Z",
].join(" ");

/** 每个目标点相对表盘圆心的极角（正北为 0，顺时针增大）。 */
const TARGET_ANGLES = TARGETS.map(({ x, y }) => {
  const deg = (Math.atan2(x - CENTER, CENTER - y) * 180) / Math.PI;
  return (deg + 360) % 360;
});

function dialStatusLabel(status: WorkRadarStatus): string {
  return status === "idle" ? "待机" : careerSearchStatusLabel(status);
}

function animateSweep(sweep: SVGGElement, status: WorkRadarStatus, seconds: number) {
  gsap.killTweensOf(sweep);
  // SVG 元素的 transformOrigin 以自身包围盒左上角为基准，光束组的包围盒起点是
  // (280, 30)，"280px 280px" 会把支点甩到表盘外。绕圆心转必须用 svgOrigin ——
  // 它按 SVG 用户坐标系取绝对值。
  gsap.set(sweep, { rotation: 0, svgOrigin: `${CENTER} ${CENTER}` });

  if (status === "queued" || status === "running") {
    gsap.to(sweep, { rotation: "+=360", duration: seconds, ease: "none", repeat: -1 });
    return;
  }
  if (status === "completed") {
    gsap.to(sweep, { rotation: 26, duration: 0.7, ease: "power2.out" });
    return;
  }
  if (status === "failed") {
    gsap.to(sweep, { rotation: -18, duration: 0.35, ease: "power2.out" });
  }
}

/**
 * 命中呼吸：光束前缘扫过某个已点亮的目标时，它涟漪一圈并短暂点亮成主题橙。
 * 光束停转（completed）时改为错峰常驻呼吸，结果依然是活的。
 *
 * 点亮的永远是 TARGETS 的前 N 个，所以 pings[i] 对应 TARGETS[i]，可直接按下标
 * 取极角。两条补间都要 immediateRender: false —— fromTo 默认会在 delay 走完前
 * 就贴上 from 值，那样所有目标会在进场瞬间一起亮起。
 */
function animateHits(
  pings: SVGCircleElement[],
  blips: SVGCircleElement[],
  spinning: boolean,
  sweepSeconds: number
) {
  const cycle = spinning ? sweepSeconds : IDLE_BREATH_SECONDS;

  pings.forEach((ping, index) => {
    const angle = TARGET_ANGLES[index] ?? 0;
    const offset = spinning
      ? ((((angle - SWEEP_LEAD_DEG) % 360) + 360) % 360 / 360) * sweepSeconds
      : (index / Math.max(pings.length, 1)) * IDLE_BREATH_SECONDS;

    gsap.fromTo(
      ping,
      { attr: { r: 7 }, opacity: 0.85 },
      {
        attr: { r: 26 },
        opacity: 0,
        duration: PING_SECONDS,
        ease: "power2.out",
        delay: offset,
        repeat: -1,
        repeatDelay: Math.max(cycle - PING_SECONDS, 0),
        immediateRender: false,
      }
    );

    const blip = blips[index];
    if (!blip) return;
    const flash = Math.min(PING_SECONDS * 1.2, cycle);
    gsap.fromTo(
      blip,
      { fill: "#ff4d00" },
      {
        fill: "#161513",
        duration: flash,
        ease: "power2.out",
        delay: offset,
        repeat: -1,
        repeatDelay: Math.max(cycle - flash, 0),
        immediateRender: false,
      }
    );
  });
}

/** 选中环常驻呼吸：锁定感由这一圈承担，不靠目标点自己闪。 */
function animateLock(lock: Element) {
  gsap.fromTo(
    lock,
    { scale: 0.9, opacity: 1 },
    {
      scale: 1.16,
      opacity: 0.4,
      duration: 1.5,
      ease: "sine.inOut",
      repeat: -1,
      yoyo: true,
      svgOrigin: `${TARGETS[0].x} ${TARGETS[0].y}`,
    }
  );
}

export default function WorkRadar({
  status = "idle",
  schematic = false,
  matched = null,
  compact = false,
  className,
}: WorkRadarProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const sweepRef = useRef<SVGGElement>(null);
  const rawId = useId();
  const id = useMemo(() => rawId.replace(/:/g, ""), [rawId]);

  const dialStatus: WorkRadarStatus = schematic ? "running" : status;

  // 亮起的目标点：示意图全亮；真实任务只在完成后按服务端确认的推荐数点亮，
  // 进行中一个都不亮——后端此时并不返回任何计数。推荐数超过表盘容量时截断，
  // 表盘是刻度盘不是计数器，准确数字由旁边的任务状态区给出。
  const detectedCount = schematic
    ? TARGETS.length
    : dialStatus === "completed"
      ? Math.min(TARGETS.length, Math.max(0, Math.trunc(matched ?? 0)))
      : 0;

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        const sweep = sweepRef.current;
        const root = rootRef.current;
        if (!sweep || !root) return;

        const spinning = dialStatus === "queued" || dialStatus === "running";
        const sweepSeconds = dialStatus === "queued" ? 5.6 : 3.2;

        animateSweep(sweep, dialStatus, sweepSeconds);
        animateHits(
          Array.from(root.querySelectorAll<SVGCircleElement>("[data-radar-ping]")),
          Array.from(root.querySelectorAll<SVGCircleElement>("[data-radar-blip]")),
          spinning,
          sweepSeconds
        );

        const lock = root.querySelector("[data-radar-lock]");
        if (lock) animateLock(lock);
      });
      return () => mm.revert();
    },
    { scope: rootRef, dependencies: [dialStatus, detectedCount], revertOnUpdate: true }
  );

  return (
    <div ref={rootRef} className={cn("relative", className)}>
      <div
        className="relative overflow-hidden border border-ink/70 bg-paper"
        style={{
          backgroundImage:
            "linear-gradient(rgba(22,21,19,.055) 1px, transparent 1px), linear-gradient(90deg, rgba(22,21,19,.055) 1px, transparent 1px)",
          backgroundSize: compact ? "18px 18px" : "24px 24px",
        }}
      >
        <div className={cn("mx-auto", compact ? "max-w-[34rem] p-4 md:p-6" : "max-w-[48rem] p-4 md:p-8")}>
          <div className="mb-3 flex items-center justify-between font-mono text-[9px] tracking-[0.2em] text-ink/55 md:text-[10px]">
            <span>WORK RADAR / WR-01</span>
            <span className={!schematic && dialStatus === "failed" ? "text-accent" : undefined}>
              {schematic ? "SCHEMATIC" : STATUS_COPY[dialStatus]}
            </span>
          </div>

          <svg
            viewBox="0 0 560 560"
            className="block h-auto w-full"
            // 装饰用示意表盘对辅助技术隐藏（DESIGN_SYSTEM 第 13 节：装饰图使用空
            // alt）；接了真实任务的表盘才播报状态。
            {...(schematic
              ? { "aria-hidden": true }
              : { role: "img", "aria-label": `求职雷达状态：${dialStatusLabel(dialStatus)}` })}
          >
            <defs>
              <linearGradient id={`${id}-sweep`} x1="280" y1="280" x2="495" y2="132" gradientUnits="userSpaceOnUse">
                <stop offset="0" stopColor="#ff4d00" stopOpacity="0.96" />
                <stop offset="0.55" stopColor="#ff4d00" stopOpacity="0.68" />
                <stop offset="1" stopColor="#ff4d00" stopOpacity="0.08" />
              </linearGradient>
              <filter id={`${id}-blur`} x="-30%" y="-30%" width="160%" height="160%">
                <feGaussianBlur stdDeviation="9" />
              </filter>
            </defs>

            {RINGS.map((radius) => (
              <circle key={radius} cx="280" cy="280" r={radius} fill="none" stroke="#161513" strokeOpacity="0.34" strokeWidth="1" />
            ))}

            <line x1="26" y1="280" x2="534" y2="280" stroke="#161513" strokeOpacity="0.62" strokeWidth="1" />
            <line x1="280" y1="26" x2="280" y2="534" stroke="#161513" strokeOpacity="0.62" strokeWidth="1" />
            <circle cx="280" cy="280" r="15" fill="none" stroke="#161513" strokeOpacity="0.6" strokeWidth="1" />

            {TICKS.map((angle, index) => {
              const outer = polarPoint(angle, 250);
              const inner = polarPoint(angle, index % 6 === 0 ? 238 : 243);
              return (
                <line
                  key={angle}
                  x1={inner.x}
                  y1={inner.y}
                  x2={outer.x}
                  y2={outer.y}
                  stroke="#161513"
                  strokeOpacity={index % 6 === 0 ? 0.72 : 0.42}
                  strokeWidth={index % 6 === 0 ? 1.4 : 0.8}
                />
              );
            })}

            {ANGLES.map((angle) => {
              const point = polarPoint(angle, 268);
              return (
                <text
                  key={angle}
                  x={point.x}
                  y={point.y}
                  textAnchor="middle"
                  dominantBaseline="middle"
                  fill="#161513"
                  fontSize="13"
                  fontFamily="monospace"
                >
                  {angle}
                </text>
              );
            })}

            <g ref={sweepRef} opacity={dialStatus === "idle" ? 0.32 : dialStatus === "failed" ? 0.42 : 1}>
              <path d={SWEEP_PATH} fill="#ff4d00" opacity="0.18" filter={`url(#${id}-blur)`} />
              <path d={SWEEP_PATH} fill={`url(#${id}-sweep)`} />
              <line
                x1={CENTER}
                y1={CENTER}
                x2={SWEEP_END.x}
                y2={SWEEP_END.y}
                stroke="#ff4d00"
                strokeOpacity="0.75"
                strokeWidth="1.2"
              />
            </g>

            {TARGETS.map((target, index) => {
              const detected = index < detectedCount;
              const selected = detected && index === 0;
              return (
                <g key={`${target.x}-${target.y}`} opacity={detected ? 1 : 0.2}>
                  {detected ? (
                    <circle
                      data-radar-ping
                      cx={target.x}
                      cy={target.y}
                      r="7"
                      fill="none"
                      stroke="#ff4d00"
                      strokeWidth="1.5"
                      opacity="0"
                    />
                  ) : null}
                  {selected ? (
                    <circle
                      data-radar-lock
                      cx={target.x}
                      cy={target.y}
                      r="14"
                      fill="none"
                      stroke="#ff4d00"
                      strokeWidth="2"
                      strokeDasharray="5 4"
                    />
                  ) : null}
                  <circle
                    {...(detected ? { "data-radar-blip": "" } : {})}
                    cx={target.x}
                    cy={target.y}
                    r={detected ? 5.5 : 4.5}
                    fill="#161513"
                  />
                </g>
              );
            })}

            <path d="M280 20 l-7 13 h14 Z" fill="#161513" />
            <path d="M540 280 l-13 -7 v14 Z" fill="#161513" />
            <path d="M280 540 l-7 -13 h14 Z" fill="#161513" />
            <path d="M20 280 l13 -7 v14 Z" fill="#161513" />
          </svg>
        </div>
      </div>
    </div>
  );
}
