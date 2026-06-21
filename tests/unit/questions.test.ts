import assert from "node:assert/strict";
import {
  formatAnswerForDisplay,
  isAnswerCorrect,
  isSupportedAnswerInput,
  toPublicQuestion,
} from "../../src/lib/questions";

const publicQuestion = toPublicQuestion({
  id: "question-1",
  courseId: "course-1",
  knowledgePointId: "kp-1",
  knowledgePointTitle: "命题逻辑",
  type: "single_choice",
  stem: "P -> Q 等价于什么？",
  options: [
    { id: "A", text: "非 P 或 Q" },
    { id: "B", text: "P 且 Q" },
  ],
  difficulty: 1,
});

assert.equal(publicQuestion.id, "question-1");
assert.equal(publicQuestion.type, "single_choice");
assert.equal(publicQuestion.options.length, 2);
assert.equal("answer" in publicQuestion, false);
assert.equal("explanation" in publicQuestion, false);

assert.equal(isAnswerCorrect("A", "A"), true);
assert.equal(isAnswerCorrect("a", "A"), true);
assert.equal(isAnswerCorrect("B", "A"), false);
assert.equal(isAnswerCorrect("true", "true"), true);
assert.equal(isAnswerCorrect(false, "true"), false);
assert.equal(isAnswerCorrect(["B", "A"], ["a", "b"]), true);

assert.equal(isSupportedAnswerInput("A"), true);
assert.equal(isSupportedAnswerInput(""), false);
assert.equal(isSupportedAnswerInput([]), false);
assert.equal(isSupportedAnswerInput(["A", "B"]), true);

assert.equal(
  formatAnswerForDisplay("A", [
    { id: "A", text: "非 P 或 Q" },
    { id: "B", text: "P 且 Q" },
  ]),
  "A. 非 P 或 Q",
);

console.log("question unit tests passed");
