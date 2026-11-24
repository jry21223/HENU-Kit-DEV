"use client";

import { useRef } from "react";
import { gsap, useGSAP, FINE_MOTION, ScrollTrigger, isOnScreen } from "@/lib/gsap";
import SectionHeading from "@/components/ui/section-heading";
import MagneticButton from "@/components/ui/magnetic-button";

const FEATURES = [
  "发布互助求助与闲置单，支持最多 6 张配图",
  "公开留言沟通，双方同意后交换联系方式",
];

/** 发单 → 留言 → 交换：同校互助与闲置沟通流程。 */
function FlowDiagram() {
  // 图里的字随图缩放。图宽不到 28rem（手机）时节点文字放大，屏幕上仍不小于 12px；
  // STEP 编号是读屏跳过的装饰，这时不显示（DESIGN_SYSTEM.md 第 4 节）。
  const nodeText = "text-[18px] @md:text-[13px]";
  return (
    <div className="@container">
      <svg viewBox="0 0 480 140" fill="none" className="w-full max-w-lg">
        {/* 连接线 */}
        <path data-flow-line d="M70 70 H 200" stroke="#161513" strokeWidth="1.5" />
        <path data-flow-line d="M280 70 H 410" stroke="#161513" strokeWidth="1.5" />
        <path data-flow-arrow d="M196 65 L 204 70 L 196 75" stroke="#ff4d00" strokeWidth="1.5" />
        <path data-flow-arrow d="M406 65 L 414 70 L 406 75" stroke="#ff4d00" strokeWidth="1.5" />

        {/* 节点 */}
        <g data-flow-node>
          <rect x="10" y="46" width="60" height="48" stroke="#161513" />
          <text x="40" y="70" textAnchor="middle" dominantBaseline="central" className={nodeText} fill="#161513">
            发单
          </text>
        </g>
        <g data-flow-node>
          <rect x="204" y="46" width="76" height="48" stroke="#161513" />
          <text x="242" y="70" textAnchor="middle" dominantBaseline="central" className={nodeText} fill="#161513">
            留言
          </text>
        </g>
        <g data-flow-node>
          <rect x="414" y="46" width="60" height="48" stroke="#161513" />
          <text x="444" y="70" textAnchor="middle" dominantBaseline="central" className={nodeText} fill="#161513">
            交换
          </text>
        </g>

        <g aria-hidden className="hidden @md:inline">
          <text x="40" y="120" textAnchor="middle" fontSize="11" fill="#161513" opacity="0.6" fontFamily="monospace">STEP 1</text>
          <text x="242" y="120" textAnchor="middle" fontSize="11" fill="#161513" opacity="0.6" fontFamily="monospace">STEP 2</text>
          <text x="444" y="120" textAnchor="middle" fontSize="11" fill="#161513" opacity="0.6" fontFamily="monospace">STEP 3</text>
        </g>
      </svg>
    </div>
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

        // 从中间位置加载、模块已经在视口里：线、节点和卡片是服务端画好的，
        // 直接进入虚线流动，不再藏起来重播描线和入场（#557）。
        if (isOnScreen(sectionRef.current)) {
          startFlow();
          return;
        }

        paths.forEach((p) => {
          const len = p.getTotalLength();
          gsap.set(p, { strokeDasharray: len, strokeDashoffset: len });
        });
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
      <div className="mx-auto grid max-w-site items-center gap-12 px-5 py-24 md:min-h-svh md:grid-cols-2 md:px-8">
        <div>
          <SectionHeading index="04" en="CAMPUS MUTUAL AID" title="互助平台" />
          <p className="mt-4 inline-block border border-accent px-2 py-0.5 font-mono text-xs text-accent-text">
            双向同意后换联系方式 · 无在线结算
          </p>
          <p className="mt-6 max-w-sm text-sm leading-7 text-ink/70">
            发布同校互助需求或闲置信息，先公开留言沟通，双方同意后再交换联系方式。
          </p>
          <ul className="mt-6 space-y-2 font-mono text-xs text-ink/60">
            {FEATURES.map((f) => (
              <li key={f}>
                <span aria-hidden className="mr-2 text-accent-text">+</span>
                {f}
              </li>
            ))}
          </ul>
          <MagneticButton href="/campus" className="mt-8">
            进入互助平台
          </MagneticButton>

          <div className="mt-12">
            <FlowDiagram />
          </div>
        </div>

        <div className="flex items-center">
          <article
            data-order-card
            className="group w-full max-w-sm border border-dashed border-ink/25 bg-paper p-6"
          >
            <div className="flex items-center justify-between font-mono text-xs text-ink/60">
              <span className="tracking-[0.25em]">CAMPUS AID</span>
              <span className="border border-ink/30 px-2 py-0.5">互助 · 闲置</span>
            </div>
            <h3 className="mt-5 font-display text-3xl font-bold">发布单子，双方同意再换联系</h3>
            <p className="mt-2 text-sm text-ink/60">
              支持发布互助与闲置单、公开留言，并在联系方式簿里选择渠道，仅在双方同意后互相展示。
            </p>
            <p className="mt-6 border-t border-line pt-4 text-xs leading-6 text-ink/60">
              不提供在线支付与资金托管，金额仅供沟通参考。
            </p>
          </article>
        </div>
      </div>
    </section>
  );
}
