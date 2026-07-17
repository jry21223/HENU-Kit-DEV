import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const quizSource = readFileSync("src/pages/Quiz.tsx", "utf8");

const requiredFragments = [
  'current ? "scale-125 bg-yellow-500',
  "return `${baseClass} bg-primary-500",
  'className="h-full bg-primary-500',
  '? "bg-green-500 dark:bg-green-600 text-white"',
  '? "bg-green-200 dark:bg-green-800/60 text-green-700 dark:text-green-100"',
  '? "bg-red-500 dark:bg-red-600 text-white"',
  '? "bg-primary-500 dark:bg-primary-600 text-white"',
  'className="w-full py-3 bg-primary-500 text-white',
  'className="inline-flex flex-1 items-center justify-center gap-2 rounded-xl bg-primary-500 text-white',
  'className="flex items-center justify-center gap-1 px-4 py-2 bg-primary-500 text-white',
];

const forbiddenFragments = [
  "bg-yellow-50 dark:bg-yellow-900/200",
  "bg-primary-50 dark:bg-primary-900/30 scale-125",
  "h-full bg-primary-50 dark:bg-primary-900/30",
  '? "bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-200"',
  '? "bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-300"',
  '? "bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-200"',
  '? "bg-primary-50 dark:bg-slate-700 text-primary-700 dark:text-slate-100"',
  'className="w-full py-3 bg-primary-50 hover:bg-[#3366BA]',
  'rounded-xl bg-primary-50 dark:bg-slate-500',
  "px-4 py-2 bg-primary-50 hover:bg-[#3366BA]",
];

const literalPattern = (fragment) =>
  new RegExp(fragment.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));

for (const fragment of requiredFragments) {
  assert.match(
    quizSource,
    literalPattern(fragment),
    `Quiz visual state must include: ${fragment}`,
  );
}

for (const fragment of forbiddenFragments) {
  assert.doesNotMatch(
    quizSource,
    literalPattern(fragment),
    `Quiz visual state must not regress to: ${fragment}`,
  );
}
