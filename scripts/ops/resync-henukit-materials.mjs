#!/usr/bin/env node
// Invoked only by the signed root control wrapper while its common lock is held.
import { spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { closeSync, constants, fsyncSync, linkSync, lstatSync, mkdirSync, openSync, readFileSync, readdirSync, statfsSync, unlinkSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

const ROOT_UID = 0;
const CONFIG_ROOT = "/etc/henukit-deploy";
const MATERIALS_ROOT = "/opt/henukit-materials";
const GIT = "/usr/bin/git";
const SYSTEMCTL = "/usr/bin/systemctl";
const MANIFEST_BASE = "https://api.github.com";
const ORCHESTRATE = "/usr/local/libexec/henukit/henukit-materials-orchestrate";
const ACTIVATOR = "/usr/local/libexec/henukit/henukit-materials-activate";
const SHA = /^[a-f0-9]{40}$/;
const HASH = /^[a-f0-9]{64}$/;
const UUID = /^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/;
const RELEASE = /^[a-f0-9]{40}-[a-f0-9]{16}$/;
const cleanEnv = { PATH: "/usr/bin:/bin", HOME: "/root", GIT_CONFIG_NOSYSTEM: "1", GIT_CONFIG_GLOBAL: "/dev/null", GIT_TERMINAL_PROMPT: "0" };
// Keep the control wrapper's lock alive even if this parent is killed.
const pipelineStdio = ["ignore", "inherit", "inherit", "ignore", "ignore", "ignore", "ignore", "ignore", "ignore", 9];

function fail(message) { throw new Error(message); }
function digest(bytes) { return createHash("sha256").update(bytes).digest("hex"); }
function present(path) { try { lstatSync(path); return true; } catch (error) { if (error.code === "ENOENT") return false; throw error; } }
function syncDirectory(path) { const fd = openSync(path, constants.O_RDONLY); try { fsyncSync(fd); } finally { closeSync(fd); } }
function writeOnce(path, value) {
  const temporary = join(dirname(path), `.writing.${randomUUID()}`);
  const fd = openSync(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o400);
  try { writeFileSync(fd, typeof value === "string" ? value : `${JSON.stringify(value, null, 2)}\n`); fsyncSync(fd); } finally { closeSync(fd); }
  // link is atomic and fails if the destination already exists. A process death
  // can leave an inert temporary file, never a partial final audit record.
  try { linkSync(temporary, path); syncDirectory(dirname(path)); }
  finally { unlinkSync(temporary); }
  syncDirectory(dirname(path));
}
function metadata(path, directory = false) {
  const info = lstatSync(path);
  if (info.isSymbolicLink() || (directory ? !info.isDirectory() : !info.isFile()) || info.uid !== ROOT_UID || (info.mode & 0o022)) fail(`untrusted ${directory ? "directory" : "file"}: ${path}`);
  return info;
}
function trustedParents(path) {
  let cursor = resolve(path);
  for (;;) {
    const info = lstatSync(cursor);
    if (info.isSymbolicLink() || !info.isDirectory() || ![0, ROOT_UID].includes(info.uid) || (info.mode & 0o022)) fail(`untrusted parent: ${cursor}`);
    if (cursor === dirname(cursor)) break;
    cursor = dirname(cursor);
  }
}
function readTrusted(path, max = 4 * 1024 * 1024) {
  trustedParents(dirname(path));
  if (metadata(path).size > max) fail(`metadata exceeds limit: ${path}`);
  return readFileSync(path);
}
function config(name) {
  const values = {};
  for (const line of readTrusted(join(CONFIG_ROOT, name)).toString("utf8").split(/\r?\n/)) {
    if (!line || line.startsWith("#")) continue;
    const match = /^([A-Z][A-Z0-9_]*)=(.*)$/.exec(line);
    if (!match || Object.hasOwn(values, match[1])) fail("invalid or duplicate configuration key");
    values[match[1]] = match[2];
  }
  return values;
}
function options(argv) {
  const [mode, ...args] = argv;
  if (!["resync", "reconcile"].includes(mode)) fail("expected resync or reconcile");
  const result = { mode };
  const names = new Set(["sha", "manifest-sha256", "request-id", "operator", "reason", "outcome"]);
  for (let i = 0; i < args.length; i++) {
    const key = args[i].replace(/^--/, "");
    if (!args[i].startsWith("--") || Object.hasOwn(result, key)) fail("invalid or duplicate option");
    if (["preflight", "execute"].includes(key)) result[key] = true;
    else {
      if (!names.has(key) || !args[i + 1] || args[i + 1].startsWith("--")) fail("invalid option value");
      result[key] = args[++i];
    }
  }
  if (!!result.preflight === !!result.execute) fail("choose exactly one of --preflight and --execute");
  if (!UUID.test(result["request-id"] || "")) fail("request ID must be a lowercase UUID v4");
  for (const [key, limit] of [["operator", 120], ["reason", 500]]) {
    if (!result[key]?.trim() || result[key].length > limit || /[\u0000-\u001f\u007f]/u.test(result[key])) fail(`invalid ${key}`);
  }
  if (mode === "resync" && (!SHA.test(result.sha || "") || !HASH.test(result["manifest-sha256"] || "") || result.outcome)) fail("resync requires exact SHA and manifest SHA256");
  if (mode === "reconcile" && (!["finish", "rollback"].includes(result.outcome) || result.sha || result["manifest-sha256"])) fail("reconcile requires --outcome finish or rollback");
  return result;
}
function run(command, args) {
  const result = spawnSync(command, args, { env: cleanEnv, encoding: "utf8", timeout: 30000, maxBuffer: 1024 * 1024 });
  if (result.error || result.status !== 0) fail(`required command failed: ${command}`);
  return result.stdout.trim();
}
function unit(name, property) { return run(SYSTEMCTL, ["show", name, `--property=${property}`, "--value"]); }
function requireQuiescent(policy, allowActivation = false, allowWaiting = false) {
  if (unit("henukit-materials-webhook.path", "ActiveState") !== "inactive" || unit("henukit-materials-webhook.path", "UnitFileState") !== "disabled") fail("disable the materials watcher before administrative recovery");
  if (unit("henukit-materials-runner.service", "MainPID") !== "0" || !["inactive", "failed"].includes(unit("henukit-materials-runner.service", "ActiveState"))) fail("materials runner must be idle");
  for (const path of [join(policy.state, "running.json"), ...(allowWaiting ? [] : [join(policy.state, "queue/latest.json")])]) {
    try { lstatSync(path); fail("materials work is queued or running"); } catch (error) { if (error.code !== "ENOENT") throw error; }
  }
  if (lstatSync(join(policy.state, "candidates")).isSymbolicLink() || readdirSync(join(policy.state, "candidates")).length) fail("candidate root must be empty");
  if (!allowActivation) {
    for (const name of [".maintenance", "activation-journal.json"]) {
      try { lstatSync(join(MATERIALS_ROOT, "public", name)); fail("activation requires existing journal recovery"); } catch (error) { if (error.code !== "ENOENT") throw error; }
    }
  }
}
function policy() {
  trustedParents(MATERIALS_ROOT);
  const webhook = config("materials-webhook.env"), seal = config("materials-seal.env");
  const repository = webhook.HENUKIT_WEBHOOK_REPOSITORY;
  if (repository !== "jry21223/HENU-Final-Review" || webhook.HENUKIT_WEBHOOK_BRANCH !== "main" || seal.HENUKIT_MATERIALS_SOURCE_REF !== "refs/heads/main") fail("unapproved source repository or ref");
  const source = `https://github.com/${repository}.git`;
  if (webhook.HENUKIT_MATERIALS_SOURCE_REPOSITORY !== source || seal.HENUKIT_MATERIALS_SOURCE_REPOSITORY !== source || seal.HENUKIT_MATERIALS_SEALED_ROOT !== join(MATERIALS_ROOT, "sealed")) fail("source policies do not match");
  const state = webhook.HENUKIT_WEBHOOK_STATE_DIR;
  if (!state?.startsWith("/") || webhook.HENUKIT_MATERIALS_CANDIDATE_ROOT !== join(state, "candidates")) fail("invalid state policy");
  return { repository, source, ref: "refs/heads/main", state };
}
async function sourceInventory(p, o) {
  const head = run(GIT, ["ls-remote", "--exit-code", p.source, p.ref]);
  if (head !== `${o.sha}\t${p.ref}`) fail("requested SHA is not current upstream main");
  const response = await fetch(`${MANIFEST_BASE}/repos/${p.repository}/contents/manifest.json?ref=${o.sha}`, {
    headers: { Accept: "application/vnd.github.raw+json", "User-Agent": "HENU-Kit-materials-admin-preflight" },
    redirect: "error", signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) fail("cannot read exact-SHA source manifest");
  const chunks = []; let length = 0;
  for await (const chunk of response.body) {
    length += chunk.length; if (length > 4 * 1024 * 1024) fail("source manifest exceeds limit"); chunks.push(chunk);
  }
  const bytes = Buffer.concat(chunks);
  if (digest(bytes) !== o["manifest-sha256"]) fail("source manifest digest differs from approval");
  const manifest = JSON.parse(bytes.toString("utf8"));
  if (!Array.isArray(manifest.subjects)) fail("invalid source manifest");
  let publicAssets = 0, reviewAssets = 0, allBytes = 0, publicBytes = 0;
  for (const subject of manifest.subjects) {
    if (!Array.isArray(subject.assets)) fail("invalid manifest subject");
    for (const asset of subject.assets) {
      if (!Number.isSafeInteger(asset.bytes) || asset.bytes < 0 || !HASH.test(asset.sha256 || "")) fail("invalid manifest asset");
      allBytes += asset.bytes;
      if (typeof asset.role !== "string") fail("invalid manifest role");
      if (asset.role.startsWith("待复核")) reviewAssets++;
      else { publicAssets++; publicBytes += asset.bytes; }
    }
  }
  const required = 2 * allBytes + publicBytes + 4 * 1024 ** 3;
  if (!Number.isSafeInteger(required) || publicAssets < 1 || publicAssets > 500) fail("manifest inventory exceeds supported release boundary");
  const fs = statfsSync(MATERIALS_ROOT, { bigint: true });
  const available = fs.bavail * fs.bsize;
  if (available < BigInt(required)) fail("insufficient free disk for source, mirror, pack estimate and 4 GiB reserve");
  return { public_assets: publicAssets, review_only_assets: reviewAssets, manifest_sha256: digest(bytes), required_available_bytes: required, available_bytes: available.toString() };
}
function previousRelease() {
  const id = readTrusted(join(MATERIALS_ROOT, "public/ACTIVE_RELEASE"), 128).toString("utf8").trim();
  if (!RELEASE.test(id)) fail("invalid previous release marker");
  const receipt = readTrusted(join(MATERIALS_ROOT, "sealed", id, "sealed-release.json"));
  if (JSON.parse(receipt).release_id !== id) fail("previous receipt does not match marker");
  metadata(join(MATERIALS_ROOT, "public/releases", id), true);
  return { previous_release: id, previous_receipt_sha256: digest(receipt) };
}

function auditRoot() { return join(MATERIALS_ROOT, "admin-resync"); }
function requirePrivateDirectory(path) {
  trustedParents(path);
  if ((metadata(path, true).mode & 0o777) !== 0o700) fail(`audit directory must have mode 0700: ${path}`);
}
function postcondition(releaseID) {
  if (readTrusted(join(MATERIALS_ROOT, "public/ACTIVE_RELEASE"), 128).toString("utf8").trim() !== releaseID) fail("active release does not match administrative outcome");
  for (const name of [".maintenance", "activation-journal.json"]) if (present(join(MATERIALS_ROOT, "public", name))) fail("activation remains fenced or unresolved");
}
function closeActive(requestID) {
  const path = join(auditRoot(), "active");
  if (readTrusted(path, 128).toString("utf8") !== `${requestID}\n`) fail("active administrative request differs");
  unlinkSync(path); syncDirectory(auditRoot());
}
function executeRequest(p, o, report) {
  if (!present(auditRoot())) { mkdirSync(auditRoot(), { mode: 0o700 }); syncDirectory(MATERIALS_ROOT); }
  requirePrivateDirectory(auditRoot());
  const directory = join(auditRoot(), o["request-id"]);
  mkdirSync(directory, { mode: 0o700 }); syncDirectory(auditRoot());
  const request = { ...report, version: 1, state: "requested", request_id: o["request-id"], caller_uid: process.getuid(), operator_label: o.operator, reason: o.reason, created_at: new Date().toISOString() };
  writeOnce(join(directory, "request.json"), request);
  writeOnce(join(directory, "approval.env"), [
    `request_id=${request.request_id}`, `sha=${o.sha}`, `manifest_sha256=${o["manifest-sha256"]}`,
    `repository=${p.repository}`, `ref=${p.ref}`, `previous_release=${report.previous_release}`, "",
  ].join("\n"));
  writeOnce(join(auditRoot(), "active"), `${request.request_id}\n`);
  let outcome;
  try {
    requireQuiescent(p);
    const run = spawnSync(ORCHESTRATE, ["--sha", o.sha, "--admin-request", request.request_id, "--repository", p.repository, "--ref", p.ref], { env: cleanEnv, stdio: pipelineStdio, timeout: 2 * 60 * 60 * 1000 });
    if (run.error || run.status !== 0) fail("administrative publication failed; retain active request and use reconcile");
    const target = `${o.sha}-${o["manifest-sha256"].slice(0, 16)}`;
    postcondition(target);
    outcome = { version: 1, state: "completed", request_id: request.request_id, release_id: target, completed_at: new Date().toISOString() };
  } catch (error) {
    writeOnce(join(directory, "result.json"), { version: 1, state: "failed", request_id: request.request_id, completed_at: new Date().toISOString(), error: error.message });
    throw error;
  }
  writeOnce(join(directory, "result.json"), outcome);
  closeActive(request.request_id);
  return outcome;
}

function readAudit(path, max = 4 * 1024 * 1024) {
  if ((metadata(path).mode & 0o777) !== 0o400) fail(`audit record must have mode 0400: ${path}`);
  return readTrusted(path, max);
}
function reconcileRequest(p, o) {
  requirePrivateDirectory(auditRoot());
  const directory = join(auditRoot(), o["request-id"]);
  requirePrivateDirectory(directory);
  if (readAudit(join(auditRoot(), "active"), 128).toString("utf8") !== `${o["request-id"]}\n`) fail("request is not the unresolved administrative request");
  const requestBytes = readAudit(join(directory, "request.json"));
  const request = JSON.parse(requestBytes.toString("utf8"));
  if (request.version !== 1 || request.state !== "requested" || request.request_id !== o["request-id"] || request.caller_uid !== ROOT_UID || request.repository !== p.repository || request.ref !== p.ref || !SHA.test(request.sha || "") || !HASH.test(request.manifest_sha256 || "") || !RELEASE.test(request.previous_release || "") || !HASH.test(request.previous_receipt_sha256 || "")) fail("invalid administrative request identity");
  const releaseID = o.outcome === "finish" ? `${request.sha}-${request.manifest_sha256.slice(0, 16)}` : request.previous_release;
  const resolutionPath = join(directory, "resolution.json");
  if (present(resolutionPath)) {
    const resolution = JSON.parse(readAudit(resolutionPath).toString("utf8"));
    if (resolution.outcome !== o.outcome || resolution.release_id !== releaseID || resolution.request_sha256 !== digest(requestBytes)) fail("existing resolution differs from requested outcome");
    postcondition(releaseID);
    if (o.execute) closeActive(request.request_id);
    return { ...resolution, state: o.execute ? "resolved" : "preflight" };
  }
  const receiptPath = join(MATERIALS_ROOT, "sealed", releaseID, "sealed-release.json");
  const receiptBytes = readTrusted(receiptPath);
  if (JSON.parse(receiptBytes).release_id !== releaseID) fail("reconciliation receipt does not match release");
  const receiptSHA = digest(receiptBytes);
  if (o.outcome === "rollback") {
    if (receiptSHA !== request.previous_receipt_sha256) fail("previous receipt has changed");
  } else {
    const approvedReceipt = readAudit(join(directory, "sealed-receipt.sha256"), 128).toString("utf8").trim();
    if (receiptSHA !== approvedReceipt || digest(readTrusted(join(MATERIALS_ROOT, "sealed", releaseID, "manifest.json"))) !== request.manifest_sha256) fail("sealed target differs from administrative approval");
  }
  const resolution = { version: 1, state: "resolved", request_id: request.request_id, request_sha256: digest(requestBytes), outcome: o.outcome, release_id: releaseID, receipt_sha256: receiptSHA, caller_uid: process.getuid(), operator_label: o.operator, reason: o.reason };
  if (o.preflight) return { ...resolution, state: "preflight" };
  const attempt = join(directory, `resolution-attempt-${randomUUID()}`);
  writeOnce(`${attempt}.request.json`, { ...resolution, created_at: new Date().toISOString() });
  try {
    // A completed result followed by a crash before unlock needs no second activation.
    const resultPath = join(directory, "result.json");
    const completed = present(resultPath) && JSON.parse(readAudit(resultPath).toString("utf8")).state === "completed";
    if (completed && o.outcome === "finish") postcondition(releaseID);
    else {
      metadata(ACTIVATOR);
      const run = spawnSync(ACTIVATOR, ["--release-id", releaseID, "--receipt-sha256", receiptSHA], { env: cleanEnv, stdio: pipelineStdio, timeout: 2 * 60 * 60 * 1000 });
      if (run.error || run.status !== 0) fail("activation reconciliation failed; retain active request and existing fence/journal");
      postcondition(releaseID);
    }
    resolution.completed_at = new Date().toISOString();
    writeOnce(resolutionPath, resolution);
    writeOnce(`${attempt}.result.json`, { state: "completed", completed_at: resolution.completed_at });
  } catch (error) {
    writeOnce(`${attempt}.result.json`, { state: "failed", error: error.message, completed_at: new Date().toISOString() });
    throw error;
  }
  closeActive(request.request_id);
  return resolution;
}

async function main() {
  if (process.getuid() !== ROOT_UID) fail("administrative recovery requires root");
  const o = options(process.argv.slice(2));
  const p = policy();
  requireQuiescent(p, o.mode === "reconcile", o.mode === "reconcile");
  if (o.mode === "reconcile") { console.log(JSON.stringify(reconcileRequest(p, o))); return; }
  if (present(join(auditRoot(), "active"))) fail("an administrative request is unresolved");
  if (present(join(auditRoot(), o["request-id"]))) fail("request ID already exists; use reconcile for an unresolved request");
  const inventory = await sourceInventory(p, o);
  const report = { state: "preflight", sha: o.sha, repository: p.repository, ref: p.ref, ...inventory, ...previousRelease() };
  if (o.preflight) console.log(JSON.stringify(report));
  else console.log(JSON.stringify(executeRequest(p, o, report)));
}
main().catch(error => { console.error(`henukit-materials-control: ${error.message}`); process.exitCode = 1; });
