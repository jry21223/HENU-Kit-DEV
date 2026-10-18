import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { chmodSync, closeSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, openSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "../../..");
const source = join(repositoryRoot, "scripts/ops/resync-henukit-materials.mjs");
const sha = "a".repeat(40);
const oldID = `${"b".repeat(40)}-${"c".repeat(16)}`;
const requestID = "12345678-1234-4234-8234-123456789abc";
const manifest = Buffer.from(JSON.stringify({ subjects: [{ name: "Math", assets: [
  { publicPath: "Math/outline.pdf", role: "复习讲义", bytes: 12, sha256: "d".repeat(64) },
  { publicPath: "Math/review.pdf", role: "待复核资料", bytes: 10, sha256: "e".repeat(64) },
] }] }));
const manifestSHA = createHash("sha256").update(manifest).digest("hex");
const targetID = `${sha}-${manifestSHA.slice(0, 16)}`;

function command(path, body) {
  writeFileSync(path, `#!${process.execPath}\n${body}`, { mode: 0o700 });
  chmodSync(path, 0o700);
}

async function fixture() {
  const template = readFileSync(source, "utf8");
  const root = realpathSync(mkdtempSync(join(tmpdir(), "materials-admin-resync-")));
  const materials = join(root, "materials");
  const state = join(root, "state");
  const config = join(root, "config");
  for (const path of [materials, state, config, join(state, "queue"), join(state, "candidates"), join(materials, "public"), join(materials, "public/releases"), join(materials, "public/releases", oldID), join(materials, "sealed"), join(materials, "sealed", oldID)]) mkdirSync(path, { mode: 0o700 });
  writeFileSync(join(materials, "public/ACTIVE_RELEASE"), `${oldID}\n`, { mode: 0o400 });
  writeFileSync(join(materials, "sealed", oldID, "sealed-release.json"), JSON.stringify({ release_id: oldID }), { mode: 0o400 });
  writeFileSync(join(config, "materials-webhook.env"), `HENUKIT_WEBHOOK_REPOSITORY=jry21223/HENU-Final-Review\nHENUKIT_WEBHOOK_BRANCH=main\nHENUKIT_WEBHOOK_STATE_DIR=${state}\nHENUKIT_MATERIALS_SOURCE_REPOSITORY=https://github.com/jry21223/HENU-Final-Review.git\nHENUKIT_MATERIALS_CANDIDATE_ROOT=${state}/candidates\n`, { mode: 0o600 });
  writeFileSync(join(config, "materials-seal.env"), `HENUKIT_MATERIALS_SOURCE_REPOSITORY=https://github.com/jry21223/HENU-Final-Review.git\nHENUKIT_MATERIALS_SOURCE_REF=refs/heads/main\nHENUKIT_MATERIALS_SEALED_ROOT=${materials}/sealed\n`, { mode: 0o600 });
  const git = join(root, "git");
  command(git, `console.log('${sha}\\trefs/heads/main');`);
  const systemctl = join(root, "systemctl");
  command(systemctl, `const a=process.argv.join(' '); console.log(a.includes('MainPID')?'0':a.includes('UnitFileState')?'disabled':'inactive');`);
  const calls = join(root, "calls");
  const orchestrate = join(root, "orchestrate");
  command(orchestrate, `const fs=require('node:fs'),marker=${JSON.stringify(join(materials, "public/ACTIVE_RELEASE"))};fs.appendFileSync(${JSON.stringify(calls)},JSON.stringify(process.argv.slice(2))+'\\n');fs.writeFileSync(marker+'.next','${targetID}\\n');fs.renameSync(marker+'.next',marker);`);
  const activator = join(root, "activator");
  command(activator, `const fs=require('node:fs'),marker=${JSON.stringify(join(materials, "public/ACTIVE_RELEASE"))};fs.appendFileSync(${JSON.stringify(calls)},JSON.stringify(process.argv.slice(2))+'\\n');fs.writeFileSync(marker+'.next',process.argv[3]+'\\n');fs.renameSync(marker+'.next',marker);`);
  const server = createServer((req, res) => {
    const pinnedRawRequest = req.url === `/repos/jry21223/HENU-Final-Review/contents/manifest.json?ref=${sha}` &&
      req.headers.accept === "application/vnd.github.raw+json" &&
      req.headers["user-agent"] === "HENU-Kit-materials-admin-preflight" && !req.headers.authorization;
    res.writeHead(pinnedRawRequest ? 200 : 404); res.end(manifest);
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
  let code = template;
  for (const [from, to] of [
    ["const ROOT_UID = 0;", `const ROOT_UID = ${process.getuid()};`],
    ["/etc/henukit-deploy", config], ["/opt/henukit-materials", materials],
    ["/usr/bin/git", git], ["/usr/bin/systemctl", systemctl],
    ["/usr/local/libexec/henukit/henukit-materials-orchestrate", orchestrate],
    ["/usr/local/libexec/henukit/henukit-materials-activate", activator],
    ["https://api.github.com", `http://127.0.0.1:${server.address().port}`],
  ]) code = code.replaceAll(from, to);
  // The test's isolated root replaces production's / trust boundary, as does its UID.
  code = code.replace('if (cursor === dirname(cursor)) break;', `if (cursor === ${JSON.stringify(root)} || cursor === dirname(cursor)) break;`);
  const helper = join(root, "resync.mjs"); writeFileSync(helper, code, { mode: 0o600 });
  return { root, helper, state, materials, git, systemctl, calls, orchestrate, activator,
    close: async () => { await new Promise(resolve => server.close(resolve)); rmSync(root, { recursive: true, force: true }); } };
}

async function invoke(f, mode, extra = []) {
  const args = mode === "resync" ? ["--sha", sha, "--manifest-sha256", manifestSHA] : [];
  return await new Promise((resolve, reject) => {
    const lock = openSync(join(f.materials, "orchestration.lock"), "a", 0o600);
    const child = spawn(process.execPath, [f.helper, mode, ...args, "--request-id", requestID, "--operator", "test-operator", "--reason", "recover expired delivery", ...extra], { env: { PATH: process.env.PATH }, stdio: ["ignore", "pipe", "pipe", "ignore", "ignore", "ignore", "ignore", "ignore", "ignore", lock], timeout: 15000 });
    closeSync(lock);
    let stdout = "", stderr = "";
    child.stdout.on("data", b => stdout += b); child.stderr.on("data", b => stderr += b);
    child.on("error", reject); child.on("close", code => resolve({ code, stdout, stderr }));
  });
}

test("admin preflight reports pinned inventory without writing audit or invoking publication", async () => {
  const f = await fixture();
  try {
    const r = await invoke(f, "resync", ["--preflight"]);
    assert.equal(r.code, 0, r.stderr);
    const report = JSON.parse(r.stdout);
    assert.equal(report.public_assets, 1); assert.equal(report.review_only_assets, 1);
    assert.equal(report.manifest_sha256, manifestSHA); assert.equal(report.previous_release, oldID);
    assert.equal(existsSync(join(f.materials, "admin-resync")), false);
    assert.equal(existsSync(f.calls), false);
  } finally { await f.close(); }
});

test("execute records a distinct admin request, publishes the pinned release, and refuses request replay", async () => {
  const f = await fixture();
  try {
    const r = await invoke(f, "resync", ["--execute"]);
    assert.equal(r.code, 0, r.stderr);
    const audit = join(f.materials, "admin-resync", requestID);
    const request = JSON.parse(readFileSync(join(audit, "request.json")));
    assert.equal(request.caller_uid, process.getuid()); assert.equal(request.operator_label, "test-operator");
    assert.equal(request.sha, sha); assert.equal(request.manifest_sha256, manifestSHA);
    assert.equal(request.previous_release, oldID);
    assert.equal(JSON.parse(readFileSync(join(audit, "result.json"))).state, "completed");
    assert.equal(existsSync(join(f.materials, "admin-resync/active")), false);
    const calls = readFileSync(f.calls, "utf8");
    assert.match(calls, /--admin-request/); assert.doesNotMatch(calls, /--delivery/);
    const again = await invoke(f, "resync", ["--execute"]);
    assert.notEqual(again.code, 0); assert.equal(readFileSync(f.calls, "utf8"), calls);
    assert.equal(readdirSync(join(f.state, "queue")).length, 0);
  } finally { await f.close(); }
});

test("failed publication remains pending until an explicit full rollback resolves it without consuming waiting work", async () => {
  const f = await fixture();
  try {
    command(f.orchestrate, "process.exit(9);");
    const failed = await invoke(f, "resync", ["--execute"]);
    assert.notEqual(failed.code, 0);
    const audit = join(f.materials, "admin-resync", requestID);
    assert.equal(JSON.parse(readFileSync(join(audit, "result.json"))).state, "failed");
    assert.equal(readFileSync(join(f.materials, "admin-resync/active"), "utf8"), `${requestID}\n`);
    writeFileSync(join(f.state, "queue/latest.json"), '{"real_delivery":"newer-event"}');
    const resolved = await invoke(f, "reconcile", ["--outcome", "rollback", "--execute"]);
    assert.equal(resolved.code, 0, resolved.stderr);
    assert.equal(JSON.parse(readFileSync(join(audit, "resolution.json"))).release_id, oldID);
    assert.equal(existsSync(join(f.materials, "admin-resync/active")), false);
    assert.equal(readFileSync(join(f.state, "queue/latest.json"), "utf8"), '{"real_delivery":"newer-event"}');
    assert.match(readFileSync(f.calls, "utf8"), /--receipt-sha256/);
  } finally { await f.close(); }
});

test("a main change after successful preflight is rejected before any execute intent", async () => {
  const f = await fixture();
  try {
    assert.equal((await invoke(f, "resync", ["--preflight"])).code, 0);
    command(f.git, `console.log('${"f".repeat(40)}\\trefs/heads/main');`);
    const r = await invoke(f, "resync", ["--execute"]);
    assert.notEqual(r.code, 0); assert.match(r.stderr, /not current upstream main/);
    assert.equal(existsSync(join(f.materials, "admin-resync")), false); assert.equal(existsSync(f.calls), false);
  } finally { await f.close(); }
});

for (const name of ["waiting", "candidate", "fence", "watcher"]) {
  test(`admin execution refuses ${name} work without changing the queue or recording a request`, async () => {
    const f = await fixture();
    try {
      if (name === "waiting") writeFileSync(join(f.state, "queue/latest.json"), '{"delivery":"real-event"}');
      if (name === "candidate") mkdirSync(join(f.state, "candidates/.attempt.ABC123def4"));
      if (name === "fence") writeFileSync(join(f.materials, "public/.maintenance"), "maintenance");
      if (name === "watcher") command(f.systemctl, "console.log('active');");
      const r = await invoke(f, "resync", ["--execute"]);
      assert.notEqual(r.code, 0); assert.equal(existsSync(join(f.materials, "admin-resync")), false); assert.equal(existsSync(f.calls), false);
      if (name === "waiting") assert.equal(readFileSync(join(f.state, "queue/latest.json"), "utf8"), '{"delivery":"real-event"}');
    } finally { await f.close(); }
  });
}

test("process death after durable intent leaves a pending request that only explicit reconciliation can resolve", async () => {
  const f = await fixture();
  try {
    command(f.orchestrate, "process.kill(process.ppid,'SIGKILL');process.exit(0);");
    const r = await invoke(f, "resync", ["--execute"]);
    assert.notEqual(r.code, 0);
    const audit = join(f.materials, "admin-resync", requestID);
    assert.equal(existsSync(join(audit, "request.json")), true); assert.equal(existsSync(join(audit, "result.json")), false);
    assert.equal(readFileSync(join(f.materials, "admin-resync/active"), "utf8"), `${requestID}\n`);
    const resolved = await invoke(f, "reconcile", ["--outcome", "rollback", "--execute"]);
    assert.equal(resolved.code, 0, resolved.stderr);
    const calls = readFileSync(f.calls, "utf8");
    // Reconstruct only the durable-unlock boundary of the completed operation.
    writeFileSync(join(f.materials, "admin-resync/active"), `${requestID}\n`, { mode: 0o400 });
    const cleanup = await invoke(f, "reconcile", ["--outcome", "rollback", "--execute"]);
    assert.equal(cleanup.code, 0, cleanup.stderr); assert.equal(readFileSync(f.calls, "utf8"), calls);
    assert.equal(existsSync(join(f.materials, "admin-resync/active")), false);
  } finally { await f.close(); }
});

for (const afterLink of [false, true]) {
  test(`a process death ${afterLink ? "after" : "before"} atomic active publication never exposes a partial final record`, async () => {
    const f = await fixture();
    try {
      const original = readFileSync(f.helper, "utf8");
      const fault = `import faultFS from 'node:fs';import {syncBuiltinESMExports} from 'node:module';const realLink=faultFS.linkSync;faultFS.linkSync=(from,to)=>{if(to.endsWith('/admin-resync/active')){${afterLink ? "realLink(from,to);" : ""}process.kill(process.pid,'SIGKILL');}return realLink(from,to);};syncBuiltinESMExports();\n`;
      writeFileSync(f.helper, fault + original.replace(/^#![^\n]*\n/, ""));
      const r = await invoke(f, "resync", ["--execute"]);
      assert.equal(r.code, null, r.stderr); assert.equal(existsSync(f.calls), false);
      const active = join(f.materials, "admin-resync/active");
      assert.equal(existsSync(active), afterLink);
      if (afterLink) {
        assert.equal(readFileSync(active, "utf8"), `${requestID}\n`);
        writeFileSync(f.helper, original);
        assert.equal((await invoke(f, "reconcile", ["--outcome", "rollback", "--execute"])).code, 0);
      }
    } finally { await f.close(); }
  });
}

test("a pipeline surviving its admin parent keeps the actual common lock until the child exits", { skip: process.platform !== "linux" }, async () => {
  const f = await fixture();
  try {
    const flock = join(f.root, "flock"); copyFileSync("/usr/bin/flock", flock); chmodSync(flock, 0o700);
    writeFileSync(join(f.materials, "orchestration.lock"), "", { mode: 0o600 });
    let control = readFileSync(join(repositoryRoot, "services/deploy-webhook/deploy/henukit-materials-control"), "utf8");
    const quote = value => `'${value.replaceAll("'", "'\\''")}'`;
    control = control.replace('readonly materials_root="/opt/henukit-materials"', `readonly materials_root=${quote(f.materials)}`)
      .replace('readonly admin_helper="/usr/local/libexec/henukit/resync-henukit-materials.mjs"', `readonly admin_helper=${quote(f.helper)}`)
      .replace('readonly flock_bin="/usr/bin/flock"', `readonly flock_bin=${quote(flock)}`)
      .replace('[[ "$(/usr/bin/id -u)" == 0 ]]', `[[ "$(/usr/bin/id -u)" == ${process.getuid()} ]]`)
      .replace('[[ "${owner_mode%%:*}" == 0 ]]', `[[ "\${owner_mode%%:*}" == ${process.getuid()} ]]`)
      .replace('[[ "$cursor" != / ]]', `[[ "$cursor" != ${quote(f.root)} ]]`);
    const controlPath = join(f.root, "control"); writeFileSync(controlPath, control, { mode: 0o700 });
    const finished = join(f.root, "pipeline-finished");
    command(f.orchestrate, `process.kill(process.ppid,'SIGKILL');setTimeout(()=>require('node:fs').writeFileSync(${JSON.stringify(finished)},'done'),2500);`);
    const child = spawn(controlPath, ["resync", "--sha", sha, "--manifest-sha256", manifestSHA, "--request-id", requestID, "--operator", "test", "--reason", "parent death", "--execute"]);
    child.stdout.resume(); child.stderr.resume();
    await new Promise((resolve, reject) => { child.on("exit", resolve); child.on("error", reject); });
    const blocked = await new Promise((resolve, reject) => {
      const p = spawn(controlPath, ["reconcile", "--request-id", requestID, "--operator", "test", "--reason", "reconcile", "--outcome", "rollback", "--execute"]);
      let stderr="";p.stderr.on("data",b=>stderr+=b);p.stdout.resume();p.on("error",reject);p.on("close",code=>resolve({code,stderr}));
    });
    assert.notEqual(blocked.code, 0); assert.match(blocked.stderr, /pipeline lock/);
    assert.equal(existsSync(f.calls), false);
    for (let i=0;i<150 && !existsSync(finished);i++) await new Promise(resolve=>setTimeout(resolve,20));
    assert.equal(existsSync(finished), true);
    const resolved = await invoke(f, "reconcile", ["--outcome", "rollback", "--execute"]);
    assert.equal(resolved.code, 0, resolved.stderr);
  } finally { await f.close(); }
});

test("a crash after atomic resolution publication converges without a second activation", async () => {
  const f = await fixture();
  try {
    command(f.orchestrate, "process.exit(9);");
    assert.notEqual((await invoke(f, "resync", ["--execute"])).code, 0);
    const original = readFileSync(f.helper, "utf8");
    const fault = "import faultFS from 'node:fs';import {syncBuiltinESMExports} from 'node:module';const realLink=faultFS.linkSync;faultFS.linkSync=(from,to)=>{realLink(from,to);if(to.endsWith('/resolution.json'))process.kill(process.pid,'SIGKILL');};syncBuiltinESMExports();\n";
    writeFileSync(f.helper, fault + original.replace(/^#![^\n]*\n/, ""));
    const killed = await invoke(f, "reconcile", ["--outcome", "rollback", "--execute"]);
    assert.equal(killed.code, null, killed.stderr);
    const calls = readFileSync(f.calls, "utf8");
    assert.equal(JSON.parse(readFileSync(join(f.materials, "admin-resync", requestID, "resolution.json"))).state, "resolved");
    writeFileSync(f.helper, original);
    const r = await invoke(f, "reconcile", ["--outcome", "rollback", "--execute"]);
    assert.equal(r.code, 0, r.stderr); assert.equal(readFileSync(f.calls, "utf8"), calls);
    assert.equal(existsSync(join(f.materials, "admin-resync/active")), false);
  } finally { await f.close(); }
});
