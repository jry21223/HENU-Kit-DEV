"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION, ScrollTrigger } from "@/lib/gsap";
import SectionHeading from "@/components/ui/section-heading";
import MagneticButton from "@/components/ui/magnetic-button";

const FEATURES = ["按类别浏览互助与闲置", "搜索你关注的信息"];

/** 浏览 → 筛选 → 查看：当前开放的信息浏览流程。 */
function FlowDiagram() {
  return (
    <svg viewBox="0 0 480 140" fill="none" className="w-full max-w-lg">
      {/* 连接线 */}
      <path data-flow-line d="M70 70 H 200" stroke="#161513" strokeWidth="1.5" />
      <path data-flow-line d="M280 70 H 410" stroke="#161513" strokeWidth="1.5" />
      <path data-flow-arrow d="M196 65 L 204 70 L 196 75" stroke="#ff4d00" strokeWidth="1.5" />
      <path data-flow-arrow d="M406 65 L 414 70 L 406 75" stroke="#ff4d00" strokeWidth="1.5" />

      {/* 节点 */}
      <g data-flow-node>
        <rect x="10" y="46" width="60" height="48" stroke="#161513" />
        <text x="40" y="74" textAnchor="middle" fontSize="13" fill="#161513">浏览</text>
      </g>
      <g data-flow-node>
        <rect x="204" y="46" width="76" height="48" stroke="#161513" />
        <text x="242" y="74" textAnchor="middle" fontSize="13" fill="#161513">筛选</text>
      </g>
      <g data-flow-node>
        <rect x="414" y="46" width="60" height="48" stroke="#161513" />
        <text x="444" y="74" textAnchor="middle" fontSize="13" fill="#161513">查看</text>
      </g>

      <text x="40" y="120" textAnchor="middle" fontSize="9" fill="#161513" opacity="0.5" fontFamily="monospace">STEP 1</text>
      <text x="242" y="120" textAnchor="middle" fontSize="9" fill="#161513" opacity="0.5" fontFamily="monospace">STEP 2</text>
      <text x="444" y="120" textAnchor="middle" fontSize="9" fill="#161513" opacity="0.5" fontFamily="monospace">STEP 3</text>
    </svg>
  );
}

export default function SectionCampus() {
  const sectionRef = useRef<HTMLElement>(null);

  useGSAP(
    () => {
      const mm = gsap.matchMedia();
      mm.add(FINE_MOTION, () => {
        // 流程线描线动画：进入视觉中心时播放一遍，描完后切换为虚线流动
        const paths = gsap.utils.toArray<SVGPathElement>(
          "[data-flow-line], [data-flow-arrow]"
        );
        paths.forEach((p) => {
          const len = p.getTotalLength();
          gsap.set(p, { strokeDasharray: len, strokeDashoffset: len });
        });

        let flowing = false;
        const startFlow = () => {
          if (flowing) return;
          flowing = true;
          paths.forEach((p) => {
            gsap.set(p, { strokeDasharray: "5 7", strokeDashoffset: 0 });
            gsap.to(p, {
              strokeDashoffset: -48,
              duration: 3,
              repeat: -1,
              ease: "none",
            });
          });
        };

        const draw = gsap.timeline({ paused: true, onComplete: startFlow });
        paths.forEach((p, i) => {
          draw.to(
            p,
            { strokeDashoffset: 0, duration: 0.6, ease: "power1.inOut" },
            i * 0.35
          );
        });

        ScrollTrigger.create({
          trigger: sectionRef.current,
          start: "top 60%",
          onEnter: () => draw.restart(),
          onEnterBack: () => draw.restart(),
        });

        gsap.from("[data-flow-node]", {
          opacity: 0,
          y: 12,
          duration: 0.5,
          stagger: 0.2,
          ease: "power2.out",
          scrollTrigger: {
            trigger: sectionRef.current,
            start: "top 60%",
            toggleActions: "play none none reverse",
          },
        });
        gsap.from("[data-order-card]", {
          y: 40,
          opacity: 0,
          duration: 0.8,
          ease: "power3.out",
          scrollTrigger: {
            trigger: sectionRef.current,
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
    <section ref={sectionRef} className="snap-screen border-t border-line bg-paper">
      <div className="mx-auto grid min-h-svh max-w-7xl items-center gap-12 px-5 py-24 md:grid-cols-2 md:px-10">
        <div>
          <SectionHeading index="04" en="CAMPUS MUTUAL AID" title="互助平台" />
          <p className="mt-6 max-w-sm text-sm leading-7 text-ink/70">
            看看同校的互助需求与闲置信息，
            按类别查找你关注的内容。
          </p>
          <ul className="mt-6 space-y-2 font-mono text-xs tracking-wider text-ink/60">
            {FEATURES.map((f) => (
              <li key={f}>
                <span className="mr-2 text-accent">+</span>
                {f}
              </li>
            ))}
          </ul>
          <MagneticButton href="/campus" className="mt-8">
            浏览互助信息
          </MagneticButton>

          <div className="mt-12">
            <FlowDiagram />
          </div>
        </div>

        {/* 订单卡：当前互助平台的发布/接单/结算尚未接通后端，
            不再展示硬编码示例订单；待真实订单可用后再接数据。 */}
        <div className="flex items-center">
          <article
            data-order-card
            className="group w-full max-w-sm border border-dashed border-ink/25 bg-paper p-6"
          >
            <div className="flex items-center justify-between font-mono text-[10px] tracking-[0.25em] text-ink/50">
              <span>CAMPUS AID</span>
              <span className="border border-ink/30 px-2 py-0.5 text-ink/50">信息浏览</span>
            </div>
            <h3 className="mt-5 font-display text-3xl font-bold">先看看校园所需</h3>
            <p className="mt-2 text-sm text-ink/60">
              可浏览互助与闲置信息，查找分类或搜索关键词。
            </p>
            <p className="mt-6 border-t border-line pt-4 text-xs leading-6 text-ink/60">
              发布、接单和结算暂未开放。
            </p>
          </article>
        </div>
      </div>
    </section>
  );
}
