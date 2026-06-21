import assert from "node:assert/strict";
import { buildWeakPointStats } from "../../src/lib/wrong-questions";

const stats = buildWeakPointStats([
  {
    course: { id: "discrete-math", name: "离散数学" },
    knowledgePointId: "logic",
    knowledgePointTitle: "命题逻辑",
  },
  {
    course: { id: "discrete-math", name: "离散数学" },
    knowledgePointId: "logic",
    knowledgePointTitle: "命题逻辑",
  },
  {
    course: { id: "discrete-math", name: "离散数学" },
    knowledgePointId: "graph",
    knowledgePointTitle: "图论基础",
  },
  {
    course: { id: "probability-statistics-a", name: "概率论与数理统计A" },
    knowledgePointId: null,
    knowledgePointTitle: null,
  },
]);

assert.equal(stats.length, 3);
assert.equal(stats[0].knowledgePointId, "logic");
assert.equal(stats[0].wrongCount, 2);
assert.equal(stats[1].wrongCount, 1);
assert.equal(
  stats.some((item) => item.knowledgePointTitle === "未关联知识点"),
  true,
);

console.log("wrong question unit tests passed");
