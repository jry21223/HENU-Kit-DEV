import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { chmodSync, mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const scriptPath = fileURLToPath(new URL("../deploy-henukit-artifact.sh", import.meta.url));
const script = readFileSync(scriptPath, "utf8");

test("artifact deployment is fixed-SHA, migration-aware, and orphan-safe", () => {
  execFileSync("bash", ["-n", scriptPath]);
  assert.match(script, /RELEASE_SHA/);
  assert.match(script, /export PORTAL_VERSION="\$release_sha"/);
  assert.match(script, /export PORTAL_DEPLOYED_AT="\$portal_deployed_at"/);
  assert.match(script, /docker compose --env-file/);
  assert.match(script, /exec -T postgres/);
  assert.match(script, /psql -v ON_ERROR_STOP=1/);
  assert.match(script, /IFS=',' read -r -a migration_names/);
  assert.match(script, /for migration_name in "\$\{migration_names\[@\]\}"/);
  assert.match(script, /ensure_account_portfolio_database/);
  assert.match(script, /ensure_postgres_ready/);
  assert.match(script, /up -d postgres/);
  assert.match(script, /pg_isready/);
  assert.match(script, /createdb -U "\$POSTGRES_USER" account_portfolio/);
  assert.match(
    script,
    /echo "Ensuring PostgreSQL is ready"\nensure_postgres_ready\n\nif \[\[ -n "\$migration_arg" \]\]/,
    "PostgreSQL must be ready before an optional Platform Core migration runs",
  );
  assert.match(script, /up -d --remove-orphans/);
  assert.match(script, /docker ps -a --format/);
  assert.match(script, /study-api\|study-worker\|quizcraft-api\|quizcraft-web/);
  assert.doesNotMatch(script, /docker\s+(build|compose[^\n]*build)/);
});

test("deployment derives Portal metadata from the fixed SHA and each real activation attempt", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-deploy-metadata-"));
  const runtime = join(root, "runtime");
  const bin = join(root, "bin");
  const log = join(root, "activations.log");
  const materialsLog = join(root, "materials-installs.log");
  mkdirSync(runtime);
  mkdirSync(bin);
  mkdirSync(join(runtime, "materials-runtime"));
  for (const owner of ["platform-core", "notice", "food", "library", "portal"]) mkdirSync(join(runtime, "migrations", owner), { recursive: true });
  const releaseSha = "b".repeat(40);
  writeFileSync(join(runtime, "RELEASE_SHA"), `${releaseSha}\n`);
  writeFileSync(join(runtime, "docker-compose.henukit.release.yml"), "services: {}\n");
  writeFileSync(
    join(runtime, "materials-runtime", "install.sh"),
    "#!/usr/bin/env bash\nset -Eeuo pipefail\nprintf '%s\\n' \"$*\" >> \"$FAKE_MATERIALS_INSTALLS\"\n",
    { mode: 0o755 },
  );
  const environment = join(root, "henukit.env");
  writeFileSync(environment, "PORTAL_VERSION=operator-value\nPORTAL_DEPLOYED_AT=1970-01-01T00:00:00Z\n");
  const now = join(root, "now");
  writeFileSync(now, "2026-08-13T01:02:03Z\n");
  writeFileSync(join(bin, "date"), "#!/usr/bin/env bash\ncat \"$FAKE_NOW\"\n", { mode: 0o755 });
  writeFileSync(join(bin, "docker"), `#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "$*" == *"exec -T postgres"* ]]; then printf '1\\n'; fi
if [[ "$*" == *"up -d --remove-orphans"* ]]; then printf '%s|%s\\n' "$PORTAL_VERSION" "$PORTAL_DEPLOYED_AT" >> "$FAKE_ACTIVATIONS"; fi
`, { mode: 0o755 });
  chmodSync(join(bin, "date"), 0o755);
  chmodSync(join(bin, "docker"), 0o755);
  const env = {
    ...process.env,
    PATH: `${bin}:${process.env.PATH}`,
    FAKE_NOW: now,
    FAKE_ACTIVATIONS: log,
    FAKE_MATERIALS_INSTALLS: materialsLog,
  };
  execFileSync(scriptPath, [runtime, environment], { env });
  writeFileSync(now, "2026-08-13T01:03:04Z\n");
  execFileSync(scriptPath, [runtime, environment], { env });
  assert.equal(readFileSync(log, "utf8"), `${releaseSha}|2026-08-13T01:02:03Z\n${releaseSha}|2026-08-13T01:03:04Z\n`);
  assert.equal(
    readFileSync(materialsLog, "utf8"),
    `--release-sha ${releaseSha}\n--release-sha ${releaseSha}\n`,
  );
});
