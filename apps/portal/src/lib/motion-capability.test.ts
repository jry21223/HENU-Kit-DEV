import { describe, expect, it } from "vitest";
import { detectMotionTier, type MotionSignals } from "./motion-capability";

const desktop: MotionSignals = {
  reducedMotion: false,
  saveData: false,
  coarsePointer: false,
  hardwareConcurrency: 8,
  deviceMemory: 8,
};

const phone: MotionSignals = { ...desktop, coarsePointer: true, hardwareConcurrency: 4, deviceMemory: 0 };

describe("detectMotionTier", () => {
  it.each([{ reducedMotion: true }, { saveData: true }])("prioritizes static for %j", (signals) => {
    expect(detectMotionTier({ ...desktop, ...signals })).toBe("static");
    expect(detectMotionTier({ ...phone, ...signals })).toBe("static");
  });

  it("keeps standard motion on a phone instead of switching animation off", () => {
    expect(detectMotionTier(phone)).toBe("standard");
  });

  it("enables WebGL on a touch device only when it reports enough cores and memory", () => {
    expect(detectMotionTier({ ...phone, hardwareConcurrency: 8, deviceMemory: 6 })).toBe("full");
    expect(detectMotionTier({ ...phone, hardwareConcurrency: 8, deviceMemory: 2 })).toBe("standard");
    expect(detectMotionTier({ ...phone, hardwareConcurrency: 0 })).toBe("standard");
  });

  it.each([{ hardwareConcurrency: 2 }, { deviceMemory: 2 }])("keeps weak desktops on standard for %j", (signals) => {
    expect(detectMotionTier({ ...desktop, ...signals })).toBe("standard");
  });

  it("does not treat unreported desktop hardware as slow hardware", () => {
    expect(detectMotionTier({ ...desktop, hardwareConcurrency: 0, deviceMemory: 0 })).toBe("full");
    expect(detectMotionTier({ ...desktop, hardwareConcurrency: -1, deviceMemory: -1 })).toBe("full");
  });
});
