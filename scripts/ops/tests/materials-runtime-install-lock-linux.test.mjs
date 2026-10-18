import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repo = join(dirname(fileURLToPath(import.meta.url)), "../../..");
const enabled = process.platform === "linux" && process.getuid() === 0;
function fixture() {
  const root = mkdtempSync(join(tmpdir(), "materials-install-lock-"));
  const materials = join(root, "materials"), payload = join(root, "release/materials-runtime"), bin = join(root, "bin");
  for (const path of [materials, payload, bin]) mkdirSync(path, { recursive: true, mode: 0o700 });
  const sha = "a".repeat(40); writeFileSync(join(root, "release/RELEASE_SHA"), `${sha}\n`);
  const required = ["bin/henukit-deploy-webhook", "bin/materials-oss-canary", "bin/materials-oss-release", "bin/library-activate-public-release", "libexec/henukit-materials-orchestrate", "libexec/henukit-materials-control", "libexec/resync-henukit-materials.mjs", "libexec/henukit-materials-prepare", "libexec/henukit-materials-seal", "libexec/henukit-materials-activate", "libexec/henukit-materials-publish-oss", "libexec/henukit-materials-publish-release-oss", "libexec/prepare-henukit-materials.mjs", "libexec/seal-henukit-materials.mjs", "libexec/activate-henukit-materials.mjs", "libexec/build-henukit-library-activation-bundle.mjs", "systemd/henukit-materials-webhook.service", "systemd/henukit-materials-webhook.path", "systemd/henukit-materials-runner.service"];
  for (const path of required) { mkdirSync(dirname(join(payload, path)), { recursive: true }); writeFileSync(join(payload, path), "fixture payload\n"); }
  const original = readFileSync(join(repo, "services/deploy-webhook/deploy/install-materials-runtime.sh"), "utf8");
  const text = original.replace("readonly materials_root=/opt/henukit-materials", `readonly materials_root='${materials}'`).replace('[[ "$lock_parent" != / ]]', `[[ "$lock_parent" != '${root}' ]]`);
  writeFileSync(join(payload, "install.sh"), text, { mode: 0o700 });
  writeFileSync(join(payload, "SHA256SUMS"), [...required, "install.sh"].map(path => `${createHash("sha256").update(readFileSync(join(payload, path))).digest("hex")}  ${path}\n`).sort().join(""));
  const mutations = join(root, "mutations");
  writeFileSync(join(bin, "systemctl"), `#!/bin/sh\nprintf called >> '${mutations}'\nexit 1\n`, { mode: 0o700 });
  // Architecture is a test fixture; no binaries are installed or executed.
  writeFileSync(join(bin, "uname"), "#!/bin/sh\nif [ \"$1\" = -m ]; then echo x86_64; else echo Linux; fi\n", { mode: 0o700 });
  writeFileSync(join(materials, "orchestration.lock"), "", { mode: 0o600 });
  const invoke = () => spawnSync("bash", [join(payload, "install.sh"), "--release-sha", sha], { env: { ...process.env, PATH: `${bin}:${process.env.PATH}` }, encoding: "utf8", timeout: 10000 });
  return { root, materials, mutations, invoke };
}

test("runtime installer refuses an unresolved administrative request before service or file mutation", { skip: !enabled }, () => {
  const f = fixture();
  try {
    mkdirSync(join(f.materials, "admin-resync"), { mode: 0o700 }); writeFileSync(join(f.materials, "admin-resync/active"), "pending\n");
    const r = f.invoke(); assert.notEqual(r.status, 0); assert.match(r.stderr, /request unresolved/);
    assert.equal(existsSync(f.mutations), false);
  } finally { rmSync(f.root, { recursive: true, force: true }); }
});

test("runtime installer excludes a live materials operation before its first mutation", { skip: !enabled }, async () => {
  const f = fixture(); let holder;
  try {
    const held = join(f.root, "held");
    holder = spawn("/usr/bin/flock", [join(f.materials, "orchestration.lock"), process.execPath, "-e", `require('node:fs').writeFileSync(${JSON.stringify(held)},'held');setTimeout(()=>{},2500);`]);
    for(let i=0;i<100 && !existsSync(held);i++) await new Promise(resolve=>setTimeout(resolve,20));
    assert.equal(existsSync(held), true);
    const r = f.invoke(); assert.notEqual(r.status, 0); assert.match(r.stderr, /pipeline lock is held/);
    assert.equal(existsSync(f.mutations), false);
  } finally { holder?.kill(); rmSync(f.root, { recursive: true, force: true }); }
});
