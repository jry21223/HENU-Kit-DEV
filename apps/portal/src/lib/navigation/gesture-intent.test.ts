import { describe, expect, it } from "vitest";
import {
  DRAG_MINIMUM_PX,
  NET_DISPLACEMENT_RATIO,
  PEAK_VELOCITY_PX_PER_SECOND,
  VELOCITY_DISPLACEMENT_RATIO,
  gestureIntent,
  netDisplacementAfter,
  readingDirection,
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
