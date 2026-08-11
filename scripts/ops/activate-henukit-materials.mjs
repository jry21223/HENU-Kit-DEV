#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import {
  closeSync,
  chmodSync,
  constants,
  copyFileSync,
  existsSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readlinkSync,
  readdirSync,
  renameSync,
  rmSync,
  symlinkSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, join, posix, relative, resolve, sep } from "node:path";

const RELEASE_PATTERN = /^[a-f0-9]{40}-[a-f0-9]{16}$/;
const HASH_PATTERN = /^[a-f0-9]{64}$/;
const MAX_METADATA_BYTES = 4 * 1024 * 1024;

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
    ["--receipt-sha256", "receiptSha256"],
    ["--sealed-root", "sealedRoot"],
    ["--public-root", "publicRoot"],
    ["--converter", "converter"],
    ["--importer", "importer"],
    ["--psql", "psql"],
    ["--legacy-inventory", "legacyInventory"],
    ["--activation-owner", "activationOwner"],
    ["--oss-audit-root", "ossAuditRoot"],
    ["--activation-staging-root", "activationStagingRoot"],
    ["--bundle-builder", "bundleBuilder"],
    ["--library-activator", "libraryActivator"],
  ]);
  if (argv.length !== names.size * 2) fail("expected exactly thirteen fixed activation options");
  const options = {};
  for (let index = 0; index < argv.length; index += 2) {
    const name = names.get(argv[index]);
    const value = argv[index + 1];
    if (!name || !value || value.startsWith("--") || Object.hasOwn(options, name)) {
      fail(`invalid activation option: ${argv[index] || ""}`);
    }
    options[name] = value;
  }
  return options;
}

