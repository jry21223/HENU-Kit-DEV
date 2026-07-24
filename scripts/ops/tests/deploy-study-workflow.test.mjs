import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const workflow = readFileSync(
  new URL("../../../.github/workflows/deploy-study.yml", import.meta.url),
  "utf8",
);

test("standalone release includes and verifies Next.js public assets", () => {
  assert.match(
    workflow,
    /cp -a apps\/web\/public release\/web\/apps\/web\/public/,
  );
  assert.match(
    workflow,
    /test -f release\/web\/apps\/web\/public\/deploy-probe\.txt/,
  );
});

test("production verification probes the homepage and a public asset", () => {
  assert.match(workflow, /https:\/\/study\.superhuazai\.me\/$/m);
  assert.match(
    workflow,
    /https:\/\/study\.superhuazai\.me\/deploy-probe\.txt/,
  );
});

test("push deployment is path-scoped and can be disabled after webhook cutover", () => {
  assert.match(workflow, /paths:\s*[\s\S]*apps\/web\/\*\*/);
  assert.match(workflow, /services\/api\/\*\*/);
  assert.match(workflow, /if: vars\.HENUKIT_DEPLOY_MODE != 'webhook'/);
  assert.match(workflow, /workflow_dispatch:/);
  assert.doesNotMatch(workflow, /- \.github\/workflows\/deploy-study\.yml/);
  assert.doesNotMatch(workflow, /- scripts\/ops\/\*\*/);
});
