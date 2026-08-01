import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const command = fileURLToPath(
  new URL("../deploy-epay-gateway-patches.sh", import.meta.url),
);

function write(path, contents, mode = 0o644) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, contents, { mode });
}

function fixture({
  healthFailures = 0,
  henukitEnabled = true,
  npmFails = false,
  serviceDiesAfterAttempts = -1,
} = {}) {
  const root = mkdtempSync(join(tmpdir(), "deploy-epay-gateway-"));
  const gateway = join(root, "epay-gateway");
  const patches = join(root, "patches");
  const backups = join(root, "backups");
  const bin = join(root, "bin");
  const log = join(root, "calls.log");
  const healthAttempts = join(root, "health-attempts");
  const serviceStarts = join(root, "service-starts");
  for (const directory of [gateway, patches, backups, bin]) mkdirSync(directory);
  write(join(gateway, "server.js"), "state=baseline\n");
  write(join(gateway, "config.js"), "module.exports = {};\n");
  write(join(gateway, "db.js"), "module.exports = {};\n");
  write(join(gateway, "package.json"), '{"name":"fixture","scripts":{"test":"true"}}\n');
  write(join(gateway, "package-lock.json"), '{"name":"fixture","lockfileVersion":3,"packages":{}}\n');
  write(join(gateway, "lib", "db.js"), "module.exports = {};\n");
  write(join(gateway, "test", "baseline.test.js"), "// baseline\n");
  const gatewayEnv = `SECRET=preserved
HENUKIT_EPAY_ENABLED=${henukitEnabled ? "1" : "0"}
HENUKIT_EPAY_PID=henukit-production
HENUKIT_EPAY_KEY=henukit-production-secret-32bytes
HENUKIT_EPAY_NOTIFY_URLS=https://henukit.cn/api/v1/payment-providers/easypay/notifications
HENUKIT_EPAY_RETURN_ORIGINS=https://henukit.cn
`;
  write(join(gateway, ".env"), gatewayEnv, 0o600);
  write(join(gateway, "data", "orders.json"), "[]\n", 0o600);
  write(
    join(patches, "0001-henukit-query-and-notify-outbox.patch"),
    "--- a/server.js\n+++ b/server.js\n@@ -1 +1 @@\n-state=baseline\n+state=one\n",
  );
  write(
    join(patches, "0002-henukit-close-refund-and-response-verification.patch"),
    "--- a/server.js\n+++ b/server.js\n@@ -1 +1 @@\n-state=one\n+state=two\n",
  );
  write(
    join(patches, "0003-henukit-private-checkout-handle.patch"),
    "--- a/server.js\n+++ b/server.js\n@@ -1 +1 @@\n-state=two\n+state=three\n",
  );
  writeFileSync(log, "");

  write(
    join(bin, "npm"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'npm %s\n' "$*" >> "$FAKE_CALL_LOG"
if [[ "$FAKE_NPM_FAILS" == "1" && "$1" == "test" ]]; then exit 1; fi
if [[ "$1" == "ci" ]]; then mkdir -p node_modules; fi
`,
    0o755,
  );
  write(
    join(bin, "systemctl"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'systemctl %s\n' "$*" >> "$FAKE_CALL_LOG"
if [[ "$1" == "start" ]]; then
  starts=0
  if [[ -f "$FAKE_SERVICE_STARTS" ]]; then starts="$(cat "$FAKE_SERVICE_STARTS")"; fi
  printf '%s' "$((starts + 1))" > "$FAKE_SERVICE_STARTS"
fi
if [[ "$1" == "is-active" ]]; then
  attempts=0
  if [[ -f "$FAKE_HEALTH_ATTEMPTS" ]]; then attempts="$(cat "$FAKE_HEALTH_ATTEMPTS")"; fi
  starts=0
  if [[ -f "$FAKE_SERVICE_STARTS" ]]; then starts="$(cat "$FAKE_SERVICE_STARTS")"; fi
  if [[ "$starts" -eq 1 && "$FAKE_SERVICE_DIES_AFTER_ATTEMPTS" -ge 0 && "$attempts" -ge "$FAKE_SERVICE_DIES_AFTER_ATTEMPTS" ]]; then
    exit 3
  fi
  printf 'active\n'
fi
`,
    0o755,
  );
  write(
    join(bin, "curl"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'curl %s\n' "$*" >> "$FAKE_CALL_LOG"
attempts=0
if [[ -f "$FAKE_HEALTH_ATTEMPTS" ]]; then attempts="$(cat "$FAKE_HEALTH_ATTEMPTS")"; fi
attempts=$((attempts + 1))
printf '%s' "$attempts" > "$FAKE_HEALTH_ATTEMPTS"
starts=0
if [[ -f "$FAKE_SERVICE_STARTS" ]]; then starts="$(cat "$FAKE_SERVICE_STARTS")"; fi
if [[ "$starts" -le 1 && "$attempts" -le "$FAKE_HEALTH_FAILURES" ]]; then exit 7; fi
printf '{"status":"ok"}\n'
`,
    0o755,
  );
  write(
    join(bin, "sleep"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'sleep %s\n' "$*" >> "$FAKE_CALL_LOG"
`,
    0o755,
  );

  return {
    root,
    gateway,
    patches,
    backups,
    log,
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      EPAY_BACKUP_ROOT: backups,
      EPAY_HEALTH_URL: "http://127.0.0.1:9219/health",
      EPAY_SERVICE: "epay-gateway.service",
      FAKE_CALL_LOG: log,
      FAKE_HEALTH_ATTEMPTS: healthAttempts,
      FAKE_HEALTH_FAILURES: String(healthFailures),
      FAKE_NPM_FAILS: npmFails ? "1" : "0",
      FAKE_SERVICE_DIES_AFTER_ATTEMPTS: String(serviceDiesAfterAttempts),
      FAKE_SERVICE_STARTS: serviceStarts,
    },
  };
}

test("check mode proves all patches and tests without touching the live gateway", () => {
  const setup = fixture();

  const output = execFileSync(
    command,
    [setup.gateway, setup.patches, "--check"],
    { encoding: "utf8", env: setup.env },
  );

  assert.match(output, /candidate verification passed/i);
  assert.equal(readFileSync(join(setup.gateway, "server.js"), "utf8"), "state=baseline\n");
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /systemctl (stop|restart|start)/);
});