function validateOptions(options) {
  options.pgServiceFile = process.env.PGSERVICEFILE || "";
  options.pgService = process.env.PGSERVICE || "";
  if (!RELEASE_PATTERN.test(options.releaseID || "")) fail("release ID is invalid");
  if (!HASH_PATTERN.test(options.receiptSha256 || "")) fail("receipt SHA-256 is invalid");
  for (const key of ["sealedRoot", "publicRoot", "converter", "importer", "psql", "legacyInventory", "ossAuditRoot", "activationStagingRoot", "bundleBuilder", "libraryActivator"]) {
    if (!isAbsolute(options[key] || "") || resolve(options[key]) === sep || options[key].includes("\0")) {
      fail(`${key} must be a non-root absolute path`);
    }
  }
  if (!isAbsolute(options.pgServiceFile) || resolve(options.pgServiceFile) === sep || options.pgServiceFile.includes("\0")) {
    fail("PostgreSQL service file is invalid");
  }
  if (!/^[A-Za-z0-9_.-]{1,64}$/.test(options.pgService)) fail("PostgreSQL service name is invalid");
  if (!/^(?:0|[1-9][0-9]*)$/.test(options.activationOwner || "")) fail("activation owner is invalid");
  options.activationOwner = Number(options.activationOwner);
  if (typeof process.getuid !== "function" || process.getuid() !== options.activationOwner) {
    fail("activation process identity does not match the fixed owner");
  }
  for (const forbidden of ["ALIBABA_CLOUD_ACCESS_KEY_ID", "ALIBABA_CLOUD_ACCESS_KEY_SECRET", "ALIBABA_CLOUD_SECURITY_TOKEN", "LIBRARY_OSS_BUCKET", "LIBRARY_OSS_REGION", "LIBRARY_OSS_INTERNAL_ENDPOINT", "LIBRARY_OSS_PUBLIC_ENDPOINT", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"]) {
    if (process.env[forbidden]) fail(`forbidden activation environment: ${forbidden}`);
  }
  if (!process.env.LIBRARY_DATABASE_URL || !process.env.LIBRARY_OSS_ECS_RAM_ROLE) fail("fixed Library activation configuration is incomplete");
}

function assertDirectory(path, label, owner) {
  const metadata = lstatSync(path);
  if (metadata.isSymbolicLink() || !metadata.isDirectory()) fail(`${label} must be a real directory`);
  if (owner !== undefined && metadata.uid !== owner && metadata.uid !== 0) fail(`${label} has an unexpected owner`);
  if ((metadata.mode & 0o022) !== 0) fail(`${label} must not be writable by group or other`);
}

function readMetadata(path, label, owner) {
  const metadata = lstatSync(path);
  if (metadata.isSymbolicLink() || !metadata.isFile()) fail(`${label} must be a regular file`);
  if (owner !== undefined && metadata.uid !== owner && metadata.uid !== 0) fail(`${label} has an unexpected owner`);
  if ((metadata.mode & 0o022) !== 0) fail(`${label} must not be writable by group or other`);
  if (metadata.size > MAX_METADATA_BYTES) fail(`${label} exceeds its maximum size`);
  return readFileSync(path);
}

function validatePublicPath(value) {
  if (typeof value !== "string" || !value || value.includes("\\") || value.includes("\0") || posix.isAbsolute(value)) {
    fail("sealed asset path is unsafe");
  }
  const segments = value.split("/");
  if (segments.some((segment) => !segment || segment === "." || segment === ".." || segment.startsWith("."))) {
    fail(`sealed asset path is unsafe: ${value}`);
  }
  return segments;
}

function parseJSON(bytes, label) {
  try {
    return JSON.parse(bytes.toString("utf8"));
  } catch {
    fail(`${label} is not valid JSON`);
  }
}

function childPath(root, segments, label, type) {
  let current = root;
  for (let index = 0; index < segments.length; index += 1) {
    current = join(current, segments[index]);
    const metadata = lstatSync(current);
    if (metadata.isSymbolicLink()) fail(`${label} must not contain symbolic links`);
    if (index < segments.length - 1 && !metadata.isDirectory()) fail(`${label} path is not a directory`);
    if (index === segments.length - 1) {
      if (type === "file" && !metadata.isFile()) fail(`${label} must be a regular file`);
      if (type === "directory" && !metadata.isDirectory()) fail(`${label} must be a directory`);
    }
  }
  return current;
}

function verifySealedRelease(options) {
  assertDirectory(options.sealedRoot, "sealed root", options.activationOwner);
  const release = join(options.sealedRoot, options.releaseID);
  assertDirectory(release, "sealed release", options.activationOwner);
  const receiptBytes = readMetadata(join(release, "sealed-release.json"), "sealed receipt", options.activationOwner);
  if (digest(receiptBytes) !== options.receiptSha256) fail("sealed receipt digest does not match approval");
  const receipt = parseJSON(receiptBytes, "sealed receipt");
  if (receipt.release_id !== options.releaseID) fail("sealed receipt release ID does not match approval");

  const inventoryBytes = readMetadata(join(release, "inventory.json"), "sealed inventory", options.activationOwner);
  if (digest(inventoryBytes) !== receipt.inventory_sha256) fail("sealed inventory digest does not match receipt");
  const inventory = parseJSON(inventoryBytes, "sealed inventory");
  const manifestBytes = readMetadata(join(release, "manifest.json"), "sealed manifest", options.activationOwner);
  if (digest(manifestBytes) !== receipt.manifest_sha256 || digest(manifestBytes) !== inventory.manifest_sha256) {
    fail("sealed manifest digest does not match receipt");
  }
  if (!Array.isArray(inventory.assets) || inventory.assets.length !== receipt.reviewed_assets) {
    fail("sealed inventory asset count does not match receipt");
  }

  const assets = [];
  for (const asset of inventory.assets) {
    const segments = validatePublicPath(asset?.public_path);
    if (!Number.isSafeInteger(asset?.bytes) || asset.bytes < 0 || !HASH_PATTERN.test(asset?.sha256 || "")) {
      fail(`sealed inventory asset is invalid: ${asset?.public_path || ""}`);
    }
    const source = childPath(release, ["public", ...segments], `sealed asset ${asset.public_path}`, "file");
    const metadata = lstatSync(source);
    if (metadata.isSymbolicLink() || !metadata.isFile()) fail(`sealed asset must be a regular file: ${asset.public_path}`);
    if (metadata.uid !== options.activationOwner && metadata.uid !== 0) fail(`sealed asset has an unexpected owner: ${asset.public_path}`);
    if ((metadata.mode & 0o022) !== 0) fail(`sealed asset must not be writable by group or other: ${asset.public_path}`);
    const bytes = readFileSync(source);
    if (bytes.length !== asset.bytes || digest(bytes) !== asset.sha256) {
      fail(`sealed asset does not match inventory: ${asset.public_path}`);
    }
    assets.push({ ...asset, segments, source });
  }
  const treeEntries = assets
    .map((asset) => ({ path: `public/${asset.public_path}`, bytes: asset.bytes, sha256: asset.sha256 }))
    .sort((left, right) => Buffer.compare(Buffer.from(left.path), Buffer.from(right.path)));
  if (digest(canonicalJSON(treeEntries)) !== receipt.tree_sha256 || receipt.tree_sha256 !== inventory.tree_sha256) {
    fail("sealed asset tree does not match receipt");
  }
  return { release, receiptBytes, inventoryBytes, manifestBytes, assets };
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { encoding: "utf8", ...options });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const detail = [result.stderr, result.stdout].filter(Boolean).join("\n").trim();
    fail(`${command} failed${detail ? `: ${detail}` : ""}`);
  }
  return result.stdout;
}

