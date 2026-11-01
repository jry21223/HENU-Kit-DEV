import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(new URL("../auto-deploy-henukit-actions.sh", import.meta.url));
const sha = "a".repeat(40);

function fixture(overrides = {}) {
  const root = mkdtempSync(join(realpathSync(tmpdir()), "henukit-auto-deploy-"));
  const bin = join(root, "bin");
  const state = join(root, "state");
  const watcher = join(root, "watcher");
  for (const p of [bin, state, watcher]) mkdirSync(p, { mode: 0o700 });
  const policy = join(root, "policy.json");
  writeFileSync(policy, JSON.stringify({ format: "henukit-actions-auto-deploy-v1", repository: "jry21223/HENU-Kit-DEV", workflow: "deploy-henukit.yml", branch: "main" }), { mode: 0o600 });
  const token = join(root, "token");
  writeFileSync(token, "fixture-token\n", { mode: 0o600 });
  const log = join(root, "calls.log");
  writeFileSync(log, "");
  const executables = {
    id: '#!/bin/sh\nif [ "$TEST_STARTUP_DELAY" = 1 ]; then printf starting > "$TEST_STARTUP_FILE"; /bin/sleep 1; fi\nprintf 0\n',
    flock: '#!/bin/sh\nif [ "$TEST_REAL_FLOCK" = 1 ]; then exec /usr/bin/flock "$@"; fi\nexit 0\n',
    sync: '#!/bin/sh\nexit 0\n',
    stat: `#!/bin/sh
if [ "$2" = "%u" ]; then
  if [ "$3" = "$TEST_POLICY" ]; then printf '%s' "$TEST_POLICY_OWNER"; else printf 0; fi
elif [ "$2" = "%a" ]; then
  if [ "$(uname)" = Darwin ]; then /usr/bin/stat -f '%Lp' "$3"; else /usr/bin/stat -c '%a' "$3"; fi
fi
`,
    systemctl: `#!/bin/sh
case "$*" in
  *LoadState*) printf 'loaded\n';;
  *ActiveState*) printf '%s\n' "$TEST_OLD_WATCHER_STATE";;
  *) exit 90;;
esac
`,
    gh: `#!/bin/sh
case "$*" in
  *"actions/workflows/"*) printf 'poll\\n' >> "$TEST_POLL_LOG"; printf '123\\t1\\t%s\\t%s\\t%s\\t.github/workflows/deploy-henukit.yml\\tpush\\tmain\\n' "$TEST_SHA" "$TEST_RUN_STATUS" "$TEST_RUN_CONCLUSION";;
  *"git/ref/heads/main"*) printf '%s\n' "$TEST_BRANCH_SHA";;
  *) exit 91;;
esac
`,
    activate: `#!/bin/sh
[ "$HENUKIT_REPO" = jry21223/HENU-Kit-DEV ] && [ "$HENUKIT_WORKFLOW" = deploy-henukit.yml ] && [ "$HENUKIT_BRANCH" = main ] || exit 94
[ "$HENUKIT_EXPECTED_ACTIONS_IDENTITY" = actions:123:1 ] || exit 95
printf '%s\n' "$*" >> "$TEST_LOG"
if [ "$TEST_ACTIVATE_DELAY" = 1 ]; then /bin/sleep 1; fi
[ "$TEST_ACTIVATE_FAILS" = 0 ] || exit 23
printf '%s\n' "$1" > "$HENUKIT_STATE_ROOT/last-activated-sha"
`,
  };
  for (const [name, contents] of Object.entries(executables)) writeFileSync(join(bin, name), contents, { mode: 0o755 });
  return { root, state, watcher, policy, log, env: {
    ...process.env, PATH: `${bin}:${process.env.PATH}`, TEST_POLICY: policy, TEST_POLICY_OWNER: "0",
    TEST_OLD_WATCHER_STATE: "inactive", TEST_SHA: sha, TEST_BRANCH_SHA: sha,
    TEST_RUN_STATUS: "completed", TEST_RUN_CONCLUSION: "success", TEST_ACTIVATE_FAILS: "0", TEST_LOG: log,
    TEST_POLL_LOG: join(root, "poll.log"), TEST_ACTIVATE_DELAY: "0", TEST_REAL_FLOCK: "0",
    TEST_STARTUP_DELAY: "0", TEST_STARTUP_FILE: join(root, "startup"),
    HENUKIT_AUTO_DEPLOY_POLICY: policy, HENUKIT_AUTO_DEPLOY_STATE_ROOT: state,
    HENUKIT_STATE_ROOT: watcher, HENUKIT_ACTIVATE_COMMAND: join(bin, "activate"), GH_TOKEN_FILE: token,
    ...overrides,
  } };
}

