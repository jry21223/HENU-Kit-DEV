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
 * 滚轮突发里两个 tick 之间的最大静默（ms）：**达到或超过它就是一个新突发**。
 *
 * 这个界要把两种物理现实分开，两头都有实测值撑着：
 *
 * - **下界：一次惯性尾巴内部的 tick 间隔。** 本机合成的突发实测：页面收到的 wheel 事件间隔
 *   min 12–25 / 中位 17–44 / max 44–90ms（#510 的测量数据；真实触控板由 UA 按帧节流到
 *   60Hz ≈ 16.7ms，只会更密）。判定看到的是 `Observer` 的**桶**回调，比事件更粗：桶累计到
 *   `tolerance: 12` 才回调一次（`Observer.js:190-210`、`:227-233`），尾巴里单个事件只有
 *   5–20px，最坏要 3 个事件才凑一回调，所以判定看到的间隔最坏 ≈ 3 × 90ms ≈ 270ms。600ms 是
 *   这个最坏回调间隔的 2.2 倍，容得下机器被抢占时的成帧抖动。
 * - **上界：读者有意一屏一屏滚的节奏。** 补间名义 1.1s，读者要等它走完、看清落点再滚下一格；
 *   界必须小于这个节奏，否则「等动画走完再滚一下」会被吞掉、滚轮变迟滞（#507 用户故事 9
 *   明确不许）。实测（原始数据见 PR 正文）：把界设成 1500ms 时，1.3s 节奏的 4 个刻度只走了
 *   2 屏；600ms 下 1.3s / 1.5s / 1.8s 三种节奏都是 4 个刻度走 4 屏。
 *
 * 于是两种情形之间有一段安全区（最坏回调间隔 270ms ≪ 600ms ≪ 1.1s 量级的刻意节奏），
 * 都落在正确的一侧。
 */
export const WHEEL_BURST_GAP_MS = 600;

/**
 * 一次滚轮突发里**一个窗口**的最长跨度（ms）。窗口从它收下的第一个 tick 起算，**达到或超过
 * 它就开新窗**（读者还在继续滚，那是继续往下读的新意图，重叠的那一下仍会被动画锁挡在外面）。
 *
 * 取 3500ms 的依据：窗口要盖住「整段补间 + 落在它之后的惯性尾巴」。补间名义 1.1s
 * （`snap-scroll.tsx` 的 `duration: 1.1`）；页面内 rAF 采样到「视野停止移动」为止的实测值是
 * 空载 ~980ms（power2.inOut 尾段位移不足一像素，所以早于名义时长看起来就停了）、帧被节流时
 * ~2000ms；而合成的衰减尾巴实测能到 ~2.5s。3500ms 覆盖了最坏观测值再加一段尾巴。
 *
 * 它同时是「同一次突发」与「读者又滚了一次」的最后一道界，所以不能无穷大：静默界
 * `WHEEL_BURST_GAP_MS` 拦住「停下来又滚」，跨度界只拦住「一直滚个不停」的那种。注意跨度是
 * 从**窗口的第一个 tick（或被 `animating` 吞掉的那一下）**起算的计数——窗口里的普通 tick 只
 * 更新 `lastTickAt`，不会顺延跨度（顺延了的话，连续滚动就永远走不出一个窗口）。所以一个窗口
 * 的墙上时间寿命 ≈ 一次补间 + 3500ms；连续滚动的实际节奏也就是这个量级。代价
 * 写在明处：一直不停手地滚，每个窗口只能走一屏。合成事件无法复现真实触控板的动量曲线，比
 * 3500ms 更长的尾巴会不会多走一屏留给 #510 的生产实机验收。
 */
export const WHEEL_BURST_SPAN_MS = 3500;

/**
 * 一次滚轮突发的聚合状态。纯数据：调用方（`snap-scroll.tsx`）持有它并把它交回
 * `wheelBurstStep` / `wheelBurstStepped` / `wheelBurstBlocked`，判定本身不碰 DOM、不认识
 * `Observer`。
 *
 * `active` 为 false 时其余字段无意义；判定遇到新突发会整份重建，不必手工重置。
 */
