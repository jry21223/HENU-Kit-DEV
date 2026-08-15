"use client";

import { useId, useMemo, useRef } from "react";

import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

export type WorkRadarStatus = "idle" | "queued" | "running" | "completed" | "failed";

type WorkRadarProps = {
  status?: WorkRadarStatus;
  sourcesCompleted?: number;
  sourcesTotal?: number;
  jobsFound?: number;
  matched?: number;
  compact?: boolean;
  className?: string;
};

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
    x: 280 + Math.cos(rad) * radius,
    y: 280 + Math.sin(rad) * radius,
  };
}

export default function WorkRadar({
  status = "running",
  sourcesCompleted = 0,
  sourcesTotal = 16,
  jobsFound = 0,
  matched = 0,
  compact = false,
  className,
}: WorkRadarProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const sweepRef = useRef<SVGGElement>(null);
  const rawId = useId();
  const id = useMemo(() => rawId.replace(/:/g, ""), [rawId]);

  const detectedCount =
    status === "completed"
      ? TARGETS.length
      : status === "running"
        ? Math.max(1, Math.min(TARGETS.length, Math.ceil((sourcesCompleted / Math.max(sourcesTotal, 1)) * TARGETS.length)))
        : status === "failed"
          ? Math.min(3, TARGETS.length)
          : 0;

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        const sweep = sweepRef.current;
        if (!sweep) return;

        gsap.killTweensOf(sweep);
        gsap.set(sweep, { transformOrigin: "280px 280px" });

        if (status === "queued" || status === "running") {
          gsap.to(sweep, {
            rotation: "+=360",
            duration: status === "queued" ? 5.6 : 3.2,
            ease: "none",
            repeat: -1,
          });
        } else if (status === "completed") {
          gsap.to(sweep, { rotation: 26, duration: 0.7, ease: "power2.out" });
        } else if (status === "failed") {
          gsap.to(sweep, { rotation: -18, duration: 0.35, ease: "power2.out" });
        }
      });
      return () => mm.revert();
    },
    { scope: rootRef, dependencies: [status] }
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
            <span className={status === "failed" ? "text-accent" : undefined}>{STATUS_COPY[status]}</span>
          </div>

          <svg viewBox="0 0 560 560" role="img" aria-label={`求职雷达状态：${STATUS_COPY[status]}`} className="block h-auto w-full">
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

            <g ref={sweepRef} opacity={status === "idle" ? 0.32 : status === "failed" ? 0.42 : 1}>
              <path
                d="M280 280 L280 30 A250 250 0 0 1 485 137 Z"
                fill="#ff4d00"
                opacity="0.18"
                filter={`url(#${id}-blur)`}
              />
              <path d="M280 280 L280 30 A250 250 0 0 1 485 137 Z" fill={`url(#${id}-sweep)`} />
              <line x1="280" y1="280" x2="485" y2="137" stroke="#ff4d00" strokeOpacity="0.75" strokeWidth="1.2" />
            </g>

            {TARGETS.map((target, index) => {
              const detected = index < detectedCount;
              const selected = detected && index === detectedCount - 1 && status === "running";
              return (
                <g key={`${target.x}-${target.y}`} opacity={detected || compact ? 1 : 0.2}>
                  {selected ? (
                    <circle cx={target.x} cy={target.y} r="14" fill="none" stroke="#ff4d00" strokeWidth="2" strokeDasharray="5 4" />
                  ) : null}
                  <circle cx={target.x} cy={target.y} r={detected ? 5.5 : 4.5} fill={detected ? "#161513" : "#161513"} />
                  {selected ? (
                    <>
                      <path d={`M${target.x + 16} ${target.y - 2} h28`} stroke="#ff4d00" strokeWidth="1.5" />
                      <text x={target.x + 50} y={target.y + 3} fill="#ff4d00" fontSize="14" fontFamily="monospace">01</text>
                    </>
                  ) : null}
                </g>
              );
            })}

            <path d="M280 20 l-7 13 h14 Z" fill="#161513" />
            <path d="M540 280 l-13 -7 v14 Z" fill="#161513" />
            <path d="M280 540 l-7 -13 h14 Z" fill="#161513" />
            <path d="M20 280 l13 -7 v14 Z" fill="#161513" />
          </svg>

          {!compact ? (
            <div className="mt-2 grid grid-cols-2 gap-x-8 gap-y-2 border-t border-ink/60 pt-4 font-mono text-[10px] tracking-[0.12em] text-ink/60 sm:grid-cols-4">
              <p><span className="text-ink/35">STATUS</span><br /><strong className="font-normal text-ink">{STATUS_COPY[status]}</strong></p>
              <p><span className="text-ink/35">SOURCES</span><br /><strong className="font-normal text-ink">{String(sourcesCompleted).padStart(2, "0")} / {String(sourcesTotal).padStart(2, "0")}</strong></p>
              <p><span className="text-ink/35">JOBS FOUND</span><br /><strong className="font-normal text-ink">{String(jobsFound).padStart(2, "0")}</strong></p>
              <p><span className="text-ink/35">MATCHED</span><br /><strong className="font-normal text-accent">{String(matched).padStart(2, "0")}</strong></p>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
