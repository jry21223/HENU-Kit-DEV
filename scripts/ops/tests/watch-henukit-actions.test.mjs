import assert from "node:assert/strict";
import { execFileSync, spawn, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";

const script = fileURLToPath(
  new URL("../watch-henukit-actions.sh", import.meta.url),
);
const releaseSha = "a".repeat(40);
const releaseImages = [
  "henukit-console",
  "henukit-console-gateway",
  "henukit-platform-core",
  "henukit-platform-mail-worker",
  "henukit-platform-smtp-provider",
  "henukit-portal",
  "henukit-portal-api",
  "henukit-account-portfolio",
  "henukit-notice",
  "henukit-notice-worker",
  "henukit-food",
  "henukit-library",
  "henukit-portal-gateway",
];

function writeExecutable(path, body) {
  writeFileSync(path, body, { mode: 0o755 });
}

function writeChecksum(directory, name, content) {
  writeFileSync(join(directory, name), content, { mode: 0o400 });
  const digest = createHash("sha256").update(content).digest("hex");
  writeFileSync(join(directory, `${name}.sha256`), `${digest}  ${name}\n`, {
    mode: 0o400,
  });
}

function writeLocalArtifacts(root) {
  const artifacts = join(root, "local-artifacts");
  const runtimeTree = join(root, "local-runtime");
  mkdirSync(artifacts);
  mkdirSync(join(runtimeTree, "bin"), { recursive: true });
  mkdirSync(join(runtimeTree, "release-gates"), { recursive: true });
  writeFileSync(join(artifacts, "RELEASE_SHA"), `${releaseSha}\n`, { mode: 0o400 });
  for (const image of releaseImages) {
    writeChecksum(
      artifacts,
      `${image}-${releaseSha}.docker.tar.gz`,
      gzipSync(`${image}\n`),
    );
  }
  writeFileSync(join(runtimeTree, "RELEASE_SHA"), `${releaseSha}\n`);
  writeFileSync(
    join(runtimeTree, "docker-compose.henukit.release.yml"),
    "services:\n  account-portfolio:\n  notice:\n  notice-worker:\n  food:\n  library:\n",
  );
  writeFileSync(
    join(runtimeTree, "release-gates", "account-production-boundary.env"),
    `release_sha=${releaseSha}\nstatus=pass\naccount_console_mock_sources=absent\naccount_transitive_mock_sources=absent\naccount_payment_provider=easypay_or_disabled\nportal_require_gateway=1\nportal_allow_mock=0\nportal_api_default_mode=live\n`,
  );
  writeExecutable(
    join(runtimeTree, "bin", "deploy-henukit-artifact.sh"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
cat "$1/RELEASE_SHA" > "$FAKE_ACTIVE_FILE"
printf 'deploy %s %s\\n' "$1" "$2" >> "$FAKE_CALL_LOG"
`,
  );
  const runtimeArchive = `henukit-runtime-${releaseSha}.tar.gz`;
  execFileSync("tar", ["-C", runtimeTree, "-czf", join(artifacts, runtimeArchive), "."]);
  const runtimeContent = readFileSync(join(artifacts, runtimeArchive));
  writeFileSync(
    join(artifacts, `${runtimeArchive}.sha256`),
    `${createHash("sha256").update(runtimeContent).digest("hex")}  ${runtimeArchive}\n`,
    { mode: 0o400 },
  );
  writeFileSync(join(artifacts, `henukit-release-${releaseSha}.manifest`), "verified-by-fixture\n", {
    mode: 0o400,
  });
  writeFileSync(join(artifacts, `henukit-release-${releaseSha}.manifest.sig`), "fixture-signature\n", {
    mode: 0o400,
  });
  return artifacts;
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
  failTargetFoodHealth = false,
  failTargetNoticeHealth = false,
  failTargetHealth = false,
  targetLibraryStartingAttempts = 0,
  missingLibraryArtifact = false,
  nonRootTrustRoot = "",
  previousHasAccountPortfolio = true,
  previousSha = "b".repeat(40),
  portalAllowMock = false,
  portalApiMode = "live",
  easypayEnabled = true,
  runConclusion = "success",
  runStatus = "completed",
} = {}) {
  // macOS exposes its temporary directory through /var, which is a symlink.
  // The production trust-root check deliberately rejects symlinked parents, so
  // create the fixture below tmpdir's canonical path instead.
  const root = mkdtempSync(join(realpathSync(tmpdir()), "henukit-actions-watch-"));
  const bin = join(root, "bin");
  const staging = join(root, "staging");
  const releases = join(root, "releases");
  const backups = join(root, "backups");
  const state = join(root, "state");
  const log = join(root, "calls.log");
  const active = join(root, "active-sha");
  const libraryHealthAttempts = join(root, "library-health-attempts");
  const token = join(root, "github.token");
  const releaseSigners = join(root, "release-signers");
  const imageInventory = join(bin, "henukit-release-images.sh");
  const localVerifier = join(bin, "verify-henukit-local-release.sh");
  const envFile = join(root, "henukit.env");
  const rollbackEnvFile = join(root, "henukit.rollback.env");
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
    `POSTGRES_USER=test\nPORTAL_API_MODE=${portalApiMode}\nNEXT_PUBLIC_PORTAL_ALLOW_MOCK=${portalAllowMock ? "1" : "0"}\nACCOUNT_PORTFOLIO_EASYPAY_ENABLED=${easypayEnabled ? "1" : "0"}\nACCOUNT_PORTFOLIO_EASYPAY_BASE_URL=https://metaview.top/epay\nACCOUNT_PORTFOLIO_EASYPAY_PID=henukit-production\nACCOUNT_PORTFOLIO_EASYPAY_KEY=henukit-production-secret-32bytes\nACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL=https://henukit.cn/api/v1/payment-providers/easypay/notifications\nACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL=https://henukit.cn/account/membership\n`,
  );
  writeFileSync(rollbackEnvFile, "ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=0\n", { mode: 0o600 });
  writeFileSync(log, "");
  writeFileSync(releaseSigners, "henukit-release fixture-key\n", { mode: 0o600 });
  writeExecutable(
    localVerifier,
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'verify-local %s\\n' "$*" >> "$FAKE_CALL_LOG"
`,
  );
  writeExecutable(
    imageInventory,
    `#!/usr/bin/env bash
set -Eeuo pipefail
images=(
  henukit-console
  henukit-console-gateway
  henukit-platform-core
  henukit-platform-mail-worker
  henukit-platform-smtp-provider
  henukit-portal
  henukit-portal-api
  henukit-account-portfolio
  henukit-notice
  henukit-notice-worker
  henukit-food
  henukit-library
  henukit-portal-gateway
)
case "\${1:-}" in
  --check) ;;
  --artifact-images|--load-images) printf '%s\\n' "\${images[@]}" ;;
  --baseline-images)
    printf '%s\\n' henukit-console henukit-console-gateway henukit-platform-core henukit-platform-mail-worker henukit-platform-smtp-provider henukit-portal henukit-portal-api henukit-portal-gateway
    ;;
  --conditional-services)
    printf '%s\\n' $'account-portfolio\\thenukit-account-portfolio' $'notice\\thenukit-notice' $'notice-worker\\thenukit-notice-worker' $'food\\thenukit-food' $'library\\thenukit-library'
    ;;
  *) exit 64 ;;
