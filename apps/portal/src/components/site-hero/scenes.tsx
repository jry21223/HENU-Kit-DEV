"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION } from "@/lib/gsap";

/** 资料库场景：书脊阵列 + 循环翻页描线 */
export function SceneBooks() {
  const ref = useRef<SVGSVGElement>(null);
  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        gsap.to("[data-spine]", {
          y: -5,
          duration: 1.6,
          yoyo: true,
          repeat: -1,
          ease: "sine.inOut",
          stagger: 0.22,
        });
        gsap.fromTo(
          "[data-page-line]",
          { strokeDasharray: 90, strokeDashoffset: 90 },
          { strokeDashoffset: -90, duration: 2.6, repeat: -1, repeatDelay: 0.9, ease: "power1.inOut" }
        );
      });
      return () => mm.revert();
    },
    { scope: ref }
  );

  const spines = [
    { x: 40, h: 96 },
    { x: 72, h: 116 },
    { x: 104, h: 88 },
    { x: 136, h: 108 },
    { x: 168, h: 98 },
  ];
  return (
    <svg ref={ref} viewBox="0 0 240 160" fill="none" stroke="currentColor" className="w-full text-ink/60">
      {spines.map((s, i) => (
        <g key={i} data-spine>
          <rect x={s.x} y={132 - s.h} width="22" height={s.h} />
          <line x1={s.x + 5} y1={132 - s.h + 10} x2={s.x + 17} y2={132 - s.h + 10} className="stroke-accent" />
        </g>
      ))}
      <line x1="24" y1="132" x2="216" y2="132" strokeWidth="1.5" />
      <path data-page-line d="M32 148 H 208" className="stroke-accent" strokeWidth="1.5" />
    </svg>
  );
}

/** 美食榜场景：定位针 + 辐射脉冲 + 热气上升线 */
export function SceneFood() {
  const ref = useRef<SVGSVGElement>(null);
  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        // 支点必须落在定位针中心 (120,92)。两点都不能省：
        // 1) 用 svgOrigin 而非 transformOrigin —— SVG 元素上的 px transformOrigin 以自身
        //    bbox 左上角为基准（GSAP _applySVGOrigin），圆环 bbox 为 (100,72,40,40)，
        //    支点会被算到 (220,164)，落在 240x160 视窗之外。
        // 2) 必须在元素仍是单位矩阵时先行设定。若把 svgOrigin 放进 fromTo 的目标参数，
        //    起始 scale 已经渲染，GSAP 会把绝对坐标换算回局部坐标（得到 150,122）
        //    并叠加平滑偏移，支点照样跑偏。
        gsap.set("[data-ring]", { svgOrigin: "120 92" });
        gsap.fromTo(
          "[data-ring]",
          { scale: 0.4, opacity: 0.8 },
          {
            scale: 1.5,
            opacity: 0,
            duration: 2.2,
            repeat: -1,
            ease: "sine.out",
            stagger: 0.7,
          }
        );
        gsap.to("[data-steam]", {
          y: -8,
          opacity: 0.3,
          duration: 1.5,
          yoyo: true,
          repeat: -1,
          ease: "sine.inOut",
          stagger: 0.3,
        });
      });
      return () => mm.revert();
    },
    { scope: ref }
  );

  return (
    <svg ref={ref} viewBox="0 0 240 160" fill="none" stroke="currentColor" className="w-full text-ink/60">
      {/* 辐射圆环 */}
      <circle data-ring cx="120" cy="92" r="20" />
      <circle data-ring cx="120" cy="92" r="20" />
      <circle data-ring cx="120" cy="92" r="20" />
      {/* 定位针 */}
      <path d="M120 116 L104 84 a18 18 0 1 1 32 0 Z" className="stroke-accent" strokeWidth="1.5" />
      <circle cx="120" cy="92" r="5" className="fill-accent stroke-none" />
      {/* 碗 + 热气 */}
      <path d="M70 138 h44 a22 10 0 0 1 -44 0 Z" />
      <path data-steam d="M84 126 q3 -6 0 -12" />
      <path data-steam d="M94 126 q3 -6 0 -12" className="stroke-accent" />
      <path data-steam d="M104 126 q3 -6 0 -12" />
      {/* 餐具 */}
      <path d="M172 126 v22 M168 126 v6 a4 4 0 0 0 8 0 v-6" />
      <path d="M190 126 v22" />
    </svg>
  );
}

/** 互助平台场景：箱体交接 + 担保盾形描线循环 */
export function SceneHandshake() {
  const ref = useRef<SVGSVGElement>(null);
  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        gsap.fromTo(
          "[data-shield]",
          { strokeDasharray: 220, strokeDashoffset: 220 },
          { strokeDashoffset: -220, duration: 3.4, repeat: -1, repeatDelay: 0.8, ease: "power1.inOut" }
        );
        gsap.to("[data-box]", {
          x: 14,
          duration: 1.8,
          yoyo: true,
          repeat: -1,
          ease: "sine.inOut",
        });
      });
      return () => mm.revert();
    },
    { scope: ref }
  );

  return (
    <svg ref={ref} viewBox="0 0 240 160" fill="none" stroke="currentColor" className="w-full text-ink/60">
      {/* 左/右手（抽象线） */}
      <path d="M20 96 h36 M20 108 h30 M20 84 h30" />
      <path d="M220 96 h-24 M220 108 h-18 M220 84 h-18" />
      {/* 交接的箱子 */}
      <g data-box>
        <rect x="76" y="80" width="36" height="30" strokeWidth="1.5" />
        <path d="M76 90 h36 M94 80 v10" />
      </g>
      {/* 担保盾 */}
      <path
        data-shield
        className="stroke-accent"
        strokeWidth="1.5"
        d="M170 36 l26 10 v22 c0 18 -12 28 -26 34 c-14 -6 -26 -16 -26 -34 v-22 Z"
      />
      <path d="M160 66 l7 7 l14 -14" className="stroke-accent" strokeWidth="1.5" />
    </svg>
  );
}
