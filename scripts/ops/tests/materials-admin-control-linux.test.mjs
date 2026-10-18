import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repo = join(dirname(fileURLToPath(import.meta.url)), "../../..");
const quote = value => `'${value.replaceAll("'", "'\\''")}'`;
function fixture() {
  const root = mkdtempSync(join(tmpdir(), "materials-control-"));
  const materials = join(root, "materials"); mkdirSync(materials, { mode: 0o700 });
  writeFileSync(join(materials, "orchestration.lock"), "", { mode: 0o600 });
  const consumer = join(root, "consumer"), admin = join(root, "admin.mjs"), flock = join(root, "flock");
  copyFileSync("/usr/bin/flock", flock); chmodSync(flock, 0o700);
  writeFileSync(consumer, `#!${process.execPath}\nrequire('node:fs').writeFileSync(${JSON.stringify(join(root, "claimed"))},'claimed');setTimeout(()=>{},1500);`, { mode: 0o700 });
  writeFileSync(admin, `import fs from 'node:fs';fs.writeFileSync(${JSON.stringify(join(root, "admin-started"))},'started');setTimeout(()=>{},1500);`, { mode: 0o600 });
  let text = readFileSync(join(repo, "services/deploy-webhook/deploy/henukit-materials-control"), "utf8");
  text = text.replace('readonly materials_root="/opt/henukit-materials"', `readonly materials_root=${quote(materials)}`)
    .replace('readonly queue_consumer="/usr/local/bin/henukit-deploy-webhook"', `readonly queue_consumer=${quote(consumer)}`)
    .replace('readonly admin_helper="/usr/local/libexec/henukit/resync-henukit-materials.mjs"', `readonly admin_helper=${quote(admin)}`)
    .replace('readonly flock_bin="/usr/bin/flock"', `readonly flock_bin=${quote(flock)}`)
    .replace('[[ "$(/usr/bin/id -u)" == 0 ]]', `[[ "$(/usr/bin/id -u)" == ${process.getuid()} ]]`)
    .replace('[[ "${owner_mode%%:*}" == 0 ]]', `[[ "\${owner_mode%%:*}" == ${process.getuid()} ]]`)
    .replace('[[ "$cursor" != / ]]', `[[ "$cursor" != ${quote(root)} ]]`);
  const control = join(root, "control"); writeFileSync(control, text, { mode: 0o700 });
  return { root, materials, control };
}
function start(f, mode) {
  const child = spawn(f.control, [mode], { env: { PATH: "/usr/bin:/bin" } });
  let stderr = ""; child.stderr.on("data", b => stderr += b); child.stdout.resume();
  const result = new Promise((resolve, reject) => { child.on("error", reject); child.on("close", code => resolve({ code, stderr })); });
  return { child, result };
}
async function waitFor(path) {
  for (let i = 0; i < 100; i++) { if (existsSync(path)) return; await new Promise(resolve => setTimeout(resolve, 20)); }
  throw new Error(`process did not reach boundary: ${path}`);
}

test("pending admin request rejects normal consumer before it can claim a real delivery", { skip: process.platform !== "linux" }, async () => {
  const f = fixture();
  try {
    mkdirSync(join(f.materials, "admin-resync"), { mode: 0o700 });
    writeFileSync(join(f.materials, "admin-resync/active"), "pending\n");
    const r = await start(f, "run").result;
    assert.notEqual(r.code, 0); assert.match(r.stderr, /request unresolved/);
    assert.equal(existsSync(join(f.root, "claimed")), false);
  } finally { rmSync(f.root, { recursive: true, force: true }); }
});

for (const [firstMode, firstMarker, blockedMode] of [["run", "claimed", "resync"], ["resync", "admin-started", "run"]]) {
  test(`${firstMode} holds the same process lock that excludes ${blockedMode}`, { skip: process.platform !== "linux" }, async () => {
    const f = fixture(); let first;
    try {
      first = start(f, firstMode); await waitFor(join(f.root, firstMarker));
      const r = await start(f, blockedMode).result;
      assert.notEqual(r.code, 0); assert.match(r.stderr, /pipeline lock/);
      if (blockedMode === "run") assert.equal(existsSync(join(f.root, "claimed")), false);
      assert.equal((await first.result).code, 0);
    } finally { first?.child.kill(); rmSync(f.root, { recursive: true, force: true }); }
  });
}
