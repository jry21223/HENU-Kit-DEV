import assert from "node:assert/strict";
import test from "node:test";

import { deriveMasteryVisuals } from "./bank-hero-mastery.ts";

const mastery = {
  subjects: [
    { label: "数据结构", value: 80 },
    { label: "高等数学", value: 60 },
    { label: "线性代数", value: 40 },
    { label: "操作系统", value: 20 },
  ],
  accuracy: 75,
  streakDays: 27,
  totalQuestions: 500,
};

test("derives one shared encoding for animated and reduced-motion heroes", () => {
  const visuals = deriveMasteryVisuals(mastery);

  assert.deepEqual(visuals.ringSubjects, mastery.subjects.slice(0, 3));
  assert.ok(Math.abs(visuals.coverage - 0.5) < Number.EPSILON);
  assert.equal(visuals.cubeCount, 3);
  assert.ok(Math.abs(visuals.orbitRadius - 2.4) < Number.EPSILON * 4);
});

test("clamps visual inputs at their supported boundaries", () => {
  const visuals = deriveMasteryVisuals({
    ...mastery,
    subjects: [{ label: "异常值", value: 140 }],
    streakDays: 200,
    totalQuestions: -1,
  });

  assert.deepEqual(visuals.ringSubjects, [{ label: "异常值", value: 100 }]);
  assert.equal(visuals.coverage, 1);
  assert.equal(visuals.cubeCount, 4);
  assert.equal(visuals.orbitRadius, 2.2);
});
