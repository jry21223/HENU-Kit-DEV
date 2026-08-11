import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  chmodSync,
  cpSync,
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
const bundleBuilder = join(repoRoot, "scripts/ops/build-henukit-library-activation-bundle.mjs");

const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
const canonicalJSON = (value) => Buffer.from(`${JSON.stringify(value, null, 2)}\n`, "utf8");

function writeExecutable(path, body) {
  writeFileSync(path, body, { mode: 0o755 });
  chmodSync(path, 0o755);
}

function createFixture({ psqlExit = 0, libraryExit = 0, sourceCharacter = "a", material = "reviewed material\n", emptyReviewed = false, reviewedCount = 1, activeMarker = true } = {}) {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-activate-"));
  const sealedRoot = join(root, "sealed");
  const publicRoot = join(root, "served");
  const ossAuditRoot = join(root, "oss-audit");
  const activationStagingRoot = join(root, "activation-staging");
  const previousRelease = join(publicRoot, "releases", "previous", "public");
  const previousReleaseID = `${"b".repeat(40)}-${"c".repeat(16)}`;
  const sourceSha = sourceCharacter.repeat(40);
  const publicPath = "软件工程/复习讲义.pdf";
  const assets = (emptyReviewed ? [] : Array.from({ length: reviewedCount }, (_, index) => {
    const path = index === 0 ? publicPath : `软件工程/复习讲义-${index}.pdf`;
    const bytes = Buffer.from(index === 0 ? material : `reviewed material ${index}\n`, "utf8");
    return { path, bytes, sha256: digest(bytes) };
  })).sort((left, right) => Buffer.compare(Buffer.from(left.path, "utf8"), Buffer.from(right.path, "utf8")));
  const manifest = canonicalJSON({
    subjects: [
      {
        name: "软件工程",
        assets: emptyReviewed ? [{
          title: "软件工程_复习讲义",
          role: "待复核讲义",
          publicPath,
          bytes: Buffer.byteLength(material),
          sha256: digest(Buffer.from(material, "utf8")),
        }] : assets.map((asset, index) => ({
          title: index === 0 ? "软件工程_复习讲义" : `软件工程_复习讲义_${index}`,
          role: "复习讲义",
          publicPath: asset.path,
          bytes: asset.bytes.length,
          sha256: asset.sha256,
        })),
      },
    ],
  });
  const manifestSha256 = digest(manifest);
  const releaseID = `${sourceSha}-${manifestSha256.slice(0, 16)}`;
  const release = join(sealedRoot, releaseID);
  const treeEntries = assets.map((asset) => ({ path: `public/${asset.path}`, bytes: asset.bytes.length, sha256: asset.sha256 }));
  const inventoryAssets = assets.map((asset) => ({ public_path: asset.path, bytes: asset.bytes.length, sha256: asset.sha256 }));
  const inventory = canonicalJSON({
    version: 1,
    source: {
      repository: "https://github.com/jry21223/HENU-Final-Review.git",
      ref: "refs/heads/main",
      sha: sourceSha,
    },
    manifest_sha256: manifestSha256,
    assets: inventoryAssets,
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
    reviewed_assets: inventoryAssets.length,
    slides: { status: "deferred", source_slide_assets: 0 },
  });
  const receiptSha256 = digest(receipt);

  mkdirSync(join(release, "public"), { recursive: true });
  if (!emptyReviewed) {
    for (const asset of assets) {
      mkdirSync(join(release, "public", dirname(asset.path)), { recursive: true });
      writeFileSync(join(release, "public", asset.path), asset.bytes);
    }
  }
  writeFileSync(join(release, "manifest.json"), manifest);
  writeFileSync(join(release, "inventory.json"), inventory);
  writeFileSync(join(release, "sealed-release.json"), receipt);
  mkdirSync(join(ossAuditRoot, releaseID), { recursive: true, mode: 0o700 });
  mkdirSync(activationStagingRoot, { mode: 0o700 });
  writeFileSync(join(ossAuditRoot, releaseID, "release-commit.json"), JSON.stringify({
    version: 1,
    state: "release_committed_not_activated",
    release_id: releaseID,
    receipt_sha256: receiptSha256,
    manifest_sha256: manifestSha256,
    inventory_sha256: digest(inventory),
    tree_sha256: digest(canonicalJSON(treeEntries)),
    asset_count: inventoryAssets.length,
    assets: assets.map((asset, index) => ({
      public_path: asset.path,
      sha256: asset.sha256,
      bytes: asset.bytes.length,
      object_key: `releases/${releaseID}/receipts/${receiptSha256}/objects/${asset.sha256}/${asset.path}`,
      object_version_id: `version-${index + 1}`,
    })),
  }), { mode: 0o400 });
  mkdirSync(previousRelease, { recursive: true });
  writeFileSync(join(previousRelease, "old.pdf"), "previous material\n");
  symlinkSync(relative(publicRoot, previousRelease), join(publicRoot, "current"));
  if (activeMarker) writeFileSync(join(publicRoot, "ACTIVE_RELEASE"), `${previousReleaseID}\n`);

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
  const libraryActivator = join(root, "library-activate-public-release");
  const libraryLog = join(root, "library-activation.json");
  const libraryState = join(root, "library-active-release");
  writeExecutable(libraryActivator, `#!${process.execPath}
import { existsSync, readFileSync, writeFileSync } from "node:fs";
const bundlePath = process.argv[process.argv.indexOf("--bundle") + 1];
const bundle = JSON.parse(readFileSync(bundlePath, "utf8"));
const release = bundle.release_id;
const previous = existsSync(${JSON.stringify(libraryState)}) ? readFileSync(${JSON.stringify(libraryState)}, "utf8").trim() : ${JSON.stringify(activeMarker ? previousReleaseID : "")};
const replayed = previous === release;
writeFileSync(${JSON.stringify(libraryState)}, release + "\\n");
writeFileSync(${JSON.stringify(libraryLog)}, JSON.stringify({ release, previous, replayed }) + "\\n");
${libraryExit === 0 ? `process.stdout.write(JSON.stringify({ release_id: release, previous_release_id: replayed ? "" : previous, material_count: bundle.objects.length, replayed }) + "\\n");` : `process.exit(${libraryExit});`}
`);

  return {
    root,
    sealedRoot,
    publicRoot,
    previousRelease,
    previousReleaseID,
    releaseID,
    receiptSha256,
    converter,
    psql,
    psqlLog,
    psqlArgsLog,
    psqlDatabaseLog,
    pgServiceFile,
    legacyInventory,
    ossAuditRoot,
    activationStagingRoot,
    libraryActivator,
    libraryLog,
    libraryState,
    publicPath,
  };
}

