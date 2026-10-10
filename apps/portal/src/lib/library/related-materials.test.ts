import { describe, expect, it } from "vitest";

import type { Material } from "./mock";
import { relatedMaterials } from "./related-materials";

function material(id: string, subject: string, type: Material["type"]): Material {
  return { id, subject, type, title: id } as Material;
}

const current = material("cur", "高等数学A（二）", "exam");

describe("relatedMaterials", () => {
  it("puts same-subject materials before same-type ones and excludes the current item", () => {
    const catalog = [
      material("java-exam", "Java程序设计", "exam"),
      current,
      material("math-notes", "高等数学A（二）", "handout"),
      material("math-exam", "高等数学A（二）", "exam"),
    ];
    expect(relatedMaterials(catalog, current).map((m) => m.id)).toEqual(["math-notes", "math-exam", "java-exam"]);
  });

  it("does not pad with same-type items when the subject already fills every slot", () => {
    const catalog = [
      material("java-exam", "Java程序设计", "exam"),
      ...["a", "b", "c", "d"].map((id) => material(id, "高等数学A（二）", "handout")),
    ];
    expect(relatedMaterials(catalog, current).map((m) => m.id)).toEqual(["a", "b", "c"]);
  });

  it("tops up with same-type items only for the remaining slots", () => {
    const catalog = [
      material("java-exam", "Java程序设计", "exam"),
      material("c-exam", "C语言", "exam"),
      material("math-notes", "高等数学A（二）", "handout"),
      material("physics-notes", "大学物理", "handout"),
    ];
    expect(relatedMaterials(catalog, current).map((m) => m.id)).toEqual(["math-notes", "java-exam", "c-exam"]);
  });

  it("returns nothing when nothing is related", () => {
    expect(relatedMaterials([material("x", "大学物理", "handout")], current)).toEqual([]);
  });
});
