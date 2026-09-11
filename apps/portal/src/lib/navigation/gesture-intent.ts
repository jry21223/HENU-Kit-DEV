/**
 * 整屏吸附的**手势意图**判定（#509）。
 *
 * 一次手势只判一次：松手（`onRelease`）时看这次手势的**净位移**与**峰值速度**，决定要不
 * 要走一屏、往哪个方向走。整屏吸附原来的判据只有 Observer 的 `tolerance: 12`——那是**桶
 * 累计**阈值：位移累加到 12px 就回调一次 `onChangeY` 并把桶清零
 * （`gsap/src/Observer.js:190-193`、`:203-210`）。于是十几像素的手抖、误触、点击前的位移
 * 都会翻一整屏。本模块要修的是**过灵敏**，不是「慢拖不动」：逐帧的小位移会被桶累加，
 * 8px/帧 × 20 帧的慢速长拖动一直都能切屏（#507 已实测证伪「慢拖失效」，不要再复用那种
 * 说法）。
 *
 * 门槛按**视口高的比例**取（#507 已决定），不写死像素数：同一段手势在不同屏高下落在同一
 * 种判定里。速度通道只负责把「净位移中等但很快」的轻扫放进来——实测单帧 flick 的速度读数
 * 恒为 0，速度不能当主判据。
 *
 * 纯函数：不碰 DOM、不认识 `Observer`。补间、`animating` 锁、滚动写入都留在
 * `snap-scroll.tsx`；速度也只能在**松手回调**里读，因为 Observer 在 `onStop` 之前就把速度
 * 清零了（同文件 `:183-188`），在 `onStop` 里读出来恒为 0。
 *
 * 滚轮侧走**另一条通道**（#510 的 `wheelBurstStep`）：滚轮没有「松手」事件、也读不到速度，
 * 所以用时间窗把一次突发聚合成一次判定。两条通道共用 `readingDirection` 的方向归一化。
 *
 * 引用的 `Observer.js:NNN` 都指 `node_modules/gsap/src/Observer.js`（gsap 3.15.0 的可读源
 * 码）：`import "gsap/Observer"` 解析到的是它的构建产物，同一个文件里行号不同。
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
 * `peakVelocity` 是**松手那一刻**的速度读数，不是整段手势的最大值：意图只能在松手时定下
 * 来，而 Observer 的速度也只有那一刻读得到（`onStop` 之前已被清零）。
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

/**
 * 滚轮突发里两个 tick 之间的最大间隔（ms）：**静默超过它就是一个新突发**。
 *
 * 取 1500ms 的依据：这个界要挡住的是「同一次物理滚动在补间结束后又被算一次」，而补间本身
 * 就要 1.1s（`snap-scroll.tsx` 的 `duration: 1.1`，实测 ~1.07s）——一次突发至少要能安静地
 * 跨过整段补间，否则尾巴上任何一个落在补间结束之后的 tick 都会另开一次手势，那正是 #510 要
 * 修的缺陷。1500ms ≈ 补间时长的 1.36 倍，留出实测抖动与慢帧的余量（本机合成的突发内部间隔
 * 实测 min 25.2 / 中位 33.3–41.7 / max 52.6ms，机器忙时更长）。
 *
 * 另一头不能无限大：离散鼠标滚轮刻意滚动、以及读者真的在连续滚时，刻度之间隔的是「一屏动画
 * 走完」的量级。1500ms 落在两种物理现实之间——触控板的惯性尾巴是连续流（间隔远小于它），
 * 刻意一屏一屏滚则要等补间结束再看下一屏（间隔大于它）。
 */
export const WHEEL_BURST_GAP_MS = 1500;