function libraryActivationOptions(fixture) {
  return [
    "--oss-audit-root", fixture.ossAuditRoot,
    "--activation-staging-root", fixture.activationStagingRoot,
    "--bundle-builder", bundleBuilder,
    "--library-activator", fixture.libraryActivator,
  ];
}

function activationEnvironment(fixture) {
  return { ...process.env, PGSERVICEFILE: fixture.pgServiceFile, PGSERVICE: "materials", LIBRARY_DATABASE_URL: "postgres://fixed.invalid/library", LIBRARY_OSS_ECS_RAM_ROLE: "henukit-library-activation" };
}

function activationArgs(fixture, releaseID = fixture.releaseID, receiptSHA256 = fixture.receiptSha256) {
  return [
    activateScript,
    "--release-id", releaseID,
    "--receipt-sha256", receiptSHA256,
    "--sealed-root", fixture.sealedRoot,
    "--public-root", fixture.publicRoot,
    "--converter", fixture.converter,
    "--importer", importScript,
    "--psql", fixture.psql,
    "--legacy-inventory", fixture.legacyInventory,
    ...libraryActivationOptions(fixture),
    "--activation-owner", String(process.getuid()),
  ];
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
      ...libraryActivationOptions(fixture),
      "--activation-owner",
      String(process.getuid()),
    ],
    { cwd: repoRoot, encoding: "utf8", env: activationEnvironment(fixture) },
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

