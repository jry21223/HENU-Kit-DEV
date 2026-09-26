"use client";

import { useEffect, useRef } from "react";
import { useSyncExternalStore } from "react";
import { gsap } from "@/lib/gsap";
import { useReveal } from "@/components/account/use-reveal";
import { morphStore, MorphKind } from "./transition-store";

/**
 * 目标页进入 hook：
 * - 若 store 中有匹配的形变载荷：首帧隐藏落点元素并上报其 rect，
 *   Provider 形变完成清空 store 后再显示真实元素；
 * - 无论是否有载荷，[data-enter] 标记的内容块错峰淡入
 *  （浏览器前进/后退、无共享元素的通用导航均走此进入动画）；水合时 SSR 已经画出的
 *   块不重播，见 useReveal（#537）。
 * 返回绑定到落点元素的 ref。
 */
export function usePageEnter<T extends HTMLElement>(
  kind: MorphKind | null,
  matchId?: string
) {
  const landingRef = useRef<T>(null);
  const state = useSyncExternalStore(
    morphStore.subscribe,
    morphStore.get,
    morphStore.getServer
  );

  // 挂载：匹配载荷 → 隐藏落点并上报 rect（同步于首帧绘制前完成）
  useEffect(() => {
    const payload = morphStore.peek().payload;
    const el = landingRef.current;
    if (
      payload &&
      kind &&
      payload.kind === kind &&
      (!matchId || payload.id === matchId) &&
      el
    ) {
      gsap.set(el, { autoAlpha: 0 });
      const r = el.getBoundingClientRect();
      morphStore.setLanding({ x: r.x, y: r.y, w: r.width, h: r.height });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 形变结束（store 清空）：显示真实落点
  useEffect(() => {
    if (!state.payload && landingRef.current) {
      gsap.set(landingRef.current, { autoAlpha: 1 });
    }
  }, [state.payload]);

  // 其余内容错峰淡入
  useReveal([], () => ({
    y: 16,
    autoAlpha: 0,
    duration: 0.5,
    ease: "power2.out",
    stagger: 0.05,
    delay: morphStore.peek().payload !== null ? 0.4 : 0.05,
  }));

  return landingRef;
}