function copyRelease(stage, sealed) {
  for (const [name, bytes] of [
    ["manifest.json", sealed.manifestBytes],
    ["inventory.json", sealed.inventoryBytes],
    ["sealed-release.json", sealed.receiptBytes],
  ]) {
    writeFileSync(join(stage, name), bytes, { mode: 0o400 });
  }
  mkdirSync(join(stage, "public"), { mode: 0o755 });
  for (const asset of sealed.assets) {
    const target = join(stage, "public", ...asset.segments);
    mkdirSync(dirname(target), { recursive: true, mode: 0o755 });
    copyFileSync(asset.source, target, constants.COPYFILE_EXCL);
    chmodSync(target, 0o444);
  }
}

function normalizeReadOnlyTree(root, label) {
  for (const name of readdirSync(root)) {
    if (!name || name.startsWith(".") || name.includes(sep)) fail(`${label} contains an unsafe path`);
    const path = join(root, name);
    const metadata = lstatSync(path);
    if (metadata.isSymbolicLink()) fail(`${label} must not contain symbolic links`);
    if (metadata.isDirectory()) {
      normalizeReadOnlyTree(path, label);
      chmodSync(path, 0o555);
    } else if (metadata.isFile()) {
      chmodSync(path, 0o444);
    } else {
      fail(`${label} contains a non-regular file`);
    }
  }
}

function collectDerivedAssets(root, owner) {
  const assets = [];
  function visit(directory, prefix) {
    const directoryMetadata = lstatSync(directory);
    if (directoryMetadata.isSymbolicLink() || !directoryMetadata.isDirectory()) fail("derived slides must be real directories");
    if (directoryMetadata.uid !== owner && directoryMetadata.uid !== 0) fail("derived slides have an unexpected owner");
    if ((directoryMetadata.mode & 0o022) !== 0) fail("derived slides must not be writable by group or other");
    for (const name of readdirSync(directory).sort((left, right) => Buffer.compare(Buffer.from(left), Buffer.from(right)))) {
      if (!name || name.startsWith(".") || name.includes("\\") || name.includes("\0")) fail("derived slides contain an unsafe path");
      const path = join(directory, name);
      const relativePath = prefix ? `${prefix}/${name}` : name;
      const metadata = lstatSync(path);
      if (metadata.isSymbolicLink()) fail("derived slides must not contain symbolic links");
      if (metadata.isDirectory()) {
        visit(path, relativePath);
      } else if (metadata.isFile()) {
        if (metadata.uid !== owner && metadata.uid !== 0) fail("derived slides have an unexpected owner");
        if ((metadata.mode & 0o022) !== 0) fail("derived slides must not be writable by group or other");
        const bytes = readFileSync(path);
        assets.push({ path: relativePath, bytes: bytes.length, sha256: digest(bytes) });
      } else {
        fail("derived slides contain a non-regular file");
      }
    }
  }
  visit(root, "");
  return assets;
}

