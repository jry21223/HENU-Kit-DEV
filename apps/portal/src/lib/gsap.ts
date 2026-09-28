"use client";

import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { ScrollToPlugin } from "gsap/ScrollToPlugin";
import { Observer } from "gsap/Observer";
import { useGSAP } from "@gsap/react";

if (typeof window !== "undefined") {
  gsap.registerPlugin(ScrollTrigger, ScrollToPlugin, Observer, useGSAP);
}

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";
const FINE_MOTION = "(prefers-reduced-motion: no-preference)";

/**
 * 元素此刻是否有一部分在视口里（#557）。滚动触发的 from() 入场一创建就把元素设成初始的
 * 隐藏态；页面从中间位置加载（刷新后浏览器恢复滚动位置）时，视口里的模块是服务端已经画好的
 * 内容，这时不再创建入场，免得它先消失再出现。
 */
function isOnScreen(element: Element | null): boolean {
  if (!element) return false;
  const box = element.getBoundingClientRect();
  return box.bottom > 0 && box.top < window.innerHeight;
}

export { gsap, ScrollTrigger, useGSAP, REDUCED_MOTION, FINE_MOTION, isOnScreen };
