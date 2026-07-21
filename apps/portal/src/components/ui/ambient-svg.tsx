"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";
import { cn } from "@/lib/cn";

/**
 * 循环非线性 SVG 环境动画（纯装饰）。
 * - dial：缓慢旋转的虚线刻度盘 + 呼吸的核心点
 * - flow：游走的虚线描边 + 漂移的橙色小方块
 * - crosshair：呼吸的十字准星组
 * reduced-motion 时保持静止（不注册任何补间）。
 */
export default function AmbientSvg({
  variant,
  className,
}: {
  variant: "dial" | "flow" | "crosshair";
  className?: string;
}) {
  const ref = useRef<SVGSVGElement>(null);

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        if (variant === "dial") {
          gsap.to("[data-ambient-ring]", {
            rotate: 360,
            duration: 46,
            repeat: -1,
            ease: "none",
            transformOrigin: "50% 50%",
          });
          gsap.to("[data-ambient-ticks]", {
            rotate: -360,
            duration: 70,
            repeat: -1,
            ease: "none",
            transformOrigin: "50% 50%",
          });
          gsap.to("[data-ambient-core]", {
            scale: 1.6,
            opacity: 0.5,
            duration: 2.2,
            yoyo: true,
            repeat: -1,
            ease: "sine.inOut",
            transformOrigin: "50% 50%",
          });
        } else if (variant === "flow") {
          gsap.to("[data-ambient-flow]", {
            strokeDashoffset: -120,
            duration: 5,
            repeat: -1,
            ease: "none",
          });
          gsap.to("[data-ambient-dot]", {
            x: 36,
            duration: 2.6,
            yoyo: true,
            repeat: -1,
            ease: "sine.inOut",
          });
        } else {
          gsap.to("[data-ambient-cross]", {
            scale: 1.35,
            opacity: 0.3,
            duration: 1.9,
            yoyo: true,
            repeat: -1,
            ease: "sine.inOut",
            transformOrigin: "50% 50%",
            stagger: 0.45,
          });
        }
      });
      return () => mm.revert();
    },
    { scope: ref }
  );

  if (variant === "dial") {
    return (
      <svg
        ref={ref}
        viewBox="0 0 200 200"
        aria-hidden
        className={cn("pointer-events-none h-40 w-40", className)}
        fill="none"
        stroke="currentColor"
      >
        <g data-ambient-ring>
          <circle cx="100" cy="100" r="86" strokeDasharray="3 9" />
          <circle cx="100" cy="100" r="58" strokeDasharray="1 7" opacity="0.6" />
        </g>
        <g data-ambient-ticks opacity="0.7">
          {Array.from({ length: 12 }).map((_, i) => {
            const a = (i * Math.PI) / 6;
            // 固定两位小数，保证 SSR 与客户端字符串一致（避免水合不匹配）
            const r = (v: number) => Number(v.toFixed(2));
            return (
              <line
                key={i}
                x1={r(100 + Math.cos(a) * 70)}
                y1={r(100 + Math.sin(a) * 70)}
                x2={r(100 + Math.cos(a) * 76)}
                y2={r(100 + Math.sin(a) * 76)}
              />
            );
          })}
        </g>
        <path d="M100 92v16M92 100h16" opacity="0.8" />
        <circle data-ambient-core cx="100" cy="100" r="4" className="fill-accent stroke-none" />
      </svg>
    );
  }

  if (variant === "flow") {
    return (
      <svg
        ref={ref}
        viewBox="0 0 1200 16"
        preserveAspectRatio="none"
        aria-hidden
        className={cn("pointer-events-none h-4 w-full", className)}
        fill="none"
        stroke="currentColor"
      >
        <line x1="0" y1="8" x2="1200" y2="8" opacity="0.35" />
        <line data-ambient-flow x1="0" y1="8" x2="1200" y2="8" strokeDasharray="4 14" />
        <rect data-ambient-dot x="0" y="4" width="8" height="8" className="fill-accent stroke-none" />
      </svg>
    );
  }

  return (
    <svg
      ref={ref}
      viewBox="0 0 120 60"
      aria-hidden
      className={cn("pointer-events-none h-12 w-24", className)}
      fill="none"
      stroke="currentColor"
    >
      <path data-ambient-cross d="M20 22v16M12 30h16" />
      <path data-ambient-cross d="M60 12v16M52 20h16" />
      <path data-ambient-cross d="M100 30v16M92 38h16" />
    </svg>
  );
}
