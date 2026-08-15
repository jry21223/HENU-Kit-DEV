"use client";

import { useRef } from "react";
import { FINE_MOTION, gsap, useGSAP } from "@/lib/gsap";

/** 雷达扫描光束示意：橙色光束循环扫动；reduced-motion 时静态显示。 */
export default function CareerRadarVisual() {
  const ref = useRef<SVGSVGElement>(null);

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        const beam = ref.current?.querySelector("[data-career-radar-beam]");
        if (!beam) return;
        gsap.to(beam, {
          rotation: 360,
          duration: 4,
          repeat: -1,
          ease: "none",
          transformOrigin: "120px 120px",
        });
      });
      return () => mm.revert();
    },
    { scope: ref }
  );

  return (
    <svg
      ref={ref}
      viewBox="0 0 240 240"
      fill="none"
      className="w-full max-w-sm"
      aria-hidden
    >
      <circle cx="120" cy="120" r="92" stroke="#161513" strokeWidth="1.5" />
      <circle cx="120" cy="120" r="60" stroke="#161513" strokeWidth="1" opacity="0.4" />
      <circle cx="120" cy="120" r="28" stroke="#161513" strokeWidth="1" opacity="0.2" />
      <path d="M120 12 V 228" stroke="#161513" strokeWidth="0.75" opacity="0.35" />
      <path d="M12 120 H 228" stroke="#161513" strokeWidth="0.75" opacity="0.35" />

      <g data-career-radar-beam>
        <path
          d="M120 120 L 120 28 A 92 92 0 0 1 212 120 Z"
          fill="#ff4d00"
          opacity="0.18"
        />
        <line x1="120" y1="120" x2="212" y2="120" stroke="#ff4d00" strokeWidth="2" />
      </g>

      <circle cx="172" cy="84" r="4" fill="#ff4d00" />
      <circle cx="60" cy="96" r="3" fill="#ff4d00" opacity="0.7" />

      <text
        x="120"
        y="214"
        textAnchor="middle"
        fontSize="9"
        fill="#161513"
        opacity="0.5"
        fontFamily="monospace"
      >
        WORK RADAR®
      </text>
    </svg>
  );
}
