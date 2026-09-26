import { describe, expect, it } from "vitest";

import { readableMaterialTitle } from "./material-title";

describe("readableMaterialTitle", () => {
  it("drops the exact subject and type prefix and shows the remaining segments with middle dots", () => {
    expect(
      readableMaterialTitle({
        title: "高等数学_讲义_2020级_第1章_扫描版_含答案",
        subject: "高等数学",
        type: "handout",
      })
    ).toBe("2020级 · 第1章 · 扫描版 · 含答案");
    expect(
      readableMaterialTitle({
        title: "Python程序设计_题库练习_期末复习题_文字版",
        subject: "Python程序设计",
        type: "exercise",
      })
    ).toBe("期末复习题 · 文字版");
  });

  it("keeps underscores inside identifiers such as primary_key", () => {
    expect(
      readableMaterialTitle({
        title: "数据库_笔记_primary_key 与 user_id",
        subject: "数据库",
        type: "note",
      })
    ).toBe("primary_key 与 user_id");
  });

  it("keeps every segment of a title whose prefix is not an exact catalog prefix", () => {
    expect(
      readableMaterialTitle({
        title: "数据库_实验_primary_key_v2",
        subject: "数据库",
        type: "note",
      })
    ).toBe("数据库 · 实验 · primary_key_v2");
    expect(
      readableMaterialTitle({ title: "极限复习笔记", subject: "高等数学", type: "note" })
    ).toBe("极限复习笔记");
  });

  it("does not leave a stray separator at either end", () => {
    expect(
      readableMaterialTitle({ title: "_期末复习_", subject: "高等数学", type: "note" })
    ).toBe("期末复习");
  });
});
