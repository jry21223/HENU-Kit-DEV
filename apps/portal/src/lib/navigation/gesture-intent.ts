/**
 * 整屏吸附的**手势意图**判定（#509）。
 *
 * 一次手势只判一次：松手（`onRelease`）时看这次手势的**净位移**与**峰值速度**，决定要不
 * 要走一屏、往哪个方向走。整屏吸附原来的判据只有 Observer 的 `tolerance: 12`——那是**桶
 * 累计**阈值：位移累加到 12px 就回调一次 `onChangeY` 并把桶清零（`Observer.js:190-193`、
 * `203-210`）。于是十几像素的手抖、误触、点击前的位移都会翻一整屏。本模块要修的是**过灵
 * 敏**，不是「慢拖不动」：逐帧的小位移会被桶累加，8px/帧 × 20 帧的慢速长拖动一直都能切屏
 * （#507 已实测证伪「慢拖失效」，不要再复用那种说法）。
 *
 * 门槛按**视口高的比例**取（#507 已决定），不写死像素数：同一段手势在不同屏高下落在同一
 * 种判定里。速度通道只负责把「净位移中等但很快」的轻扫放进来——实测单帧 flick 的速度读数
 * 恒为 0，速度不能当主判据。
 *
 * 纯函数：不碰 DOM、不认识 `Observer`。补间、`animating` 锁、滚动写入都留在
 * `snap-scroll.tsx`；速度也只能在**松手回调**里读，因为 Observer 在 `onStop` 之前就把速度
 * 清零了（`Observer.js:183-188`），在 `onStop` 里读出来恒为 0。
 *
 * 滚轮侧本票不动（兄弟票 #510 负责 burst 聚合）：`snap-scroll.tsx` 仍按每个 tick 直接起跳。
 * 这里对 `wheel` 也给出同一套门槛，是为了让「输入符号 → 读者方向」的归一化只有一处，等
 * #510 落地后直接接上。
 */

/** 手势来源：滚轮，或触摸/指针拖动。两套位移的符号约定相反，见 `readingDirection`。 */
export type GestureSource = "wheel" | "touch";

/** 判定结果：走一屏并给出方向，或者什么都不做。 */
export type GestureIntent = { action: "step"; direction: 1 | -1 } | { action: "none" };

/**
 * 构成一次「拖动」的最小位移（px）。与 Observer 的 `dragMinimum` 同义：小于它的位移不进入
 * 拖动状态，松手也顺带把速度清零。比例门槛永远不会低于它——否则矮视口会把「连拖动都不算」
 * 的抖动放行。
 */
export const DRAG_MINIMUM_PX = 8;

/**
 * 净位移门槛：视口高的 6%（1024×800 下 48px）。#507 已决定按视口取比例，不是绝对像素。
 */
export const NET_DISPLACEMENT_RATIO = 0.06;

/**
 * 速度通道的净位移下限：视口高的 3%（1024×800 下 24px）。轻扫靠速度放行，但太短的位移
 * 即使很快也不该翻屏。
 */
export const VELOCITY_DISPLACEMENT_RATIO = 0.03;

/**
 * 速度通道的峰值速度门槛（px/s）。**暂定值，待真机实测校准**：#507 的实测显示同一量级的
 * 手势随帧节奏会在 −59…−960 px/s 之间漂（单帧 flick 恒为 0），这个数只能等真机手感定档后
 * 再调。
 */
export const PEAK_VELOCITY_PX_PER_SECOND = 600;

/**
 * 这次位移在读者眼里是「往下读」还是「往上读」。两种输入的符号约定相反：滚轮的 `deltaY`
 * 就是页面滚动量（正数 = 向下），触摸/指针给的是手指位移（负数 = 手指上滑 = 页面向下）。
 * 只判正负会让触摸端整屏反向——手指上滑退回上一屏、手指下滑反而进下一屏。
 *
 * 位移为 0 时不会走到这里（调用方只在 |位移| 过门槛时取方向）；真取到 0 就归「往上」，与
 * 改造前 `snap-scroll.tsx` 里滚轮路径的 `deltaY > 0` 判断一致。
 */
export function readingDirection(source: GestureSource, displacement: number): 1 | -1 {
  return source === "wheel" ? (displacement > 0 ? 1 : -1) : displacement < 0 ? 1 : -1;
}

/**
 * 把手势里的一段位移并进净位移：**同向累加，反向就以新方向重新起算**（读者最后想做的是
 * 什么，由最后那一段决定）。零位移不动上一次的结果。
 */
export function netDisplacementAfter(previous: number, delta: number): number {
  if (delta === 0) return previous;
  if (previous === 0 || Math.sign(delta) === Math.sign(previous)) return previous + delta;
  return delta;
}

/**
 * 视口比例门槛，但不低于拖动下限：视口极矮时（`viewportHeight × ratio` 小于 8px）以
 * `DRAG_MINIMUM_PX` 为准。
 */
const thresholdFor = (viewportHeight: number, ratio: number) =>
  Math.max(viewportHeight * ratio, DRAG_MINIMUM_PX);

/**
 * 这次松手算不算一次「走一屏」的意图。
 *
 * - 净位移到视口高的 6%：算（慢速长拖动走这条）。
 * - 净位移到 3% 且峰值速度到 600 px/s：算（轻扫走这条）。
 * - 其余：不算。
 *
 * 视口高不是有效正数时不判：比例门槛无从谈起，宁可什么都不做（这也是防误触的方向）。
 */
export function gestureIntent({
  source,
  netDisplacement,
  peakVelocity,
  viewportHeight,
}: {
  source: GestureSource;
  netDisplacement: number;
  peakVelocity: number;
  viewportHeight: number;
}): GestureIntent {
  if (!Number.isFinite(viewportHeight) || viewportHeight <= 0) return { action: "none" };

  const net = Math.abs(netDisplacement);
  if (net >= thresholdFor(viewportHeight, NET_DISPLACEMENT_RATIO)) {
    return { action: "step", direction: readingDirection(source, netDisplacement) };
  }

  const flick =
    Number.isFinite(peakVelocity) && Math.abs(peakVelocity) >= PEAK_VELOCITY_PX_PER_SECOND;
  if (flick && net >= thresholdFor(viewportHeight, VELOCITY_DISPLACEMENT_RATIO)) {
    return { action: "step", direction: readingDirection(source, netDisplacement) };
  }

  return { action: "none" };
}