function verifyInstalledRelease(finalRelease, sealed, owner, releaseID) {
  assertDirectory(finalRelease, "installed release", owner);
  const expectedRootEntries = ["derived-inventory.json", "inventory.json", "manifest.json", "public", "sealed-release.json", "slides"];
  const actualRootEntries = readdirSync(finalRelease).sort();
  if (JSON.stringify(actualRootEntries) !== JSON.stringify(expectedRootEntries)) {
    fail("installed release contains unexpected entries");
  }
  for (const [name, expected] of [
    ["manifest.json", sealed.manifestBytes],
    ["inventory.json", sealed.inventoryBytes],
    ["sealed-release.json", sealed.receiptBytes],
  ]) {
    if (!readMetadata(join(finalRelease, name), `installed ${name}`, owner).equals(expected)) {
      fail("installed release metadata does not match the sealed release");
    }
  }
  for (const asset of sealed.assets) {
    const installed = childPath(finalRelease, ["public", ...asset.segments], `installed asset ${asset.public_path}`, "file");
    const bytes = readFileSync(installed);
    if (bytes.length !== asset.bytes || digest(bytes) !== asset.sha256) {
      fail(`installed asset does not match the sealed release: ${asset.public_path}`);
    }
  }
  const expectedPublicPaths = sealed.assets.map((asset) => asset.public_path).sort();
  const actualPublicPaths = [];
  function collectPublic(directory, prefix) {
    for (const name of readdirSync(directory).sort()) {
      const path = join(directory, name);
      const relativePath = prefix ? `${prefix}/${name}` : name;
      const metadata = lstatSync(path);
      if (metadata.isSymbolicLink()) fail("installed public tree must not contain symbolic links");
      if (metadata.uid !== owner && metadata.uid !== 0) fail("installed public tree has an unexpected owner");
      if ((metadata.mode & 0o022) !== 0) fail("installed public tree must not be writable by group or other");
      if (metadata.isDirectory()) collectPublic(path, relativePath);
      else if (metadata.isFile()) actualPublicPaths.push(relativePath);
      else fail("installed public tree contains a non-regular file");
    }
  }
  collectPublic(join(finalRelease, "public"), "");
  if (JSON.stringify(actualPublicPaths.sort()) !== JSON.stringify(expectedPublicPaths)) {
    fail("installed public tree does not match the sealed inventory");
  }
  const slidesRoot = childPath(finalRelease, ["slides"], "installed slides", "directory");
  const derivedInventory = parseJSON(
    readMetadata(join(finalRelease, "derived-inventory.json"), "installed derived inventory", owner),
    "installed derived inventory",
  );
  const actualDerivedAssets = collectDerivedAssets(slidesRoot, owner);
  if (
    derivedInventory?.version !== 1 ||
    derivedInventory.release_id !== releaseID ||
    !Array.isArray(derivedInventory.assets) ||
    !canonicalJSON(derivedInventory.assets).equals(canonicalJSON(actualDerivedAssets))
  ) {
    fail("installed derived slides do not match their inventory");
  }
}

function replaceCurrent(publicRoot, target) {
  const next = join(publicRoot, ".current.next");
  const stale = lstatIfExists(next);
  if (stale) {
    if (!stale.isSymbolicLink()) fail("stale current pointer is invalid");
    unlinkSync(next);
  }
  symlinkSync(relative(publicRoot, target), next);
  renameSync(next, join(publicRoot, "current"));
  fsyncDirectory(publicRoot);
}

function restoreCurrent(publicRoot, previousTarget) {
  const current = join(publicRoot, "current");
  if (previousTarget === null) {
    if (existsSync(current) || lstatIfExists(current)) unlinkSync(current);
    return;
  }
  replaceCurrent(publicRoot, resolve(publicRoot, previousTarget));
}

function lstatIfExists(path) {
  try {
    return lstatSync(path);
  } catch (error) {
    if (error?.code === "ENOENT") return null;
    throw error;
  }
}

function fsyncDirectory(path) {
  const descriptor = openSync(path, constants.O_RDONLY);
  try {
    fsyncSync(descriptor);
  } finally {
    closeSync(descriptor);
  }
}

function fsyncTree(root) {
  const metadata = lstatSync(root);
  if (metadata.isSymbolicLink()) fail("release tree must not contain symbolic links");
  if (metadata.isFile()) {
    const descriptor = openSync(root, constants.O_RDONLY);
    try {
      fsyncSync(descriptor);
    } finally {
      closeSync(descriptor);
    }
    return;
  }
  if (!metadata.isDirectory()) fail("release tree contains a non-regular entry");
  for (const name of readdirSync(root)) fsyncTree(join(root, name));
  fsyncDirectory(root);
}