test("activates the first complete release when no prior active marker exists", () => {
  const fixture = createFixture({ activeMarker: false });

  execFileSync(process.execPath, activationArgs(fixture), {
    cwd: repoRoot,
    encoding: "utf8",
    env: activationEnvironment(fixture),
  });

  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.releaseID}\n`);
  assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), false);
  assert.equal(existsSync(join(fixture.publicRoot, "activation-journal.json")), false);
  assert.match(readFileSync(fixture.psqlLog, "utf8"), /BEGIN;[\s\S]*COMMIT;/);
});

test("rollback is a forward activation of a retained complete OSS and derived release", () => {
  const fixture = createFixture();
  const next = createFixture({ sourceCharacter: "d", material: "next reviewed material\n" });
  cpSync(join(next.sealedRoot, next.releaseID), join(fixture.sealedRoot, next.releaseID), { recursive: true });
  cpSync(join(next.ossAuditRoot, next.releaseID), join(fixture.ossAuditRoot, next.releaseID), { recursive: true });
  const options = { cwd: repoRoot, encoding: "utf8", env: activationEnvironment(fixture) };

  execFileSync(process.execPath, activationArgs(fixture), options);
  execFileSync(process.execPath, activationArgs(fixture, next.releaseID, next.receiptSha256), options);
  execFileSync(process.execPath, activationArgs(fixture), options);

  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.releaseID}\n`);
  assert.equal(resolve(fixture.publicRoot, readlinkSync(join(fixture.publicRoot, "current"))), join(fixture.publicRoot, "releases", fixture.releaseID, "public"));
  assert.equal(readFileSync(fixture.libraryState, "utf8"), `${fixture.releaseID}\n`);
  assert.equal(existsSync(join(fixture.publicRoot, "releases", next.releaseID)), true, "forward rollback deleted the newer retained release");
  assert.equal(existsSync(join(fixture.ossAuditRoot, next.releaseID, "release-commit.json")), true, "forward rollback deleted OSS audit evidence");
});

test("activates a complete empty reviewed release with a zero-material Library snapshot", () => {
  const fixture = createFixture({ emptyReviewed: true });
  execFileSync(process.execPath, activationArgs(fixture), { cwd: repoRoot, encoding: "utf8", env: activationEnvironment(fixture) });
  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.releaseID}\n`);
  assert.equal(JSON.parse(readFileSync(fixture.libraryLog, "utf8")).release, fixture.releaseID);
  const bundle = JSON.parse(readFileSync(join(fixture.activationStagingRoot, fixture.releaseID, `${fixture.receiptSha256}.json`), "utf8"));
  assert.deepEqual(bundle.objects, []);
});

test("rejects 501 reviewed materials before maintenance, static switching, journaling, or Library activation", () => {
  const fixture = createFixture({ reviewedCount: 501 });
  const beforeCurrent = readlinkSync(join(fixture.publicRoot, "current"));

  assert.throws(
    () => execFileSync(process.execPath, activationArgs(fixture), {
      cwd: repoRoot,
      encoding: "utf8",
      env: activationEnvironment(fixture),
      stdio: "pipe",
    }),
    /at most 500 reviewed materials/,
  );

  assert.equal(readlinkSync(join(fixture.publicRoot, "current")), beforeCurrent);
  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.previousReleaseID}\n`);
  assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), false);
  assert.equal(existsSync(join(fixture.publicRoot, "activation-journal.json")), false);
  assert.equal(existsSync(fixture.libraryLog), false);
  assert.equal(existsSync(fixture.psqlLog), false);
});

