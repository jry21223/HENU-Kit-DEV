import assert from 'node:assert/strict';
import { mkdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import * as esbuild from 'esbuild';

const outdir = join(tmpdir(), 'quizcraft-answer-submission-test');
mkdirSync(outdir, { recursive: true });

const outfile = join(outdir, `answerSubmission-${Date.now()}.mjs`);
await esbuild.build({
  entryPoints: ['src/utils/answerSubmission.ts'],
  outfile,
  bundle: true,
  platform: 'node',
  format: 'esm',
  logLevel: 'silent',
});

const {
  ApiRequestError,
  classifyApiErrorKind,
  describeAnswerSubmissionFailure,
  runAnswerSubmission,
} = await import(pathToFileURL(outfile).href);

assert.equal(classifyApiErrorKind({ response: { status: 503 } }), 'http');
assert.equal(classifyApiErrorKind({ code: 'ECONNABORTED' }), 'timeout');
assert.equal(classifyApiErrorKind({ code: 'ETIMEDOUT' }), 'timeout');
assert.equal(classifyApiErrorKind({ request: {} }), 'network');

const success = await runAnswerSubmission(async () => ({ correct: true }));
assert.deepEqual(success, { ok: true, response: { correct: true } });

const cases = [
  {
    kind: 'http',
    error: new ApiRequestError('http', '服务暂不可用', 503),
    expectedTitle: '提交失败',
  },
  {
    kind: 'timeout',
    error: new ApiRequestError('timeout', 'timeout'),
    expectedTitle: '提交超时',
  },
  {
    kind: 'network',
    error: new ApiRequestError('network', 'Network Error'),
    expectedTitle: '网络连接失败',
  },
];

for (const testCase of cases) {
  const failure = await runAnswerSubmission(async () => {
    throw testCase.error;
  });
  assert.equal(failure.ok, false);
  assert.equal(failure.error.kind, testCase.kind);
  assert.equal(failure.error.title, testCase.expectedTitle);
  assert.match(failure.error.message, /答案仍已保留/);
  assert.deepEqual(
    failure.error,
    describeAnswerSubmissionFailure(testCase.error),
  );
}
