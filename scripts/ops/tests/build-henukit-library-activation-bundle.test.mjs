import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";

const script = new URL("../build-henukit-library-activation-bundle.mjs", import.meta.url).pathname;
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const canonical = (value) => Buffer.from(`${JSON.stringify(value, null, 2)}\n`);

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "henukit-library-bundle-"));
  const sealed = join(root, "sealed");
  const installed = join(root, "installed");
  const audit = join(root, "audit");
  mkdirSync(join(installed, "slides"), { recursive: true });
  mkdirSync(sealed, { recursive: true });
  mkdirSync(audit, { recursive: true });
  const sourceSHA = "1".repeat(40);
  const assetBody = Buffer.from("public material\n");
  const assetSHA = hash(assetBody);
  const manifest = Buffer.from(JSON.stringify({ version: 1, subjects: [{ name: "软件工程", assets: [{ role: "复习讲义", title: "期末复习", publicPath: "软件工程/讲义.pdf", bytes: assetBody.length, sha256: assetSHA }] }] }));
  const releaseID = `${sourceSHA}-${hash(manifest).slice(0, 16)}`;
  const receipt = Buffer.from(JSON.stringify({ version: 1, release_id: releaseID, manifest_sha256: hash(manifest), inventory_sha256: "2".repeat(64), tree_sha256: "3".repeat(64), reviewed_assets: 1, slides: { status: "disabled", source_slide_assets: 0 } }));
  const receiptSHA = hash(receipt);
  const derivedAssets = [];
  const derived = canonical({ version: 1, release_id: releaseID, assets: derivedAssets });
  const key = `releases/${releaseID}/receipts/${receiptSHA}/objects/${assetSHA}/软件工程/讲义.pdf`;
  const commit = Buffer.from(JSON.stringify({ version: 1, state: "release_committed_not_activated", release_id: releaseID, receipt_sha256: receiptSHA, manifest_sha256: hash(manifest), inventory_sha256: "2".repeat(64), tree_sha256: "3".repeat(64), asset_count: 1, assets: [{ public_path: "软件工程/讲义.pdf", sha256: assetSHA, bytes: assetBody.length, object_key: key, object_version_id: "version-1" }] }));
  for (const [path, bytes] of [[join(sealed, "manifest.json"), manifest], [join(sealed, "sealed-release.json"), receipt], [join(installed, "derived-inventory.json"), derived], [join(audit, "release-commit.json"), commit]]) {
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, bytes, { mode: 0o400 });
    chmodSync(path, 0o400);
  }
  return { root, sealed, installed, audit, releaseID, receiptSHA, commit, manifest, receipt, derivedAssets, key };
}

function run(f, output = join(f.root, "bundle.json")) {
  return { output, result: spawnSync(process.execPath, [script, "--release-id", f.releaseID, "--receipt-sha256", f.receiptSHA, "--oss-commit", join(f.audit, "release-commit.json"), "--sealed-release", join(f.sealed, "sealed-release.json"), "--installed-release", f.installed, "--output", output], { encoding: "utf8" }) };
}

test("builds one exact activation package from OSS commit, sealed bytes, and derived release", () => {
  const f = fixture();
  const { result, output } = run(f);
  assert.equal(result.status, 0, result.stderr);
  const bundle = JSON.parse(readFileSync(output, "utf8"));
  assert.equal(bundle.release_id, f.releaseID);
  assert.deepEqual(Buffer.from(bundle.manifest_json, "base64"), f.manifest);
  assert.deepEqual(Buffer.from(bundle.sealed_receipt_json, "base64"), f.receipt);
  assert.deepEqual(Buffer.from(bundle.release_commit_json, "base64"), f.commit);
  assert.equal(bundle.derived.release_id, f.releaseID);
  assert.equal(bundle.derived.slides_sha256, hash(canonical(f.derivedAssets)));
  assert.equal(bundle.derived.index_sha256, hash(readFileSync(join(f.installed, "derived-inventory.json"))));
  assert.deepEqual(bundle.objects, [{ public_path: "软件工程/讲义.pdf", object_key: f.key, object_version_id: "version-1" }]);
  const evidence = JSON.parse(result.stdout);
  assert.equal(evidence.bundle_sha256, hash(readFileSync(output)));
  assert.equal(evidence.oss_commit_sha256, hash(f.commit));
});

test("mismatched or incomplete OSS identity fails without an activation package", () => {
  const f = fixture();
  const commitPath = join(f.audit, "release-commit.json");
  const commit = JSON.parse(readFileSync(commitPath, "utf8"));
  commit.assets[0].object_version_id = "";
  chmodSync(commitPath, 0o600);
  writeFileSync(commitPath, JSON.stringify(commit), { mode: 0o400 });
  chmodSync(commitPath, 0o400);
  const { result, output } = run(f);
  assert.notEqual(result.status, 0);
  assert.equal(existsSync(output), false);
  assert.equal(result.stdout, "");
});

test("unsafe OSS VersionId fails before an activation package is written", () => {
  for (const versionID of ["null", " version-1", "version-1 ", "version\n1", "v".repeat(1025)]) {
    const f = fixture();
    const commitPath = join(f.audit, "release-commit.json");
    const commit = JSON.parse(readFileSync(commitPath, "utf8"));
    commit.assets[0].object_version_id = versionID;
    chmodSync(commitPath, 0o600);
    writeFileSync(commitPath, JSON.stringify(commit), { mode: 0o400 });
    chmodSync(commitPath, 0o400);
    const { result, output } = run(f);
    assert.notEqual(result.status, 0, `accepted ${JSON.stringify(versionID)}`);
    assert.equal(existsSync(output), false);
  }
});

test("derived release drift fails without an activation package", () => {
  const f = fixture();
  const slide = join(f.installed, "slides", "软件工程", "1.svg");
  mkdirSync(dirname(slide), { recursive: true });
  writeFileSync(slide, "drift", { mode: 0o400 });
  const { result, output } = run(f);
  assert.notEqual(result.status, 0);
  assert.equal(existsSync(output), false);
});

test("non-canonical empty derived inventory fails without an activation package", () => {
  const f = fixture();
  const inventoryPath = join(f.installed, "derived-inventory.json");
  chmodSync(inventoryPath, 0o600);
  writeFileSync(
    inventoryPath,
    canonical({ version: 1, release_id: f.releaseID, assets: [], unexpected: true }),
    { mode: 0o400 },
  );
  chmodSync(inventoryPath, 0o400);
  const { result, output } = run(f);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /canonical empty inventory/);
  assert.equal(existsSync(output), false);
});

test("derived online-preview assets fail when preview is disabled", () => {
  const f = fixture();
  const slide = Buffer.from("slide\n");
  const slidePath = join(f.installed, "slides", "软件工程", "1.svg");
  mkdirSync(dirname(slidePath), { recursive: true });
  writeFileSync(slidePath, slide, { mode: 0o400 });
  const inventoryPath = join(f.installed, "derived-inventory.json");
  chmodSync(inventoryPath, 0o600);
  writeFileSync(inventoryPath, canonical({ version: 1, release_id: f.releaseID, assets: [{ path: "软件工程/1.svg", bytes: slide.length, sha256: hash(slide) }] }), { mode: 0o400 });
  chmodSync(inventoryPath, 0o400);

  const { result, output } = run(f);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /online preview is disabled/);
  assert.equal(existsSync(output), false);
});
