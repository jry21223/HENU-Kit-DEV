import { describe, expect, it } from "vitest";

import { MATERIAL_TYPES } from "./material-types";

describe("Library material types", () => {
  it("matches the publishable HENU-Final-Review canonical roles one-to-one", () => {
    expect(MATERIAL_TYPES).toEqual({
      handout: { name: "复习讲义" },
      exam: { name: "往年真题" },
      slides: { name: "课件" },
      exercise: { name: "题库练习" },
      answer: { name: "答案解析" },
      note: { name: "笔记总结" },
      textbook: { name: "电子版教材" },
    });
  });

  it("does not expose retired synthetic filters", () => {
    expect(Object.keys(MATERIAL_TYPES)).not.toEqual(
      expect.arrayContaining(["mock", "path", "lab"]),
    );
  });
});