test("execute mode atomically activates the tested candidate and preserves private state", () => {
  const setup = fixture();

  const output = execFileSync(
    command,
    [setup.gateway, setup.patches, "--execute"],
    { encoding: "utf8", env: setup.env },
  );

  assert.match(output, /gateway patches activated/i);
  assert.equal(readFileSync(join(setup.gateway, "server.js"), "utf8"), "state=three\n");
  assert.match(readFileSync(join(setup.gateway, ".env"), "utf8"), /^SECRET=preserved$/m);
  assert.match(readFileSync(join(setup.gateway, ".env"), "utf8"), /^HENUKIT_EPAY_ENABLED=1$/m);
  assert.equal(readFileSync(join(setup.gateway, "data", "orders.json"), "utf8"), "[]\n");
  assert.equal(existsSync(join(setup.gateway, ".henukit-patches.sha256")), true);
  assert.match(readFileSync(setup.log, "utf8"), /systemctl stop epay-gateway\.service/);
  assert.match(readFileSync(setup.log, "utf8"), /systemctl start epay-gateway\.service/);
  assert.equal(existsSync(setup.backups), true);
});

test("execute mode enables the prepared HENU tenant during atomic activation", () => {
  const setup = fixture({ henukitEnabled: false });

  const output = execFileSync(
    command,
    [setup.gateway, setup.patches, "--execute"],
    { encoding: "utf8", env: setup.env },
  );

  assert.match(output, /gateway patches activated/i);
  assert.match(readFileSync(join(setup.gateway, ".env"), "utf8"), /^HENUKIT_EPAY_ENABLED=1$/m);
});

test("execute mode waits for a candidate that becomes healthy after startup", () => {
  const setup = fixture({ healthFailures: 3 });

  const output = execFileSync(
    command,
    [setup.gateway, setup.patches, "--execute"],
    { encoding: "utf8", env: setup.env },
  );

  assert.match(output, /gateway patches activated/i);
  assert.equal(readFileSync(join(setup.gateway, "server.js"), "utf8"), "state=three\n");
  assert.equal(readFileSync(join(setup.root, "health-attempts"), "utf8"), "4");
  assert.equal((readFileSync(setup.log, "utf8").match(/^sleep 1$/gm) ?? []).length, 3);
});

test("execute mode rolls back promptly when the candidate service exits during readiness", () => {
  const setup = fixture({ healthFailures: 30, serviceDiesAfterAttempts: 1 });

  const result = spawnSync(
    command,
    [setup.gateway, setup.patches, "--execute"],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /original gateway restored/i);
  assert.equal(readFileSync(join(setup.gateway, "server.js"), "utf8"), "state=baseline\n");
  assert.equal(readFileSync(join(setup.root, "health-attempts"), "utf8"), "2");
});

test("execute mode bounds readiness attempts and verifies the restored gateway", () => {
  const setup = fixture({ healthFailures: 30 });

  const result = spawnSync(
    command,
    [setup.gateway, setup.patches, "--execute"],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /original gateway restored/i);
  assert.equal(readFileSync(join(setup.gateway, "server.js"), "utf8"), "state=baseline\n");
  assert.equal(readFileSync(join(setup.root, "health-attempts"), "utf8"), "31");
  assert.equal((readFileSync(setup.log, "utf8").match(/^curl /gm) ?? []).length, 31);
  assert.equal((readFileSync(setup.log, "utf8").match(/^sleep 1$/gm) ?? []).length, 29);
});

test("a failed candidate test stops before service interruption", () => {
  const setup = fixture({ npmFails: true });

  const result = spawnSync(
    command,
    [setup.gateway, setup.patches, "--execute"],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.equal(readFileSync(join(setup.gateway, "server.js"), "utf8"), "state=baseline\n");
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /systemctl (stop|restart|start)/);
});
