import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import test from "node:test";

const scriptPath = fileURLToPath(new URL("../deploy-henukit-artifact.sh", import.meta.url));
const script = readFileSync(scriptPath, "utf8");

test("artifact deployment is fixed-SHA, migration-aware, and orphan-safe", () => {
  execFileSync("bash", ["-n", scriptPath]);
  assert.match(script, /RELEASE_SHA/);
  assert.match(script, /docker compose --env-file/);
  assert.match(script, /exec -T postgres/);
  assert.match(script, /psql -v ON_ERROR_STOP=1/);
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
