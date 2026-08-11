import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readlinkSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const activateScript = join(repoRoot, "scripts/ops/activate-henukit-materials.mjs");
const importScript = join(repoRoot, "scripts/ops/import-henukit-materials.mjs");

const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
const canonicalJSON = (value) => Buffer.from(`${JSON.stringify(value, null, 2)}\n`, "utf8");

function writeExecutable(path, body) {
  writeFileSync(path, body, { mode: 0o755 });
  chmodSync(path, 0o755);
}

function createFixture({ psqlExit = 0 } = {}) {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-activate-"));
  const sealedRoot = join(root, "sealed");
  const publicRoot = join(root, "served");
  const previousRelease = join(publicRoot, "releases", "previous", "public");
  const previousReleaseID = `${"b".repeat(40)}-${"c".repeat(16)}`;
  const sourceSha = "a".repeat(40);
  const publicPath = "软件工程/复习讲义.pdf";
  const asset = Buffer.from("reviewed material\n", "utf8");
  const manifest = canonicalJSON({
    subjects: [
      {
        name: "软件工程",
        assets: [
          {
            title: "软件工程_复习讲义",
            role: "复习讲义",
            publicPath,
            bytes: asset.length,
            sha256: digest(asset),
          },
        ],
      },
    ],
  });
  const manifestSha256 = digest(manifest);
  const releaseID = `${sourceSha}-${manifestSha256.slice(0, 16)}`;
  const release = join(sealedRoot, releaseID);
  const treeEntries = [{ path: `public/${publicPath}`, bytes: asset.length, sha256: digest(asset) }];
  const inventory = canonicalJSON({
    version: 1,
    source: {
      repository: "https://github.com/jry21223/HENU-Final-Review.git",
      ref: "refs/heads/main",
      sha: sourceSha,
    },
    manifest_sha256: manifestSha256,
    assets: [{ public_path: publicPath, bytes: asset.length, sha256: digest(asset) }],
    slides: { status: "deferred", source_slide_assets: 0 },
    tree_sha256: digest(canonicalJSON(treeEntries)),
  });
  const receipt = canonicalJSON({
    version: 1,
    release_id: releaseID,
    source: {
      repository: "https://github.com/jry21223/HENU-Final-Review.git",
      ref: "refs/heads/main",
      sha: sourceSha,
    },
    manifest_sha256: manifestSha256,
    inventory_sha256: digest(inventory),
    tree_sha256: digest(canonicalJSON(treeEntries)),
    reviewed_assets: 1,
    slides: { status: "deferred", source_slide_assets: 0 },
  });

  mkdirSync(join(release, "public", dirname(publicPath)), { recursive: true });
  writeFileSync(join(release, "public", publicPath), asset);
  writeFileSync(join(release, "manifest.json"), manifest);
  writeFileSync(join(release, "inventory.json"), inventory);
  writeFileSync(join(release, "sealed-release.json"), receipt);
  mkdirSync(previousRelease, { recursive: true });
  writeFileSync(join(previousRelease, "old.pdf"), "previous material\n");
  symlinkSync(relative(publicRoot, previousRelease), join(publicRoot, "current"));
  writeFileSync(join(publicRoot, "ACTIVE_RELEASE"), `${previousReleaseID}\n`);

  const converter = join(root, "convert-slides");
  writeExecutable(converter, "#!/bin/sh\nset -eu\nmkdir -p \"$4\"\n");
  const psqlLog = join(root, "psql.sql");
  const psqlArgsLog = join(root, "psql.args");
  const psqlDatabaseLog = join(root, "psql.database");
  const pgServiceFile = join(root, "pg_service.conf");
  writeFileSync(pgServiceFile, "[materials]\nhost=trusted.example\ndbname=study\n", { mode: 0o600 });
  chmodSync(pgServiceFile, 0o600);
  const legacyInventory = join(root, "legacy-inventory.json");
  writeFileSync(legacyInventory, JSON.stringify({ version: 1, storage_keys: [] }), { mode: 0o600 });
  chmodSync(legacyInventory, 0o600);
  const psql = join(root, "psql");
  writeExecutable(psql, `#!/bin/sh\nset -eu\nprintf '%s\\n' "$@" > ${JSON.stringify(psqlArgsLog)}\nprintf '%s|%s' "\${PGSERVICEFILE:-}" "\${PGSERVICE:-}" > ${JSON.stringify(psqlDatabaseLog)}\ncat > ${JSON.stringify(psqlLog)}\nexit ${psqlExit}\n`);

  return {
    root,
    sealedRoot,
    publicRoot,
    previousRelease,
    previousReleaseID,
    releaseID,
    receiptSha256: digest(receipt),
    converter,
    psql,
    psqlLog,
    psqlArgsLog,
    psqlDatabaseLog,
    pgServiceFile,
    legacyInventory,
    publicPath,
  };
}

