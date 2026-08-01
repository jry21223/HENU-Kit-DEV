import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(
  new URL("../watch-henukit-actions.sh", import.meta.url),
);
const releaseSha = "a".repeat(40);

function writeExecutable(path, body) {
  writeFileSync(path, body, { mode: 0o755 });
}

function fixture({
  accountOperatorRole = "operations-operator",
  accountPortfolioSchemaPresent = true,
  approved = true,
  badAccountBoundary = false,
  badChecksum = false,
  branchSha = releaseSha,
  canonicalQuizRedirect = false,
  failTargetAccountPortfolioHealth = false,
  failTargetHealth = false,
  previousHasAccountPortfolio = true,
  previousSha = "b".repeat(40),
  portalAllowMock = false,
  portalApiMode = "live",
  runConclusion = "success",
  runStatus = "completed",
} = {}) {
  const root = mkdtempSync(join(tmpdir(), "henukit-actions-watch-"));
  const bin = join(root, "bin");
  const staging = join(root, "staging");
  const releases = join(root, "releases");
  const backups = join(root, "backups");
  const state = join(root, "state");
  const log = join(root, "calls.log");
  const active = join(root, "active-sha");
  const token = join(root, "github.token");
  const envFile = join(root, "henukit.env");
  mkdirSync(bin);
  for (const directory of [staging, releases, backups, state]) {
    mkdirSync(directory);
  }
  mkdirSync(join(state, "approvals"));
  if (approved) {
    writeFileSync(join(state, "approvals", releaseSha), `${releaseSha}\n`, {
      mode: 0o600,
    });
  }
  writeFileSync(token, "test-read-only-token\n", { mode: 0o600 });
  chmodSync(token, 0o600);
  writeFileSync(
    envFile,
    `POSTGRES_USER=test\nPORTAL_API_MODE=${portalApiMode}\nNEXT_PUBLIC_PORTAL_ALLOW_MOCK=${portalAllowMock ? "1" : "0"}\n`,
  );
  writeFileSync(log, "");
  if (previousSha) {
    const previousRelease = join(releases, previousSha);
    mkdirSync(join(previousRelease, "bin"), { recursive: true });
    writeFileSync(join(previousRelease, "RELEASE_SHA"), `${previousSha}\n`);
    writeFileSync(
      join(previousRelease, "docker-compose.henukit.release.yml"),
      `services:\n${previousHasAccountPortfolio ? "  account-portfolio:\n" : ""}`,
    );
    writeExecutable(
      join(previousRelease, "bin", "deploy-henukit-artifact.sh"),
      `#!/usr/bin/env bash
set -Eeuo pipefail
cat "$1/RELEASE_SHA" > "$FAKE_ACTIVE_FILE"
printf 'deploy %s %s\\n' "$1" "$2" >> "$FAKE_CALL_LOG"
`,
    );
    writeFileSync(active, `${previousSha}\n`);
  }

  writeExecutable(
    join(bin, "gh"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "$1 $2" == "run list" ]]; then
  if [[ "$FAKE_NO_SUCCESS" == "1" ]]; then
    exit 0
  fi
  printf '123\\t%s\\t%s\\t%s\\thttps://github.example/actions/runs/123\\n' \
    "$FAKE_RELEASE_SHA" "$FAKE_RUN_STATUS" "$FAKE_RUN_CONCLUSION"
  exit 0
fi
if [[ "$1" == "api" ]]; then
  printf '%s\\n' "$FAKE_BRANCH_SHA"
  exit 0
fi
if [[ "$1 $2" != "run download" ]]; then
  exit 70
fi
dest=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--dir" ]]; then
    dest="$2"
    shift 2
  else
    shift
  fi
done
images=(
  henukit-console
  henukit-console-gateway
  henukit-platform-core
  henukit-platform-mail-worker
  henukit-platform-smtp-provider
  henukit-portal
  henukit-portal-api
  henukit-account-portfolio
  henukit-portal-gateway
)
for image in "\${images[@]}"; do
  artifact="$dest/$image-$FAKE_RELEASE_SHA"
  mkdir -p "$artifact"
  printf '%s\\n' "$image" | gzip > "$artifact/$image-$FAKE_RELEASE_SHA.docker.tar.gz"
  (
    cd "$artifact"
    sha256sum "$image-$FAKE_RELEASE_SHA.docker.tar.gz" > "$image-$FAKE_RELEASE_SHA.docker.tar.gz.sha256"
  )
done
runtime_artifact="$dest/henukit-runtime-$FAKE_RELEASE_SHA"
runtime_tree="$(mktemp -d "\${TMPDIR:-/tmp}/henukit-runtime-tree.XXXXXX")"
mkdir -p "$runtime_artifact" "$runtime_tree/bin" "$runtime_tree/release-gates"
printf '%s\\n' "$FAKE_RELEASE_SHA" > "$runtime_tree/RELEASE_SHA"
printf 'services:\\n  account-portfolio:\\n' > "$runtime_tree/docker-compose.henukit.release.yml"
cat > "$runtime_tree/release-gates/account-production-boundary.env" <<EOF
release_sha=$FAKE_RELEASE_SHA
status=$([[ "$FAKE_BAD_ACCOUNT_BOUNDARY" == "1" ]] && printf fail || printf pass)
account_console_mock_sources=absent
account_payment_provider=easypay_or_disabled
portal_require_gateway=1
portal_allow_mock=0
portal_api_default_mode=live
EOF
cat > "$runtime_tree/bin/deploy-henukit-artifact.sh" <<'HELPER'
#!/usr/bin/env bash
set -Eeuo pipefail
cat "$1/RELEASE_SHA" > "$FAKE_ACTIVE_FILE"
printf 'deploy %s %s\\n' "$1" "$2" >> "$FAKE_CALL_LOG"
HELPER
chmod 0555 "$runtime_tree/bin/deploy-henukit-artifact.sh"
tar -C "$runtime_tree" -czf "$runtime_artifact/henukit-runtime-$FAKE_RELEASE_SHA.tar.gz" .
rm -rf "$runtime_tree"
(
  cd "$runtime_artifact"
  sha256sum "henukit-runtime-$FAKE_RELEASE_SHA.tar.gz" > "henukit-runtime-$FAKE_RELEASE_SHA.tar.gz.sha256"
)
if [[ "$FAKE_BAD_CHECKSUM" == "1" ]]; then
  printf 'tampered\\n' >> "$dest/henukit-console-$FAKE_RELEASE_SHA/henukit-console-$FAKE_RELEASE_SHA.docker.tar.gz"
fi
`,
  );

  writeExecutable(
    join(bin, "flock"),
    `#!/usr/bin/env bash
exit 0
`,
  );

  writeExecutable(
    join(bin, "docker"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'docker %s\\n' "$*" >> "$FAKE_CALL_LOG"
if [[ "$1" == "ps" ]]; then
  if [[ -s "$FAKE_ACTIVE_FILE" ]]; then
    sha="$(cat "$FAKE_ACTIVE_FILE")"
    for image in henukit-console henukit-console-gateway henukit-platform-core henukit-platform-mail-worker henukit-platform-smtp-provider henukit-portal henukit-portal-api henukit-portal-gateway; do
      printf '%s:%s\\n' "$image" "$sha"
    done
    if [[ "$sha" == "$FAKE_RELEASE_SHA" || "$FAKE_PREVIOUS_HAS_ACCOUNT_PORTFOLIO" == "1" ]]; then
      printf 'henukit-account-portfolio:%s\\n' "$sha"
    fi
  fi
elif [[ "$1" == "inspect" ]]; then
  if [[ "$FAKE_FAIL_TARGET_ACCOUNT_PORTFOLIO_HEALTH" == "1" &&
        -s "$FAKE_ACTIVE_FILE" &&
        "$(cat "$FAKE_ACTIVE_FILE")" == "$FAKE_RELEASE_SHA" ]]; then
    printf 'unhealthy\n'
  else
    printf 'healthy\n'
  fi
elif [[ "$1" == "exec" && "$*" == *"pg_dump"* ]]; then
  printf 'verified-platform-backup\\n'
elif [[ "$1" == "exec" && "$*" == *"to_regclass('public.account_portfolio_accounts')"* ]]; then
  printf '%s\\n' "$FAKE_ACCOUNT_PORTFOLIO_SCHEMA_PRESENT"
elif [[ "$1" == "exec" && "$*" == *"account_portfolio_ticket_messages"* ]]; then
  printf '1,1,1,0,0,0,0,0,0,1\\n'
elif [[ "$1" == "exec" && "$*" == *"SHOW server_version"* ]]; then
  printf '16.3\\n'
elif [[ "$1" == "exec" && "$*" == *"count(*) FROM users"* ]]; then
  printf '1,1,1\\n'
elif [[ "$1" == "exec" && "$*" == *"permission_codes"* && "$*" == *"account.orders.refund"* ]]; then
  printf '8\\n'
elif [[ "$1" == "exec" && "$*" == *"role_permissions"* && "$*" == *"account.orders.refund"* ]]; then
  printf '8\\n'
elif [[ "$1" == "exec" && "$*" == *"authorization_roles"* && "$*" == *"role_code"* ]]; then
  printf '1\\n'
fi
`,
  );

  writeExecutable(
    join(bin, "curl"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'curl %s\\n' "$*" >> "$FAKE_CALL_LOG"
if [[ "$FAKE_FAIL_TARGET_HEALTH" == "1" &&
      -s "$FAKE_ACTIVE_FILE" &&
      "$(cat "$FAKE_ACTIVE_FILE")" == "$FAKE_RELEASE_SHA" ]]; then
  exit 22
fi
if [[ "$*" == *"--write-out"* ]]; then
  if [[ "$FAKE_CANONICAL_QUIZ_REDIRECT" == "1" &&
        "$*" == *"https://example.test/quiz/"* &&
        "$*" != *"--location"* ]]; then
    printf '308'
    exit 0
  fi
  printf '404'
fi
`,
  );

  return {
    root,
    log,
    state,
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      FAKE_ACTIVE_FILE: active,
      FAKE_ACCOUNT_PORTFOLIO_SCHEMA_PRESENT: accountPortfolioSchemaPresent ? "t" : "f",
      FAKE_BAD_ACCOUNT_BOUNDARY: badAccountBoundary ? "1" : "0",
      FAKE_BAD_CHECKSUM: badChecksum ? "1" : "0",
      FAKE_BRANCH_SHA: branchSha,
      FAKE_CANONICAL_QUIZ_REDIRECT: canonicalQuizRedirect ? "1" : "0",
      FAKE_CALL_LOG: log,
      FAKE_FAIL_TARGET_ACCOUNT_PORTFOLIO_HEALTH: failTargetAccountPortfolioHealth ? "1" : "0",
      FAKE_FAIL_TARGET_HEALTH: failTargetHealth ? "1" : "0",
      FAKE_PREVIOUS_HAS_ACCOUNT_PORTFOLIO: previousHasAccountPortfolio ? "1" : "0",
      FAKE_RELEASE_SHA: releaseSha,
      FAKE_NO_SUCCESS: "0",
      FAKE_RUN_CONCLUSION: runConclusion,
      FAKE_RUN_STATUS: runStatus,
      GH_TOKEN_FILE: token,
      HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE: accountOperatorRole,
      HENUKIT_BACKUP_ROOT: backups,
      HENUKIT_ENV_FILE: envFile,
      HENUKIT_RELEASE_ROOT: releases,
      HENUKIT_STAGING_ROOT: staging,
      HENUKIT_STATE_ROOT: state,
      HENUKIT_PUBLIC_BASE_URL: "https://example.test",
    },
  };
}

test("one-shot downloads, verifies, backs up, and deploys one successful main artifact set", () => {
  const setup = fixture();

  const output = execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.match(
    output,
    new RegExp(`release ${releaseSha} activated and deterministic smoke checks passed`),
  );
  assert.match(calls, /docker exec henukit-postgres-1 .*pg_dump/);
  assert.equal((calls.match(/docker load/g) ?? []).length, 9);
  assert.match(calls, /deploy .*releases.*henukit\.env/);
  assert.match(calls, /role_permissions/);
  assert.match(calls, /account\.orders\.refund/);
  assert.match(calls, /operations-operator/);
  assert.match(calls, /revision = role\.revision \+ 1/);
  assert.match(calls, /curl .*https:\/\/example\.test\/api\/v1\/healthz/);
  assert.equal(
    readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(),
    releaseSha,
  );

  execFileSync(script, ["--once"], { env: setup.env });
  const secondCalls = readFileSync(setup.log, "utf8");
  assert.equal((secondCalls.match(/^deploy /gm) ?? []).length, 1);
});

test("one-shot accepts the canonical retired Quiz redirect when its final response is 404", () => {
  const setup = fixture({ canonicalQuizRedirect: true });

  const output = execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.match(
    output,
    new RegExp(`release ${releaseSha} activated and deterministic smoke checks passed`),
  );
});

test("one-shot does nothing when no main workflow run has completed successfully", () => {
  const setup = fixture();
  setup.env.FAKE_NO_SUCCESS = "1";

  const output = execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.match(output, /no completed successful/);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});

test("one-shot refuses an older successful artifact when the latest main run failed", () => {
  const setup = fixture({ runConclusion: "failure" });

  const output = execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.match(output, /refusing stale artifacts/);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});

test("one-shot refuses a successful run whose SHA is no longer the main branch head", () => {
  const setup = fixture({ branchSha: "d".repeat(40) });

  const output = execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.match(output, /no longer current main/);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});

test("one-shot prepares a verified backup but does not activate without exact-SHA approval", () => {
  const setup = fixture({ approved: false });

  const output = execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.match(output, /awaits exact-SHA approval/);
  assert.match(calls, /pg_dump/);
  assert.doesNotMatch(calls, /docker load|^deploy /m);
});

test("checksum failure stops before backup, image load, and deploy", () => {
  const setup = fixture({ badChecksum: true });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /checksum|FAILED/i);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});

test("watch mode exits on a failed check so systemd can restart it cleanly", () => {
  const setup = fixture({ badChecksum: true });

  const result = spawnSync(script, ["--watch"], {
    encoding: "utf8",
    env: { ...setup.env, HENUKIT_POLL_SECONDS: "1" },
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /checksum|FAILED/i);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});

test("failed public verification restores and verifies the previous fixed-SHA release", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ failTargetHealth: true, previousSha });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 2);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
});

test("failed Account Portfolio health restores and verifies the previous fixed-SHA release", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ failTargetAccountPortfolioHealth: true, previousSha });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.match(calls, /docker inspect .*henukit-account-portfolio-1/);
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 2);
});

test("first Account Portfolio rollout accepts a legacy eight-image release and records an empty-database recovery baseline", () => {
  const setup = fixture({
    accountPortfolioSchemaPresent: false,
    previousHasAccountPortfolio: false,
  });

  const output = execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.match(
    output,
    new RegExp(`release ${releaseSha} activated and deterministic smoke checks passed`),
  );
  assert.match(calls, /docker exec henukit-postgres-1 .*pg_dump.*account_portfolio/);
  assert.equal((calls.match(/docker load/g) ?? []).length, 9);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), releaseSha);
});

test("first Account Portfolio rollout rolls back to a legacy eight-image release after failed verification", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    accountPortfolioSchemaPresent: false,
    failTargetHealth: true,
    previousHasAccountPortfolio: false,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 2);
});

test("one-shot refuses a release whose Account production-boundary manifest did not pass", () => {
  const setup = fixture({ badAccountBoundary: true });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Account production-boundary manifest/i);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});

test("one-shot refuses production Portal API mock mode before backup or activation", () => {
  const setup = fixture({ portalApiMode: "mock" });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /PORTAL_API_MODE must be explicitly live/i);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});

test("one-shot refuses an environment that opts Portal into mock mode", () => {
  const setup = fixture({ portalAllowMock: true });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /NEXT_PUBLIC_PORTAL_ALLOW_MOCK must be 0 or absent/i);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});

test("one-shot requires an explicit Account operator role before backup or activation", () => {
  const setup = fixture({ accountOperatorRole: "" });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE/i);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});