function run(setup) { return spawnSync("bash", [script, "--once"], { encoding: "utf8", env: setup.env }); }

test("current successful main build invokes the existing full release entry once", () => {
  const s = fixture();
  const r = run(s);
  assert.equal(r.status, 0, r.stderr);
  assert.equal(readFileSync(s.log, "utf8"), `${sha} --execute\n`);
  assert.equal(existsSync(join(s.state, `${sha}.attempted.json`)), true);
  assert.equal(existsSync(join(s.state, `${sha}.activated.json`)), true);
  assert.equal(run(s).status, 0);
  assert.equal(readFileSync(s.log, "utf8"), `${sha} --execute\n`);
});

test("the actual activator receives only the policy source despite inherited overrides", () => {
  const s = fixture({ HENUKIT_REPO: "other/repo", HENUKIT_BRANCH: "unreviewed", HENUKIT_WORKFLOW: "other.yml" });
  const r = run(s);
  assert.equal(r.status, 0, r.stderr);
  assert.equal(readFileSync(s.log, "utf8"), `${sha} --execute\n`);
});

async function waitUntil(check) {
  const deadline = Date.now() + 20000;
  while (!check()) {
    assert.ok(Date.now() < deadline, "controller did not reach the expected public behavior");
    await delay(50);
  }
}

function watch(setup, t) {
  const child = spawn("bash", [script, "--watch"], { env: { ...setup.env, HENUKIT_POLL_SECONDS: "60" }, stdio: "pipe" });
  let errors = "";
  child.stderr.on("data", chunk => { errors += chunk; });
  const exited = new Promise(resolve => child.once("close", (code, signal) => resolve({ code, signal, errors })));
  t.after(async () => {
    if (child.exitCode === null) child.kill("SIGTERM");
    await exited;
  });
  return { child, exited };
}

test("USR1 wakes the scheduler before its 60-second fallback poll", { timeout: 30000 }, async t => {
  const s = fixture({ TEST_RUN_STATUS: "in_progress" });
  const { child, exited } = watch(s, t);
  const polls = () => existsSync(s.env.TEST_POLL_LOG) ? readFileSync(s.env.TEST_POLL_LOG, "utf8").trim().split("\n").length : 0;
  await waitUntil(() => polls() >= 1);
  child.kill("SIGUSR1");
  await waitUntil(() => polls() >= 2);
  child.kill("SIGTERM");
  assert.equal((await exited).code, 0);
});

test("a notification during startup is retained without terminating the controller", { timeout: 30000 }, async t => {
  const s = fixture({ TEST_RUN_STATUS: "in_progress", TEST_STARTUP_DELAY: "1" });
  const { child, exited } = watch(s, t);
  await waitUntil(() => existsSync(s.env.TEST_STARTUP_FILE));
  child.kill("SIGUSR1");
  await waitUntil(() => existsSync(s.env.TEST_POLL_LOG));
  child.kill("SIGTERM");
  assert.equal((await exited).code, 0);
});

