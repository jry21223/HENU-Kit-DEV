import { describe, expect, it } from "vitest";
import {
  DRAG_MINIMUM_PX,
  IDLE_WHEEL_BURST,
  NET_DISPLACEMENT_RATIO,
  PEAK_VELOCITY_PX_PER_SECOND,
  VELOCITY_DISPLACEMENT_RATIO,
  WHEEL_BURST_GAP_MS,
  gestureIntent,
  netDisplacementAfter,
  readingDirection,
  WHEEL_BURST_SPAN_MS,
  wheelBurstBlocked,
  wheelBurstStep,
  wheelBurstStepped,
  type WheelBurstState,
} from "./gesture-intent";

/** 1024×800 视口：6% = 48px、3% = 24px，都是整数，边界才好写。 */
const VIEWPORT = 800;
const STEP_THRESHOLD = VIEWPORT * NET_DISPLACEMENT_RATIO;
const VELOCITY_FLOOR = VIEWPORT * VELOCITY_DISPLACEMENT_RATIO;

/** 触摸手势：负位移 = 手指上滑 = 读者想往下读。 */
const touch = (netDisplacement: number, peakVelocity = 0, viewportHeight = VIEWPORT) =>
  gestureIntent({ source: "touch", netDisplacement, peakVelocity, viewportHeight });

describe("readingDirection", () => {
  it("normalises the two sign conventions to 'the reader is going down'", () => {
    // 滚轮的 deltaY 就是页面滚动量：正数 = 向下。
    expect(readingDirection("wheel", 40)).toBe(1);
    expect(readingDirection("wheel", -40)).toBe(-1);
    // 触摸/指针给的是手指位移：负数 = 手指上滑 = 页面向下。
    expect(readingDirection("touch", -40)).toBe(1);
    expect(readingDirection("touch", 40)).toBe(-1);
  });

  it("gives the same direction for the same physical intent", () => {
    expect(readingDirection("wheel", 40)).toBe(readingDirection("touch", -40));
    expect(readingDirection("wheel", -40)).toBe(readingDirection("touch", 40));
  });
});

describe("netDisplacementAfter", () => {
  it("adds movement that keeps the direction", () => {
    expect(netDisplacementAfter(0, -16)).toBe(-16);
    expect(netDisplacementAfter(-16, -8)).toBe(-24);
    expect(netDisplacementAfter(24, 8)).toBe(32);
  });

  it("restarts from the new direction when the reader turns around", () => {
    expect(netDisplacementAfter(-120, 25)).toBe(25);
    expect(netDisplacementAfter(25, -300)).toBe(-300);
  });

  it("leaves the running total alone when a sample brings no movement", () => {
    expect(netDisplacementAfter(-30, 0)).toBe(-30);
  });
});