for (const scenario of [
  {
    name: "a non-replayed Library activation whose previous release drifted",
    response: (fixture) => ({
      release_id: fixture.releaseID,
      previous_release_id: `${"d".repeat(40)}-${"e".repeat(16)}`,
      material_count: 1,
      replayed: false,
    }),
  },
  {
    name: "a Library activation response with a partial material count",
    response: (fixture) => ({
      release_id: fixture.releaseID,
      previous_release_id: fixture.previousReleaseID,
      material_count: 0,
      replayed: false,
    }),
  },
]) {
  test(`keeps the cross-surface activation fenced after ${scenario.name}`, () => {
    const fixture = createFixture();
    writeExecutable(fixture.libraryActivator, `#!/bin/sh\nset -eu\nprintf '%s\\n' '${JSON.stringify(scenario.response(fixture))}'\n`);

    assert.throws(
      () => execFileSync(process.execPath, activationArgs(fixture), {
        cwd: repoRoot,
        encoding: "utf8",
        env: activationEnvironment(fixture),
        stdio: "pipe",
      }),
      /Library activation response is invalid/,
    );

    const journal = JSON.parse(readFileSync(join(fixture.publicRoot, "activation-journal.json"), "utf8"));
    assert.equal(journal.phase, "library_running");
    assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), true);
    assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.previousReleaseID}\n`);
    assert.equal(existsSync(fixture.psqlLog), false);
  });
}

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
    ...libraryActivationOptions(fixture),
    "--activation-owner", String(process.getuid()),
  ];
  const execOptions = { cwd: repoRoot, encoding: "utf8", env: activationEnvironment(fixture) };

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

test("Library commit-then-exit remains fenced and same-release replay recovers without phase regression", () => {
  const fixture = createFixture({ libraryExit: 37 });
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
    ...libraryActivationOptions(fixture),
    "--activation-owner", String(process.getuid()),
  ];
  assert.throws(() => execFileSync(process.execPath, args, { cwd: repoRoot, encoding: "utf8", stdio: "pipe", env: activationEnvironment(fixture) }));
  assert.equal(JSON.parse(readFileSync(fixture.libraryLog, "utf8")).release, fixture.releaseID);
  assert.equal(resolve(fixture.publicRoot, readlinkSync(join(fixture.publicRoot, "current"))), join(fixture.publicRoot, "releases", fixture.releaseID, "public"));
  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.previousReleaseID}\n`);
  assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), true);
  const journalPath = join(fixture.publicRoot, "activation-journal.json");
  assert.equal(JSON.parse(readFileSync(journalPath, "utf8")).phase, "library_running");
  assert.equal(existsSync(fixture.psqlLog), false);

  assert.throws(() => execFileSync(process.execPath, args, { cwd: repoRoot, encoding: "utf8", stdio: "pipe", env: activationEnvironment(fixture) }));
  assert.equal(JSON.parse(readFileSync(journalPath, "utf8")).phase, "library_running", "restart regressed the uncertain phase");

  writeExecutable(fixture.libraryActivator, `#!/bin/sh\nset -eu\nprintf '%s\\n' '${JSON.stringify({ release_id: fixture.releaseID, previous_release_id: "", material_count: 1, replayed: true })}'\n`);
  execFileSync(process.execPath, args, { cwd: repoRoot, encoding: "utf8", env: activationEnvironment(fixture) });
  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.releaseID}\n`);
  assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), false);
  assert.equal(existsSync(journalPath), false);
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
    ...libraryActivationOptions(fixture),
    "--activation-owner", String(process.getuid()),
  ];
  const execOptions = { cwd: repoRoot, encoding: "utf8", env: activationEnvironment(fixture) };
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
    ...libraryActivationOptions(fixture),
    "--activation-owner", String(process.getuid()),
  ], { cwd: repoRoot, encoding: "utf8", stdio: "pipe", env: activationEnvironment(fixture) }));

  assert.equal(existsSync(join(fixture.publicRoot, ".maintenance")), false);
  assert.equal(
    resolve(fixture.publicRoot, readlinkSync(join(fixture.publicRoot, "current"))),
    fixture.previousRelease,
  );
  assert.equal(readFileSync(join(fixture.publicRoot, "ACTIVE_RELEASE"), "utf8"), `${fixture.previousReleaseID}\n`);
  rmSync(installed);
});
