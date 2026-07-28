import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const workflow = readFileSync(
  new URL("../../../.github/workflows/account-portfolio.yml", import.meta.url),
  "utf8",
);

test("Account Portfolio CI enforces Go vulnerability and image scans", () => {
  assert.match(workflow, /golang\.org\/x\/vuln\/cmd\/govulncheck@v1\.6\.0 \.\/\.\.\./);
  assert.match(workflow, /aquasec\/trivy:0\.70\.0 fs --scanners secret --exit-code 1/);
  assert.match(workflow, /aquasec\/trivy:0\.70\.0 image --scanners vuln,secret --severity HIGH,CRITICAL --exit-code 1 henukit-account-portfolio/);
  assert.match(workflow, /go build -o \/tmp\/account-portfolio-healthcheck \.\/cmd\/healthcheck/);
});

function assertUsesPostgres17BackupClients(candidate) {
  const flattenedShell = candidate.replace(/\\\r?\n\s*/g, " ");

  assert.match(
    flattenedShell,
    /docker run --rm --network host -e PGPASSWORD="\$PGPASSWORD" postgres:17-alpine\s+pg_dump/
  );
  assert.match(
    flattenedShell,
    /docker run --rm --network host -e PGPASSWORD="\$PGPASSWORD"\s+-v \/tmp\/account-portfolio-recovery\.dump:\/tmp\/account-portfolio-recovery\.dump:ro\s+postgres:17-alpine\s+pg_restore/
  );
  assert.doesNotMatch(flattenedShell, /^\s*pg_dump\b/m);
  assert.doesNotMatch(flattenedShell, /^\s*pg_restore\b/m);
}

test("Account Portfolio recovery uses PostgreSQL 17 backup clients", () => {
  assertUsesPostgres17BackupClients(workflow);
});

test("Account Portfolio recovery round-trips every ordered migration", () => {
  assert.match(workflow, /mapfile -t up_migrations < <\(find services\/account-portfolio\/db\/migrations/);
  assert.match(workflow, /mapfile -t down_migrations < <\(find services\/account-portfolio\/db\/migrations/);
  assert.match(workflow, /for migration in "\$\{up_migrations\[@\]\}"; do/);
  assert.match(workflow, /for migration in "\$\{down_migrations\[@\]\}"; do/);
  assert.match(workflow, /account_portfolio_ticket_events/);
  assert.match(workflow, /account_portfolio_command_idempotency/);
});

test("Account Portfolio recovery rejects bare runner backup clients", () => {
  assert.throws(() =>
    assertUsesPostgres17BackupClients(`${workflow}\n          pg_dump -h 127.0.0.1 -U account_portfolio`),
  );
  assert.throws(() =>
    assertUsesPostgres17BackupClients(`${workflow}\n          pg_restore -h 127.0.0.1 -U account_portfolio`),
  );
});