function atomicWrite(path, bytes, mode) {
  const temporary = `${path}.${process.pid}.${randomUUID()}.next`;
  const descriptor = openSync(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL, mode);
  try {
    writeFileSync(descriptor, bytes);
    fsyncSync(descriptor);
  } finally {
    closeSync(descriptor);
  }
  renameSync(temporary, path);
  fsyncDirectory(dirname(path));
}

function removeRegular(path, label) {
  const metadata = lstatIfExists(path);
  if (!metadata) return;
  if (metadata.isSymbolicLink() || !metadata.isFile()) fail(`${label} must be a regular file`);
  unlinkSync(path);
  fsyncDirectory(dirname(path));
}

function readActiveRelease(publicRoot) {
  const path = join(publicRoot, "ACTIVE_RELEASE");
  const metadata = lstatIfExists(path);
  if (!metadata) return null;
  if (metadata.isSymbolicLink() || !metadata.isFile() || metadata.size > 256) fail("active release marker is invalid");
  const value = readFileSync(path, "utf8").trim();
  if (!RELEASE_PATTERN.test(value)) fail("active release marker is invalid");
  return value;
}

function writeJournal(publicRoot, state) {
  atomicWrite(join(publicRoot, "activation-journal.json"), canonicalJSON(state), 0o400);
}

function readJournal(publicRoot) {
  const path = join(publicRoot, "activation-journal.json");
  const metadata = lstatIfExists(path);
  if (!metadata) return null;
  if (metadata.isSymbolicLink() || !metadata.isFile() || metadata.size > MAX_METADATA_BYTES) {
    fail("activation journal is invalid");
  }
  const state = parseJSON(readFileSync(path), "activation journal");
  if (
    state?.version !== 1 ||
    !RELEASE_PATTERN.test(state.target_release || "") ||
    !HASH_PATTERN.test(state.receipt_sha256 || "") ||
    !HASH_PATTERN.test(state.oss_commit_sha256 || "") ||
    !HASH_PATTERN.test(state.library_bundle_sha256 || "") ||
    !["prepared", "static_switched", "library_running", "library_committed", "database_running", "database_committed"].includes(state.phase) ||
    (state.previous_target !== null && typeof state.previous_target !== "string") ||
    (state.previous_release !== null && !RELEASE_PATTERN.test(state.previous_release || ""))
  ) {
    fail("activation journal is invalid");
  }
  return state;
}

function ensureFence(publicRoot) {
  const fence = join(publicRoot, ".maintenance");
  const metadata = lstatIfExists(fence);
  if (metadata) {
    if (metadata.isSymbolicLink() || !metadata.isFile()) fail("maintenance fence is invalid");
    return fence;
  }
  const descriptor = openSync(fence, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL, 0o400);
  try {
    fsyncSync(descriptor);
  } finally {
    closeSync(descriptor);
  }
  fsyncDirectory(publicRoot);
  return fence;
}

function currentTarget(publicRoot) {
  const current = join(publicRoot, "current");
  const metadata = lstatIfExists(current);
  if (!metadata) return null;
  if (!metadata.isSymbolicLink()) fail("current release pointer must be a symbolic link");
  const target = readlinkSync(current);
  const resolvedTarget = resolve(publicRoot, target);
  const releasesRoot = `${resolve(publicRoot, "releases")}${sep}`;
  if (!resolvedTarget.startsWith(releasesRoot)) fail("current release pointer escapes the releases root");
  return target;
}

function restoreMarker(publicRoot, previousRelease) {
  const marker = join(publicRoot, "ACTIVE_RELEASE");
  if (previousRelease === null) {
    removeRegular(marker, "active release marker");
    return;
  }
  atomicWrite(marker, `${previousRelease}\n`, 0o444);
}

