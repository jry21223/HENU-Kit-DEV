"use client";

import { useEffect, useRef } from "react";
import { useSyncExternalStore } from "react";
import { usePathname } from "next/navigation";
import { gsap } from "@/lib/gsap";
import { morphStore } from "./transition-store";

/**
 * 形变覆盖层 + 页面容器。
 * payload（来源 rect/快照）+ landing（落点 rect）齐备后，
 * overlay 从来源 rect 形变到落点 rect，交叉淡化后撤掉，清空 store。
 */
export default function TransitionProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const state = useSyncExternalStore(
    morphStore.subscribe,
    morphStore.get,
    morphStore.getServer
  );
  const overlayRef = useRef<HTMLDivElement>(null);
  const pageRef = useRef<HTMLDivElement>(null);
  const pathname = usePathname();

  // 退出动画（TransitionLink）在没有 [data-block] 的页面上直接给这个容器写
  // autoAlpha/y。容器位于 practice/layout，导航不会重新挂载它，所以路由提交后
  // 必须清掉 GSAP 的内联样式，否则新页面继承 opacity:0 而整页不可见。
  useEffect(() => {
    if (pageRef.current) gsap.set(pageRef.current, { clearProps: "all" });
  }, [pathname]);

  // 落点上报：执行形变
  useEffect(() => {
    const { payload, landing } = state;
    const el = overlayRef.current;
    if (!payload || !landing || !el) return;
    const tl = gsap.timeline({ onComplete: () => morphStore.clear() });
    tl.to(el, {
      x: landing.x,
      y: landing.y,
      width: landing.w,
      height: landing.h,
      duration: 0.5,
      ease: "power3.inOut",
    })
      .to("[data-morph-label]", { autoAlpha: 0, duration: 0.15 }, 0.3)
      .to(el, { autoAlpha: 0, duration: 0.18 }, "-=0.12");
    return () => {
      tl.kill();
    };
  }, [state]);

  // 兜底：payload 存在但迟迟无落点上报（如落点不存在），超时清空避免卡死
  useEffect(() => {
    if (!state.payload || state.landing) return;
    const t = setTimeout(() => morphStore.clear(), 800);
    return () => clearTimeout(t);
  }, [state.payload, state.landing]);

  const p = state.payload;

  return (
    <>
      <div ref={pageRef} data-transition-page>
        {children}
      </div>
      {p && (
        <div
          ref={overlayRef}
          aria-hidden
          className="pointer-events-none fixed left-0 top-0 z-[70] overflow-hidden border border-ink bg-paper"
          style={{
            transform: `translate(${p.rect.x}px, ${p.rect.y}px)`,
            width: p.rect.w,
            height: p.rect.h,
          }}
        >
          <div
            data-morph-label
            className="flex h-full flex-col justify-between p-4"
          >
            <span className="font-mono text-[10px] tracking-[0.3em] text-accent">
              {p.kind === "list" ? "LIST" : "QUESTION"}
            </span>
            <div>
              <p className="font-display text-lg font-bold leading-snug">
                {p.title}
              </p>
              {p.sub && (
                <p className="mt-1 font-mono text-[10px] tracking-wider text-ink/50">
                  {p.sub}
                </p>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  );
}
