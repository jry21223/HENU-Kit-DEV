"use client";

import { useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import { gsap, FINE_MOTION } from "@/lib/gsap";

const DEFAULT_REVEAL: gsap.TweenVars = {
  y: 14,
  autoAlpha: 0,
  duration: 0.45,
  ease: "power2.out",
  stagger: 0.05,
};

const subscribeNothing = () => () => {};

/**
 * 这次挂载是不是在水合服务端已经画出来的 DOM：水合渲染读 getServerSnapshot（false），
 * 客户端导航后的新挂载直接读 getSnapshot（true）。只取首次渲染的值。
 */
function useHydratedFromServer() {
  const mountedOnClient = useSyncExternalStore(subscribeNothing, () => true, () => false);
  const [hydrated] = useState(!mountedOnClient);
  return hydrated;
}

/**
 * [data-enter] 内容块入场 reveal：淡入 + y 位移 stagger（reduced-motion 直显）。
 *
 * 只揭示还没显示过的块（#537）：
 * - 水合时这些块 SSR 已经画在屏幕上，再 from() 一次就是「可见 → 消失 → 重播」，所以
 *   直接记为已显示；
 * - deps 变化（如数据到达、切换筛选）只揭示新挂上的块，已经显示的页头不跟着重播；
 * - 放在布局副作用里，新块在浏览器绘制前就处于初始态，不会先闪一帧再消失。
 *
 * vars 在每次揭示时才取值：刷题页要按当时有没有形变载荷决定延迟（usePageEnter）。
 */
export function useReveal(deps: unknown[] = [], vars: () => gsap.TweenVars = () => DEFAULT_REVEAL) {
  const hydrated = useHydratedFromServer();
  const reveal = useRef<{ mm: gsap.MatchMedia; shown: WeakSet<Element> } | null>(null);

  // 挂载期：一份 matchMedia 上下文 + 已显示块的记录，卸载时整体还原。
  useLayoutEffect(() => {
    const mm = gsap.matchMedia();
    const painted = hydrated ? gsap.utils.toArray<Element>("[data-enter]") : [];
    reveal.current = { mm, shown: new WeakSet(painted) };
    return () => {
      mm.revert();
      reveal.current = null;
    };
  }, [hydrated]);

  useLayoutEffect(() => {
    const current = reveal.current;
    if (!current) return;
    const fresh = gsap.utils
      .toArray<Element>("[data-enter]")
      .filter((block) => !current.shown.has(block));
    if (fresh.length === 0) return;
    fresh.forEach((block) => current.shown.add(block));
    current.mm.add(FINE_MOTION, () => {
      gsap.from(fresh, { ...vars(), clearProps: "all" });
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
}
