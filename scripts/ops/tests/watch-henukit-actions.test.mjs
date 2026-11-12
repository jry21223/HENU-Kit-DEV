import assert from "node:assert/strict";
import { execFileSync, spawn, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  chmodSync,
  chownSync,
  existsSync,
  lstatSync,
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
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";
import { forwarderContract } from "./active-alias-fixture.mjs";

const script = fileURLToPath(
  new URL("../watch-henukit-actions.sh", import.meta.url),
);
const releaseSha = "a".repeat(40);
const localManifestDigest = createHash("sha256")
  .update("verified-by-fixture\n")
  .digest("hex");
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
  "henukit-getwork-mcp",
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
    "services:\n  portal-summary:\n  account-portfolio:\n  notice:\n  notice-worker:\n  food:\n  library:\n  getwork-mcp:\n  quizcraft:\n",
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
  materialsRuntimeBackup = {},
  materialsRuntimeInstalled = {},
  materialsRuntimeTargets = [],
  removeMaterialsRuntimeRecord = false,
  danglingMaterialsRuntimeRecord = false,
  unreadableMaterialsRecordDirectory = false,
  materialsRuntimeDestinationIsDirectory = "",
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
  includeGetworkProvenanceArtifact = false,
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
  getWorkToken = "fixture-getwork-mcp-random-credential-48bytes",
  careerGetWorkEnabled = "1",
  runConclusion = "success",
  runStatus = "completed",
  runAttempt = 1,
  legacyRuntimePresent = false,
  activationFreeKiB = 8 * 1024 * 1024,
  recoveryIdentity = `local:${localManifestDigest}`,
} = {}) {
  // macOS exposes its temporary directory through /var, which is a symlink.
  // The production trust-root check deliberately rejects symlinked parents, so
  // create the fixture below tmpdir's canonical path instead.
  const root = mkdtempSync(join(realpathSync(tmpdir()), "henukit-actions-watch-"));
  const writeTree = (base, files) => {
    for (const [relative, contents] of Object.entries(files)) {
      const target = join(base, relative);
      mkdirSync(dirname(target), { recursive: true });
      writeFileSync(target, contents);
    }
  };
  const bin = join(root, "bin");
  const staging = join(root, "staging");
  const releases = join(root, "releases");
  const backups = join(root, "backups");
  const state = join(root, "state");
  const materialsRoot = join(root, "materials");
  const materialsRuntimePrefix = join(root, "runtime-prefix");
  const log = join(root, "calls.log");
  const active = join(root, "active-sha");
  const currentLink = join(root, "current");
  const compatibilityLink = join(root, "henukit", "current");
  const sourceLock = join(state, "source-deploy.lock");
  const sourceConfig = join(state, "source-deploy.env");
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
  mkdirSync(join(state, "prepared"));
  mkdirSync(dirname(compatibilityLink), { mode: 0o755 });
  writeFileSync(sourceLock, "", { mode: 0o640 });
  writeFileSync(sourceConfig, `HENUKIT_DEPLOY_LOCK_FILE=${sourceLock}\n`, { mode: 0o600 });
  if (approved) {
    writeFileSync(join(state, "approvals", releaseSha), `${releaseSha}\n`, {
      mode: 0o600,
    });
    writeFileSync(
      join(state, "prepared", `${releaseSha}.artifact-source`),
      `${recoveryIdentity}\n`,
      { mode: 0o600 },
    );
    writeFileSync(
      join(state, "prepared", `${releaseSha}.recovery-baseline`),
      `${previousSha}\n`,
      { mode: 0o600 },
    );
  }
  writeFileSync(token, "test-read-only-token\n", { mode: 0o600 });
  chmodSync(token, 0o600);
  writeFileSync(
    envFile,
    `POSTGRES_USER=test\nPORTAL_API_MODE=${portalApiMode}\nNEXT_PUBLIC_PORTAL_ALLOW_MOCK=${portalAllowMock ? "1" : "0"}\nACCOUNT_PORTFOLIO_EASYPAY_ENABLED=${easypayEnabled ? "1" : "0"}\nACCOUNT_PORTFOLIO_EASYPAY_BASE_URL=https://metaview.top/epay\nACCOUNT_PORTFOLIO_EASYPAY_PID=henukit-production\nACCOUNT_PORTFOLIO_EASYPAY_KEY=henukit-production-secret-32bytes\nACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL=https://henukit.cn/api/v1/payment-providers/easypay/notifications\nACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL=https://henukit.cn/account/membership\nGETWORK_MCP_ACCESS_TOKEN=${getWorkToken}\nCAREER_GETWORK_ENABLED=${careerGetWorkEnabled}\nCAREER_AI_BASE_URL=${careerAIBaseURL}\nCAREER_AI_API_KEY=${careerAIKey}\nCAREER_AI_MODEL=qwen3.6-plus\nCAREER_ALLOW_INSECURE_AI_HTTP=${careerAllowInsecureAIHTTP ? "1" : "0"}\nCAREER_SUIFY_ALLOW_INSECURE_AI_HTTP=${careerSuifyAllowInsecureAIHTTP ? "1" : "0"}\nPLATFORM_CORE_CAREER_DIGEST_CLIENT_ID=career-opportunities\nPLATFORM_CORE_CAREER_DIGEST_KEY_ID=career-digest-key-1\nPLATFORM_CORE_CAREER_DIGEST_SECRET=${careerDigestSecret}\n`,
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
  henukit-getwork-mcp
  henukit-portal-gateway
)
case "\${1:-}" in
  --check) ;;
  --artifact-images|--load-images) printf '%s\\n' "\${images[@]}" ;;
  --baseline-images)
    printf '%s\\n' henukit-console henukit-console-gateway henukit-platform-core henukit-platform-mail-worker henukit-platform-smtp-provider henukit-portal henukit-portal-summary henukit-portal-api henukit-portal-gateway
    ;;
  --conditional-services)
    printf '%s\\n' $'account-portfolio\\thenukit-account-portfolio' $'notice\\thenukit-notice' $'notice-worker\\thenukit-notice-worker' $'food\\thenukit-food' $'library\\thenukit-library' $'getwork-mcp\\thenukit-getwork-mcp'
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
  elif [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "materials-record-directory" && -d "$path" &&
          "$path" == "$FAKE_MATERIALS_ROOT/runtime-backups/"*/* ]]; then
    id -u
  elif [[ -d "$path" ]]; then
    printf '0'
  elif [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "inventory" && "$path" == "$FAKE_TRUSTED_INVENTORY" ]] ||
       [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "verifier" && "$path" == "$FAKE_TRUSTED_VERIFIER" ]] ||
       [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "signers" && "$path" == "$FAKE_TRUSTED_SIGNERS" ]]; then
    id -u
  elif [[ "$FAKE_NON_ROOT_TRUST_ROOT" == "materials-backup" && "$path" == "$FAKE_MATERIALS_ROOT/runtime-backups/"* ]]; then
    id -u
  elif [[ "$path" == "$FAKE_MATERIALS_ROOT"/* ]]; then
    printf '0'
  elif [[ "$path" == "$FAKE_QUIZCRAFT_OWNER" || "$path" == "$FAKE_TRUSTED_INVENTORY" || "$path" == "$FAKE_TRUSTED_VERIFIER" || "$path" == "$FAKE_TRUSTED_SIGNERS" || "$path" == "$FAKE_CURRENT_LINK" || "$path" == "$FAKE_ROLLBACK_ENV_FILE" || "$path" == "$FAKE_RELEASE_ROOT"/* || "$path" == "$FAKE_BACKUP_ROOT"/* || "$path" == "$FAKE_STATE_ROOT/approvals/"* || "$path" == "$FAKE_STATE_ROOT/prepared/"* || "$path" == "$FAKE_STATE_ROOT/quizcraft-releases/"* || "$path" == "$FAKE_STATE_ROOT/last-activated-sha" || "$path" == "$FAKE_STATE_ROOT/practice-smoke-"* ]]; then
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
    symlinkSync(previousRelease, compatibilityLink, "dir");
  }

  forwarderContract(state, releases, compatibilityLink, currentLink);
  writeExecutable(
    join(bin, "gh"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "$FAKE_GH_MUST_NOT_RUN" == "1" ]]; then exit 99; fi
if [[ "$1 $2" == "run list" ]]; then
  if [[ "$FAKE_NO_SUCCESS" == "1" ]]; then
    exit 0
  fi
  printf '123\\t%s\\t%s\\t%s\\t%s\\thttps://github.example/actions/runs/123\\n' \
    "$FAKE_RUN_ATTEMPT" "$FAKE_RUN_RELEASE_SHA" "$FAKE_RUN_STATUS" "$FAKE_RUN_CONCLUSION"
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
  henukit-getwork-mcp
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
if [[ "$FAKE_INCLUDE_GETWORK_PROVENANCE_ARTIFACT" == "1" ]]; then
  provenance_artifact="$dest/henukit-getwork-actions-provenance-$FAKE_RELEASE_SHA"
  mkdir -p "$provenance_artifact"
  printf 'getwork-manifest\n' > "$provenance_artifact/henukit-getwork-actions-$FAKE_RELEASE_SHA.manifest"
  printf '{"bundle":true}\n' > "$provenance_artifact/henukit-getwork-actions-$FAKE_RELEASE_SHA.attestation.json"
fi
runtime_artifact="$dest/henukit-runtime-$FAKE_RELEASE_SHA"
runtime_tree="$(mktemp -d "\${TMPDIR:-/tmp}/henukit-runtime-tree.XXXXXX")"
mkdir -p "$runtime_artifact" "$runtime_tree/bin" "$runtime_tree/release-gates" "$runtime_tree/materials-runtime"
printf '%s\\n' "$FAKE_RELEASE_SHA" > "$runtime_tree/RELEASE_SHA"
printf 'services:\\n  portal-summary:\\n  account-portfolio:\\n  notice:\\n  notice-worker:\\n  food:\\n  library:\\n  getwork-mcp:\\n  quizcraft:\\n' > "$runtime_tree/docker-compose.henukit.release.yml"
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
if [[ "\${FAKE_KILL_CANDIDATE_BEFORE_SWITCH:-0}" == "1" ]]; then
  kill -KILL "$PPID"
  exit 137
fi
if [[ "$FAKE_MUTATE_ROLLBACK_ENV_ON_FAILURE" == "1" ]]; then
  printf 'TAMPERED=1\\n' >> "$FAKE_ROLLBACK_ENV_FILE"
fi
if [[ "$FAKE_BREAK_MATERIALS_WEBHOOK_BEFORE_FAILURE" == "1" ]]; then
  : > "$FAKE_MATERIALS_WEBHOOK_UNHEALTHY"
fi
if [[ -n "$FAKE_MATERIALS_RUNTIME_TARGETS" ]]; then
  backup_root="$FAKE_MATERIALS_ROOT/runtime-backups/$(cat "$1/RELEASE_SHA")"
  mkdir -p "$backup_root"
  while IFS= read -r target; do
    [[ -n "$target" ]] || continue
    live="$FAKE_MATERIALS_RUNTIME_PREFIX/$target"
    [[ -e "$live" ]] || continue
    mkdir -p "$backup_root/$(dirname "$target")"
    [[ -e "$backup_root/$target" ]] || cp -a "$live" "$backup_root/$target"
    printf 'candidate-runtime\\n' > "$live"
  done <<< "$FAKE_MATERIALS_RUNTIME_TARGETS"
fi
record_root="$FAKE_MATERIALS_ROOT/runtime-backups/$(cat "$1/RELEASE_SHA")"
if [[ "$FAKE_REMOVE_MATERIALS_RUNTIME_RECORD" == "1" ]]; then
  rm -rf "$record_root"
fi
if [[ "$FAKE_DANGLING_MATERIALS_RUNTIME_RECORD" == "1" ]]; then
  mv "$record_root" "$record_root.real"
  ln -s "$record_root.real" "$record_root"
fi
if [[ "$FAKE_UNREADABLE_MATERIALS_RECORD_DIRECTORY" == "1" ]]; then
  chmod 000 "$record_root/usr/local/libexec/henukit"
fi
if [[ -n "$FAKE_MATERIALS_DESTINATION_IS_DIRECTORY" ]]; then
  rm -f "$FAKE_MATERIALS_RUNTIME_PREFIX/$FAKE_MATERIALS_DESTINATION_IS_DIRECTORY"
  mkdir -p "$FAKE_MATERIALS_RUNTIME_PREFIX/$FAKE_MATERIALS_DESTINATION_IS_DIRECTORY"
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
    join(bin, "df"),
    `#!/usr/bin/env bash
set -Eeuo pipefail
printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
printf '/dev/fake 16777216 8388608 %s 50%% /\n' "$FAKE_ACTIVATION_FREE_KIB"
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
      printf 'henukit-notice:%s\\nhenukit-notice-worker:%s\\nhenukit-food:%s\\nhenukit-library:%s\\nhenukit-getwork-mcp:%s\\n' "$sha" "$sha" "$sha" "$sha" "$sha"
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

  // The watcher fixture models the companion as an external OS boundary.
  // Its real receipt/provenance and failure behavior run in the Python suite.
  const coreReceipt = join(state,"quizcraft-releases",releaseSha,"identity.json");
  mkdirSync(dirname(coreReceipt),{recursive:true});
  writeFileSync(coreReceipt,JSON.stringify({archive_path:join(staging,releaseSha,`henukit-runtime-${releaseSha}.tar.gz`)}),{mode:0o400});
  const quizcraftOwner = join(bin, "quizcraft-release-owner.py");
  writeFileSync(quizcraftOwner, `import os,sys,json,pathlib
with open(os.environ['FAKE_CALL_LOG'],'a') as stream:stream.write('quizcraft-gate '+sys.argv[1]+'\\n')
if os.environ.get('FAKE_FAIL_QUIZCRAFT_GATE')==sys.argv[1]:raise SystemExit(1)
if sys.argv[1]=='begin':
 path=pathlib.Path(os.environ['FAKE_STATE_ROOT'])/'quizcraft-releases'/os.environ['FAKE_RELEASE_SHA']/'identity.json'
 path.chmod(0o600);path.write_text(json.dumps({'archive_path':sys.argv[sys.argv.index('--archive')+1]}));path.chmod(0o400)
print('{}')
`, { mode: 0o555 });

  writeTree(join(materialsRoot, "runtime-backups", releaseSha), materialsRuntimeBackup);
  writeTree(materialsRuntimePrefix, materialsRuntimeInstalled);

  return {
    active,
    libraryHealthAttempts,
    root,
    log,
    materialsPathState,
    mixedRuntime,
    state,
    materialsRoot,
    materialsRuntimePrefix,
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      HENUKIT_QUIZCRAFT_RELEASE_OWNER: quizcraftOwner,
      FAKE_QUIZCRAFT_OWNER: quizcraftOwner,
      FAKE_ACTIVE_FILE: active,
      FAKE_ACTIVATION_FREE_KIB: String(activationFreeKiB),
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
      FAKE_INCLUDE_GETWORK_PROVENANCE_ARTIFACT: includeGetworkProvenanceArtifact ? "1" : "0",
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
      FAKE_RUN_ATTEMPT: String(runAttempt),
      FAKE_RUN_RELEASE_SHA: releaseSha,
      FAKE_RUN_STATUS: runStatus,
      FAKE_STATE_ROOT: state,
      FAKE_MATERIALS_ROOT: materialsRoot,
      FAKE_MATERIALS_RUNTIME_TARGETS: materialsRuntimeTargets.join("\n"),
      FAKE_MATERIALS_RUNTIME_PREFIX: materialsRuntimePrefix,
      FAKE_REMOVE_MATERIALS_RUNTIME_RECORD: removeMaterialsRuntimeRecord ? "1" : "0",
      FAKE_DANGLING_MATERIALS_RUNTIME_RECORD: danglingMaterialsRuntimeRecord ? "1" : "0",
      FAKE_UNREADABLE_MATERIALS_RECORD_DIRECTORY: unreadableMaterialsRecordDirectory ? "1" : "0",
      FAKE_MATERIALS_DESTINATION_IS_DIRECTORY: materialsRuntimeDestinationIsDirectory,
      HENUKIT_MATERIALS_ROOT: materialsRoot,
      HENUKIT_MATERIALS_RUNTIME_PREFIX: materialsRuntimePrefix,
      GH_TOKEN_FILE: token,
      HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE: accountOperatorRole,
      HENUKIT_ACTIVE_RELEASE_ATTEMPTS: "5",
      HENUKIT_BACKUP_ROOT: backups,
      HENUKIT_ENV_FILE: envFile,
      HENUKIT_CURRENT_LINK: currentLink,
      HENUKIT_COMPAT_CURRENT_LINK: compatibilityLink,
      HENUKIT_SOURCE_DEPLOY_LOCK: sourceLock,
      HENUKIT_SOURCE_DEPLOY_CONFIG: sourceConfig,
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

function aliasReplaceFault(setup, destination, action) {
  const python = execFileSync("python3", ["-I", "-c", "import sys;print(sys.executable)"], { encoding: "utf8" }).trim();
  const wrapper = join(setup.root, "bin", "python3");
  writeExecutable(wrapper, `#!${python}
import os, signal, stat, sys
if sys.argv[1:3] != ['-I', '-']: os.execv(${JSON.stringify(python)}, [${JSON.stringify(python)}, *sys.argv[1:]])
assert sys.argv[1:3] == ['-I', '-']
source = sys.stdin.read()
sys.argv = ['-', *sys.argv[3:]]
replace = os.replace
symlink = os.symlink
fsync = os.fsync
sync_log = ${JSON.stringify(join(setup.root, "alias-parent-fsync.log"))}
alias_parents = ${JSON.stringify([dirname(setup.env.HENUKIT_COMPAT_CURRENT_LINK)])}
def traced_fsync(fd):
    result = fsync(fd)
    held = os.fstat(fd)
    if ${JSON.stringify(action)} == 'assert-parent-sync' and stat.S_ISDIR(held.st_mode):
        for directory in alias_parents:
            expected = os.stat(directory)
            if (held.st_dev, held.st_ino) == (expected.st_dev, expected.st_ino):
                with open(sync_log, 'a') as stream: stream.write(directory + '\\n')
    return result
os.fsync = traced_fsync
def changed_symlink(src, dst, *args, **kwargs):
    result = symlink(src, dst, *args, **kwargs)
    if ${JSON.stringify(action)} == 'replace-source-config' and os.path.basename(os.fspath(dst)).startswith('.henukit-active-'):
        destination = ${JSON.stringify(destination)}
        incoming = destination + '.same-bytes-replacement'
        with open(destination, 'rb') as stream: contents = stream.read()
        fd = os.open(incoming, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        with os.fdopen(fd, 'wb') as stream: stream.write(contents)
        replace(incoming, destination)
    return result
os.symlink = changed_symlink
def interrupted_replace(src, dst, *args, **kwargs):
    selected = os.fspath(dst) == ${JSON.stringify(destination)} and os.path.basename(os.fspath(src)).startswith(('.henukit-active-', '.active-marker-'))
    if selected and ${JSON.stringify(action)} == 'error':
        raise OSError('controlled active-alias filesystem failure')
    if selected and ${JSON.stringify(action)} == 'kill-before':
        os.kill(os.getppid(), signal.SIGKILL); os._exit(137)
    if selected and ${JSON.stringify(action)} == 'assert-parent-sync':
        observed = set(open(sync_log).read().splitlines()) if os.path.exists(sync_log) else set()
        if not set(alias_parents).issubset(observed):
            raise OSError('alias parents were not fsynced after interrupted replacement')
    result = replace(src, dst, *args, **kwargs)
    if selected and ${JSON.stringify(action)} == 'kill-after':
        os.kill(os.getppid(), signal.SIGKILL); os._exit(137)
    return result
os.replace = interrupted_replace
exec(compile(source, '<stdin>', 'exec'), {'__name__': '__main__'})
`);
  return () => unlinkSync(wrapper);
}

function contractDurabilityFault(setup) {
  const python = execFileSync("python3", ["-I", "-c", "import sys;print(sys.executable)"], { encoding: "utf8" }).trim();
  const wrapper = join(setup.root, "bin", "python3");
  const trace = join(setup.root, "contract-fsync.trace");
  const contract = join(setup.state, "rollback-contracts", "pending", releaseSha);
  writeExecutable(wrapper, `#!${python}
import os, stat, sys
if sys.argv[1:3] != ['-I', '-']: os.execv(${JSON.stringify(python)}, [${JSON.stringify(python)}, *sys.argv[1:]])
assert sys.argv[1:3] == ['-I', '-']
source = sys.stdin.read()
sys.argv = ['-', *sys.argv[3:]]
fsync, link = os.fsync, os.link
contract, trace = ${JSON.stringify(contract)}, ${JSON.stringify(trace)}
def traced_fsync(fd):
    result = fsync(fd)
    held = os.fstat(fd)
    for label, path in [('file', contract), ('parent', os.path.dirname(contract))]:
        if os.path.exists(path):
            current = os.stat(path)
            if (held.st_dev, held.st_ino) == (current.st_dev, current.st_ino):
                with open(trace, 'a') as stream: stream.write(label + '\\n')
    return result
def checked_link(src, dst, *args, **kwargs):
    if os.path.basename(os.fspath(dst)) == 'intent.json':
        observed = open(trace).read().splitlines() if os.path.exists(trace) else []
        if 'file' not in observed or 'parent' not in observed or observed.index('file') > observed.index('parent'):
            raise OSError('rollback contract file and parent were not durable before alias intent')
    return link(src, dst, *args, **kwargs)
os.fsync, os.link = traced_fsync, checked_link
exec(compile(source, '<stdin>', 'exec'), {'__name__': '__main__'})
`);
  return () => unlinkSync(wrapper);
}

async function holdLease(path) {
  const holder = spawn("python3", ["-I", "-c", "import fcntl,os,sys; fd=os.open(sys.argv[1],os.O_RDONLY); fcntl.flock(fd,fcntl.LOCK_EX); print('locked',flush=True); sys.stdin.read()", path], { stdio: ["pipe", "pipe", "pipe"] });
  const exited = new Promise(resolve => holder.once("close", resolve));
  await new Promise((resolve, reject) => {
    holder.stdout.once("data", resolve);
    holder.once("error", reject);
    holder.once("exit", code => { if (code !== null) reject(new Error(`lease holder exited: ${code}`)); });
  });
  return async () => { holder.stdin.end(); await exited; };
}

test("the watcher refuses an Actions attempt different from automatic authorization", () => {
  const setup = fixture({ runAttempt: 2, approved: false });
  const result = spawnSync(script, ["--once"], {
    encoding: "utf8", env: { ...setup.env, HENUKIT_EXPECTED_ACTIONS_IDENTITY: "actions:123:1" },
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /identity differs from the automatic authorization/);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
});

test("Notice reconciliation inherits all original locks without any full-release or business POST", () => {
  const setup = fixture({ previousSha: releaseSha, approved: false });
  writeFileSync(join(setup.state, "last-activated-sha"), releaseSha + "\n", { mode: 0o600 });
  const lock = join(setup.state, "direct-deploy.lock");
  writeFileSync(lock, "", { mode: 0o600 });
  const helper = join(setup.env.HENUKIT_RELEASE_ROOT, releaseSha, "bin", "notice-url-fixture.py");
  mkdirSync(dirname(helper), { recursive: true });
  writeFileSync(helper, `import fcntl,os,sys
for fd in (8,9,6,7):
 os.fstat(fd);fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
assert sys.argv[sys.argv.index('--mode')+1]=='apply'
print('notice-fixture-four-locks-validated')
`, { mode: 0o555 });
  const plan = join(setup.root, "notice-plan.json"); writeFileSync(plan, "{}\n", { mode: 0o600 });
  const wrapper = `import fcntl,os,sys
fd=os.open(sys.argv[1],os.O_RDONLY);os.dup2(fd,8,inheritable=True)
if fd!=8:os.close(fd)
fcntl.flock(8,fcntl.LOCK_EX|fcntl.LOCK_NB)
os.execv(sys.argv[2],sys.argv[2:])
`;
  const result = spawnSync("python3", ["-I", "-c", wrapper, lock, script,
    "--notice-url", "apply", "--sha", releaseSha, "--plan", plan], {
    encoding: "utf8", env: { ...setup.env, HENUKIT_NOTICE_URL_HELPER: helper },
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /notice-fixture-four-locks-validated/);
  const calls = readFileSync(setup.log, "utf8");
  assert.doesNotMatch(calls, /docker |curl |deploy |grant-account|gh run|artifact/);
});

test("full release stages Notice before rollback binding and keeps committed activation when its separate receipt needs reconciliation", () => {
  const setup = fixture();
  const helper = join(setup.env.HENUKIT_RELEASE_ROOT, "b".repeat(40), "bin", "notice-url-fixture.py");
  mkdirSync(dirname(helper), { recursive: true });
  writeFileSync(helper, `import os,pathlib,sys
mode=sys.argv[sys.argv.index('--mode')+1];state=pathlib.Path(os.environ['HENUKIT_STATE_ROOT']);sha='a'*40
with open(os.environ['FAKE_CALL_LOG'],'a') as log:log.write('notice-hook '+mode+'\\n')
if mode=='stage':
 assert pathlib.Path(sys.argv[sys.argv.index('--baseline-env')+1]).is_file()
 assert not (state/'rollback-contracts/pending'/sha).exists()
elif mode=='health':
 assert (state/'last-activated-sha').read_text().strip()==sha
 raise SystemExit(13)
else:raise SystemExit(14)
`, { mode: 0o555 });
  const plan = join(setup.root, "notice-plan.json"); writeFileSync(plan, "{}\n", { mode: 0o600 });
  // The formal scheduler already holds direct-deploy.lock in production.
  // The fake helper checks hook ordering; the real helper's lock/FS contract
  // is exercised separately by test_notice_url_transaction.py.
  const result = spawnSync(script, ["--once"], { encoding: "utf8",
    env: { ...setup.env, HENUKIT_NOTICE_URL_HELPER: helper, HENUKIT_NOTICE_URL_PLAN: plan },
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(readFileSync(setup.log, "utf8"), /notice-hook stage[\s\S]*notice-hook health/);
  assert.match(result.stdout, /release active; Notice URL receipt requires explicit reconciliation/);
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
});

test("one-shot downloads, verifies, backs up, and deploys one successful main artifact set", () => {
  const setup = fixture();
  setup.env.HENUKIT_PLATFORM_MIGRATIONS = "000021_future_reviewed.up.sql";

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
  assert.equal((calls.match(/docker load/g) ?? []).length, 15);
  assert.match(calls, /docker exec henukit-portal-summary-1 .*verify-summary/);
  assert.match(calls, /deploy .*releases.*henukit\.env/);
  assert.match(
    calls,
    /deploy .*000019_career_digest_mail\.up\.sql,000020_mail_outbox_allow_bulk_priority\.up\.sql,000021_future_reviewed\.up\.sql/,
  );
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
  assert.equal(realpathSync(setup.env.HENUKIT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, releaseSha));
  assert.equal(realpathSync(setup.env.HENUKIT_COMPAT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, releaseSha));

  execFileSync(script, ["--once"], { env: setup.env });
  const secondCalls = readFileSync(setup.log, "utf8");
  assert.equal((secondCalls.match(/^deploy /gm) ?? []).length, 1);
});

test("activation preserves its approval when disk headroom is below the production floor", () => {
  const setup = fixture({ activationFreeKiB: 3 * 1024 * 1024 });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /at least 4096 MiB.*approval remains unconsumed/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
  assert.match(calls, /pg_dump/);
  assert.doesNotMatch(calls, /docker load|^deploy /m);
});

test("activation configuration cannot lower the production disk floor", () => {
  const setup = fixture();

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: { ...setup.env, HENUKIT_MIN_ACTIVATION_FREE_MIB: "1024" },
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /cannot lower the 4096 MiB production floor/i);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
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

test("one-shot ignores only the getWork provenance pair from the full Actions run", () => {
  const setup = fixture({ includeGetworkProvenanceArtifact: true });

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
  // Three starting reads plus readiness, pre-publication and pre-marker checks.
  assert.equal(readFileSync(join(setup.root, "library-health-attempts"), "utf8"), "6");
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
  assert.equal(readFileSync(setup.libraryHealthAttempts, "utf8"), "6");
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
});

test("an activation marker cannot complete a pending contract over a foreign alias", () => {
  const setup = fixture();
  execFileSync(script, ["--once"], { env: setup.env, stdio: "pipe" });
  reopenCompletedRollbackContract(setup);
  const alias = setup.env.HENUKIT_COMPAT_CURRENT_LINK;
  unlinkSync(alias);
  symlinkSync(join(setup.env.HENUKIT_RELEASE_ROOT, "b".repeat(40)), alias, "dir");
  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(result.status, 0);
  assert.equal(existsSync(join(setup.state, "rollback-contracts/pending", releaseSha)), true);
  assert.equal(realpathSync(alias), join(setup.env.HENUKIT_RELEASE_ROOT, "b".repeat(40)));
  assert.equal((readFileSync(setup.log, "utf8").match(/^deploy /gm) ?? []).length, 1);
});

test("an already-consistent pre-invariant release does not fabricate a new alias attempt", () => {
  const setup = fixture({ previousSha: releaseSha });
  writeFileSync(join(setup.state, "last-activated-sha"), releaseSha + "\n", { mode: 0o600 });
  const before = lstatSync(join(setup.state, "last-activated-sha")).ino;
  execFileSync(script, ["--once"], { env: setup.env, stdio: "pipe" });
  assert.equal(lstatSync(join(setup.state, "last-activated-sha")).ino, before);
  assert.equal(existsSync(join(setup.state, "active-aliases")), false);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
});

test("the versioned fixed outer alias remains unchanged through a normal activation", () => {
  const setup = fixture();
  const outer = setup.env.HENUKIT_CURRENT_LINK;
  const before = lstatSync(outer).ino;
  execFileSync(script, ["--once"], { env: setup.env, stdio: "pipe" });
  assert.equal(lstatSync(outer).ino, before);
  assert.equal(realpathSync(outer), join(setup.env.HENUKIT_RELEASE_ROOT, releaseSha));
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
});

test("a new approval cannot replay an unresolved previously-published attempt", () => {
  const setup = fixture();
  execFileSync(script, ["--once"], { env: setup.env, stdio: "pipe" });
  reopenCompletedRollbackContract(setup);
  unlinkSync(join(setup.state, "last-activated-sha"));
  writeFileSync(setup.active, "b".repeat(40) + "\n");
  writeFileSync(join(setup.state, "approvals", releaseSha), releaseSha + "\n", { mode: 0o600 });
  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(result.status, 0);
  assert.equal((readFileSync(setup.log, "utf8").match(/^deploy /gm) ?? []).length, 1);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
});

test("an inner-alias I/O failure remains pending and resumes without another deployment", () => {
  const setup = fixture();
  const removeFault = aliasReplaceFault(setup, setup.env.HENUKIT_COMPAT_CURRENT_LINK, "error");
  const first = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  removeFault();
  assert.notEqual(first.status, 0);
  assert.equal(realpathSync(setup.env.HENUKIT_COMPAT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, "b".repeat(40)));
  assert.equal(realpathSync(setup.env.HENUKIT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, "b".repeat(40)));
  assert.equal(existsSync(join(setup.state, "last-activated-sha")), false);
  assert.equal(existsSync(join(setup.state, "rollback-contracts/pending", releaseSha)), true);
  execFileSync(script, ["--once"], { env: setup.env, stdio: "pipe" });
  assert.equal(realpathSync(setup.env.HENUKIT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, releaseSha));
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
  assert.equal((readFileSync(setup.log, "utf8").match(/^deploy /gm) ?? []).length, 1);
});

test("a killed alias transaction resumes its persisted attempt without replaying deployment", () => {
  for (const boundary of ["before-first-alias", "after-first-alias", "after-marker"]) {
    const setup = fixture();
    const destination = boundary === "after-marker" ? join(setup.state, "last-activated-sha") : setup.env.HENUKIT_COMPAT_CURRENT_LINK;
    const removeFault = aliasReplaceFault(setup, destination, boundary === "before-first-alias" ? "kill-before" : "kill-after");
    const interrupted = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
    removeFault();
    assert.equal(interrupted.signal, "SIGKILL", boundary);
    assert.equal(existsSync(join(setup.state, "rollback-contracts/pending", releaseSha)), true);
    setup.env.FAKE_GH_MUST_NOT_RUN = "1";
    const verifySync = boundary === "after-first-alias" ? aliasReplaceFault(setup, join(setup.state, "last-activated-sha"), "assert-parent-sync") : () => {};
    try {
      execFileSync(script, ["--once"], { env: setup.env, stdio: "pipe" });
    } finally {
      verifySync();
    }
    assert.equal(realpathSync(setup.env.HENUKIT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, releaseSha));
    assert.equal(realpathSync(setup.env.HENUKIT_COMPAT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, releaseSha));
    assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
    assert.equal(existsSync(join(setup.state, "rollback-contracts/pending", releaseSha)), false);
    assert.equal((readFileSync(setup.log, "utf8").match(/^deploy /gm) ?? []).length, 1);
  }
});

test("a foreign same-target inode after interruption is preserved without declaring activation", () => {
  const setup = fixture();
  const alias = setup.env.HENUKIT_COMPAT_CURRENT_LINK;
  const removeFault = aliasReplaceFault(setup, alias, "kill-after");
  const interrupted = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  removeFault();
  assert.equal(interrupted.signal, "SIGKILL");
  const foreign = join(dirname(alias), ".foreign-active-link");
  symlinkSync(join(setup.env.HENUKIT_RELEASE_ROOT, releaseSha), foreign, "dir");
  renameSync(foreign, alias);
  const foreignInode = lstatSync(alias).ino;
  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(result.status, 0);
  assert.equal(lstatSync(alias).ino, foreignInode);
  assert.equal(existsSync(join(setup.state, "last-activated-sha")), false);
  assert.equal(existsSync(join(setup.state, "rollback-contracts/pending", releaseSha)), true);
  assert.equal((readFileSync(setup.log, "utf8").match(/^deploy /gm) ?? []).length, 1);
});

test("a fresh same-SHA approval cannot replay a consumed attempt before alias preparation", () => {
  const setup = fixture();
  setup.env.FAKE_KILL_CANDIDATE_BEFORE_SWITCH = "1";
  const interrupted = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  assert.equal(interrupted.signal, "SIGKILL");
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), false);
  assert.equal(existsSync(join(setup.state, "rollback-contracts", "pending", releaseSha)), true);
  const attempts = join(setup.state, "active-aliases", releaseSha);
  assert.equal(readdirSync(join(attempts, readdirSync(attempts)[0])).includes("prepared-0.json"), false);
  setup.env.FAKE_KILL_CANDIDATE_BEFORE_SWITCH = "0";
  writeFileSync(join(setup.state, "approvals", releaseSha), `${releaseSha}\n`, { mode: 0o600 });

  const resumed = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(resumed.status, 0);
  assert.match(resumed.stderr, /interrupted release .* reconciled/i);
  assert.equal((readFileSync(setup.log, "utf8").match(/^deploy /gm) ?? []).length, 1);
  assert.equal(readFileSync(setup.active, "utf8").trim(), "b".repeat(40));
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), "b".repeat(40));
});

test("the original rollback contract is durable before the alias intent is published", () => {
  const setup = fixture();
  const removeFault = contractDurabilityFault(setup);
  try {
    execFileSync(script, ["--once"], { env: setup.env, stdio: "pipe" });
  } finally {
    removeFault();
  }
  assert.equal((readFileSync(setup.log, "utf8").match(/^deploy /gm) ?? []).length, 1);
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
});

test("the original group-readable source config permits activation without changing its authority", () => {
  const setup = fixture();
  const path = setup.env.HENUKIT_SOURCE_DEPLOY_CONFIG;
  chmodSync(path, 0o640);
  const before = { info: lstatSync(path), bytes: readFileSync(path) };

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.equal(result.status, 0, result.stderr);
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
  const after = lstatSync(path);
  for (const key of ["dev", "ino", "uid", "gid", "mode"]) assert.equal(after[key], before.info[key], key);
  assert.deepEqual(readFileSync(path), before.bytes);
});

function assertSourceConfigRejected(setup) {
  const path = setup.env.HENUKIT_SOURCE_DEPLOY_CONFIG;
  const before = { info: lstatSync(path), bytes: readFileSync(path) };
  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(result.status, 0, result.stderr);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
  assert.equal(existsSync(join(setup.state, "active-aliases")), false);
  assert.equal(existsSync(join(setup.state, "last-activated-sha")), false);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
  const after = lstatSync(path);
  for (const key of ["dev", "ino", "uid", "gid", "mode"]) assert.equal(after[key], before.info[key], key);
  assert.deepEqual(readFileSync(path), before.bytes);
}

test("writable source configs stop before approval consumption", () => {
  for (const mode of [0o660, 0o602]) {
    const setup = fixture();
    chmodSync(setup.env.HENUKIT_SOURCE_DEPLOY_CONFIG, mode);
    assertSourceConfigRejected(setup);
  }
});

test("a group-readable source config belonging to another group is refused", (t) => {
  const setup = fixture();
  const path = setup.env.HENUKIT_SOURCE_DEPLOY_CONFIG;
  const other = process.getgroups().find((gid) => gid !== process.getgid()) ?? (process.getuid() === 0 ? 1 : null);
  if (other === null) return t.skip("no alternate permitted GID for a real foreign-group filesystem fixture");
  chmodSync(path, 0o640);
  chownSync(path, process.getuid(), other);
  assertSourceConfigRejected(setup);
});

test("a source config belonging to another owner is refused", (t) => {
  const setup = fixture();
  const path = setup.env.HENUKIT_SOURCE_DEPLOY_CONFIG;
  chmodSync(path, 0o640);
  try {
    chownSync(path, process.getuid() === 0 ? 1 : 0, process.getgid());
  } catch (error) {
    if (["EPERM", "EACCES"].includes(error.code)) {
      if (process.env.HENUKIT_REQUIRE_FOREIGN_OWNER === "1") {
        assert.fail(`required foreign-owner filesystem fixture is unavailable: ${error.code}`);
      }
      return t.skip(`actual chown probe cannot construct foreign-owner fixture: ${error.code}; Linux/root gate remains unverified`);
    }
    throw error;
  }
  assertSourceConfigRejected(setup);
});

test("same-byte source-config inode drift is rejected before any alias replacement", () => {
  const setup = fixture();
  chmodSync(setup.env.HENUKIT_SOURCE_DEPLOY_CONFIG, 0o640);
  const removeFault = aliasReplaceFault(setup, setup.env.HENUKIT_SOURCE_DEPLOY_CONFIG, "replace-source-config");
  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  removeFault();
  assert.notEqual(result.status, 0);
  assert.equal(existsSync(join(setup.state, "last-activated-sha")), false);
  for (const alias of [setup.env.HENUKIT_COMPAT_CURRENT_LINK, setup.env.HENUKIT_CURRENT_LINK]) {
    assert.equal(realpathSync(alias), join(setup.env.HENUKIT_RELEASE_ROOT, "b".repeat(40)));
  }
});

test("competing watcher, adoption and source leases stop before approval consumption", { timeout: 30000 }, async () => {
  for (const kind of ["watcher", "adoption", "source"]) {
    const setup = fixture();
    const watcherLock = join(setup.state, "watcher.lock");
    writeFileSync(watcherLock, "", { mode: 0o600 });
    const path = kind === "watcher" ? watcherLock : kind === "adoption" ? setup.state : setup.env.HENUKIT_SOURCE_DEPLOY_LOCK;
    const release = await holdLease(path);
    try {
      const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
      assert.notEqual(result.status, 0, kind);
      assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
      assert.equal(existsSync(join(setup.state, "active-aliases")), false);
      assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
      assert.equal(realpathSync(setup.env.HENUKIT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, "b".repeat(40)));
    } finally {
      await release();
    }
  }
});

test("isolated active-alias overrides require a private deployment root", () => {
  const setup = fixture();
  chmodSync(setup.root, 0o755);
  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(result.status, 0);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
});

test("legacy aliases and a foreign fixed outer link stop before approval consumption", () => {
  for (const kind of ["missing-contract", "direct-legacy", "foreign-outer"]) {
    const setup = fixture();
    const outer = setup.env.HENUKIT_CURRENT_LINK;
    if (kind === "missing-contract") unlinkSync(join(setup.state, "active-alias-contract.json"));
    else {
      const incoming = join(setup.root, ".foreign-outer");
      symlinkSync(kind === "direct-legacy" ? join(setup.env.HENUKIT_RELEASE_ROOT, "b".repeat(40)) : setup.env.HENUKIT_COMPAT_CURRENT_LINK, incoming, "dir");
      renameSync(incoming, outer);
    }
    const before = lstatSync(outer).ino;
    const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
    assert.notEqual(result.status, 0, kind);
    assert.equal(lstatSync(outer).ino, before);
    assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
    assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
  }
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
    assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), previousSha);
  });
}

test("an approved pending candidate blocks a newer main candidate from creating a second contract", () => {
  const setup = fixture();
  setup.env.FAKE_ACTIVATION_FREE_KIB = "1";
  const interrupted = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(interrupted.status, 0);
  assert.match(interrupted.stderr, /approval remains unconsumed/i);
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
  assert.equal((calls.match(/docker load/g) ?? []).length, 15);
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
  // Register before the first await: a fast safe-boundary exit must not be
  // lost while the test is writing the maintenance request.
  let closed = false;
  const completion = new Promise(resolve => child.once("close", (status, signal) => {
    closed = true; resolve({ status, signal });
  }));
  let stdout = "";
  let stderr = "";
  child.stdout.setEncoding("utf8");
  child.stderr.setEncoding("utf8");
  child.stdout.on("data", (chunk) => { stdout += chunk; });
  child.stderr.on("data", (chunk) => { stderr += chunk; });

  let timer;
  const timeout = new Promise(resolve => { timer = setTimeout(() => resolve({ timeout: true }), 30_000); });
  try {
  const deadline = Date.now() + 10_000;
  while (!existsSync(instance) && Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  assert.equal(existsSync(instance), true, `watcher did not publish its instance id: ${stderr}`);
  const instanceId = readFileSync(instance, "utf8").trim();
  assert.match(instanceId, /^[1-9][0-9]*$/);
  const nonce = "b".repeat(32);
  writeFileSync(quiesce, `${releaseSha} ${instanceId} ${nonce}\n`, { mode: 0o600 });
  chmodSync(quiesce, 0o600);

  const outcome = await Promise.race([completion, timeout]);
  assert.notEqual(outcome.timeout, true, `owned fixture watcher PID ${child.pid} did not close: ${stdout}\n${stderr}`);

  assert.equal(outcome.status, 0, stderr);
  assert.match(stdout, /quiesce requested (at a safe boundary|after a completed check)/i);
  assert.equal(
    readFileSync(join(setup.state, "quiesced"), "utf8").trim(),
    `${releaseSha} ${instanceId} ${nonce}`,
  );
  } finally {
    clearTimeout(timer);
    if (!closed && child.exitCode === null && child.signalCode === null) child.kill("SIGTERM");
  }
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
  assert.equal(realpathSync(setup.env.HENUKIT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, previousSha));
  assert.equal(realpathSync(setup.env.HENUKIT_COMPAT_CURRENT_LINK), join(setup.env.HENUKIT_RELEASE_ROOT, previousSha));
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), previousSha);
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
  assert.match(result.stdout, /remained active; rollback needs no container replacement/);
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
  assert.match(result.stdout, /rollback needs no container replacement/);
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

test("activation accepts a candidate whose materials runtime differs from the previous release", () => {
  const setup = fixture({ candidateMaterialsDiffer: true });

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: setup.env,
  });
  const calls = readFileSync(setup.log, "utf8");

  assert.equal(result.status, 0, result.stderr);
  assert.doesNotMatch(result.stderr, /do not satisfy the exact rollback contract/i);
  assert.equal(
    readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(),
    releaseSha,
  );
  assert.match(calls, /^deploy /m);
});

// What the previous release left on the host. The fake release helper backs
// each of these up before overwriting it, the way install-materials-runtime.sh
// does, so a rollback has a record to replay.
const previousRuntime = {
  "usr/local/libexec/henukit/henukit-materials-activate": "previous-tooling\n",
  "usr/local/bin/henukit-deploy-webhook": "previous-receiver\n",
};
const previousRuntimeTargets = Object.keys(previousRuntime);

function runtimeFile(setup, relative) {
  return readFileSync(join(setup.materialsRuntimePrefix, relative), "utf8");
}

function assertRuntimeIsStillTheCandidate(setup) {
  for (const relative of previousRuntimeTargets) {
    assert.equal(runtimeFile(setup, relative), "candidate-runtime\n", relative);
  }
}

function assertPreviousRuntimeIsBack(setup) {
  for (const [relative, contents] of Object.entries(previousRuntime)) {
    assert.equal(runtimeFile(setup, relative), contents, relative);
  }
}

// The helper installs the materials runtime before it switches containers, so
// this failure leaves the previous images running and the candidate's tooling on
// the host. The rollback returns early without touching containers; it must
// still put the tooling back.
test("a rollback that keeps the previous containers still restores the materials runtime", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failCandidateDeployBeforeSwitch: true,
    materialsPathInitiallyEnabled: true,
    materialsRuntimeInstalled: previousRuntime,
    materialsRuntimeTargets: previousRuntimeTargets,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /remained active; rollback needs no container replacement/);
  assert.match(result.stdout, /restored 2 materials runtime file\(s\) overwritten by release/);
  assertPreviousRuntimeIsBack(setup);
});

test("a container rollback restores the materials runtime the candidate overwrote", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failTargetHealth: true,
    materialsRuntimeInstalled: previousRuntime,
    materialsRuntimeTargets: previousRuntimeTargets,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assertPreviousRuntimeIsBack(setup);
});

// install.sh keeps a target's first backup and skips it afterwards, so a record
// left by an earlier activation of the same SHA would describe whatever release
// preceded that attempt. The watcher clears it before the helper runs.
test("a rollback ignores a materials runtime record left by an earlier attempt at the same SHA", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failTargetHealth: true,
    materialsRuntimeBackup: {
      "usr/local/libexec/henukit/henukit-materials-activate": "stale-from-an-older-attempt\n",
      "usr/local/bin/henukit-deploy-webhook": "stale-from-an-older-attempt\n",
    },
    materialsRuntimeInstalled: previousRuntime,
    materialsRuntimeTargets: previousRuntimeTargets,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assertPreviousRuntimeIsBack(setup);
});

// A release that overwrote nothing leaves its record empty, and that is the
// normal outcome: restoring nothing is correct and is not a failure.
test("a rollback with an empty materials runtime record restores nothing", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({ failTargetHealth: true, previousSha });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /restored 0 materials runtime file\(s\)/);
  assert.match(result.stderr, /rolled back/);
  assert.doesNotMatch(result.stderr, /also failed/);
});

test("a materials knob pointed somewhere else alongside a production root is refused", () => {
  const setup = fixture();

  const result = spawnSync(script, ["--once"], {
    encoding: "utf8",
    env: {
      ...setup.env,
      HENUKIT_MATERIALS_ROOT: "/tmp/somewhere-else",
      HENUKIT_RELEASE_ROOT: "/opt/henukit-releases",
    },
  });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /HENUKIT_MATERIALS_ROOT cannot be combined with the production roots/);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
});

// The watcher creates the record before the helper runs, so a record that is
// gone by rollback time was removed -- the runbook sends operators into this
// directory to reclaim disk. Restoring nothing here would report a clean
// rollback while leaving the candidate's tooling installed.
test("a rollback refuses a materials runtime record that was removed", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failTargetHealth: true,
    materialsRuntimeInstalled: previousRuntime,
    materialsRuntimeTargets: previousRuntimeTargets,
    removeMaterialsRuntimeRecord: true,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /materials runtime record .* is missing/);
  assert.match(result.stderr, /rollback to .* also failed/);
});

// A record it cannot trust or replay fails the rollback -- but only after the
// containers are back, never instead of them.
test("a rollback refuses a materials runtime record that is not root-owned", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failTargetHealth: true,
    materialsRuntimeInstalled: previousRuntime,
    materialsRuntimeTargets: previousRuntimeTargets,
    nonRootTrustRoot: "materials-backup",
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /is not a trusted root-owned path/);
  assert.match(result.stderr, /rollback to .* also failed/);
  assert.match(
    readFileSync(setup.log, "utf8"),
    /docker compose --env-file .*henukit\.rollback\.env .* up -d --remove-orphans/,
  );
});

// The replay stages every file before it replaces any, so a record it cannot
// finish reading leaves the host exactly as it was. The good entry sorts first,
// so it is staged before the bad one is refused: if staging and replacing were
// interleaved it would already be back in place by then.
test("a record with one unusable entry replaces nothing at all", () => {
  const previousSha = "c".repeat(40);
  const good = "usr/local/libexec/henukit/henukit-materials-activate";
  const bad = "var/lib/henukit-not-an-install-target";
  const setup = fixture({
    failTargetHealth: true,
    materialsRuntimeInstalled: { [good]: "previous-tooling\n", [bad]: "previous-stray\n" },
    materialsRuntimeTargets: [good, bad],
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /which this unit may not write/);
  assert.equal(runtimeFile(setup, good), "candidate-runtime\n");
  assert.equal(
    existsSync(
      join(setup.materialsRuntimePrefix, "usr/local/libexec/henukit/.henukit-rollback-henukit-materials-activate"),
    ),
    false,
    "staging must not be left behind",
  );
});

// A record's directories are checked too. Trusting them because their files
// passed would let a subdirectory anyone can write to smuggle entries in.
test("a rollback refuses a materials runtime record with an untrusted directory", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failTargetHealth: true,
    materialsRuntimeInstalled: previousRuntime,
    materialsRuntimeTargets: previousRuntimeTargets,
    nonRootTrustRoot: "materials-record-directory",
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /record entry .* is not a trusted root-owned path/);
  assert.match(result.stderr, /rollback to .* also failed/);
  assertRuntimeIsStillTheCandidate(setup);
});

test("a rollback refuses a materials runtime record root that is a symlink", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failTargetHealth: true,
    danglingMaterialsRuntimeRecord: true,
    materialsRuntimeInstalled: previousRuntime,
    materialsRuntimeTargets: previousRuntimeTargets,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /is not a directory/);
  assert.match(result.stderr, /rollback to .* also failed/);
});

// find's exit status is easy to lose to a pipeline, and losing it would turn an
// unreadable record into an empty one -- a rollback that restores nothing and
// says so cheerfully.
test("a rollback refuses a materials runtime record it cannot read through", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failTargetHealth: true,
    materialsRuntimeInstalled: previousRuntime,
    materialsRuntimeTargets: previousRuntimeTargets,
    unreadableMaterialsRecordDirectory: true,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /could not be listed/);
  assert.match(result.stderr, /rollback to .* also failed/);
});

// mv would move the file into the directory rather than replace it.
test("a rollback refuses to replace a runtime path that became a directory", () => {
  const previousSha = "c".repeat(40);
  const target = "usr/local/bin/henukit-deploy-webhook";
  const setup = fixture({
    failTargetHealth: true,
    materialsRuntimeDestinationIsDirectory: target,
    materialsRuntimeInstalled: previousRuntime,
    materialsRuntimeTargets: previousRuntimeTargets,
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /which this unit may not write/);
  assert.match(result.stderr, /rollback to .* also failed/);
});

// install.sh also records files under /etc/henukit-deploy, which ProtectSystem
// keeps out of this unit's ReadWritePaths. A record it cannot replay fails the
// rollback loudly rather than being skipped.
test("a rollback refuses a materials runtime record outside the paths it may write", () => {
  const previousSha = "c".repeat(40);
  const outside = "etc/henukit-deploy/materials-activate.env";
  const setup = fixture({
    failTargetHealth: true,
    materialsRuntimeInstalled: { [outside]: "previous-config\n" },
    materialsRuntimeTargets: [outside],
    previousSha,
  });

  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stdout, /which this unit may not write/);
  assert.match(result.stderr, /rollback to .* also failed/);
  assert.equal(runtimeFile(setup, outside), "candidate-runtime\n");
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

test("explicit Actions recovery activates the newest successful main release", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    materialsPathInitiallyEnabled: true,
    previousSha,
    recoveryIdentity: "actions:123:1",
  });

  const output = execFileSync(
    script,
    ["--once", "--recover-degraded-baseline", previousSha],
    { encoding: "utf8", env: setup.env },
  );

  assert.match(output, /authorized degraded-baseline recovery/i);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), releaseSha);
  assert.match(
    readFileSync(
      join(setup.state, "degraded-recoveries", `${releaseSha}.activated`),
      "utf8",
    ),
    /status=activated/,
  );
  assert.equal(readFileSync(setup.materialsPathState, "utf8").trim(), "enabled");
});

test("Actions recovery rejects an approval bound to signed local artifacts", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    previousSha,
    recoveryIdentity: `local:${localManifestDigest}`,
  });

  const result = spawnSync(
    script,
    ["--once", "--recover-degraded-baseline", previousSha],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not bound to the selected artifact identity/i);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
});

test("Actions recovery rejects an approval bound to an earlier run attempt", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    previousSha,
    recoveryIdentity: "actions:123:1",
    runAttempt: 2,
  });

  const result = spawnSync(
    script,
    ["--once", "--recover-degraded-baseline", previousSha],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not bound to the selected artifact identity/i);
  assert.equal(readFileSync(join(setup.root, "active-sha"), "utf8").trim(), previousSha);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
});

test("Actions recovery rejects an approval bound to a different degraded baseline", () => {
  const previousSha = "c".repeat(40);
  const setup = fixture({
    failPreviousHealth: true,
    previousSha,
    recoveryIdentity: "actions:123:1",
  });
  writeFileSync(
    join(setup.state, "prepared", `${releaseSha}.recovery-baseline`),
    `${"d".repeat(40)}\n`,
    { mode: 0o600 },
  );

  const result = spawnSync(
    script,
    ["--once", "--recover-degraded-baseline", previousSha],
    { encoding: "utf8", env: setup.env },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /not bound to baseline/i);
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
  // Stop the original attempt after its authorization, with its original
  // approval inode still present; do not fabricate this phase from success.
  setup.env.FAKE_ACTIVATION_FREE_KIB = "1";
  const interrupted = spawnSync(script, args, { encoding: "utf8", env: setup.env });
  assert.notEqual(interrupted.status, 0);
  assert.match(interrupted.stderr, /approval remains unconsumed/i);
  assert.equal(existsSync(join(setup.state, "degraded-recoveries", `${releaseSha}.authorized`)), true);
  assert.equal(existsSync(join(setup.state, "approvals", releaseSha)), true);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /^deploy /m);
  setup.env.FAKE_ACTIVATION_FREE_KIB = "8388608";

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
  unlinkSync(setup.env.HENUKIT_COMPAT_CURRENT_LINK);
  symlinkSync(join(setup.root, "releases", "d".repeat(40)), setup.env.HENUKIT_COMPAT_CURRENT_LINK, "dir");

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
  assert.equal((calls.match(/docker load/g) ?? []).length, 15);
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

test("explicitly disabled job scanning does not require a getWork credential or contact the WSL relay", () => {
  const setup = fixture({ careerGetWorkEnabled: "0", getWorkToken: "" });
  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /activated and deterministic smoke checks passed/);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /18100|18101|getwork-mcp-relay/);
});

test("job scanning requires an explicit production mode before backup or activation", () => {
  for (const careerGetWorkEnabled of ["", "false", "2"]) {
    const setup = fixture({ careerGetWorkEnabled });
    const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /CAREER_GETWORK_ENABLED must explicitly be 0.*or 1/);
    assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
  }
});

test("disabled job scanning retains all independent Career credential gates", () => {
  const setup = fixture({ careerGetWorkEnabled: "0", getWorkToken: "", careerDigestSecret: "" });
  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Career digest credentials are missing or invalid/);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /pg_dump|docker load|^deploy /m);
});

test("one-shot refuses Career LLM, digest, and getWork MCP placeholder credentials", () => {
  for (const options of [
    { careerAIKey: "replace-career-ai-key-32bytes-minimum" },
    { careerDigestSecret: "replace-career-digest-secret-32bytes-minimum" },
    { careerDigestSecret: "local-career-digest-secret-32bytes-only!" },
    { getWorkToken: "replace-getwork-mcp-access-token-32chars!!" },
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

test("full watcher pauses before artifact or runtime mutations while a scoped release is active", () => {
  const setup = fixture();
  writeFileSync(join(setup.state, "scoped-food-release"), "private scoped receipt\n", { mode: 0o600 });
  const result = spawnSync(script, ["--once"], { encoding: "utf8", env: setup.env });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /scoped Food release/);
  const calls = readFileSync(setup.log, "utf8");
  assert.doesNotMatch(calls, /docker load|pg_dump|deploy |iptables|gh run download/);
});

function preservationWatcherFixture(options = {}) {
  const setup = fixture({ previousHasAccountPortfolio: true, failAccountGrant: true, ...options });
  const bin = join(setup.root, "bin");
  const helper = join(bin, "henukit-preserve-existing.py");
  writeExecutable(helper, readFileSync(new URL("../henukit-preserve-existing.py", import.meta.url), "utf8"));
  setup.command = join(bin, "watch-henukit-actions.sh");
  writeExecutable(setup.command, readFileSync(script, "utf8"));
  setup.env.HENUKIT_TRUST_ANCHOR = setup.root;
  setup.env.HENUKIT_ROLLBACK_ENV_FILE = setup.env.HENUKIT_ENV_FILE;
  setup.env.FAKE_ROLLBACK_ENV_FILE = setup.env.HENUKIT_ENV_FILE;
  setup.env.FAKE_TRUSTED_VERIFIER = helper;
  chmodSync(setup.env.HENUKIT_ENV_FILE, 0o600);
  chmodSync(setup.state, 0o700);
  chmodSync(join(setup.state, "prepared"), 0o700);
  const baseline = join(setup.env.HENUKIT_RELEASE_ROOT, "b".repeat(40));
  const patches = join(baseline, "infra/epay-gateway/patches");
  mkdirSync(patches, { recursive: true });
  for (const name of ["0001-henukit-query-and-notify-outbox.patch", "0002-henukit-close-refund-and-response-verification.patch", "0003-henukit-private-checkout-handle.patch"]) {
    writeFileSync(join(patches, name), readFileSync(new URL(`../../../infra/epay-gateway/patches/${name}`, import.meta.url)));
  }
  setup.env.FAKE_PRESERVE_PATCHES = patches;
  const gh = join(bin, "gh");
  writeExecutable(gh, readFileSync(gh, "utf8").replace('tar -C "$runtime_tree" -czf', 'mkdir -p "$runtime_tree/infra/epay-gateway"\ncp -R "$FAKE_PRESERVE_PATCHES" "$runtime_tree/infra/epay-gateway/patches"\ntar -C "$runtime_tree" -czf'));
  renameSync(join(bin, "docker"), join(bin, "docker-original"));
  const config = { services: {
    "portal-gateway": { environment: { QUIZCRAFT_CORE_URL: "http://quizcraft:10089" } },
    "account-portfolio": { environment: {
      ACCOUNT_PORTFOLIO_EASYPAY_ENABLED: "1", ACCOUNT_PORTFOLIO_EASYPAY_PID: "henukit-production",
      ACCOUNT_PORTFOLIO_EASYPAY_KEY: "henukit-production-secret-32bytes",
      ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL: "https://metaview.top/epay",
      ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL: "https://henukit.cn/api/v1/payment-providers/easypay/notifications",
      ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL: "https://henukit.cn/account/membership",
    } },
  } };
  const grants = { role: { code: "operations-operator", status: "active", revision: 1 },
    permissions: ["account.tickets.read", "account.tickets.reply", "account.tickets.transition", "account.membership.write", "account.points.adjust", "account.orders.read", "account.orders.close", "account.orders.refund"], bindings: [], audit: [] };
  writeFileSync(join(setup.root, "preserve-config.json"), JSON.stringify(config));
  writeFileSync(join(setup.root, "preserve-grants.json"), JSON.stringify(grants));
  setup.env.PRESERVE_FIXTURE_ROOT = setup.root;
  writeExecutable(join(bin, "docker"), `#!/usr/bin/env python3
import json,os,pathlib,sys
r=pathlib.Path(os.environ['PRESERVE_FIXTURE_ROOT']);a=sys.argv[1:]
c=json.loads((r/'preserve-config.json').read_text())
if a[0]=='compose' and a[-3:]==['config','--format','json']:print(json.dumps(c))
elif a[0]=='compose' and 'ps' in a:print(a[-1])
elif a[0]=='inspect' and len(a)==2:print(json.dumps([{'Config':{'Env':[k+'='+v for k,v in c['services'][a[1]]['environment'].items()]}}]))
elif a[0]=='exec' and a[1]=='-i' and 'psql -X -qAt' in a[-1]:
 sql=sys.stdin.read();assert 'READ ONLY' in sql
 print((r/'preserve-grants.json').read_text())
else:os.execv(str(r/'bin/docker-original'),[str(r/'bin/docker-original'),*a])
`);
  writeExecutable(join(bin, "ssh"), `#!/usr/bin/env python3
import json,sys
source=sys.stdin.read();assert 'urlopen' in source and 'write_text' not in source
print(json.dumps({'env':{'ino':1},'patches':{'ino':2}}))
`);
  const capture = spawnSync("python3", ["-I", helper, "capture", "--sha", releaseSha,
    "--env-file", setup.env.HENUKIT_ENV_FILE, "--receipt", join(setup.state, "prepared", `${releaseSha}.preserve-existing.json`),
    "--release-dir", baseline, "--role", setup.env.HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE], { encoding: "utf8", env: setup.env });
  assert.equal(capture.status, 0, capture.stderr);
  return setup;
}

test("preservation activates through the real watcher with no grant or security mutation", () => {
  const setup = preservationWatcherFixture();
  const before = lstatSync(setup.env.HENUKIT_ENV_FILE);
  const contents = readFileSync(setup.env.HENUKIT_ENV_FILE);
  const result = spawnSync(setup.command, ["--once"], { encoding: "utf8", env: setup.env });
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.equal(readFileSync(join(setup.state, "last-activated-sha"), "utf8").trim(), releaseSha);
  assert.equal(lstatSync(setup.env.HENUKIT_ENV_FILE).ino, before.ino);
  assert.deepEqual(readFileSync(setup.env.HENUKIT_ENV_FILE), contents);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /grant-account-operator-role|iptables|openssl|deploy-epay/);
});

test("preservation rollback keeps existing configuration and never replays a grant", () => {
  const setup = preservationWatcherFixture({ failTargetHealth: true });
  const before = lstatSync(setup.env.HENUKIT_ENV_FILE).ino;
  const result = spawnSync(setup.command, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /rolled back/);
  assert.equal(readFileSync(setup.active, "utf8").trim(), "b".repeat(40));
  assert.equal(lstatSync(setup.env.HENUKIT_ENV_FILE).ino, before);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /grant-account-operator-role|iptables|openssl|deploy-epay/);
});

test("a restarted preserved activation fails closed when its receipt disappears", () => {
  const setup = preservationWatcherFixture();
  const first = spawnSync(setup.command, ["--once"], { encoding: "utf8", env: setup.env });
  assert.equal(first.status, 0, first.stdout + first.stderr);
  unlinkSync(join(setup.state, "prepared", `${releaseSha}.preserve-existing.json`));
  const resumed = spawnSync(setup.command, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(resumed.status, 0);
  assert.match(resumed.stderr, /preservation/);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /grant-account-operator-role/);
});

test("prepared preservation cannot downgrade after a crash before rollback capture", () => {
  const setup = preservationWatcherFixture({ approved: false });
  const first = spawnSync(setup.command, ["--once"], { encoding: "utf8", env: setup.env });
  assert.equal(first.status, 0, first.stdout + first.stderr);
  unlinkSync(join(setup.state, "prepared", `${releaseSha}.preserve-existing.json`));
  writeFileSync(join(setup.state, "approvals", releaseSha), `${releaseSha}\n`, { mode: 0o600 });
  const resumed = spawnSync(setup.command, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(resumed.status, 0);
  assert.match(resumed.stderr, /preservation/);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /grant-account-operator-role|docker load|deploy /);
});

test("a restarted preserved activation rejects a replaced same-byte receipt", () => {
  const setup = preservationWatcherFixture();
  const first = spawnSync(setup.command, ["--once"], { encoding: "utf8", env: setup.env });
  assert.equal(first.status, 0, first.stdout + first.stderr);
  const receipt = join(setup.state, "prepared", `${releaseSha}.preserve-existing.json`);
  const incoming = `${receipt}.replacement`;
  writeFileSync(incoming, readFileSync(receipt), { mode: 0o400 });
  renameSync(incoming, receipt);
  const resumed = spawnSync(setup.command, ["--once"], { encoding: "utf8", env: setup.env });
  assert.notEqual(resumed.status, 0);
  assert.match(resumed.stderr, /preservation/);
  assert.doesNotMatch(readFileSync(setup.log, "utf8"), /grant-account-operator-role/);
});


test("Core freeze/schema failure invokes the same compatible rollback owner before aliases advance", () => {
  const setup=fixture();
  const result=spawnSync(script,["--once"],{encoding:"utf8",env:{...setup.env,FAKE_FAIL_QUIZCRAFT_GATE:"migrate"}});
  assert.notEqual(result.status,0);
  const calls=readFileSync(setup.log,"utf8");
  assert.match(calls,/quizcraft-gate begin[\s\S]*quizcraft-gate migrate[\s\S]*quizcraft-gate rollback/);
  assert.doesNotMatch(calls,/^deploy /m);
  assert.equal(realpathSync(setup.env.HENUKIT_CURRENT_LINK),join(setup.env.HENUKIT_RELEASE_ROOT,"b".repeat(40)));
});

test("Core backup rejection prevents exact approval consumption and image loading", () => {
  const setup=fixture({approved:false});
  const result=spawnSync(script,["--once"],{encoding:"utf8",env:{...setup.env,FAKE_FAIL_QUIZCRAFT_GATE:"prepare"}});
  assert.notEqual(result.status,0);
  assert.doesNotMatch(readFileSync(setup.log,"utf8"),/docker load|^deploy /m);
});

test("incompatible Core rollback cannot replace images or advance aliases", () => {
  const setup=fixture({failTargetHealth:true});
  const result=spawnSync(script,["--once"],{encoding:"utf8",env:{...setup.env,FAKE_FAIL_QUIZCRAFT_GATE:"rollback"}});
  assert.notEqual(result.status,0);
  const calls=readFileSync(setup.log,"utf8");
  assert.match(calls,/quizcraft-gate rollback/);
  assert.doesNotMatch(calls,/docker compose .* up -d --remove-orphans/);
  assert.equal(realpathSync(setup.env.HENUKIT_CURRENT_LINK),join(setup.env.HENUKIT_RELEASE_ROOT,"b".repeat(40)));
});