esac
`,
  );
  writeExecutable(
    join(bin, "stat"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "\${1:-}" != "-c" ]]; then
  exec /usr/bin/stat "$@"
fi
format="\${2:-}"
path="\${3:-}"
if [[ "$format" == "%a" ]]; then
  if [[ -d "$path" ]]; then printf '700'; else printf '600'; fi
  exit 0
fi
if [[ "$format" == "%u" ]]; then
  if [[ -d "$path" ]]; then
    printf '0'
  elif [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "inventory" && "$path" == "$FAKE_TRUSTED_INVENTORY" ]] ||
       [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "verifier" && "$path" == "$FAKE_TRUSTED_VERIFIER" ]] ||
       [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "signers" && "$path" == "$FAKE_TRUSTED_SIGNERS" ]]; then
    id -u
  elif [[ "$path" == "$FAKE_TRUSTED_INVENTORY" || "$path" == "$FAKE_TRUSTED_VERIFIER" || "$path" == "$FAKE_TRUSTED_SIGNERS" ]]; then
    printf '0'
  else
    id -u
  fi
  exit 0
fi
exec /usr/bin/stat "$@"
`,
  );
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
  henukit-notice
  henukit-notice-worker
  henukit-food
  henukit-library
  henukit-portal-gateway
)
for image in "\${images[@]}"; do
  if [[ "$image" == "henukit-library" && "$FAKE_MISSING_LIBRARY_ARTIFACT" == "1" ]]; then
    continue
  fi
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
printf 'services:\\n  account-portfolio:\\n  notice:\\n  notice-worker:\\n  food:\\n  library:\\n' > "$runtime_tree/docker-compose.henukit.release.yml"
cat > "$runtime_tree/release-gates/account-production-boundary.env" <<EOF
release_sha=$FAKE_RELEASE_SHA
status=$([[ "$FAKE_BAD_ACCOUNT_BOUNDARY" == "1" ]] && printf fail || printf pass)
account_console_mock_sources=absent
account_transitive_mock_sources=absent
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
    if [[ "$sha" == "$FAKE_RELEASE_SHA" ]]; then
      printf 'henukit-notice:%s\\nhenukit-notice-worker:%s\\nhenukit-food:%s\\nhenukit-library:%s\\n' "$sha" "$sha" "$sha" "$sha"
    fi
  fi
