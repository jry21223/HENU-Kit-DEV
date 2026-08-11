#!/usr/bin/env node

import { createHash, randomUUID } from "node:crypto";
import {
  closeSync,
  constants,
  existsSync,
  fsyncSync,
  linkSync,
  lstatSync,
  openSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, join, posix, resolve, sep } from "node:path";

const RELEASE_PATTERN = /^[a-f0-9]{40}-[a-f0-9]{16}$/;
const HASH_PATTERN = /^[a-f0-9]{64}$/;
const MAX_METADATA_BYTES = 16 * 1024 * 1024;

function fail(message) {
  throw new Error(message);
}

function digest(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function canonicalJSON(value) {
  return Buffer.from(`${JSON.stringify(value, null, 2)}\n`, "utf8");
}

function parseOptions(argv) {
  const names = new Map([
    ["--release-id", "releaseID"],
    ["--receipt-sha256", "receiptSHA256"],
    ["--oss-commit", "ossCommit"],
    ["--sealed-release", "sealedRelease"],
    ["--installed-release", "installedRelease"],
    ["--output", "output"],
  ]);
  if (argv.length !== names.size * 2) fail("expected exactly six fixed bundle options");
  const options = {};
  for (let index = 0; index < argv.length; index += 2) {
    const key = names.get(argv[index]);
    const value = argv[index + 1];
    if (!key || !value || value.startsWith("--") || Object.hasOwn(options, key)) fail("invalid bundle option");
    options[key] = value;
  }
  if (!RELEASE_PATTERN.test(options.releaseID) || !HASH_PATTERN.test(options.receiptSHA256)) fail("release identity is invalid");
  for (const key of ["ossCommit", "sealedRelease", "installedRelease", "output"]) {
    if (!isAbsolute(options[key]) || resolve(options[key]) === sep || options[key].includes("\0")) fail(`${key} must be a non-root absolute path`);
  }
  return options;
}

function readRegular(path, label) {
  const metadata = lstatSync(path);
  if (metadata.isSymbolicLink() || !metadata.isFile() || (metadata.mode & 0o022) !== 0 || metadata.size > MAX_METADATA_BYTES) {
    fail(`${label} must be a bounded non-writable regular file`);
  }
  return readFileSync(path);
}

function parseJSON(bytes, label) {
  try {
    return JSON.parse(bytes.toString("utf8"));
  } catch {
    fail(`${label} is not valid JSON`);
  }
}

function validatePath(value) {
  if (typeof value !== "string" || !value || value.includes("\\") || value.includes("\0") || posix.isAbsolute(value)) fail("public path is unsafe");
  const parts = value.split("/");
  if (parts.some((part) => !part || part === "." || part === ".." || part.startsWith("."))) fail("public path is unsafe");
}

function validateCommit(commit, options, receiptBytes) {
  if (
    commit?.version !== 1 ||
    commit.state !== "release_committed_not_activated" ||
    commit.release_id !== options.releaseID ||
    commit.receipt_sha256 !== options.receiptSHA256 ||
    digest(receiptBytes) !== options.receiptSHA256 ||
    !Number.isSafeInteger(commit.asset_count) ||
    commit.asset_count < 0 ||
    commit.asset_count > 500 ||
    !Array.isArray(commit.assets) ||
    commit.assets.length !== commit.asset_count
  ) {
    if (Number.isSafeInteger(commit?.asset_count) && commit.asset_count > 500) {
      fail("activation bundle supports at most 500 reviewed materials");
    }
    fail("OSS release commit receipt does not match the approved complete release");
  }
  const seenPaths = new Set();
  return commit.assets.map((asset) => {
    validatePath(asset?.public_path);
    if (
      seenPaths.has(asset.public_path) ||
      !HASH_PATTERN.test(asset.sha256 || "") ||
      !Number.isSafeInteger(asset.bytes) ||
      asset.bytes < 0 ||
      typeof asset.object_version_id !== "string" ||
      !asset.object_version_id ||
      asset.object_key !== `releases/${options.releaseID}/receipts/${options.receiptSHA256}/objects/${asset.sha256}/${asset.public_path}`
    ) fail("OSS release commit contains an invalid or duplicate object binding");
    seenPaths.add(asset.public_path);
    return { public_path: asset.public_path, object_key: asset.object_key, object_version_id: asset.object_version_id };
  }).sort((left, right) => Buffer.compare(Buffer.from(left.public_path), Buffer.from(right.public_path)));
}

function validateDerived(installedRelease, releaseID) {
  const inventoryBytes = readRegular(join(installedRelease, "derived-inventory.json"), "derived inventory");
  const inventory = parseJSON(inventoryBytes, "derived inventory");
  if (inventory?.version !== 1 || inventory.release_id !== releaseID || !Array.isArray(inventory.assets)) fail("derived inventory does not match the release");
  const normalized = [];
  let prior = null;
  for (const asset of inventory.assets) {
    validatePath(asset?.path);
    if (!Number.isSafeInteger(asset.bytes) || asset.bytes < 0 || !HASH_PATTERN.test(asset.sha256 || "")) fail("derived inventory asset is invalid");
    if (prior !== null && Buffer.compare(Buffer.from(prior), Buffer.from(asset.path)) >= 0) fail("derived inventory is not uniquely bytewise sorted");
    const bytes = readRegular(join(installedRelease, "slides", ...asset.path.split("/")), "derived slide");
    if (bytes.length !== asset.bytes || digest(bytes) !== asset.sha256) fail("derived slide does not match its inventory");
    normalized.push({ path: asset.path, bytes: asset.bytes, sha256: asset.sha256 });
    prior = asset.path;
  }
  return { release_id: releaseID, slides_sha256: digest(canonicalJSON(normalized)), index_sha256: digest(inventoryBytes) };
}

function atomicWrite(path, bytes) {
  if (existsSync(path)) {
    if (readRegular(path, "existing activation bundle").equals(bytes)) return;
    fail("existing activation bundle conflicts with the complete release");
  }
  const temporary = `${path}.${process.pid}.${randomUUID()}.next`;
  const descriptor = openSync(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL, 0o400);
  try {
    writeFileSync(descriptor, bytes);
    fsyncSync(descriptor);
  } finally {
    closeSync(descriptor);
  }
  try {
    linkSync(temporary, path);
  } catch (error) {
    unlinkSync(temporary);
    if (error?.code === "EEXIST" && readRegular(path, "existing activation bundle").equals(bytes)) return;
    throw error;
  }
  unlinkSync(temporary);
  const directory = openSync(dirname(path), constants.O_RDONLY);
  try {
    fsyncSync(directory);
  } finally {
    closeSync(directory);
  }
}

function main() {
  const options = parseOptions(process.argv.slice(2));
  const receiptBytes = readRegular(options.sealedRelease, "sealed receipt");
  const receipt = parseJSON(receiptBytes, "sealed receipt");
  if (receipt?.version !== 1 || receipt.release_id !== options.releaseID || digest(receiptBytes) !== options.receiptSHA256) fail("sealed receipt does not match the approved identity");
  const manifestBytes = readRegular(join(dirname(options.sealedRelease), "manifest.json"), "sealed manifest");
  if (digest(manifestBytes) !== receipt.manifest_sha256) fail("sealed manifest does not match the receipt");
  const commitBytes = readRegular(options.ossCommit, "OSS release commit");
  const commit = parseJSON(commitBytes, "OSS release commit");
  const objects = validateCommit(commit, options, receiptBytes);
  if (commit.manifest_sha256 !== receipt.manifest_sha256 || commit.inventory_sha256 !== receipt.inventory_sha256 || commit.tree_sha256 !== receipt.tree_sha256) {
    fail("OSS release commit metadata does not match the sealed receipt");
  }
  const bundle = {
    version: 1,
    release_id: options.releaseID,
    manifest_json: manifestBytes.toString("base64"),
    sealed_receipt_json: receiptBytes.toString("base64"),
    release_commit_json: commitBytes.toString("base64"),
    derived: validateDerived(options.installedRelease, options.releaseID),
    objects,
  };
  atomicWrite(options.output, canonicalJSON(bundle));
  process.stdout.write(`${JSON.stringify({
    release_id: options.releaseID,
    bundle_sha256: digest(canonicalJSON(bundle)),
    oss_commit_sha256: digest(commitBytes),
    material_count: objects.length,
  })}\n`);
}

try {
  main();
} catch (error) {
  console.error(`build-henukit-library-activation-bundle: ${error.message}`);
  process.exit(1);
}