function main() {
  const options = parseOptions(process.argv.slice(2));
  validateOptions(options);
  assertDirectory(options.publicRoot, "public root", options.activationOwner);
  assertDirectory(options.ossAuditRoot, "OSS audit root", options.activationOwner);
  assertDirectory(options.activationStagingRoot, "activation staging root", options.activationOwner);
  for (const [path, label] of [[options.converter, "converter"], [options.importer, "importer"], [options.psql, "psql"], [options.bundleBuilder, "bundle builder"], [options.libraryActivator, "Library activator"]]) {
    const metadata = lstatSync(path);
    if (metadata.isSymbolicLink() || !metadata.isFile()) fail(`${label} must be a regular file`);
    if (metadata.uid !== options.activationOwner && metadata.uid !== 0) fail(`${label} has an unexpected owner`);
    if ((metadata.mode & 0o022) !== 0) fail(`${label} must not be writable by group or other`);
  }
  const serviceMetadata = lstatSync(options.pgServiceFile);
  if (serviceMetadata.isSymbolicLink() || !serviceMetadata.isFile()) fail("PostgreSQL service file must be a regular file");
  if (serviceMetadata.uid !== options.activationOwner && serviceMetadata.uid !== 0) fail("PostgreSQL service file has an unexpected owner");
  if ((serviceMetadata.mode & 0o077) !== 0) fail("PostgreSQL service file must be private");
  const legacyMetadata = lstatSync(options.legacyInventory);
  if (legacyMetadata.isSymbolicLink() || !legacyMetadata.isFile()) fail("legacy inventory must be a regular file");
  if (legacyMetadata.uid !== options.activationOwner && legacyMetadata.uid !== 0) fail("legacy inventory has an unexpected owner");
  if ((legacyMetadata.mode & 0o077) !== 0 || legacyMetadata.size > MAX_METADATA_BYTES) fail("legacy inventory must be private and bounded");
  const sealed = verifySealedRelease(options);
  const releasesRoot = join(options.publicRoot, "releases");
  mkdirSync(releasesRoot, { recursive: true, mode: 0o755 });
  const finalRelease = join(releasesRoot, options.releaseID);
  if (!lstatIfExists(finalRelease)) {
    const stage = mkdtempSync(join(releasesRoot, ".incoming."));
    let installed = false;
    try {
      copyRelease(stage, sealed);
      mkdirSync(join(stage, "slides"), { recursive: true, mode: 0o755 });
      run(options.converter, ["--mirror", join(stage, "public"), "--out", join(stage, "slides"), "--manifest", join(stage, "manifest.json")]);
      normalizeReadOnlyTree(join(stage, "slides"), "derived slides");
      normalizeReadOnlyTree(join(stage, "public"), "installed public tree");
      const derivedInventory = canonicalJSON({
        version: 1,
        release_id: options.releaseID,
        assets: collectDerivedAssets(join(stage, "slides"), options.activationOwner),
      });
      writeFileSync(join(stage, "derived-inventory.json"), derivedInventory, { mode: 0o400 });
      chmodSync(join(stage, "public"), 0o555);
      chmodSync(join(stage, "slides"), 0o555);
      chmodSync(stage, 0o555);
      fsyncTree(stage);
      renameSync(stage, finalRelease);
      fsyncDirectory(releasesRoot);
      installed = true;
    } finally {
      if (!installed && existsSync(stage)) rmSync(stage, { recursive: true, force: true });
    }
  }
  verifyInstalledRelease(finalRelease, sealed, options.activationOwner, options.releaseID);

  const bundleDirectory = join(options.activationStagingRoot, options.releaseID);
  if (!existsSync(bundleDirectory)) mkdirSync(bundleDirectory, { mode: 0o700 });
  assertDirectory(bundleDirectory, "release activation staging directory", options.activationOwner);
  const libraryBundle = join(bundleDirectory, `${options.receiptSha256}.json`);
  const cleanEnvironment = { PATH: "/usr/bin:/bin" };
  const bundleEvidence = parseJSON(run(process.execPath, [options.bundleBuilder,
    "--release-id", options.releaseID,
    "--receipt-sha256", options.receiptSha256,
    "--oss-commit", join(options.ossAuditRoot, options.releaseID, "release-commit.json"),
    "--sealed-release", join(sealed.release, "sealed-release.json"),
    "--installed-release", finalRelease,
    "--output", libraryBundle,
  ], { env: cleanEnvironment }), "activation bundle response");
  if (bundleEvidence?.release_id !== options.releaseID || !HASH_PATTERN.test(bundleEvidence?.bundle_sha256 || "") || !HASH_PATTERN.test(bundleEvidence?.oss_commit_sha256 || "") || !Number.isSafeInteger(bundleEvidence?.material_count) || bundleEvidence.material_count < 0 || bundleEvidence.material_count > 500) {
    fail("activation bundle response is invalid");
  }

  const existingJournal = readJournal(options.publicRoot);
  if (existingJournal && existingJournal.target_release !== options.releaseID) {
    fail(`activation recovery requires release ${existingJournal.target_release}`);
  }
  const state = existingJournal || {
    version: 1,
    target_release: options.releaseID,
    receipt_sha256: options.receiptSha256,
    previous_target: currentTarget(options.publicRoot),
    previous_release: readActiveRelease(options.publicRoot),
    oss_commit_sha256: bundleEvidence.oss_commit_sha256,
    library_bundle_sha256: bundleEvidence.bundle_sha256,
    phase: "prepared",
  };
  if (state.receipt_sha256 !== options.receiptSha256) fail("activation journal receipt does not match approval");
  if (state.oss_commit_sha256 !== bundleEvidence.oss_commit_sha256 || state.library_bundle_sha256 !== bundleEvidence.bundle_sha256) fail("activation journal does not match the complete OSS release bundle");

  const fence = ensureFence(options.publicRoot);
  if (!existingJournal) writeJournal(options.publicRoot, state);
  let phase = state.phase;
  try {
    if (phase === "prepared") {
      replaceCurrent(options.publicRoot, join(finalRelease, "public"));
      phase = "static_switched";
      writeJournal(options.publicRoot, { ...state, phase });
    }
    if (phase === "static_switched") {
      phase = "library_running";
      writeJournal(options.publicRoot, { ...state, phase });
    }
    if (phase === "library_running") {
      const libraryEnvironment = {
        PATH: "/usr/bin:/bin",
        LIBRARY_DATABASE_URL: process.env.LIBRARY_DATABASE_URL,
        LIBRARY_OSS_ECS_RAM_ROLE: process.env.LIBRARY_OSS_ECS_RAM_ROLE,
      };
      const libraryResponse = parseJSON(run(options.libraryActivator, ["--bundle", libraryBundle], { env: libraryEnvironment }), "Library activation response");
      const previousReleaseMatches = libraryResponse?.replayed === true
        ? libraryResponse.previous_release_id === ""
        : libraryResponse?.previous_release_id === (state.previous_release ?? "");
      if (libraryResponse?.release_id !== options.releaseID || !previousReleaseMatches || libraryResponse.material_count !== bundleEvidence.material_count || typeof libraryResponse.replayed !== "boolean") {
        fail("Library activation response is invalid");
      }
      phase = "library_committed";
      writeJournal(options.publicRoot, { ...state, phase });
    }
    if (phase !== "database_committed") {
      replaceCurrent(options.publicRoot, join(finalRelease, "public"));
      const sql = run(process.execPath, [
        options.importer,
        "--manifest", join(finalRelease, "manifest.json"),
        "--slides-dir", join(finalRelease, "slides"),
        "--release-id", options.releaseID,
        "--legacy-inventory", options.legacyInventory,
      ]);
      phase = "database_running";
      writeJournal(options.publicRoot, { ...state, phase });
      run(options.psql, ["-v", "ON_ERROR_STOP=1", "-f", "-"], {
        input: sql,
        env: { ...process.env, PGSERVICEFILE: options.pgServiceFile, PGSERVICE: options.pgService },
      });
      phase = "database_committed";
      writeJournal(options.publicRoot, { ...state, phase });
    }
    replaceCurrent(options.publicRoot, join(finalRelease, "public"));
    atomicWrite(join(options.publicRoot, "ACTIVE_RELEASE"), `${options.releaseID}\n`, 0o444);
    removeRegular(join(options.publicRoot, "activation-journal.json"), "activation journal");
    removeRegular(fence, "maintenance fence");
  } catch (error) {
    if (phase === "prepared" || phase === "static_switched") {
      restoreCurrent(options.publicRoot, state.previous_target);
      restoreMarker(options.publicRoot, state.previous_release);
      removeRegular(join(options.publicRoot, "activation-journal.json"), "activation journal");
      removeRegular(fence, "maintenance fence");
    }
    throw error;
  }
}

try {
  main();
} catch (error) {
  console.error(`activate-henukit-materials: ${error.message}`);
  process.exit(1);
}