test("TERM lets an in-flight activation finish before stopping the scheduler", { timeout: 30000 }, async t => {
  const s = fixture({ TEST_ACTIVATE_DELAY: "1" });
  const { child, exited } = watch(s, t);
  await waitUntil(() => readFileSync(s.log, "utf8").includes("--execute"));
  child.kill("SIGTERM");
  const result = await exited;
  assert.equal(result.code, 0, result.errors);
  assert.equal(existsSync(join(s.state, `${sha}.activated.json`)), true);
});

test("the real Linux controller lock prevents concurrent schedulers", { skip: process.platform !== "linux", timeout: 30000 }, async t => {
  const s = fixture({ TEST_RUN_STATUS: "in_progress", TEST_REAL_FLOCK: "1" });
  const { child, exited } = watch(s, t);
  await waitUntil(() => existsSync(s.env.TEST_POLL_LOG));
  const second = run(s);
  assert.notEqual(second.status, 0);
  assert.match(second.stderr, /another automatic release controller/);
  child.kill("SIGTERM");
  assert.equal((await exited).code, 0);
});

for (const [name, env] of Object.entries({
  "failed workflow": { TEST_RUN_CONCLUSION: "failure" },
  "incomplete workflow": { TEST_RUN_STATUS: "in_progress", TEST_RUN_CONCLUSION: "null" },
  "stale successful build": { TEST_BRANCH_SHA: "b".repeat(40) },
  "active old watcher": { TEST_OLD_WATCHER_STATE: "active" },
})) {
  test(`${name} never invokes the release entry or creates an authorization`, () => {
    const s = fixture(env);
    assert.equal(run(s).status, 0);
    assert.equal(readFileSync(s.log, "utf8"), "");
    assert.equal(existsSync(join(s.state, `${sha}.attempted.json`)), false);
  });
}

test("failed activation retains one attempt and never automatically retries that SHA", () => {
  const s = fixture({ TEST_ACTIVATE_FAILS: "1" });
  assert.notEqual(run(s).status, 0);
  assert.equal(existsSync(join(s.state, `${sha}.failed.json`)), true);
  assert.equal(existsSync(join(s.state, `${sha}.activated.json`)), false);
  s.env.TEST_ACTIVATE_FAILS = "0";
  assert.equal(run(s).status, 0);
  assert.equal(readFileSync(s.log, "utf8"), `${sha} --execute\n`);
});

test("an unresolved scoped marker or pending rollback blocks scheduling", () => {
  for (const kind of ["scoped", "rollback"]) {
    const s = fixture();
    if (kind === "scoped") symlinkSync("/nonexistent-scoped-receipt", join(s.watcher, "scoped-food-release"));
    else {
      const pending = join(s.watcher, "rollback-contracts", "pending");
      mkdirSync(pending, { recursive: true });
      writeFileSync(join(pending, "b".repeat(40)), "pending");
    }
    assert.equal(run(s).status, 0);
    assert.equal(readFileSync(s.log, "utf8"), "");
    assert.equal(existsSync(join(s.state, `${sha}.attempted.json`)), false);
  }
});

test("automatic policy must be explicit, private, root-owned, canonical and match the pinned repository", () => {
  for (const kind of ["missing", "mode", "owner", "symlink", "repository"]) {
    const s = fixture();
    if (kind === "missing") s.env.HENUKIT_AUTO_DEPLOY_POLICY = "";
    if (kind === "mode") chmodSync(s.policy, 0o644);
    if (kind === "owner") s.env.TEST_POLICY_OWNER = "1000";
    if (kind === "symlink") {
      symlinkSync(s.policy, join(s.root, "policy-link"));
      s.env.HENUKIT_AUTO_DEPLOY_POLICY = join(s.root, "policy-link");
    }
    if (kind === "repository") writeFileSync(s.policy, JSON.stringify({ format: "henukit-actions-auto-deploy-v1", repository: "someone/else", workflow: "deploy-henukit.yml", branch: "main" }));
    const r = run(s);
    assert.notEqual(r.status, 0, kind);
    assert.equal(readFileSync(s.log, "utf8"), "", kind);
  }
});
