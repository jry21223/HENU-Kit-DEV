import { describe, expect, it } from "vitest";

import { publicDisplayName } from "./display-name";

describe("publicDisplayName", () => {
  it.each([undefined, "", "   "])(
    "uses a neutral label when display_name is %j",
    (displayName) => {
      expect(publicDisplayName(displayName)).toBe("用户");
    }
  );

  it("uses the trimmed registration display name", () => {
    expect(publicDisplayName("  小河同学  ")).toBe("小河同学");
  });
});
