import assert from "node:assert/strict";
import { execFileSync, spawn, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  chmodSync,
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  realpathSync,
  renameSync,
  symlinkSync,
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
  "henukit-portal-summary",
  "henukit-portal-api",
  "henukit-account-portfolio",
  "henukit-notice",
  "henukit-notice-worker",
  "henukit-food",
  "henukit-food-mcp",
  "henukit-library",
  "henukit-career-opportunities",
  "henukit-career-mcp",
  "henukit-portal-gateway",
  "henukit-quizcraft",
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

function writeMaterialsRuntime(runtimeRoot, content = "fixture-materials-runtime\n") {
  const materials = join(runtimeRoot, "materials-runtime");
  mkdirSync(materials, { recursive: true });
  writeFileSync(join(materials, "marker"), content, { mode: 0o444 });
  const digest = createHash("sha256").update(content).digest("hex");
  writeFileSync(join(materials, "SHA256SUMS"), `${digest}  marker\n`, {
    mode: 0o444,
  });
}

function reopenCompletedRollbackContract(setup) {
  const completed = join(setup.state, "rollback-contracts", "completed");
  const contracts = readdirSync(completed).filter((name) => name.startsWith(`${releaseSha}.`));
  assert.equal(contracts.length, 1);
  renameSync(
    join(completed, contracts[0]),
    join(setup.state, "rollback-contracts", "pending", releaseSha),
  );
}

function writeLocalArtifacts(root) {
  const artifacts = join(root, "local-artifacts");
  const runtimeTree = join(root, "local-runtime");
  mkdirSync(artifacts);
  mkdirSync(join(runtimeTree, "bin"), { recursive: true });
  mkdirSync(join(runtimeTree, "release-gates"), { recursive: true });
  writeMaterialsRuntime(runtimeTree);
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
    "services:\n  portal-summary:\n  account-portfolio:\n  notice:\n  notice-worker:\n  food:\n  library:\n  quizcraft:\n",
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
printf 'deploy %s\\n' "$*" >> "$FAKE_CALL_LOG"
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
  failTargetPracticeFlow = false,
  failTargetHealth = false,
  failCandidateDeployBeforeSwitch = false,
  partialCandidateSwitch = false,
  candidateMaterialsDiffer = false,
  mutateRollbackEnvOnFailure = false,
  missingMaterialsRunnerUnit = false,
  failedMaterialsRunnerUnit = false,
  maskedMaterialsPath = false,
  materialsPathInitiallyEnabled = false,
  breakMaterialsWebhookBeforeFailure = false,
  runnerActiveAtQuiesceAttempts = 0,
  runnerActiveAfterEnableAttempts = 0,
  deployWebhookPresent = false,
  failDeployWebhookAfterRestart = false,
  failPreviousHealth = false,
  failAccountGrant = false,
  targetLibraryStartingAttempts = 0,
  missingLibraryArtifact = false,
  nonRootTrustRoot = "",
  incompletePreviousRelease = "",
  includeOAuthContinuationArtifact = true,
  tamperOAuthContinuationArtifact = false,
  previousHasAccountPortfolio = true,
  previousSha = "b".repeat(40),
  portalAllowMock = false,
  portalApiMode = "live",
  easypayEnabled = true,
  careerAIBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1",
  careerAIKey = "sk-live-fixture-credential-32bytes",
  careerAllowInsecureAIHTTP = false,
  careerSuifyAllowInsecureAIHTTP = false,
  careerDigestSecret = "fixture-career-digest-random-credential-48bytes",
  runConclusion = "success",
  runStatus = "completed",
  legacyRuntimePresent = false,
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
  const currentLink = join(root, "current");
  const libraryHealthAttempts = join(root, "library-health-attempts");
  const mixedRuntime = join(root, "mixed-runtime");
  const materialsWebhookUnhealthy = join(root, "materials-webhook-unhealthy");
  const materialsPathState = join(root, "materials-path-state");
  const runnerQuiesceAttempts = join(root, "runner-quiesce-attempts");
  const runnerRestoreAttempts = join(root, "runner-restore-attempts");
  const deployWebhookRestarted = join(root, "deploy-webhook-restarted");
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
    `POSTGRES_USER=test\nPORTAL_API_MODE=${portalApiMode}\nNEXT_PUBLIC_PORTAL_ALLOW_MOCK=${portalAllowMock ? "1" : "0"}\nACCOUNT_PORTFOLIO_EASYPAY_ENABLED=${easypayEnabled ? "1" : "0"}\nACCOUNT_PORTFOLIO_EASYPAY_BASE_URL=https://metaview.top/epay\nACCOUNT_PORTFOLIO_EASYPAY_PID=henukit-production\nACCOUNT_PORTFOLIO_EASYPAY_KEY=henukit-production-secret-32bytes\nACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL=https://henukit.cn/api/v1/payment-providers/easypay/notifications\nACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL=https://henukit.cn/account/membership\nCAREER_SOURCE_ALLOWLIST=official.meituan\nCAREER_AI_BASE_URL=${careerAIBaseURL}\nCAREER_AI_API_KEY=${careerAIKey}\nCAREER_AI_MODEL=qwen3.6-plus\nCAREER_ALLOW_INSECURE_AI_HTTP=${careerAllowInsecureAIHTTP ? "1" : "0"}\nCAREER_SUIFY_ALLOW_INSECURE_AI_HTTP=${careerSuifyAllowInsecureAIHTTP ? "1" : "0"}\nPLATFORM_CORE_CAREER_DIGEST_CLIENT_ID=career-opportunities\nPLATFORM_CORE_CAREER_DIGEST_KEY_ID=career-digest-key-1\nPLATFORM_CORE_CAREER_DIGEST_SECRET=${careerDigestSecret}\n`,
  );
  writeFileSync(rollbackEnvFile, "ACCOUNT_PORTFOLIO_EASYPAY_ENABLED=0\n", { mode: 0o600 });
  writeFileSync(log, "");
  writeFileSync(materialsPathState, materialsPathInitiallyEnabled ? "enabled\n" : "disabled\n");
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
  henukit-portal-summary
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
    printf '%s\\n' henukit-console henukit-console-gateway henukit-platform-core henukit-platform-mail-worker henukit-platform-smtp-provider henukit-portal henukit-portal-summary henukit-portal-api henukit-portal-gateway
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
  if [[ -d "$path" ]]; then
    printf '700'
  elif [[ "$path" == "$FAKE_STATE_ROOT/degraded-recoveries/"* ||
          "$path" == "$FAKE_STATE_ROOT/rollback-contracts/"* ]]; then
    printf '400'
  else
    printf '600'
  fi
  exit 0
fi
if [[ "$format" == "%u" ]]; then
  if [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "current-link" && "$path" == "$FAKE_CURRENT_LINK" ]]; then
    printf '1000'
  elif [[ -d "$path" ]]; then
    printf '0'
  elif [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "inventory" && "$path" == "$FAKE_TRUSTED_INVENTORY" ]] ||
       [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "verifier" && "$path" == "$FAKE_TRUSTED_VERIFIER" ]] ||
       [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "signers" && "$path" == "$FAKE_TRUSTED_SIGNERS" ]]; then
    id -u
  elif [[ "$path" == "$FAKE_TRUSTED_INVENTORY" || "$path" == "$FAKE_TRUSTED_VERIFIER" || "$path" == "$FAKE_TRUSTED_SIGNERS" || "$path" == "$FAKE_CURRENT_LINK" || "$path" == "$FAKE_ROLLBACK_ENV_FILE" || "$path" == "$FAKE_RELEASE_ROOT"/* || "$path" == "$FAKE_BACKUP_ROOT"/* || "$path" == "$FAKE_STATE_ROOT/approvals/"* || "$path" == "$FAKE_STATE_ROOT/prepared/"* || "$path" == "$FAKE_STATE_ROOT/practice-smoke-"* ]]; then
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
    writeMaterialsRuntime(previousRelease);
    writeExecutable(
      join(previousRelease, "bin", "deploy-henukit-artifact.sh"),
      `#!/usr/bin/env bash
set -Eeuo pipefail
cat "$1/RELEASE_SHA" > "$FAKE_ACTIVE_FILE"
printf 'deploy %s\\n' "$*" >> "$FAKE_CALL_LOG"
`,
    );
    if (incompletePreviousRelease === "marker") {
      unlinkSync(join(previousRelease, "RELEASE_SHA"));
    } else if (incompletePreviousRelease === "helper") {
      unlinkSync(join(previousRelease, "bin", "deploy-henukit-artifact.sh"));
    } else if (incompletePreviousRelease === "compose") {
      unlinkSync(join(previousRelease, "docker-compose.henukit.release.yml"));
    }
    writeFileSync(active, `${previousSha}\n`);
    symlinkSync(previousRelease, currentLink, "dir");
  }

  writeExecutable(
    join(bin, "gh"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "$FAKE_GH_MUST_NOT_RUN" == "1" ]]; then exit 99; fi
if [[ "$1 $2" == "run list" ]]; then
  if [[ "$FAKE_NO_SUCCESS" == "1" ]]; then
    exit 0
  fi
  printf '123\\t%s\\t%s\\t%s\\thttps://github.example/actions/runs/123\\n' \
    "$FAKE_RUN_RELEASE_SHA" "$FAKE_RUN_STATUS" "$FAKE_RUN_CONCLUSION"
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
  henukit-portal-summary
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
if [[ "$FAKE_INCLUDE_OAUTH_CONTINUATION_ARTIFACT" == "1" ]]; then
  continuation_artifact="$dest/oauth-continuation-$FAKE_RELEASE_SHA"
  mkdir -p "$continuation_artifact"
  cat > "$continuation_artifact/oauth-continuation-$FAKE_RELEASE_SHA.env" <<EOF
format=henukit-oauth-continuation-gate-v1
release_sha=$FAKE_RELEASE_SHA
source_tree=cccccccccccccccccccccccccccccccccccccccc
result=$([[ "$FAKE_TAMPER_OAUTH_CONTINUATION_ARTIFACT" == "1" ]] && printf fail || printf pass)
EOF
fi
runtime_artifact="$dest/henukit-runtime-$FAKE_RELEASE_SHA"
runtime_tree="$(mktemp -d "\${TMPDIR:-/tmp}/henukit-runtime-tree.XXXXXX")"
mkdir -p "$runtime_artifact" "$runtime_tree/bin" "$runtime_tree/release-gates" "$runtime_tree/materials-runtime"
printf '%s\\n' "$FAKE_RELEASE_SHA" > "$runtime_tree/RELEASE_SHA"
printf 'services:\\n  portal-summary:\\n  account-portfolio:\\n  notice:\\n  notice-worker:\\n  food:\\n  library:\\n  quizcraft:\\n' > "$runtime_tree/docker-compose.henukit.release.yml"
if [[ "$FAKE_CANDIDATE_MATERIALS_DIFFER" == "1" ]]; then
  printf 'different-candidate-materials\\n' > "$runtime_tree/materials-runtime/marker"
else
  printf 'fixture-materials-runtime\\n' > "$runtime_tree/materials-runtime/marker"
fi
(
  cd "$runtime_tree/materials-runtime"
  sha256sum marker > SHA256SUMS
)
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
cat > "$runtime_tree/release-gates/oauth-continuation.env" <<EOF
format=henukit-oauth-continuation-gate-v1
release_sha=$FAKE_RELEASE_SHA
source_tree=cccccccccccccccccccccccccccccccccccccccc
result=pass
EOF
cat > "$runtime_tree/bin/deploy-henukit-artifact.sh" <<'HELPER'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'deploy %s\\n' "$*" >> "$FAKE_CALL_LOG"
if [[ "$FAKE_MUTATE_ROLLBACK_ENV_ON_FAILURE" == "1" ]]; then
  printf 'TAMPERED=1\\n' >> "$FAKE_ROLLBACK_ENV_FILE"
fi
if [[ "$FAKE_BREAK_MATERIALS_WEBHOOK_BEFORE_FAILURE" == "1" ]]; then
  : > "$FAKE_MATERIALS_WEBHOOK_UNHEALTHY"
fi
if [[ "$FAKE_FAIL_CANDIDATE_DEPLOY_BEFORE_SWITCH" == "1" ]]; then exit 1; fi
printf 'disabled\\n' > "$FAKE_MATERIALS_PATH_STATE"
if [[ "$FAKE_PARTIAL_CANDIDATE_SWITCH" == "1" && "$(cat "$1/RELEASE_SHA")" == "$FAKE_RELEASE_SHA" ]]; then
  : > "$FAKE_MIXED_RUNTIME"
  exit 1
fi
rm -f -- "$FAKE_MIXED_RUNTIME"
cat "$1/RELEASE_SHA" > "$FAKE_ACTIVE_FILE"
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
if [[ "$1 \${2:-}" == "ps -a" ]]; then
  if [[ "$FAKE_LEGACY_RUNTIME_PRESENT" == "1" && -s "$FAKE_ACTIVE_FILE" &&
        "$(cat "$FAKE_ACTIVE_FILE")" == "$FAKE_RELEASE_SHA" ]]; then
    printf 'henukit-study-api-1\\n'
  fi
elif [[ "$1" == "ps" ]]; then
  if [[ -s "$FAKE_ACTIVE_FILE" ]]; then
    sha="$(cat "$FAKE_ACTIVE_FILE")"
    for image in henukit-console henukit-console-gateway henukit-platform-core henukit-platform-mail-worker henukit-platform-smtp-provider henukit-portal henukit-portal-api henukit-portal-gateway; do
      printf '%s:%s\\n' "$image" "$sha"
    done
    if [[ "$sha" == "$FAKE_RELEASE_SHA" ]]; then
      printf 'henukit-portal-summary:%s\\n' "$sha"
    fi
    if [[ "$sha" == "$FAKE_RELEASE_SHA" || "$FAKE_PREVIOUS_HAS_ACCOUNT_PORTFOLIO" == "1" ]]; then
      printf 'henukit-account-portfolio:%s\\n' "$sha"
    fi
    if [[ "$sha" == "$FAKE_RELEASE_SHA" ]]; then
      printf 'henukit-notice:%s\\nhenukit-notice-worker:%s\\nhenukit-food:%s\\nhenukit-library:%s\\n' "$sha" "$sha" "$sha" "$sha"
    fi
    if [[ -e "$FAKE_MIXED_RUNTIME" ]]; then
      printf 'henukit-future-owner:%s\\n' "$FAKE_RELEASE_SHA"
    fi
  fi
elif [[ "$1" == "compose" && "$*" == *" up -d --remove-orphans"* ]]; then
  printf '%s\\n' "$RELEASE_SHA" > "$FAKE_ACTIVE_FILE"
  rm -f -- "$FAKE_MIXED_RUNTIME"
elif [[ "$1" == "inspect" ]]; then
  if [[ "$*" == *".Config.Image"* ]]; then
    container="\${@: -1}"
    image="\${container#henukit-}"
    image="\${image%-1}"
    printf 'henukit-%s:%s\n' "$image" "$(cat "$FAKE_ACTIVE_FILE")"
  elif [[ "$*" == *"henukit-library-1"* && -s "$FAKE_ACTIVE_FILE" &&
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
elif [[ "$1" == "exec" && "$*" == *"grant-account-operator-role"* && "$FAKE_FAIL_ACCOUNT_GRANT" == "1" ]]; then
  exit 1
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
if [[ "$FAKE_FAIL_PREVIOUS_HEALTH" == "1" &&
      -s "$FAKE_ACTIVE_FILE" &&
      "$(cat "$FAKE_ACTIVE_FILE")" != "$FAKE_RELEASE_SHA" ]]; then
  exit 22
fi
if [[ "$FAKE_FAIL_TARGET_PRACTICE_FLOW" == "1" &&
      -s "$FAKE_ACTIVE_FILE" &&
      "$(cat "$FAKE_ACTIVE_FILE")" == "$FAKE_RELEASE_SHA" &&
      "$*" == *"/api/v1/practice/catalog"* ]]; then
  exit 22
fi
if [[ "$*" == *"/api/v1/practice/catalog"* ]]; then
  printf '{"banks":[{"bank_id":"11111111-1111-4111-8111-111111111111","bank_version_id":"22222222-2222-4222-8222-222222222222","available":true,"question_count":1}]}'
  exit 0
fi
if [[ "$*" == *"/api/v1/practice/sessions/"*"/answers"* ]]; then
  if [[ "$*" == *"--write-out"* ]]; then printf '404'; fi
  exit 0
fi
if [[ "$*" == *"/api/v1/practice/sessions"* ]]; then
  if [[ "$*" == *"--write-out"* ]]; then printf '400'; fi
  exit 0
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
    join(bin, "systemctl"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'systemctl %s\\n' "$*" >> "$FAKE_CALL_LOG"
if [[ "\${1:-} \${2:-} \${3:-} \${4:-}" == "show -p LoadState --value" ]]; then
  unit="\${5:-}"
  if [[ "$unit" == "henukit-materials-runner.service" && "$FAKE_MISSING_MATERIALS_RUNNER_UNIT" == "1" ]]; then
    printf 'not-found\\n'
  elif [[ "$unit" == "henukit-deploy-webhook.service" && "$FAKE_DEPLOY_WEBHOOK_PRESENT" != "1" ]]; then
    printf 'not-found\\n'
  else
    printf 'loaded\\n'
  fi
  exit 0
fi
if [[ "\${1:-} \${2:-} \${3:-} \${4:-}" == "show -p ActiveState --value" ]]; then
  unit="\${5:-}"
  if [[ "$unit" == "henukit-materials-webhook.service" && -e "$FAKE_MATERIALS_WEBHOOK_UNHEALTHY" ]]; then
    printf 'failed\\n'
  elif [[ "$unit" == "henukit-materials-runner.service" && -e "$FAKE_RUNNER_QUIESCE_ATTEMPTS" ]]; then
    attempts="$(cat "$FAKE_RUNNER_QUIESCE_ATTEMPTS")"
    attempts=$((attempts + 1))
    printf '%s' "$attempts" > "$FAKE_RUNNER_QUIESCE_ATTEMPTS"
    if ((attempts <= FAKE_RUNNER_ACTIVE_AT_QUIESCE_ATTEMPTS)); then
      printf 'active\\n'
    else
      rm -f -- "$FAKE_RUNNER_QUIESCE_ATTEMPTS"
      printf 'inactive\\n'
    fi
  elif [[ "$unit" == "henukit-materials-runner.service" && -e "$FAKE_RUNNER_RESTORE_ATTEMPTS" ]]; then
    attempts="$(cat "$FAKE_RUNNER_RESTORE_ATTEMPTS")"
    attempts=$((attempts + 1))
    printf '%s' "$attempts" > "$FAKE_RUNNER_RESTORE_ATTEMPTS"
    if ((attempts <= FAKE_RUNNER_ACTIVE_AFTER_ENABLE_ATTEMPTS)); then
      printf 'active\\n'
    else
      rm -f -- "$FAKE_RUNNER_RESTORE_ATTEMPTS"
      printf 'inactive\\n'
    fi
  elif [[ "$unit" == "henukit-materials-runner.service" && "$FAKE_FAILED_MATERIALS_RUNNER_UNIT" == "1" ]]; then
    printf 'failed\\n'
  elif [[ "$unit" == "henukit-materials-runner.service" ]]; then
    printf 'inactive\\n'
  elif [[ "$unit" == "henukit-deploy-webhook.service" && "$FAKE_FAIL_DEPLOY_WEBHOOK_AFTER_RESTART" == "1" && -e "$FAKE_DEPLOY_WEBHOOK_RESTARTED" ]]; then
    printf 'failed\\n'
  else
    printf 'active\\n'
  fi
  exit 0
fi
if [[ "\${1:-} \${2:-}" == "is-enabled henukit-materials-webhook.path" ]]; then
  if [[ "$FAKE_MASKED_MATERIALS_PATH" == "1" ]]; then
    printf 'masked\\n'
  else
    cat "$FAKE_MATERIALS_PATH_STATE"
  fi
  exit 1
fi
if [[ "\${1:-} \${2:-} \${3:-}" == "disable --now henukit-materials-webhook.path" ]]; then
  printf 'disabled\\n' > "$FAKE_MATERIALS_PATH_STATE"
  if ((FAKE_RUNNER_ACTIVE_AT_QUIESCE_ATTEMPTS > 0)); then printf '0' > "$FAKE_RUNNER_QUIESCE_ATTEMPTS"; fi
  exit 0
fi
if [[ "\${1:-} \${2:-} \${3:-}" == "enable --now henukit-materials-webhook.path" ]]; then
  printf 'enabled\\n' > "$FAKE_MATERIALS_PATH_STATE"
  if ((FAKE_RUNNER_ACTIVE_AFTER_ENABLE_ATTEMPTS > 0)); then printf '0' > "$FAKE_RUNNER_RESTORE_ATTEMPTS"; fi
  exit 0
fi
if [[ "\${1:-} \${2:-}" == "restart henukit-materials-webhook.service" ]]; then
  rm -f -- "$FAKE_MATERIALS_WEBHOOK_UNHEALTHY"
  exit 0
fi
if [[ "\${1:-} \${2:-}" == "restart henukit-deploy-webhook.service" ]]; then
  : > "$FAKE_DEPLOY_WEBHOOK_RESTARTED"
  exit 0
fi
exit 0
`,
  );

  writeExecutable(
    join(bin, "sleep"),
    `#!/usr/bin/env bash
printf 'sleep %s\n' "$*" >> "$FAKE_CALL_LOG"
`,
  );

  return {
    active,
    libraryHealthAttempts,
    root,
    log,
    materialsPathState,
    mixedRuntime,
    state,
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      FAKE_ACTIVE_FILE: active,
      FAKE_ACCOUNT_PORTFOLIO_SCHEMA_PRESENT: accountPortfolioSchemaPresent ? "t" : "f",
      FAKE_BAD_ACCOUNT_BOUNDARY: badAccountBoundary ? "1" : "0",
      FAKE_BAD_CHECKSUM: badChecksum ? "1" : "0",
      FAKE_BACKUP_ROOT: backups,
      FAKE_CANDIDATE_MATERIALS_DIFFER: candidateMaterialsDiffer ? "1" : "0",
      FAKE_BREAK_MATERIALS_WEBHOOK_BEFORE_FAILURE: breakMaterialsWebhookBeforeFailure ? "1" : "0",
      FAKE_DEPLOY_WEBHOOK_PRESENT: deployWebhookPresent ? "1" : "0",
      FAKE_DEPLOY_WEBHOOK_RESTARTED: deployWebhookRestarted,
      FAKE_FAIL_DEPLOY_WEBHOOK_AFTER_RESTART: failDeployWebhookAfterRestart ? "1" : "0",
      FAKE_BRANCH_SHA: branchSha,
      FAKE_CANONICAL_QUIZ_REDIRECT: canonicalQuizRedirect ? "1" : "0",
      FAKE_CALL_LOG: log,
      FAKE_CURRENT_LINK: currentLink,
      FAKE_FAIL_TARGET_ACCOUNT_PORTFOLIO_HEALTH: failTargetAccountPortfolioHealth ? "1" : "0",
      FAKE_FAIL_TARGET_FOOD_HEALTH: failTargetFoodHealth ? "1" : "0",
      FAKE_FAIL_TARGET_NOTICE_HEALTH: failTargetNoticeHealth ? "1" : "0",
      FAKE_FAIL_TARGET_PRACTICE_FLOW: failTargetPracticeFlow ? "1" : "0",
      FAKE_FAIL_TARGET_HEALTH: failTargetHealth ? "1" : "0",
      FAKE_FAIL_CANDIDATE_DEPLOY_BEFORE_SWITCH: failCandidateDeployBeforeSwitch ? "1" : "0",
      FAKE_PARTIAL_CANDIDATE_SWITCH: partialCandidateSwitch ? "1" : "0",
      FAKE_MUTATE_ROLLBACK_ENV_ON_FAILURE: mutateRollbackEnvOnFailure ? "1" : "0",
      FAKE_MIXED_RUNTIME: mixedRuntime,
      FAKE_FAIL_PREVIOUS_HEALTH: failPreviousHealth ? "1" : "0",
      FAKE_FAIL_ACCOUNT_GRANT: failAccountGrant ? "1" : "0",
      FAKE_LIBRARY_HEALTH_ATTEMPTS: libraryHealthAttempts,
      FAKE_LEGACY_RUNTIME_PRESENT: legacyRuntimePresent ? "1" : "0",
      FAKE_INCLUDE_OAUTH_CONTINUATION_ARTIFACT: includeOAuthContinuationArtifact ? "1" : "0",
      FAKE_TAMPER_OAUTH_CONTINUATION_ARTIFACT: tamperOAuthContinuationArtifact ? "1" : "0",
      FAKE_MISSING_LIBRARY_ARTIFACT: missingLibraryArtifact ? "1" : "0",
      FAKE_MISSING_MATERIALS_RUNNER_UNIT: missingMaterialsRunnerUnit ? "1" : "0",
      FAKE_FAILED_MATERIALS_RUNNER_UNIT: failedMaterialsRunnerUnit ? "1" : "0",
      FAKE_MASKED_MATERIALS_PATH: maskedMaterialsPath ? "1" : "0",
      FAKE_MATERIALS_WEBHOOK_UNHEALTHY: materialsWebhookUnhealthy,
      FAKE_MATERIALS_PATH_STATE: materialsPathState,
      FAKE_RUNNER_ACTIVE_AT_QUIESCE_ATTEMPTS: String(runnerActiveAtQuiesceAttempts),
      FAKE_RUNNER_ACTIVE_AFTER_ENABLE_ATTEMPTS: String(runnerActiveAfterEnableAttempts),
      FAKE_RUNNER_QUIESCE_ATTEMPTS: runnerQuiesceAttempts,
      FAKE_RUNNER_RESTORE_ATTEMPTS: runnerRestoreAttempts,
      FAKE_PREVIOUS_HAS_ACCOUNT_PORTFOLIO: previousHasAccountPortfolio ? "1" : "0",
      FAKE_RELEASE_SHA: releaseSha,
      FAKE_RELEASE_ROOT: releases,
      FAKE_ROLLBACK_ENV_FILE: rollbackEnvFile,
      FAKE_TARGET_LIBRARY_STARTING_ATTEMPTS: String(targetLibraryStartingAttempts),
      FAKE_NO_SUCCESS: "0",
      FAKE_GH_MUST_NOT_RUN: "0",
      FAKE_RUN_CONCLUSION: runConclusion,
      FAKE_RUN_RELEASE_SHA: releaseSha,
      FAKE_RUN_STATUS: runStatus,
      FAKE_STATE_ROOT: state,
      GH_TOKEN_FILE: token,
      HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE: accountOperatorRole,
      HENUKIT_ACTIVE_RELEASE_ATTEMPTS: "5",
      HENUKIT_BACKUP_ROOT: backups,
      HENUKIT_ENV_FILE: envFile,
      HENUKIT_CURRENT_LINK: currentLink,
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
  assert.equal((calls.match(/docker load/g) ?? []).length, 14);
  assert.match(calls, /docker exec henukit-portal-summary-1 .*verify-summary/);
  assert.match(calls, /deploy .*releases.*henukit\.env/);
  assert.match(calls, /grant-account-operator-role/);
  assert.match(calls, /operations-operator/);
  assert.match(calls, /curl .*https:\/\/example\.test\/api\/v1\/healthz/);
  assert.match(calls, /curl .*\/api\/v1\/practice\/catalog/);
  assert.match(calls, /curl .*\/api\/v1\/practice\/sessions/);
  assert.match(calls, /curl .*\/answers/);
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

test("one-shot accepts the SHA-bound OAuth continuation gate receipt", () => {
  const setup = fixture({ includeOAuthContinuationArtifact: true });

  const output = execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.match(
    output,
    new RegExp(`release ${releaseSha} activated and deterministic smoke checks passed`),
  );
});

test("one-shot rejects a continuation gate receipt that differs from the packaged runtime", () => {
  const setup = fixture({
    includeOAuthContinuationArtifact: true,
    tamperOAuthContinuationArtifact: true,
  });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /continuation gate receipt does not match the packaged runtime/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
});

test("one-shot rejects a missing workflow continuation gate receipt", () => {
  const setup = fixture({ includeOAuthContinuationArtifact: false });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /workflow continuation gate receipt is missing/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
});

test("activation restores an enabled materials path after the quiesced release window", () => {
  const setup = fixture({ materialsPathInitiallyEnabled: true });

  execFileSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");
  assert.match(calls, /systemctl disable --now henukit-materials-webhook\.path/);
  assert.match(calls, /systemctl enable --now henukit-materials-webhook\.path/);
});

test("activation drains a runner that starts at quiesce and after path restoration", () => {
  const setup = fixture({
    materialsPathInitiallyEnabled: true,
    runnerActiveAtQuiesceAttempts: 2,
    runnerActiveAfterEnableAttempts: 2,
  });

  execFileSync(script, ["--once"], { env: setup.env });
  const calls = readFileSync(setup.log, "utf8");

  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");
  assert.equal((calls.match(/^sleep 2$/gm) ?? []).length, 4);
  assert.doesNotMatch(calls, /systemctl stop henukit-materials-runner\.service/);
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
  const setup = fixture({ materialsPathInitiallyEnabled: true });
  execFileSync(script, ["--once"], { env: setup.env });
  reopenCompletedRollbackContract(setup);
  unlinkSync(join(setup.state, "last-activated-sha"));
  writeFileSync(setup.materialsPathState, "disabled\n");
  writeFileSync(setup.libraryHealthAttempts, "0");
  setup.env.FAKE_TARGET_LIBRARY_STARTING_ATTEMPTS = "3";

  execFileSync(script, ["--once"], { env: setup.env });
  const calls = readFileSync(setup.log, "utf8");

  assert.equal((calls.match(/^deploy /gm) ?? []).length, 1);
  assert.equal((calls.match(/grant-account-operator-role/g) ?? []).length, 2);
  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");
  assert.equal(readFileSync(setup.libraryHealthAttempts, "utf8"), "4");
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
});

test("restart after materials quiesce restores the previous runtime before requesting a new approval", () => {
  const previousSha = "b".repeat(40);
  const setup = fixture({ materialsPathInitiallyEnabled: true, previousSha });
  execFileSync(script, ["--once"], { env: setup.env });
  reopenCompletedRollbackContract(setup);
  unlinkSync(join(setup.state, "last-activated-sha"));
  writeFileSync(setup.active, `${previousSha}\n`);
  writeFileSync(setup.materialsPathState, "disabled\n");

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /interrupted release .* reconciled/i);
  assert.equal(readFileSync(setup.active, "utf8").trim(), previousSha);
  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");
  assert.doesNotMatch(calls, /docker compose --env-file .*henukit\.rollback\.env/);
});

test("restart after a partial candidate switch uses the persisted hash contract to roll back", () => {
  const previousSha = "b".repeat(40);
  const setup = fixture({ materialsPathInitiallyEnabled: true, previousSha });
  execFileSync(script, ["--once"], { env: setup.env });
  reopenCompletedRollbackContract(setup);
  unlinkSync(join(setup.state, "last-activated-sha"));
  writeFileSync(setup.active, `${previousSha}\n`);
  writeFileSync(setup.materialsPathState, "disabled\n");
  writeFileSync(setup.mixedRuntime, "mixed\n");
  setup.env.FAKE_BRANCH_SHA = "d".repeat(40);
  setup.env.FAKE_NO_SUCCESS = "1";
  setup.env.FAKE_GH_MUST_NOT_RUN = "1";

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /interrupted release .* reconciled/i);
  assert.equal(existsSync(setup.mixedRuntime), false);
  assert.equal(readFileSync(setup.active, "utf8").trim(), previousSha);
  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");
  assert.match(calls, /docker compose --env-file .*henukit\.rollback\.env .* up -d --remove-orphans/);
});

for (const [label, envKey] of [
  ["an unhealthy exact candidate", "FAKE_FAIL_TARGET_HEALTH"],
  ["an exact candidate with a legacy runtime container", "FAKE_LEGACY_RUNTIME_PRESENT"],
]) {
  test(`restart rolls back ${label} instead of recording activation`, () => {
    const previousSha = "b".repeat(40);
    const setup = fixture({ previousSha });
    execFileSync(script, ["--once"], { env: setup.env });
    reopenCompletedRollbackContract(setup);
    unlinkSync(join(setup.state, "last-activated-sha"));
    setup.env[envKey] = "1";

    const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /interrupted release .* reconciled/i);
    assert.equal(readFileSync(setup.active, "utf8").trim(), previousSha);
    assert.equal(existsSync(join(setup.state, "last-activated-sha")), false);
  });
}

test("an approved pending candidate blocks a newer main candidate from creating a second contract", () => {
  const setup = fixture();
  execFileSync(script, ["--once"], { env: setup.env });
  reopenCompletedRollbackContract(setup);
  unlinkSync(join(setup.state, "last-activated-sha"));
  writeFileSync(join(setup.state, "approvals", releaseSha), `${releaseSha}\n`, { mode: 0o600 });
  const nextSha = "d".repeat(40);
  setup.env.FAKE_BRANCH_SHA = nextSha;
  setup.env.FAKE_RUN_RELEASE_SHA = nextSha;

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /approved pending release .* must finish or be withdrawn/i);
  assert.equal(
    readdirSync(join(setup.state, "rollback-contracts", "pending")).length,
    1,
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

test("one-shot rejects a symlinked exact-SHA approval", () => {
  const setup = fixture({ approved: false });
  const target = join(setup.root, "approval-target");
  writeFileSync(target, `${releaseSha}\n`, { mode: 0o600 });
  symlinkSync(target, join(setup.state, "approvals", releaseSha));

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /approval must be a regular non-symlink file/i);
});

test("prepared backup reuse rejects a symlinked marker", () => {
  const setup = fixture({ approved: false });
  execFileSync(script, ["--once"], { env: setup.env });
  const marker = join(setup.state, "prepared", releaseSha);
  const target = join(setup.root, "prepared-marker-target");
  writeFileSync(target, readFileSync(marker, "utf8"), { mode: 0o600 });
  unlinkSync(marker);
  symlinkSync(target, marker);

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /prepared backup marker must be a regular non-symlink file/i);
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
  assert.equal((calls.match(/docker load/g) ?? []).length, 14);
  assert.match(calls, /docker exec henukit-portal-summary-1 .*verify-summary/);
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
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 1);
  assert.match(calls, /docker compose --env-file .*henukit\.rollback\.env .* up -d --remove-orphans/);
  assert.doesNotMatch(calls, /--runtime-only/);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
});

test("a reconciled candidate can receive a fresh exact-SHA approval and complete a new attempt", () => {
  const setup = fixture({
    failTargetHealth: true,
    materialsPathInitiallyEnabled: true,
  });
  const first = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(first.status, 0);

  writeFileSync(setup.materialsPathState, "disabled\n");
  writeFileSync(join(setup.state, "approvals", releaseSha), `${releaseSha}\n`, { mode: 0o600 });
  setup.env.FAKE_FAIL_TARGET_HEALTH = "0";
  execFileSync(script, ["--once"], { env: setup.env });

  const completed = readdirSync(join(setup.state, "rollback-contracts", "completed"))
    .filter((name) => name.startsWith(`${releaseSha}.`));
  assert.equal(completed.length, 2);
  assert.equal(existsSync(join(setup.state, "rollback-contracts", "pending", releaseSha)), false);
  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "disabled");
});

test("a pre-switch migration failure keeps the verified previous runtime without replaying its migrations", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failCandidateDeployBeforeSwitch: true,
    materialsPathInitiallyEnabled: true,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /remained active; rollback needs no runtime replacement/);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 1);
  assert.doesNotMatch(calls, /docker compose --env-file .*henukit\.rollback\.env/);
  assert.doesNotMatch(calls, /--runtime-only/);
});

test("a pre-switch failure repairs an unhealthy materials receiver before declaring rollback", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    breakMaterialsWebhookBeforeFailure: true,
    failCandidateDeployBeforeSwitch: true,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assert.match(result.stdout, /rollback needs no runtime replacement/);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.doesNotMatch(calls, /docker compose --env-file .*henukit\.rollback\.env/);
  assert.match(calls, /systemctl restart henukit-materials-webhook\.service/);
});

test("a partial candidate switch cannot masquerade as the exact previous runtime", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ partialCandidateSwitch: true, previousSha });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 1);
  assert.match(calls, /docker compose --env-file .*henukit\.rollback\.env .* up -d --remove-orphans/);
  assert.doesNotMatch(calls, /--runtime-only/);
});

test("activation refuses a candidate whose materials payload cannot be rolled back atomically", () => {
  const setup = fixture({ candidateMaterialsDiffer: true });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /do not satisfy the exact rollback contract/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
  assert.doesNotMatch(calls, /docker load|^deploy /m);
});

test("activation refuses a rollback baseline with a missing required materials unit", () => {
  const setup = fixture({ missingMaterialsRunnerUnit: true });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /do not satisfy the exact rollback contract/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
  assert.doesNotMatch(calls, /docker load|^deploy /m);
});

test("activation refuses a failed materials runner state", () => {
  const setup = fixture({ failedMaterialsRunnerUnit: true });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /do not satisfy the exact rollback contract/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
});

test("activation refuses a masked materials path instead of treating it as disabled", () => {
  const setup = fixture({ maskedMaterialsPath: true });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /do not satisfy the exact rollback contract/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
});

test("rollback refuses a rollback environment that changed after the contract was bound", () => {
  const setup = fixture({
    failTargetHealth: true,
    mutateRollbackEnvOnFailure: true,
  });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rollback .* also failed/i);
  assert.doesNotMatch(calls, /docker compose --env-file .*henukit\.rollback\.env .* up -d --remove-orphans/);
});

test("rollback fails closed when the optional deploy webhook dies after restart", () => {
  const setup = fixture({
    deployWebhookPresent: true,
    failDeployWebhookAfterRestart: true,
    failTargetHealth: true,
  });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rollback .* also failed/i);
  assert.match(calls, /systemctl restart henukit-deploy-webhook\.service/);
  assert.match(calls, /systemctl show -p ActiveState --value henukit-deploy-webhook\.service/);
});

test("failed Practice catalog-session-answer verification rolls back the candidate", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ failTargetPracticeFlow: true, previousSha });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assert.match(calls, /\/api\/v1\/practice\/catalog/);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
});

test("default activation still refuses a degraded rollback baseline", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ failPreviousHealth: true, previousSha });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /no healthy fixed-SHA rollback release is ready/i);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
});

test("explicit recovery activates from the exact degraded baseline and records immutable evidence", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    materialsPathInitiallyEnabled: true,
    previousSha,
  });
  const artifacts = writeLocalArtifacts(setup.root);

  const output = execFileSync(
    script,
    [
      "--local-artifacts", artifacts,
      "--sha", releaseSha,
      "--recover-degraded-baseline", previousSha,
    ],
    { encoding: "utf8", env: setup.env },
  );

  assert.match(output, /authorized degraded-baseline recovery/i);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), releaseSha);
  const audit = readFileSync(
    join(setup.state, "degraded-recoveries", `${releaseSha}.activated`),
    "utf8",
  );
  assert.match(audit, new RegExp(`candidate_sha=${releaseSha}`));
  assert.match(audit, new RegExp(`previous_sha=${previousSha}`));
  assert.match(audit, /status=activated/);
  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");
});

test("degraded recovery resumes after authorization but before approval consumption", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    materialsPathInitiallyEnabled: true,
    previousSha,
  });
  const artifacts = writeLocalArtifacts(setup.root);
  const args = [
    "--local-artifacts", artifacts,
    "--sha", releaseSha,
    "--recover-degraded-baseline", previousSha,
  ];
  execFileSync(script, args, { env: setup.env });
  reopenCompletedRollbackContract(setup);
  unlinkSync(join(setup.state, "last-activated-sha"));
  unlinkSync(join(setup.state, "degraded-recoveries", `${releaseSha}.activated`));
  writeFileSync(setup.active, `${previousSha}\n`);
  writeFileSync(join(setup.state, "approvals", releaseSha), `${releaseSha}\n`, { mode: 0o600 });

  execFileSync(script, args, { env: setup.env });

  assert.equal(readFileSync(setup.active, "utf8").trim(), releaseSha);
  assert.equal(existsSync(join(setup.state, "rollback-contracts", "pending", releaseSha)), false);
  assert.match(
    readFileSync(join(setup.state, "degraded-recoveries", `${releaseSha}.activated`), "utf8"),
    /status=activated/,
  );
});

test("explicit recovery retry repairs a missing activated terminal audit", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ failPreviousHealth: true, previousSha });
  const artifacts = writeLocalArtifacts(setup.root);
  const args = [
    "--local-artifacts", artifacts,
    "--sha", releaseSha,
    "--recover-degraded-baseline", previousSha,
  ];
  execFileSync(script, args, { env: setup.env });
  const terminal = join(setup.state, "degraded-recoveries", `${releaseSha}.activated`);
  unlinkSync(terminal);
  unlinkSync(join(setup.state, "last-activated-sha"));

  execFileSync(script, args, { env: setup.env });

  assert.match(readFileSync(terminal, "utf8"), /status=activated/);
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
});

test("active recovery resume rolls back before publishing terminal audit when permission grant fails", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    materialsPathInitiallyEnabled: true,
    previousSha,
  });
  const artifacts = writeLocalArtifacts(setup.root);
  const args = [
    "--local-artifacts", artifacts,
    "--sha", releaseSha,
    "--recover-degraded-baseline", previousSha,
  ];
  execFileSync(script, args, { env: setup.env });
  reopenCompletedRollbackContract(setup);
  const terminal = join(setup.state, "degraded-recoveries", `${releaseSha}.activated`);
  unlinkSync(terminal);
  unlinkSync(join(setup.state, "last-activated-sha"));
  writeFileSync(setup.materialsPathState, "disabled\n");
  setup.env.FAKE_FAIL_ACCOUNT_GRANT = "1";

  const failed = spawnSync(script, args, { encoding: "utf8", env: setup.env });
  assert.notEqual(failed.status, 0);
  assert.match(failed.stderr, /interrupted degraded recovery .* reconciled/i);
  assert.equal(existsSync(terminal), false);
  assert.equal(existsSync(join(setup.state, "last-activated-sha")), false);
  assert.equal(readFileSync(setup.active, "utf8").trim(), previousSha);
  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");

  assert.equal(existsSync(join(setup.state, "rollback-contracts", "pending", releaseSha)), false);
});

test("explicit recovery refuses a healthy baseline instead of weakening the normal path", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ previousSha });
  const artifacts = writeLocalArtifacts(setup.root);

  const result = spawnSync(
    script,
    [
      "--local-artifacts", artifacts,
      "--sha", releaseSha,
      "--recover-degraded-baseline", previousSha,
    ],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /baseline is healthy/i);
  assert.equal(existsSync(join(setup.state, "degraded-recoveries")), true);
  assert.equal(
    existsSync(join(setup.state, "degraded-recoveries", `${releaseSha}.authorized`)),
    false,
  );
});

test("explicit recovery refuses a baseline that does not own the current release link", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ failPreviousHealth: true, previousSha });
  const artifacts = writeLocalArtifacts(setup.root);
  unlinkSync(join(setup.root, "current"));
  symlinkSync(join(setup.root, "releases", "d".repeat(40)), join(setup.root, "current"), "dir");

  const result = spawnSync(
    script,
    [
      "--local-artifacts", artifacts,
      "--sha", releaseSha,
      "--recover-degraded-baseline", previousSha,
    ],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /does not match the current release link and exact image set/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /docker load|^deploy /m);
});

for (const missing of ["marker", "helper", "compose"]) {
  test(`explicit recovery refuses an incomplete baseline missing ${missing}`, () => {
    const previousSha = "c".repeat(40);
    const setup = fixture({
      failPreviousHealth: true,
      incompletePreviousRelease: missing,
      previousSha,
    });
    const artifacts = writeLocalArtifacts(setup.root);

    const result = spawnSync(
      script,
      [
        "--local-artifacts", artifacts,
        "--sha", releaseSha,
        "--recover-degraded-baseline", previousSha,
      ],
      { encoding: "utf8", env: setup.env },
    );

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /degraded baseline/i);
    assert.doesNotMatch(readFileSync(setup.log, "utf8"), /docker load|^deploy /m);
    assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
  });
}

test("explicit recovery refuses an untrusted current symlink before loading images", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    nonRootTrustRoot: "current-link",
    previousSha,
  });
  const artifacts = writeLocalArtifacts(setup.root);
  const args = [
    "--local-artifacts", artifacts,
    "--sha", releaseSha,
    "--recover-degraded-baseline", previousSha,
  ];

  const result = spawnSync(
    script,
    args,
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /current link must be owned by root/i);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /docker load|^deploy /m);
});

test("failed recovery restores the exact known-degraded baseline without claiming health", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    failTargetHealth: true,
    materialsPathInitiallyEnabled: true,
    previousSha,
  });
  const artifacts = writeLocalArtifacts(setup.root);
  const args = [
    "--local-artifacts", artifacts,
    "--sha", releaseSha,
    "--recover-degraded-baseline", previousSha,
  ];

  const result = spawnSync(
    script,
    args,
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /restored known degraded baseline/i);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  const audit = readFileSync(
    join(setup.state, "degraded-recoveries", `${releaseSha}.restored`),
    "utf8",
  );
  assert.match(audit, /status=restored_known_degraded_baseline/);
  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");

  writeFileSync(join(setup.state, "approvals", releaseSha), `${releaseSha}\n`, { mode: 0o600 });
  const sameShaRetry = spawnSync(script, args, { encoding: "utf8", env: setup.env });
  assert.notEqual(sameShaRetry.status, 0);
  assert.match(sameShaRetry.stderr, /retry requires a new candidate SHA/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
});

test("explicit recovery retry repairs a missing restored terminal audit without loading again", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    failTargetHealth: true,
    previousSha,
  });
  const artifacts = writeLocalArtifacts(setup.root);
  const args = [
    "--local-artifacts", artifacts,
    "--sha", releaseSha,
    "--recover-degraded-baseline", previousSha,
  ];
  spawnSync(script, args, { encoding: "utf8", env: setup.env });
  const terminal = join(setup.state, "degraded-recoveries", `${releaseSha}.restored`);
  unlinkSync(terminal);
  const before = readFileSync(setup.log, "utf8");

  const result = spawnSync(script, args, { encoding: "utf8", env: setup.env });
  const after = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /prior degraded recovery attempt converged/i);
  assert.match(readFileSync(terminal, "utf8"), /status=restored_known_degraded_baseline/);
  assert.equal((after.match(/docker load/g) ?? []).length, (before.match(/docker load/g) ?? []).length);
  assert.equal((after.match(/^deploy /gm) ?? []).length, (before.match(/^deploy /gm) ?? []).length);
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
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 1);
  assert.match(calls, /docker compose --env-file .*henukit\.rollback\.env .* up -d --remove-orphans/);
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
  assert.equal((calls.match(/docker load/g) ?? []).length, 14);
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
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 1);
  assert.match(calls, /docker compose --env-file .*henukit\.rollback\.env .* up -d --remove-orphans/);
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
  assert.equal((calls.match(/^deploy /gm) ?? []).length, 1);
  assert.match(calls, /docker compose --env-file .*henukit\.rollback\.env .* up -d --remove-orphans/);
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

test("one-shot refuses a plaintext non-loopback Career extraction LLM", () => {
  const setup = fixture({ careerAIBaseURL: "http://10.0.0.8:30000/v1" });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /CAREER_AI_BASE_URL must use HTTPS, loopback, or the exact approved HTTP endpoint/i);
  assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
});

test("one-shot accepts only the explicitly approved plaintext Career extraction LLM", () => {
  const approved = fixture({
    careerAIBaseURL: "http://125.46.96.207:30000/v1",
    careerAllowInsecureAIHTTP: true,
  });
  const approvedResult = spawnSync(script, ["--once"], { encoding: "utf8", env: approved.env });
  assert.equal(approvedResult.status, 0, approvedResult.stderr);

  const missingFlag = fixture({ careerAIBaseURL: "http://125.46.96.207:30000/v1" });
  const missingFlagResult = spawnSync(script, ["--once"], { encoding: "utf8", env: missingFlag.env });
  assert.notEqual(missingFlagResult.status, 0);
  assert.match(missingFlagResult.stderr, /CAREER_ALLOW_INSECURE_AI_HTTP=1/i);

  const wrongHost = fixture({
    careerAIBaseURL: "http://125.46.96.208:30000/v1",
    careerAllowInsecureAIHTTP: true,
  });
  const wrongHostResult = spawnSync(script, ["--once"], { encoding: "utf8", env: wrongHost.env });
  assert.notEqual(wrongHostResult.status, 0);
  assert.doesNotMatch(readFileSync(wrongHost.log, "utf8"), /pg_dump|docker load|^deploy /m);
});

test("one-shot accepts the Suification plaintext exception only for the exact approved Career LLM", () => {
  const approved = fixture({
    careerAIBaseURL: "http://125.46.96.207:30000/v1",
    careerAllowInsecureAIHTTP: true,
    careerSuifyAllowInsecureAIHTTP: true,
  });
  const approvedResult = spawnSync(script, ["--once"], { encoding: "utf8", env: approved.env });
  assert.equal(approvedResult.status, 0, approvedResult.stderr);

  const httpsWithException = fixture({ careerSuifyAllowInsecureAIHTTP: true });
  const httpsResult = spawnSync(script, ["--once"], { encoding: "utf8", env: httpsWithException.env });
  assert.notEqual(httpsResult.status, 0);
  assert.match(httpsResult.stderr, /CAREER_SUIFY_ALLOW_INSECURE_AI_HTTP=1 is valid only/i);
  assert.doesNotMatch(readFileSync(httpsWithException.log, "utf8"), /pg_dump|docker load|^deploy /m);
});

test("one-shot refuses Career LLM and digest placeholder credentials", () => {
  for (const options of [
    { careerAIKey: "replace-career-ai-key-32bytes-minimum" },
    { careerDigestSecret: "replace-career-digest-secret-32bytes-minimum" },
    { careerDigestSecret: "local-career-digest-secret-32bytes-only!" },
  ]) {
    const setup = fixture(options);
    const result = spawnSync(script, ["--once"], {
      encoding: "utf8",
      env: setup.env,
    });
    const calls = readFileSync(setup.log, "utf8");

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /placeholder/i);
    assert.doesNotMatch(calls, /pg_dump|docker load|^deploy /m);
  }
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