export type WheelBurstState = {
  active: boolean;
  /** 当前窗口收下的第一个 tick 的时刻（ms，时钟由调用方决定，只需单调）。 */
  startedAt: number;
  /** 上一个被这个突发收下的 tick 的时刻（ms）。 */
  lastTickAt: number;
  /**
   * 这个窗口已经走了一屏没有。**一个窗口只走一屏**，方向也换不回这一屏：滚轮没有「松手」，
   * 没法像触摸那样把「读者最后想干什么」收敛成一次判定，所以窗口中途反向按同一次手势处理，
   * 读者想往回走就等静默界（600ms）之后重新滚——这与 #510 验收标准里「一次突发只走一屏」
   * 的字面要求一致（本模块把「突发」按窗口切分，所以「一次突发一屏」在这里就是「一个窗口
   * 一屏」）。
   */
  consumed: boolean;
};

/** 还没进入任何突发的初始状态。 */
export const IDLE_WHEEL_BURST: WheelBurstState = {
  active: false,
  startedAt: 0,
  lastTickAt: 0,
  consumed: false,
};

/**
 * 滚轮的一次 tick 该怎么判（#510）——**时间窗聚合**版的「一次手势一屏」。
 *
 * 滚轮没有「松手」事件（`Observer` 的 `onStop` 要等 250ms 静默，且速度已被清零），也读不到
 * 峰值速度，所以判定只能按 tick 的时刻把一次突发聚合起来：
 *
 * - 距上一个 tick 静默达到或超过 `WHEEL_BURST_GAP_MS`，或距当前窗口的第一个 tick 达到或超过
 *   `WHEEL_BURST_SPAN_MS`：新的一次手势，开新窗，可以走一屏。
 * - 两者都不是：同一个突发。这个窗口只消费一次判定，之后的 tick（触控板惯性尾巴、以及窗口内
 *   的反向滚动）一律不消费——这就是「一次突发一屏」，也是修掉「惯性尾巴在补间结束后又跳一屏」
 *   的那道门。
 *
 * **判定本身不消费**：判定说「可以走」而实际没走成（`animating` 挡住、首末屏空转）时，这次
 * 突发不该被花掉——`snap-scroll.tsx` 只在真的起跳之后才调 `wheelBurstStepped` 记账。这也与
 * #509 触摸端「抬手前什么都不消费」的语义对齐。
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
  const inBurst =
    state.active &&
    at - state.lastTickAt < WHEEL_BURST_GAP_MS &&
    at - state.startedAt < WHEEL_BURST_SPAN_MS;

  if (!inBurst) {
    return {
      intent: { action: "step", direction },
      state: { active: true, startedAt: at, lastTickAt: at, consumed: false },
    };
  }

  const next: WheelBurstState = { ...state, lastTickAt: at };
  if (state.consumed) return { intent: { action: "none" }, state: next };
  return { intent: { action: "step", direction }, state: next };
}

/** 这次判定真的起跳了：把窗口标成已消费，之后的 tick 不再消费。 */
export function wheelBurstStepped(state: WheelBurstState): WheelBurstState {
  return { ...state, consumed: true };
}

/**
 * 一次 tick 被 `animating` 吞掉时的收尾：**把这个 tick 记进当前突发，把窗口从它重新起算**。
 *
 * 为什么需要它：补间时长是名义值，帧被节流时会明显变长——#510 用页面内 rAF 采样量到同一段
 * `duration: 1.1` 的补间在空载时约 980ms 就看起来停住了、被抢占时约 2000ms。动画还在跑的时候
 * 读者看不到结果，这段滚动属于同一次意图：不把它记进来的话，窗口会从**动画开始那一刻**起算
 * 3500ms，动画一旦拖长，读者还在流的那段滚动就会被当成新的一次手势——缺陷换个位置又回来。
 * 记进来之后窗口一路覆盖到动画真的走完，而读者仍然要等到静默超过 `WHEEL_BURST_GAP_MS` 才算
 * 新的一次意图。
 *
 * 被吞掉的 tick 可能是同向的，也可能是读者在动画期间反手往回滚的那一下；两种都只是把窗口
 * 顺延，**不记账**（记账只在 `wheelBurstStepped`），所以那次没走成的判定没有被花掉。
 */
export function wheelBurstBlocked(state: WheelBurstState, at: number): WheelBurstState {
  return { ...state, startedAt: at, lastTickAt: at };
}