elif [[ "$1" == "inspect" ]]; then
  if [[ "$*" == *"henukit-library-1"* && -s "$FAKE_ACTIVE_FILE" &&
        "$(cat "$FAKE_ACTIVE_FILE")" == "$FAKE_RELEASE_SHA" ]]; then
    attempts=0
    if [[ -f "$FAKE_LIBRARY_HEALTH_ATTEMPTS" ]]; then
      attempts="$(cat "$FAKE_LIBRARY_HEALTH_ATTEMPTS")"
    fi
    attempts=$((attempts + 1))
    printf '%s' "$attempts" > "$FAKE_LIBRARY_HEALTH_ATTEMPTS"
    if ((attempts <= FAKE_TARGET_LIBRARY_STARTING_ATTEMPTS)); then
      printf 'starting\n'
    else
      printf 'healthy\n'
    fi
  elif [[ "$*" == *"henukit-notice-1"* && "$FAKE_FAIL_TARGET_NOTICE_HEALTH" == "1" &&
        -s "$FAKE_ACTIVE_FILE" &&
        "$(cat "$FAKE_ACTIVE_FILE")" == "$FAKE_RELEASE_SHA" ]]; then
    printf 'unhealthy\\n'
  elif [[ "$*" == *"henukit-food-1"* && "$FAKE_FAIL_TARGET_FOOD_HEALTH" == "1" &&
        -s "$FAKE_ACTIVE_FILE" &&
        "$(cat "$FAKE_ACTIVE_FILE")" == "$FAKE_RELEASE_SHA" ]]; then
    printf 'unhealthy\\n'
  elif [[ "$FAKE_FAIL_TARGET_ACCOUNT_PORTFOLIO_HEALTH" == "1" &&
        -s "$FAKE_ACTIVE_FILE" &&
        "$(cat "$FAKE_ACTIVE_FILE")" == "$FAKE_RELEASE_SHA" ]]; then
    printf 'unhealthy\\n'
  else
    printf 'healthy\\n'
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
  if [[ "$*" == *"https://henukit.cn/api/v1/account/summary"* ]]; then
    printf '401'
    exit 0
  fi
  if [[ "$*" == *"https://henukit.cn/api/v1/payment-providers/easypay/notifications"* ]]; then
    printf '400'
    exit 0
  fi
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

  writeExecutable(
    join(bin, "sleep"),
    `#!/usr/bin/env bash
printf 'sleep %s\n' "$*" >> "$FAKE_CALL_LOG"
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
      FAKE_FAIL_TARGET_FOOD_HEALTH: failTargetFoodHealth ? "1" : "0",
      FAKE_FAIL_TARGET_NOTICE_HEALTH: failTargetNoticeHealth ? "1" : "0",
      FAKE_FAIL_TARGET_HEALTH: failTargetHealth ? "1" : "0",
      FAKE_LIBRARY_HEALTH_ATTEMPTS: libraryHealthAttempts,
      FAKE_MISSING_LIBRARY_ARTIFACT: missingLibraryArtifact ? "1" : "0",
      FAKE_PREVIOUS_HAS_ACCOUNT_PORTFOLIO: previousHasAccountPortfolio ? "1" : "0",
      FAKE_RELEASE_SHA: releaseSha,
      FAKE_TARGET_LIBRARY_STARTING_ATTEMPTS: String(targetLibraryStartingAttempts),
      FAKE_NO_SUCCESS: "0",
      FAKE_RUN_CONCLUSION: runConclusion,
      FAKE_RUN_STATUS: runStatus,
      GH_TOKEN_FILE: token,
      HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE: accountOperatorRole,
      HENUKIT_BACKUP_ROOT: backups,
      HENUKIT_ENV_FILE: envFile,
      HENUKIT_IMAGE_INVENTORY: imageInventory,
      HENUKIT_RELEASE_ROOT: releases,
      HENUKIT_RELEASE_SIGNERS_FILE: releaseSigners,
      HENUKIT_STAGING_ROOT: staging,
      HENUKIT_STATE_ROOT: state,
      HENUKIT_LOCAL_ARTIFACT_VERIFIER: localVerifier,
      FAKE_NON_ROOT_TRUST_ROOT: nonRootTrustRoot,
      FAKE_TRUSTED_INVENTORY: imageInventory,
      FAKE_TRUSTED_SIGNERS: releaseSigners,
      FAKE_TRUSTED_VERIFIER: localVerifier,
      HENUKIT_PUBLIC_BASE_URL: "https://example.test",
      HENUKIT_ROLLBACK_ENV_FILE: rollbackEnvFile,
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
  assert.equal((calls.match(/docker load/g) ?? []).length, 13);
  assert.match(calls, /deploy .*releases.*henukit\.env/);
  assert.match(calls, /grant-account-operator-role/);
  assert.match(calls, /operations-operator/);
  assert.match(calls, /curl .*https:\/\/example\.test\/api\/v1\/healthz/);
  assert.match(calls, /curl .*https:\/\/henukit\.cn\/api\/v1\/account\/summary/);
  assert.match(calls, /curl .*https:\/\/henukit\.cn\/api\/v1\/payment-providers\/easypay\/notifications/);
  assert.equal(
    readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(),
    releaseSha,
  );

  execFileSync(script, ["--once"], { env: setup.env });
  const secondCalls = readFileSync(setup.log, "utf8");
  assert.equal((secondCalls.match(/^deploy /gm) ?? []).length, 1);
});

test("one-shot waits for a newly started Library healthcheck before accepting the release", () => {
  const setup = fixture({ targetLibraryStartingAttempts: 3 });

  const output = execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.match(
    output,
    new RegExp(`release ${releaseSha} activated and deterministic smoke checks passed`),
  );
  assert.equal(readFileSync(join(setup.root, "library-health-attempts"), "utf8"), "4");
  assert.equal((calls.match(/^sleep 2$/gm) ?? []).length, 3);
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

test("an active release without an activation record converges the audited permission grant", () => {
  const setup = fixture();
  execFileSync(script, ["--once"], { env: setup.env });
  unlinkSync(join(setup.state, "last-activated-sha"));

  execFileSync(script, ["--once"], { env: setup.env });
  const calls = readFileSync(setup.log, "utf8");

  assert.equal((calls.match(/^deploy /gm) ?? []).length, 1);
  assert.equal((calls.match(/grant-account-operator-role/g) ?? []).length, 2);
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
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

test("a signed local artifact set uses the same backup, exact-SHA approval, and rollback path", () => {
  const setup = fixture({ approved: false });
  const artifacts = writeLocalArtifacts(setup.root);
  const args = ["--local-artifacts", artifacts, "--sha", releaseSha];

  const prepared = execFileSync(script, args, {
    encoding: "utf8",
    env: setup.env,
  });
  let calls = readFileSync(setup.log, "utf8");
  assert.match(prepared, /awaits exact-SHA approval/);
  assert.match(calls, /verify-local .*--artifact-dir/);
  assert.equal(
    (calls.match(/verify-local/g) ?? []).length,
    2,
    "the root-owned staging copy is signature-verified again after transfer",
  );
  assert.match(calls, /pg_dump/);
  assert.doesNotMatch(calls, /docker load|^deploy /m);

  writeFileSync(join(setup.state, "approvals", releaseSha), `${releaseSha}\n`, {
    mode: 0o600,
  });
  const activated = execFileSync(script, args, {
    encoding: "utf8",
    env: setup.env,
  });
  calls = readFileSync(setup.log, "utf8");
  assert.match(activated, /activated and deterministic smoke checks passed/);
  assert.equal((calls.match(/docker load/g) ?? []).length, 13);
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
});

test("a local artifact path rejects every non-root trust root before backup or deployment", () => {
  for (const trustRoot of ["inventory", "verifier", "signers"]) {
    const setup = fixture({ nonRootTrustRoot: trustRoot });
    const artifacts = writeLocalArtifacts(setup.root);
    const result = spawnSync(
      script,
      ["--local-artifacts", artifacts, "--sha", releaseSha],
      { encoding: "utf8", env: setup.env },
    );

    assert.notEqual(result.status, 0, `${trustRoot} must be root-owned`);
    assert.match(result.stderr, /must be owned by root/i);
    assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
  }
});

test("a relative trust-root override fails closed before backup or deployment", () => {
  const setup = fixture();
  const artifacts = writeLocalArtifacts(setup.root);
  const result = spawnSync(
    script,
    ["--local-artifacts", artifacts, "--sha", releaseSha],
    {
      encoding: "utf8",
      env: { ...setup.env, HENUKIT_IMAGE_INVENTORY: "relative-inventory" },
    },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /absolute path/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
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

test("a missing Library archive stops before backup, image load, and deploy", () => {
  const setup = fixture({ missingLibraryArtifact: true });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /missing henukit-library/i);
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

test("watch mode honors a maintenance request bound to its live instance at a safe boundary", async () => {
  const setup = fixture();
  const quiesce = join(setup.state, "quiesce.request");
  const instance = join(setup.state, "watcher.instance");
  const child = spawn(script, ["--watch"], {
    env: { ...setup.env, HENUKIT_POLL_SECONDS: "1" },
  });
  let stdout = "";
  let stderr = "";
  child.stdout.setEncoding("utf8");
  child.stderr.setEncoding("utf8");
  child.stdout.on("data", (chunk) => { stdout += chunk; });
  child.stderr.on("data", (chunk) => { stderr += chunk; });

  const deadline = Date.now() + 10_000;
  while (!existsSync(instance) && Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  assert.equal(existsSync(instance), true, "watcher did not publish its instance id");
  const instanceId = readFileSync(instance, "utf8").trim();
  assert.match(instanceId, /^[1-9][0-9]*$/);
  const nonce = "b".repeat(32);
  writeFileSync(quiesce, `${releaseSha} ${instanceId} ${nonce}\n`, { mode: 0o600 });
  chmodSync(quiesce, 0o600);

  const status = await new Promise((resolve) => child.on("close", resolve));

  assert.equal(status, 0, stderr);
  assert.match(stdout, /quiesce requested (at a safe boundary|after a completed check)/i);
  assert.equal(
    readFileSync(join(setup.state, "quiesced"), "utf8").trim(),
    `${releaseSha} ${instanceId} ${nonce}`,
  );
});

test("watch mode ignores a stale request bound to a prior watcher instance", () => {
  const setup = fixture({ badChecksum: true });
  writeFileSync(
    join(setup.state, "quiesce.request"),
    `${releaseSha} 999999 ${"c".repeat(32)}\n`,
    { mode: 0o600 },
  );

  const result = spawnSync(script, ["--watch"], {
    encoding: "utf8",
    env: { ...setup.env, HENUKIT_POLL_SECONDS: "1" },
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /ignoring stale quiesce request/i);
  assert.match(result.stderr, /checksum|FAILED/i);
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
  assert.match(calls, /deploy .*henukit\.rollback\.env/);
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
  assert.equal((calls.match(/docker load/g) ?? []).length, 13);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), releaseSha);
});

test("failed Notice health restores and verifies the previous fixed-SHA release", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ failTargetNoticeHealth: true, previousSha });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.match(calls, /docker inspect .*henukit-notice-1/);
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 2);
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

test("one-shot refuses while the Account EasyPay provider remains disabled", () => {
  const setup = fixture({ easypayEnabled: false });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /ACCOUNT_PORTFOLIO_EASYPAY_ENABLED must be 1/i);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});
