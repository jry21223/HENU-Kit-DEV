import { afterEach, describe, expect, it } from "vitest";
import { quizCraftV2ReadsEnabled } from "./env";

const initialFlag = process.env.NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS;

afterEach(() => {
  if (initialFlag === undefined) {
    delete process.env.NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS;
  } else {
    process.env.NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS = initialFlag;
  }
});

describe("quizCraftV2ReadsEnabled", () => {
  it("stays dark unless the explicit cutover flag is exactly 1", () => {
    delete process.env.NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS;
    expect(quizCraftV2ReadsEnabled()).toBe(false);

    process.env.NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS = "true";
    expect(quizCraftV2ReadsEnabled()).toBe(false);

    process.env.NEXT_PUBLIC_PORTAL_ENABLE_QUIZCRAFT_V2_READS = "1";
    expect(quizCraftV2ReadsEnabled()).toBe(true);
  });
});