test("activates one sealed release and its catalog behind the public maintenance fence", () => {
  const fixture = createFixture();

  execFileSync(
    process.execPath,
    [
      activateScript,
      "--release-id",
      fixture.releaseID,
      "--receipt-sha256",
      fixture.receiptSha256,
      "--sealed-root",
      fixture.sealedRoot,
      "--public-root",
      fixture.publicRoot,
      "--converter",
      fixture.converter,
      "--importer",
      importScript,
      "--psql",
      fixture.psql,
      "--legacy-inventory",
      fixture.legacyInventory,
      "--activation-owner",
      String(process.getuid()),
    ],
    { cwd: repoRoot, encoding: "utf8", env: { ...process.env, PGSERVICEFILE: fixture.pgServiceFile, PGSERVICE: "materials" } },
  );

  const current = resolve(fixture.publicRoot, readlinkSync(join(fixture.publicRoot, "current")));
  assert.equal(current, join(fixture.publicRoot, "releases", fixture.releaseID, "public"));
  assert.equal(readFileSync(join(current, fixture.publicPath), "utf8"), "reviewed material\n");
  assert.deepEqual(
    JSON.parse(readFileSync(join(fixture.publicRoot, "releases", fixture.releaseID, "derived-inventory.json"), "utf8")),
    { version: 1, release_id: fixture.releaseID, assets: [] },
  );
  assert.match(readFileSync(fixture.psqlLog, "utf8"), /BEGIN;[\s\S]*COMMIT;/);
  assert.doesNotMatch(readFileSync(fixture.psqlArgsLog, "utf8"), /postgres:\/\//);
  assert.equal(readFileSync(fixture.psqlDatabaseLog, "utf8"), `${fixture.pgServiceFile}|materials`);
  assert.match(
    readFileSync(fixture.psqlLog, "utf8"),
    new RegExp(`releases/${fixture.releaseID}/软件工程/复习讲义\\.pdf`),
  );
  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.releaseID}\n`);
  assert.equal(readFileSync(join(fixture.previousRelease, "old.pdf"), "utf8"), "previous material\n");
  assert.throws(() => readFileSync(join(fixture.publicRoot, ".maintenance")), /ENOENT/);
});

test("keeps an uncertain database failure fenced and recovers the same release on retry", () => {
  const fixture = createFixture({ psqlExit: 29 });
  const args = [
    activateScript,
    "--release-id", fixture.releaseID,
    "--receipt-sha256", fixture.receiptSha256,
    "--sealed-root", fixture.sealedRoot,
    "--public-root", fixture.publicRoot,
    "--converter", fixture.converter,
    "--importer", importScript,
    "--psql", fixture.psql,
    "--legacy-inventory", fixture.legacyInventory,
    "--activation-owner", String(process.getuid()),
  ];
  const execOptions = { cwd: repoRoot, encoding: "utf8", env: { ...process.env, PGSERVICEFILE: fixture.pgServiceFile, PGSERVICE: "materials" } };

  assert.throws(() => execFileSync(process.execPath, args, { ...execOptions, stdio: "pipe" }));
  assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), true);
  assert.equal(existsSync(join(fixture.publicRoot, "activation-journal.json")), true);
  assert.equal(
    resolve(fixture.publicRoot, readlinkSync(join(fixture.publicRoot, "current"))),
    join(fixture.publicRoot, "releases", fixture.releaseID, "public"),
  );
  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.previousReleaseID}\n`);

  writeExecutable(fixture.psql, `#!/bin/sh\nset -eu\ncat > ${JSON.stringify(fixture.psqlLog)}\n`);
  execFileSync(process.execPath, args, execOptions);

  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.releaseID}\n`);
  assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), false);
  assert.equal(existsSync(join(fixture.publicRoot, "activation-journal.json")), false);
});

test("finishes a durable database-committed journal without running the catalog transaction again", () => {
  const fixture = createFixture({ psqlExit: 31 });
  const args = [
    activateScript,
    "--release-id", fixture.releaseID,
    "--receipt-sha256", fixture.receiptSha256,
    "--sealed-root", fixture.sealedRoot,
    "--public-root", fixture.publicRoot,
    "--converter", fixture.converter,
    "--importer", importScript,
    "--psql", fixture.psql,
    "--legacy-inventory", fixture.legacyInventory,
    "--activation-owner", String(process.getuid()),
  ];
  const execOptions = { cwd: repoRoot, encoding: "utf8", env: { ...process.env, PGSERVICEFILE: fixture.pgServiceFile, PGSERVICE: "materials" } };
  assert.throws(() => execFileSync(process.execPath, args, { ...execOptions, stdio: "pipe" }));
  const journalPath = join(fixture.publicRoot, "activation-journal.json");
  const journal = JSON.parse(readFileSync(journalPath, "utf8"));
  chmodSync(journalPath, 0o600);
  writeFileSync(journalPath, `${JSON.stringify({ ...journal, phase: "database_committed" }, null, 2)}\n`);
  chmodSync(journalPath, 0o400);
  writeExecutable(fixture.psql, "#!/bin/sh\nexit 47\n");

  execFileSync(process.execPath, args, execOptions);

  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.releaseID}\n`);
  assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), false);
  assert.equal(existsSync(journalPath), false);
});

test("rejects a pre-seeded release symlink without fencing or changing the active release", () => {
  const fixture = createFixture();
  const installed = join(fixture.publicRoot, "releases", fixture.releaseID);
  symlinkSync(join(fixture.sealedRoot, fixture.releaseID), installed);

  assert.throws(() => execFileSync(process.execPath, [
    activateScript,
    "--release-id", fixture.releaseID,
    "--receipt-sha256", fixture.receiptSha256,
    "--sealed-root", fixture.sealedRoot,
    "--public-root", fixture.publicRoot,
    "--converter", fixture.converter,
    "--importer", importScript,
    "--psql", fixture.psql,
    "--legacy-inventory", fixture.legacyInventory,
    "--activation-owner", String(process.getuid()),
  ], { cwd: repoRoot, encoding: "utf8", stdio: "pipe", env: { ...process.env, PGSERVICEFILE: fixture.pgServiceFile, PGSERVICE: "materials" } }));

  assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), false);
  assert.equal(
    resolve(fixture.publicRoot, readlinkSync(join(fixture.publicRoot, "current"))),
    fixture.previousRelease,
  );
  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.previousReleaseID}\n`);
  rmSync(installed);
});
