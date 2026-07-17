import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import ts from "typescript";

const importTypeScriptModule = async (relativePath) => {
  const sourcePath = path.resolve(import.meta.dirname, relativePath);
  const source = readFileSync(sourcePath, "utf8");
  const output = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2022,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const compiledPath = path.join(
    tmpdir(),
    `quiz-card-state-${Date.now()}-${Math.random().toString(16).slice(2)}.mjs`,
  );
  writeFileSync(compiledPath, output);
  return import(pathToFileURL(compiledPath));
};

const { getQuestionOptionKey } = await importTypeScriptModule(
  "../src/pages/quizCardState.ts",
);

test("option keys are scoped to the question so selected styles cannot transition between questions", () => {
  assert.notEqual(
    getQuestionOptionKey("old-question", 1),
    getQuestionOptionKey("new-question", 1),
  );
  assert.equal(getQuestionOptionKey("new-question", 1), "new-question-option-1");
});
