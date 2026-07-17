import assert from "node:assert/strict";
import { mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import * as esbuild from "esbuild";

const outdir = join(tmpdir(), "quizcraft-auto-advance-test");
mkdirSync(outdir, { recursive: true });

const outfile = join(outdir, `quizAutoAdvance-${Date.now()}.mjs`);
await esbuild.build({
  entryPoints: ["src/utils/quizAutoAdvance.ts"],
  outfile,
  bundle: true,
  platform: "node",
  format: "esm",
  logLevel: "silent",
});

const module = await import(pathToFileURL(outfile).href);
const shouldAutoAdvanceAfterAnswer = module.shouldAutoAdvanceAfterAnswer;

assert.equal(
  shouldAutoAdvanceAfterAnswer({
    isCorrect: true,
    currentIndex: 0,
    questionCount: 2,
  }),
  true,
  "correct answers before the last question should auto-advance",
);

assert.equal(
  shouldAutoAdvanceAfterAnswer({
    isCorrect: false,
    currentIndex: 0,
    questionCount: 2,
  }),
  false,
  "wrong answers should stay on the current question",
);

assert.equal(
  shouldAutoAdvanceAfterAnswer({
    isCorrect: true,
    currentIndex: 1,
    questionCount: 2,
  }),
  false,
  "the final question should not auto-advance to the result screen",
);

assert.equal(
  shouldAutoAdvanceAfterAnswer({
    isCorrect: true,
    currentIndex: -1,
    questionCount: 2,
  }),
  false,
  "invalid indexes should not auto-advance",
);
