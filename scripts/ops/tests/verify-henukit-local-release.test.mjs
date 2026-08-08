import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { gzipSync } from "node:zlib";
import {
  appendFileSync,
  chmodSync,
  mkdtempSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const verifier = fileURLToPath(
  new URL("../verify-henukit-local-release.sh", import.meta.url),
);
const inventory = fileURLToPath(
  new URL("../henukit-release-images.sh", import.meta.url),
);
const deploymentGuide = readFileSync(
  new URL("../../../docs/operations/henukit-artifact-deployment.md", import.meta.url),
  "utf8",
);
const releaseSha = "a".repeat(40);
const images = [
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

function sha256(buffer) {
  return createHash("sha256").update(buffer).digest("hex");
}

function writeArchive(directory, name, content) {
  const archive = gzipSync(content);
  writeFileSync(join(directory, name), archive, { mode: 0o400 });
  writeFileSync(join(directory, `${name}.sha256`), `${sha256(archive)}  ${name}\n`, {
    mode: 0o400,
  });
  return sha256(archive);
}

function signedFixture() {
  const root = mkdtempSync(join(tmpdir(), "henukit-local-release-"));
  const artifacts = join(root, "artifacts");
  const key = join(root, "release-key");
  const signers = join(root, "allowed-signers");
  execFileSync("mkdir", ["-p", artifacts]);
  writeFileSync(join(artifacts, "RELEASE_SHA"), `${releaseSha}\n`, { mode: 0o400 });

  const entries = [];
  for (const image of images) {
    const name = `${image}-${releaseSha}.docker.tar.gz`;
    entries.push([name, writeArchive(artifacts, name, `${image}\n`)]);
  }
  const runtimeName = `henukit-runtime-${releaseSha}.tar.gz`;
  entries.push([runtimeName, writeArchive(artifacts, runtimeName, "runtime\n")]);

  execFileSync("ssh-keygen", ["-q", "-t", "ed25519", "-N", "", "-f", key]);
  const publicKey = readFileSync(`${key}.pub`, "utf8").trim();
  writeFileSync(signers, `henukit-release ${publicKey}\n`, { mode: 0o600 });
  chmodSync(signers, 0o600);

  const manifest = join(artifacts, `henukit-release-${releaseSha}.manifest`);
  const inventorySha = sha256(readFileSync(inventory));
  const body = [
    "format=henukit-local-release-v1",
    `release_sha=${releaseSha}`,
    "source_ref=refs/heads/main",
    "builder_platform=linux/amd64",
    "signer=henukit-release",
    "signature_namespace=henukit-release",
    `inventory_sha256=${inventorySha}`,
    ...entries.map(([name, digest]) => `artifact_sha256=${digest}  ${name}`),
    "",
  ].join("\n");
  writeFileSync(manifest, body, { mode: 0o400 });
  execFileSync("ssh-keygen", ["-Y", "sign", "-f", key, "-n", "henukit-release", manifest]);
  chmodSync(`${manifest}.sig`, 0o400);

  return { artifacts, signers };
}

test("a local release bundle must have an exact signed manifest and verified artifact contents", () => {
  const { artifacts, signers } = signedFixture();
  const args = [
    "--artifact-dir", artifacts,
    "--sha", releaseSha,
    "--inventory", inventory,
    "--allowed-signers", signers,
  ];

  execFileSync(verifier, args, { stdio: "pipe" });
  const tamperedArchive = join(artifacts, `henukit-library-${releaseSha}.docker.tar.gz`);
  chmodSync(tamperedArchive, 0o600);
  appendFileSync(tamperedArchive, "tamper");
  assert.throws(
    () => execFileSync(verifier, args, { stdio: "pipe" }),
    /artifact digest|checksum|manifest/i,
  );
});

test("the operator guide keeps the WSL artifact path main-only, signed, and approval-gated", () => {
  assert.match(deploymentGuide, /WSL/i);
  assert.match(deploymentGuide, /henu-prod/);
  assert.match(deploymentGuide, /StrictHostKeyChecking=yes/);
  assert.match(deploymentGuide, /source worktree.*WSL/i);
  assert.match(deploymentGuide, /build-henukit-release-local\.sh/);
  assert.match(deploymentGuide, /deploy-henukit-release-from-wsl\.sh/);
  assert.match(deploymentGuide, /directly from WSL2 to production/i);
  assert.match(deploymentGuide, /--preflight/);
  assert.match(deploymentGuide, /--execute/);
  assert.match(deploymentGuide, /restores the service from an `EXIT` trap/i);
  assert.match(deploymentGuide, /resumes activation without retransferring/i);
  assert.doesNotMatch(deploymentGuide, /macOS relay/i);
  assert.match(deploymentGuide, /--local-artifacts/);
  assert.match(deploymentGuide, /allowed-signers/i);
  assert.match(deploymentGuide, /out-of-band/i);
  assert.match(deploymentGuide, /release-trust-root/i);
  assert.match(deploymentGuide, /sha256sum -c/);
  assert.match(deploymentGuide, /origin\/main/);
  assert.match(deploymentGuide, /backup/i);
  assert.match(deploymentGuide, /rollback/i);
});
