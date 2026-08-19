import assert from "node:assert/strict";
import { mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { test } from "node:test";
import * as esbuild from "esbuild";

// quizStore pulls in zustand and the @/types alias, so it is bundled rather
// than transpiled file-by-file the way quizCardState is.
const outdir = join(tmpdir(), "quizcraft-quiz-store-test");
mkdirSync(outdir, { recursive: true });
const outfile = join(outdir, `quizStore-${Date.now()}.mjs`);

await esbuild.build({
  entryPoints: ["src/stores/quizStore.ts"],
  outfile,
  bundle: true,
  platform: "node",
  format: "esm",
  logLevel: "silent",
  alias: { "@": join(process.cwd(), "src") },
});

const { useQuizStore } = await import(pathToFileURL(outfile).href);

const initialState = { ...useQuizStore.getState() };

function reset() {
  useQuizStore.setState({
    currentBank: "sixiu",
    banks: [],
    user: null,
    practice: null,
    history: [],
    wrongQuestions: [],
  });
}

function question(id) {
  return { id, stem: `stem-${id}`, type: "single", options: [] };
}

function startWith(count) {
  reset();
  const questions = Array.from({ length: count }, (_, index) => question(`q-${index}`));
  useQuizStore.getState().startPractice(questions, "sixiu");
  return questions;
}

test("the store exposes its practice actions", () => {
  assert.equal(typeof initialState.startPractice, "function");
  assert.equal(typeof initialState.answerQuestion, "function");
});

test("startPractice begins an unfinished session at the first question", () => {
  startWith(3);
  const { practice } = useQuizStore.getState();

  assert.equal(practice.bankKey, "sixiu");
  assert.equal(practice.currentIndex, 0);
  assert.equal(practice.isFinished, false);
  assert.deepEqual(practice.answers, {});
});

test("answering records the answer, the result, and the supplied explanation", () => {
  startWith(2);
  useQuizStore.getState().answerQuestion({
    questionId: "q-0",
    answer: "A",
    isCorrect: true,
    correctAnswer: "A",
    analysis: "because",
  });

  const { practice } = useQuizStore.getState();
  assert.equal(practice.answers["q-0"], "A");
  assert.equal(practice.results["q-0"], true);
  assert.equal(practice.correctAnswers["q-0"], "A");
  assert.equal(practice.analyses["q-0"], "because");
});

test("a wrong answer files the question in the wrong-question book exactly once", () => {
  startWith(2);
  const store = useQuizStore.getState();
  store.answerQuestion({ questionId: "q-0", answer: "B", isCorrect: false });
  store.answerQuestion({ questionId: "q-0", answer: "C", isCorrect: false });

  assert.deepEqual(useQuizStore.getState().wrongQuestions, ["q-0"]);
});

test("re-submitting an identical answer does not replace the practice object", () => {
  startWith(2);
  useQuizStore.getState().answerQuestion({ questionId: "q-0", answer: "A", isCorrect: true });
  const before = useQuizStore.getState().practice;

  useQuizStore.getState().answerQuestion({ questionId: "q-0", answer: "A", isCorrect: true });

  // Identity matters: a new object on every keystroke would re-render the card
  // and is what the equality guard in the store exists to prevent.
  assert.equal(useQuizStore.getState().practice, before);
});

test("a multi-select answer is compared by contents, not by reference", () => {
  startWith(2);
  const store = useQuizStore.getState();
  store.answerQuestion({ questionId: "q-0", answer: ["A", "B"], isCorrect: true });
  const before = useQuizStore.getState().practice;

  store.answerQuestion({ questionId: "q-0", answer: ["A", "B"], isCorrect: true });
  assert.equal(useQuizStore.getState().practice, before, "same contents must be a no-op");

  store.answerQuestion({ questionId: "q-0", answer: ["B", "A"], isCorrect: true });
  assert.notEqual(useQuizStore.getState().practice, before, "a different order is a different answer");
});

test("navigation clamps at both ends instead of leaving the question range", () => {
  startWith(3);
  const store = useQuizStore.getState();

  store.prevQuestion();
  assert.equal(useQuizStore.getState().practice.currentIndex, 0);

  store.nextQuestion();
  store.nextQuestion();
  store.nextQuestion();
  store.nextQuestion();
  assert.equal(useQuizStore.getState().practice.currentIndex, 2);
});

test("practice actions are inert when no practice is running", () => {
  reset();
  const store = useQuizStore.getState();

  store.answerQuestion({ questionId: "q-0", answer: "A", isCorrect: true });
  store.nextQuestion();
  store.prevQuestion();
  store.jumpToQuestion(5);
  store.finishPractice();

  assert.equal(useQuizStore.getState().practice, null);
  assert.deepEqual(useQuizStore.getState().wrongQuestions, []);
});

test("finishPractice marks the session finished and resetPractice clears it", () => {
  startWith(2);
  useQuizStore.getState().finishPractice();
  assert.equal(useQuizStore.getState().practice.isFinished, true);

  useQuizStore.getState().resetPractice();
  assert.equal(useQuizStore.getState().practice, null);
});

test("updateUserStats accumulates totals and recomputes the rate", () => {
  reset();
  useQuizStore.getState().setUser({ correct: 0, total: 0, rate: 0 });

  useQuizStore.getState().updateUserStats(1, 2);
  assert.deepEqual(useQuizStore.getState().user, { correct: 1, total: 2, rate: 50 });

  useQuizStore.getState().updateUserStats(1, 2);
  assert.deepEqual(useQuizStore.getState().user, { correct: 2, total: 4, rate: 50 });
});

test("updateUserStats is inert without a user", () => {
  reset();
  useQuizStore.getState().updateUserStats(1, 1);
  assert.equal(useQuizStore.getState().user, null);
});

test("history is newest-first and capped at 1000 entries", () => {
  reset();
  const store = useQuizStore.getState();
  for (let index = 0; index < 1002; index += 1) {
    store.addToHistory({ questionId: `q-${index}` });
  }

  const { history } = useQuizStore.getState();
  assert.equal(history.length, 1000);
  assert.equal(history[0].questionId, "q-1001");
});

test("the wrong-question book removes only the question named", () => {
  reset();
  const store = useQuizStore.getState();
  store.addWrongQuestion("q-0");
  store.addWrongQuestion("q-1");
  store.removeWrongQuestion("q-0");

  assert.deepEqual(useQuizStore.getState().wrongQuestions, ["q-1"]);
});
