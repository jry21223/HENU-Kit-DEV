import assert from "node:assert/strict";
import {
  formatAnswerForDisplay,
  isAnswerCorrect,
  isSupportedAnswerInput,
  normalizeOptions,
  toPublicQuestion,
} from "../../src/lib/questions";

const rawQuestion = {
  id: "question-1",
  courseId: "course-1",
  knowledgePointId: "kp-1",
  knowledgePointTitle: "Logic",
  type: "single_choice" as const,
  stem: "Which option is equivalent to P -> Q?",
  options: [
    { id: "A", text: "not P or Q" },
    { id: "B", text: "P and Q" },
  ],
  difficulty: 1,
  answer: "A",
  explanation: "Implication equivalence.",
};

const publicQuestion = toPublicQuestion(rawQuestion);

assert.equal(publicQuestion.id, "question-1");
assert.equal(publicQuestion.type, "single_choice");
assert.equal(publicQuestion.options.length, 2);
assert.equal("answer" in publicQuestion, false);
assert.equal("explanation" in publicQuestion, false);

assert.deepEqual(
  normalizeOptions([
    { id: "A", text: "valid" },
    { id: "B", text: 123 },
    null,
    "bad",
  ]),
  [{ id: "A", text: "valid" }],
);

assert.equal(isAnswerCorrect("A", "A"), true);
assert.equal(isAnswerCorrect("a", "A"), true);
assert.equal(isAnswerCorrect(" B ", "A"), false);
assert.equal(isAnswerCorrect("true", "true"), true);
assert.equal(isAnswerCorrect(false, "true"), false);
assert.equal(isAnswerCorrect(["B", "A"], ["a", "b"]), true);
assert.equal(isAnswerCorrect(["A", "A"], ["A"]), false);
assert.equal(isAnswerCorrect("", "A"), false);

assert.equal(isSupportedAnswerInput("A"), true);
assert.equal(isSupportedAnswerInput(1), true);
assert.equal(isSupportedAnswerInput(true), true);
assert.equal(isSupportedAnswerInput(""), false);
assert.equal(isSupportedAnswerInput([]), false);
assert.equal(isSupportedAnswerInput(["A", "B"]), true);
assert.equal(isSupportedAnswerInput(["A", {}]), false);
assert.equal(isSupportedAnswerInput({ value: "A" }), false);

assert.equal(
  formatAnswerForDisplay("A", [
    { id: "A", text: "not P or Q" },
    { id: "B", text: "P and Q" },
  ]),
  "A. not P or Q",
);
assert.equal(formatAnswerForDisplay("true", []), "正确");
assert.equal(formatAnswerForDisplay("false", []), "错误");

console.log("question unit tests passed");
