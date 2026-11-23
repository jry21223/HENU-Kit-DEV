"use client";

import { useSyncExternalStore } from "react";

/**
 * 动效分档（按偏好与设备能力，不按屏宽）：
 * - static：用户开了「减少动态效果」或省流量，只留静态画面；
 * - standard：手机与普通设备的默认档，入场、展开、按压、描线和场景循环都保留，
 *   循环只在可见时播放；只是不加载 WebGL；
 * - full：在 standard 之上再加首页 3D 线框（WebGL），只给硬件够用的设备。
 */
export type MotionTier = "static" | "standard" | "full";

export interface MotionSignals {
  reducedMotion: boolean;
  saveData: boolean;
  coarsePointer: boolean;
  /** navigator.hardwareConcurrency；0 或负数表示浏览器没有报告。 */
  hardwareConcurrency: number;
  /** navigator.deviceMemory（GB）；0 或负数表示浏览器没有报告。 */
  deviceMemory: number;
}

/** 没报告的硬件信息不当成弱设备；报告了就按门槛判断。 */
function atLeast(value: number, threshold: number): boolean {
  return value <= 0 || value >= threshold;
}

export function detectMotionTier(signals: MotionSignals): MotionTier {
  const { reducedMotion, saveData, coarsePointer, hardwareConcurrency, deviceMemory } = signals;
  if (reducedMotion || saveData) return "static";
  // 触屏设备跑 WebGL 更费电：要求报告出来的核数至少 6，未报告时不开 3D。
  const coresOk = coarsePointer ? hardwareConcurrency >= 6 : atLeast(hardwareConcurrency, 4);
  if (coresOk && atLeast(deviceMemory, 4)) return "full";
  return "standard";
}

const REDUCED = "(prefers-reduced-motion: reduce)";
const COARSE = "(pointer: coarse)";

type CapabilityNavigator = Navigator & {
  connection?: EventTarget & { saveData?: boolean };
  deviceMemory?: number;
};

function subscribe(onChange: () => void): () => void {
  const queries = [REDUCED, COARSE].map((query) => window.matchMedia(query));
  const { connection } = navigator as CapabilityNavigator;
  queries.forEach((query) => query.addEventListener("change", onChange));
  connection?.addEventListener("change", onChange);
  return () => {
    queries.forEach((query) => query.removeEventListener("change", onChange));
    connection?.removeEventListener("change", onChange);
  };
}

function getSnapshot(): MotionTier {
  const { connection, deviceMemory, hardwareConcurrency } = navigator as CapabilityNavigator;
  return detectMotionTier({
    reducedMotion: window.matchMedia(REDUCED).matches,
    saveData: connection?.saveData ?? false,
    coarsePointer: window.matchMedia(COARSE).matches,
    hardwareConcurrency: hardwareConcurrency ?? 0,
    deviceMemory: deviceMemory ?? 0,
  });
}

/** 服务端不知道设备：按 standard 渲染，不在首屏先画 WebGL 再撤掉。 */
function getServerSnapshot(): MotionTier {
  return "standard";
}

export function useMotionCapability(): MotionTier {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