/**
 * 一次滚轮突发的最长跨度（ms）：从这次突发的第一个 tick 起算，**跨过它之后 tick 重新开窗**。
 *
 * 这个数不能小于整屏补间时长，否则就是留了个缺口：补间 1.1s 一结束、惯性尾巴还在流，下一个
 * tick 就开新窗再跳一屏——那正是 #510 要修的缺陷（实测：一次 60 tick 的衰减序列把页面从第
 * 1 屏推到第 3 屏，第二次起跳由补间结束之后到达的 tick 触发）。
 *
 * 取 2000ms 的依据：补间时长实测 ~1.07s（名义 1.1s，见 `snap-scroll.tsx` 的
 * `duration: 1.1`），窗口要**盖住整段补间**，否则尾巴一定能在补间刚结束时补一屏；而按
 * `40 → 5` 的指数衰减算，delta 掉到 Observer 的 `tolerance: 12` 以下大约在第 1.5s，之后
 * 即使还有 tick 也不再触发判定。2000ms 同时是长于一次惯性尾巴（不再留缺口）与短于「读者还在
 * 有意连续滚」之间的分界：跨度更长的连续滚动会在窗口合上后按新的一次手势再走一屏。
 *
 * 代价写在明处：比真实触控板惯性尾巴更长的连续滚动会在窗口合上后再走一屏——合成事件无法复现
 * 真实触控板的动量曲线，这一条留给 #510 的生产实机验收。
 */
export const WHEEL_BURST_SPAN_MS = 2000;

/**
 * 一次滚轮突发的聚合状态。纯数据：调用方（`snap-scroll.tsx`）持有它并把它交回
 * `wheelBurstStep`，判定本身不碰 DOM、不认识 `Observer`。
 *
 * `active` 为 false 时其余字段无意义；`wheelBurstStep` 遇到新突发会整份重建，不必手工重置。
 */
export type WheelBurstState = {
  active: boolean;
  /** 这次突发的第一个 tick 的时刻（ms，时钟由调用方决定，只需单调）。 */
  startedAt: number;
  /** 上一个被这次突发收下的 tick 的时刻（ms）。 */
  lastTickAt: number;
  /** 这次突发在当前方向上已经消费过判定没有。方向翻转会把它重置。 */
  consumed: boolean;
  /** 这次突发当前的方向；`null` 表示还没定过（第一个 tick 定方向）。 */
  direction: 1 | -1 | null;
};

/** 还没进入任何突发的初始状态。 */
export const IDLE_WHEEL_BURST: WheelBurstState = {
  active: false,
  startedAt: 0,
  lastTickAt: 0,
  consumed: false,
  direction: null,
};

/**
 * 滚轮的一次 tick 该怎么判（#510）——**时间窗聚合**版的「一次手势一屏」。
 *
 * 滚轮没有「松手」事件（`Observer` 的 `onStop` 要等 250ms 静默，且速度已被清零），也读不到
 * 峰值速度，所以判定只能按 tick 的时刻来聚合：
 *
 * - 距上一个 tick 超过 `WHEEL_BURST_GAP_MS`：新的一次手势，开新窗。
 * - 距第一个 tick 超过 `WHEEL_BURST_SPAN_MS`：窗口合上，这一 tick 也开新窗（读者还在滚，
 *   但已经跨过了一整段补间，算他继续往下读的新意图）。
 * - 两者都不是：同一个突发。**只有这个突发的第一个 tick 消费判定**，之后同方向的 tick 一律
 *   不消费——这就是「一次突发一屏」，也是修掉「惯性尾巴在补间结束后又跳一屏」的那道门。
 * - 突发中途反向：方向是读者最新的意图，重置消费，让新方向能走一屏（同一方向仍然只走一屏）。
 *
 * 返回新的状态，不修改传入的那个：判定是纯的，组件那边的 `animating` 锁、补间、滚动写入
 * 照旧。
 *
 * 视口高不参与：比例门槛是触摸端的语义（净位移要跟手指走过的距离比），滚轮这边一次刻度就是
 * 一个明确的方向意图，没有「位移多大才算」这一层。
 */
export function wheelBurstStep(
  state: WheelBurstState,
  tick: { delta: number; at: number }
): { intent: GestureIntent; state: WheelBurstState } {
  const { delta, at } = tick;
  const direction = readingDirection("wheel", delta);
  const fresh =
    !state.active ||
    at - state.lastTickAt > WHEEL_BURST_GAP_MS ||
    at - state.startedAt > WHEEL_BURST_SPAN_MS;

  if (fresh) {
    return {
      intent: { action: "step", direction },
      state: { active: true, startedAt: at, lastTickAt: at, consumed: true, direction },
    };
  }

  const turned = state.direction !== null && direction !== state.direction;
  const before = { ...state, lastTickAt: at, consumed: turned ? false : state.consumed, direction };
  if (before.consumed) return { intent: { action: "none" }, state: before };
  return { intent: { action: "step", direction }, state: { ...before, consumed: true } };
}