describe("gestureIntent", () => {
  it("steps once the net displacement reaches 6% of the viewport", () => {
    expect(touch(-STEP_THRESHOLD)).toEqual({ action: "step", direction: 1 });
    expect(touch(-STEP_THRESHOLD * 4)).toEqual({ action: "step", direction: 1 });
    expect(touch(STEP_THRESHOLD)).toEqual({ action: "step", direction: -1 });
  });

  it("does not step just below 6% when the flick is not there either", () => {
    expect(touch(-(STEP_THRESHOLD - 1))).toEqual({ action: "none" });
    expect(touch(STEP_THRESHOLD - 1)).toEqual({ action: "none" });
  });

  it("lets a fast flick through above the 3% floor", () => {
    expect(touch(-VELOCITY_FLOOR, -PEAK_VELOCITY_PX_PER_SECOND)).toEqual({
      action: "step",
      direction: 1,
    });
    // 速度够，但位移还不到速度通道的下限。
    expect(touch(-(VELOCITY_FLOOR - 1), -PEAK_VELOCITY_PX_PER_SECOND)).toEqual({
      action: "none",
    });
    // 位移过了下限，但速度差一点。
    expect(touch(-VELOCITY_FLOOR, -(PEAK_VELOCITY_PX_PER_SECOND - 1))).toEqual({
      action: "none",
    });
  });

  it("says no when neither channel is enough", () => {
    expect(touch(-20, -300)).toEqual({ action: "none" });
    expect(touch(-40)).toEqual({ action: "none" });
    expect(touch(-4, 0)).toEqual({ action: "none" });
  });

  it("reads the same pixels differently on a taller or shorter viewport", () => {
    // 40px 在 800 高的视口里没过 6%（48px），在 600 高的视口里过了（36px）。
    expect(touch(-40, 0, 800)).toEqual({ action: "none" });
    expect(touch(-40, 0, 600)).toEqual({ action: "step", direction: 1 });
    // 反过来：50px 在 800 高里过了（48px），在 1000 高里没过（60px）。
    expect(touch(-50, 0, 800)).toEqual({ action: "step", direction: 1 });
    expect(touch(-50, 0, 1000)).toEqual({ action: "none" });
  });

  it("falls back to no step when the viewport height is unusable", () => {
    for (const viewportHeight of [0, -800, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(touch(-400, -2000, viewportHeight)).toEqual({ action: "none" });
    }
  });

  it("never steps below the drag minimum, however short the viewport is", () => {
    // 100px 高的视口：6% 只有 6px，比例门槛不能低于「构成拖动」的 8px。
    expect(touch(-(DRAG_MINIMUM_PX - 1), -5000, 100)).toEqual({ action: "none" });
    expect(touch(-DRAG_MINIMUM_PX, 0, 100)).toEqual({ action: "step", direction: 1 });
  });

  it("keeps a displacement below the drag minimum out of the judgement", () => {
    expect(touch(-(DRAG_MINIMUM_PX - 1))).toEqual({ action: "none" });
    expect(touch(-(DRAG_MINIMUM_PX - 1), -5000)).toEqual({ action: "none" });
  });

  it("ignores velocity noise instead of reading it as a flick", () => {
    for (const peakVelocity of [
      Number.NaN,
      Number.POSITIVE_INFINITY,
      Number.NEGATIVE_INFINITY,
      599.9,
      0,
    ]) {
      expect(touch(-30, peakVelocity)).toEqual({ action: "none" });
    }
  });

  it("takes the direction from the displacement, not from the velocity's sign", () => {
    expect(touch(-30, -900)).toEqual({ action: "step", direction: 1 });
    expect(touch(30, 900)).toEqual({ action: "step", direction: -1 });
  });

  it("keeps the wheel's own sign convention", () => {
    expect(
      gestureIntent({
        source: "wheel",
        netDisplacement: STEP_THRESHOLD,
        peakVelocity: 0,
        viewportHeight: VIEWPORT,
      })
    ).toEqual({ action: "step", direction: 1 });
    expect(
      gestureIntent({
        source: "wheel",
        netDisplacement: -STEP_THRESHOLD,
        peakVelocity: 0,
        viewportHeight: VIEWPORT,
      })
    ).toEqual({ action: "step", direction: -1 });
  });
});

/**
 * 滚轮的一次突发只走一屏（#510）。窗口与阈值的实测依据写在 `gesture-intent.ts` 的常量注释
 * 里；这里按阈值两侧钉死判定，端到端只负责断言落在第几屏。
 */
/** 一次突发的第一个 tick 走成之后的状态（`at` 是它的时刻）。 */
const openedAt = (at: number) => {
  const judged = wheelBurstStep(IDLE_WHEEL_BURST, { delta: 40, at });
  return wheelBurstStepped(judged.state, 1);
};

/**
 * 把一串 tick 依次喂进去，**并模拟组件的行为**：判定说走、而且真的走成了（这里假设 `go()`
 * 总能走成）才调 `wheelBurstStepped` 记账。判定本身不消费，消费由起跳那一刻记——这条与
 * #509 触摸端「抬手前什么都不消费」是同一个语义。
 */
const run = (ticks: Array<[delta: number, at: number]>, from = IDLE_WHEEL_BURST) => {
  let state: WheelBurstState = from;
  return ticks.map(([delta, at]) => {
    const judged = wheelBurstStep(state, { delta, at });
    state = judged.state;
    if (judged.intent.action === "step") state = wheelBurstStepped(state, judged.intent.direction);
    return judged.intent;
  });
};

describe("wheelBurstStep", () => {
  it("steps on the first tick of a burst", () => {
    expect(run([[40, 0]])).toEqual([{ action: "step", direction: 1 }]);
    expect(run([[-40, 0]])).toEqual([{ action: "step", direction: -1 }]);
  });

  it("keeps the wheel's own sign convention", () => {
    // deltaY 正数 = 页面向下 = 读者往下读。
    expect(wheelBurstStep(IDLE_WHEEL_BURST, { delta: 40, at: 0 }).intent).toEqual({
      action: "step",
      direction: 1,
    });
    expect(wheelBurstStep(IDLE_WHEEL_BURST, { delta: -40, at: 0 }).intent).toEqual({
      action: "step",
      direction: -1,
    });
  });

  it("consumes the step only once inside the window, however long the tail is", () => {
    // 60 个 tick、delta 40→5、每 33ms 一个：#510 的测量用的就是这一串（真实代码里它把页面
    // 推了两屏）。整个序列 ~1.95s，全在 3500ms 的跨度窗口之内，所以只有第一个 tick 算数。
    const ticks: Array<[number, number]> = Array.from({ length: 60 }, (_, i) => [
      Math.max(5, Math.round(40 * Math.pow(5 / 40, i / 59))),
      i * 33,
    ]);
    const intents = run(ticks);
    expect(intents[0]).toEqual({ action: "step", direction: 1 });
    expect(intents.slice(1)).toEqual(Array.from({ length: 59 }, () => ({ action: "none" })));
  });

  it("does not spend the burst when the step never happened", () => {
    // 判定说走、但组件没走成（`animating` 挡住）时只调 `wheelBurstBlocked`，不调
    // `wheelBurstStepped`：后面的同向 tick 仍然可以走一屏。
    const judged = wheelBurstStep(IDLE_WHEEL_BURST, { delta: 40, at: 0 });
    const blocked = wheelBurstBlocked(judged.state, 0);
    expect(wheelBurstStep(blocked, { delta: 40, at: 200 }).intent).toEqual({
      action: "step",
      direction: 1,
    });
  });

  it("opens a new burst once the silence between ticks reaches the threshold", () => {
    expect(run([[40, WHEEL_BURST_GAP_MS - 1]], openedAt(0))).toEqual([{ action: "none" }]);
    // 达到阈值：新的一次手势，可以再走一屏（读者等补间走完再滚一下就是这个节奏）。
    expect(run([[40, WHEEL_BURST_GAP_MS]], openedAt(0))).toEqual([{ action: "step", direction: 1 }]);
  });

  it("reopens the window once one window's span is reached", () => {
    // 密集的连续滚动：每 50ms 一个 tick（远小于静默界 600ms），所以窗口只能靠跨度合上。
    // 73 个 tick = 0…3600ms，跨过 3500ms 的那一刻正好覆盖。
    const ticks: Array<[number, number]> = Array.from(
      { length: 73 },
      (_, i) => [40, i * 50] as [number, number]
    );
    const steps = run(ticks)
      .map((intent, i) => (intent.action === "step" ? ticks[i][1] : null))
      .filter((at): at is number => at !== null);
    // 窗口之内一个都不再消费，达到跨度的那一刻重新起跳——连续滚动大约每 3.5s 一屏。
    expect(steps).toEqual([0, WHEEL_BURST_SPAN_MS]);
  });

  it("never lets the same direction step twice in one window, even after turning around", () => {
    // 读者在同一个窗口里来回翻转：+1 走过一屏之后，之后再怎么翻回 +1 都不再算数，
    // -1 还可以走一屏。所以一个窗口最多两屏，而且是「一上一下」。
    const intents = run(
      [
        [-40, 100],
        [-40, 200],
        [40, 300],
        [-40, 400],
        [40, 500],
        [40, 600],
      ],
      openedAt(0)
    );
    expect(intents).toEqual([
      { action: "step", direction: -1 },
      { action: "none" },
      { action: "none" },
      { action: "none" },
      { action: "none" },
      { action: "none" },
    ]);
  });

  it("leaves the state it was given untouched", () => {
    const opened = openedAt(1000);
    const snapshot = { ...opened };
    wheelBurstStep(opened, { delta: 40, at: 1100 });
    wheelBurstStepped(opened, 1);
    wheelBurstBlocked(opened, 1200);
    expect(opened).toEqual(snapshot);
  });

  it("does not read a zero delta as a direction", () => {
    // deltaY 为 0 不是一次滚动；`readingDirection` 把它归成「往上」，与改造前一致。
    expect(wheelBurstStep(IDLE_WHEEL_BURST, { delta: 0, at: 0 }).intent).toEqual({
      action: "step",
      direction: -1,
    });
  });
});

/**
 * 被 `animating` 吞掉的 tick 要把窗口重新起算（#510）：补间在负载高时会比名义的 1.1s 长得多、
 * 甚至长过跨度窗口，不这样做，尾巴上落在补间结束之后的 tick 就会被当成新的一次手势。
 */
describe("wheelBurstBlocked", () => {
  it("restarts the window from the tick the animation swallowed", () => {
    const opened = openedAt(0);
    // 动画一直占着，读者一直在滚：每 800ms 被吞一下，窗口就被顺延一次。
    const blocked = wheelBurstBlocked(wheelBurstBlocked(opened, 3000), 3800);
    expect(blocked.startedAt).toBe(3800);
    // 不记窗口的话，3800 已经超过 0 + 3500，会被当成新的一次手势；记了就不是。
    expect(wheelBurstStep(blocked, { delta: 40, at: 4000 }).intent).toEqual({ action: "none" });
    expect(wheelBurstStep(opened, { delta: 40, at: 3600 }).intent).toEqual({
      action: "step",
      direction: 1,
    });
  });

  it("keeps the other direction open for a reader who turns around mid-animation", () => {
    const opened = openedAt(0);
    const blocked = wheelBurstBlocked(opened, 1200);
    // 补间期间读者反手往上滚：新方向可以走一屏（原来那个方向已经走过一屏）。
    expect(wheelBurstStep(blocked, { delta: -40, at: 1250 }).intent).toEqual({
      action: "step",
      direction: -1,
    });
  });
});
