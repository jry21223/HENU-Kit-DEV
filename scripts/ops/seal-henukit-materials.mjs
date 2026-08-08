#!/usr/bin/env node
/**
 * Build one inert, root-owned raw-material release from a root-owned source
 * policy. The installed public boundary is the fixed shell wrapper: this
 * implementation receives only wrapper-bound values. An attempt locator is
 * retained solely for operator audit correlation; no candidate path or bytes
 * are accepted or read here.
 */

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  closeSync,
  constants,
  existsSync,
  fstatSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readSync,
  readdirSync,
  realpathSync,
  renameSync,
  rmSync,
  writeSync,
} from "node:fs";
import { dirname, isAbsolute, join, posix, resolve, sep } from "node:path";

const ATTEMPT_PATTERN = /^\.attempt\.[A-Za-z0-9]{10}$/;
const SHA_PATTERN = /^[a-f0-9]{40}$/;
const HASH_PATTERN = /^[a-f0-9]{64}$/;
const REVIEW_MARKER = "待复核";
const SLIDE_ROLE = "课件PPT";
const MAX_METADATA_BYTES = 4 * 1024 * 1024;

function fail(message) {
  throw new Error(message);
}

function compareBytewise(left, right) {
  return Buffer.compare(Buffer.from(left, "utf8"), Buffer.from(right, "utf8"));
}

function parseOptions(argv) {
  const names = new Map([
    ["--attempt", "attempt"],
    ["--sealed-root", "sealedRoot"],
    ["--repository", "repository"],
    ["--ref", "ref"],
    ["--sha", "sha"],
    ["--sealed-owner", "sealedOwner"],
  ]);
  if (argv.length !== 12) fail("expected exactly six fixed sealing options");
  const options = {};
  for (let index = 0; index < argv.length; index += 2) {
    const name = names.get(argv[index]);
    const value = argv[index + 1];
    if (!name || !value || value.startsWith("--") || Object.hasOwn(options, name)) {
      fail(`invalid sealing option: ${argv[index] || ""}`);
    }
    options[name] = value;
  }
  return options;
}

function validateOptions(options) {
  if (!ATTEMPT_PATTERN.test(options.attempt || "")) fail("attempt locator is invalid");
  if (!options.sealedRoot || !isAbsolute(options.sealedRoot) || resolve(options.sealedRoot) === sep || options.sealedRoot.includes("\0")) {
    fail("sealed root must be a non-root absolute path");
  }
  if (!options.repository || options.repository.startsWith("-") || /\s/.test(options.repository)) {
    fail("source repository is invalid");
  }
  if (!options.ref.startsWith("refs/heads/") || options.ref === "refs/heads/" || options.ref.includes("..") || options.ref.includes("//") || /[^A-Za-z0-9._/-]/.test(options.ref)) {
    fail("source ref is invalid");
  }
  if (!SHA_PATTERN.test(options.sha || "")) fail("configured source SHA is invalid");
  if (!/^(?:0|[1-9][0-9]*)$/.test(options.sealedOwner || "")) fail("fixed root owner is invalid");
  options.sealedOwner = Number(options.sealedOwner);
  if (!Number.isSafeInteger(options.sealedOwner) || options.sealedOwner < 0) fail("fixed root owner is invalid");
  if (typeof process.getuid !== "function" || process.getuid() !== options.sealedOwner) {
    fail("sealing process identity does not match the fixed root owner");
  }
}

function run(command, args, { cwd } = {}) {
  const result = spawnSync(command, args, { cwd, encoding: "utf8" });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const detail = [result.stderr, result.stdout].filter(Boolean).join("\n").trim();
    fail(`${command} ${args.join(" ")} failed${detail ? `: ${detail}` : ""}`);
  }
  return result.stdout.trim();
}

function assertTrustedMetadata(metadata, label, type, owner) {
  if (metadata.isSymbolicLink()) fail(`${label} must not be a symbolic link`);
  if (type === "directory" && !metadata.isDirectory()) fail(`${label} must be a directory`);
  if (type === "file" && !metadata.isFile()) fail(`${label} must be a regular file`);
  if (metadata.uid !== owner) fail(`${label} must be owned by the fixed root account`);
  if ((metadata.mode & 0o022) !== 0) fail(`${label} must not be writable by group or other`);
}

function assertTrustedDirectory(path, label, owner) {
  const metadata = lstatSync(path);
  assertTrustedMetadata(metadata, label, "directory", owner);
  return resolve(path);
}

function existsNoFollow(path) {
  try {
    lstatSync(path);
    return true;
  } catch (error) {
    if (error?.code === "ENOENT") return false;
    throw error;
  }
}

function assertTrustedAncestor(metadata, owner) {
  if (metadata.isSymbolicLink() || !metadata.isDirectory()) fail("sealed root must be a real directory");
  if (metadata.uid !== owner && metadata.uid !== 0) {
    if (owner === 0) fail("sealed root has an ancestor not owned by the fixed root account");
    fail("sealed root has an ancestor not owned by the sealing account or root");
  }
  if ((metadata.mode & 0o022) !== 0) fail("sealed root has an unsafe writable ancestor");
}

function assertTrustedSealedRoot(path, owner) {
  const requested = lstatSync(path);
  if (requested.isSymbolicLink() || !requested.isDirectory()) fail("sealed root must be a real directory");
  const resolved = realpathSync(path);
  const segments = resolved.split(sep).filter(Boolean);
  let current = sep;
  for (let index = 0; index < segments.length; index += 1) {
    current = join(current, segments[index]);
    const metadata = lstatSync(current);
    if (metadata.isSymbolicLink() || !metadata.isDirectory()) fail("sealed root must be a real directory");
    if (index === segments.length - 1) {
      assertTrustedMetadata(metadata, "sealed root", "directory", owner);
    } else {
      assertTrustedAncestor(metadata, owner);
    }
  }
  return resolved;
}

function childPath(root, segments, label, type) {
  let current = root;
  for (let index = 0; index < segments.length; index += 1) {
    const segment = segments[index];
    if (!segment || segment === "." || segment === ".." || segment.includes(sep)) fail(`${label} path is unsafe`);
    current = join(current, segment);
    const metadata = lstatSync(current);
    if (metadata.isSymbolicLink()) fail(`${label} must not contain symbolic links`);
    if (index < segments.length - 1 && !metadata.isDirectory()) fail(`${label} path is not a directory`);
    if (index === segments.length - 1) {
      if (type === "directory" && !metadata.isDirectory()) fail(`${label} must be a directory`);
      if (type === "file" && !metadata.isFile()) fail(`${label} must be a regular file`);
    }
  }
  return current;
}

function openRegular(path, label, { owner, maxBytes = Number.MAX_SAFE_INTEGER } = {}) {
  const before = lstatSync(path);
  if (before.isSymbolicLink() || !before.isFile()) fail(`${label} must be a regular file`);
  if (owner !== undefined) assertTrustedMetadata(before, label, "file", owner);
  if (before.size > maxBytes) fail(`${label} exceeds its maximum metadata size`);
  const descriptor = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  const metadata = fstatSync(descriptor);
  if (!metadata.isFile() || metadata.dev !== before.dev || metadata.ino !== before.ino) {
    closeSync(descriptor);
    fail(`${label} changed while being opened`);
  }
  if (owner !== undefined) assertTrustedMetadata(metadata, label, "file", owner);
  if (metadata.size > maxBytes) {
    closeSync(descriptor);
    fail(`${label} exceeds its maximum metadata size`);
  }
  return { descriptor, bytes: metadata.size };
}

function readRegular(path, label, options) {
  const { descriptor, bytes } = openRegular(path, label, options);
  try {
    const chunks = [];
    let remaining = bytes;
    while (remaining > 0) {
      const buffer = Buffer.allocUnsafe(Math.min(remaining, 128 * 1024));
      const read = readSync(descriptor, buffer, 0, buffer.length, null);
      if (read === 0) fail(`${label} changed while being read`);
      chunks.push(buffer.subarray(0, read));
      remaining -= read;
    }
    if (readSync(descriptor, Buffer.allocUnsafe(1), 0, 1, null) !== 0) fail(`${label} changed while being read`);
    return Buffer.concat(chunks);
  } finally {
    closeSync(descriptor);
  }
}

function hashRegular(path, label, options) {
  const { descriptor, bytes } = openRegular(path, label, options);
  const digest = createHash("sha256");
  try {
    let total = 0;
    const buffer = Buffer.allocUnsafe(128 * 1024);
    for (;;) {
      const read = readSync(descriptor, buffer, 0, buffer.length, null);
      if (read === 0) break;
      digest.update(buffer.subarray(0, read));
      total += read;
    }
    if (total !== bytes) fail(`${label} changed while being read`);
    return { bytes: total, sha256: digest.digest("hex") };
  } finally {
    closeSync(descriptor);
  }
}

function writeFully(descriptor, buffer) {
  let offset = 0;
  while (offset < buffer.length) {
    const written = writeSync(descriptor, buffer, offset, buffer.length - offset);
    if (written === 0) fail("could not write sealed release");
    offset += written;
  }
}

function writePrivate(path, content, mode = 0o400) {
  const descriptor = openSync(path, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL, mode);
  try {
    writeFully(descriptor, content);
    fsyncSync(descriptor);
  } finally {
    closeSync(descriptor);
  }
}

function copyVerified(source, target, expected, label) {
  const { descriptor, bytes } = openRegular(source, label);
  mkdirSync(dirname(target), { recursive: true, mode: 0o700 });
  const output = openSync(target, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL, 0o400);
  const digest = createHash("sha256");
  try {
    let total = 0;
    const buffer = Buffer.allocUnsafe(128 * 1024);
    for (;;) {
      const read = readSync(descriptor, buffer, 0, buffer.length, null);
      if (read === 0) break;
      const chunk = buffer.subarray(0, read);
      writeFully(output, chunk);
      digest.update(chunk);
      total += read;
    }
    if (total !== bytes || total !== expected.bytes || digest.digest("hex") !== expected.sha256) {
      fail(`${label} does not match its reviewed source asset`);
    }
    fsyncSync(output);
  } finally {
    closeSync(output);
    closeSync(descriptor);
  }
}

function validatePublicPath(value) {
  if (typeof value !== "string" || !value || value.includes("\\") || value.includes("\0") || posix.isAbsolute(value)) {
    fail("reviewed asset path is unsafe");
  }
  const segments = value.split("/");
  if (segments.some((segment) => !segment || segment === "." || segment === ".." || segment.startsWith("."))) {
    fail(`reviewed asset path is unsafe: ${value}`);
  }
  return segments;
}

function parseManifest(bytes) {
  let manifest;
  try {
    manifest = JSON.parse(bytes.toString("utf8"));
  } catch {
    fail("manifest.json is not valid JSON");
  }
  if (!Array.isArray(manifest.subjects)) fail("manifest.subjects must be an array");
  const assets = [];
  const paths = new Set();
  const hashes = new Set();
  let sourceSlideAssets = 0;
  for (const subject of manifest.subjects) {
    if (!subject || !Array.isArray(subject.assets)) fail("every manifest subject must have an assets array");
    for (const asset of subject.assets) {
      const role = typeof asset?.role === "string" ? asset.role : "";
      if (role.startsWith(REVIEW_MARKER)) {
        continue;
      }
      const segments = validatePublicPath(asset?.publicPath);
      if (!HASH_PATTERN.test(asset?.sha256 || "")) fail(`reviewed asset has an invalid SHA-256: ${asset.publicPath}`);
      if (!Number.isSafeInteger(asset?.bytes) || asset.bytes < 0) fail(`reviewed asset has an invalid byte count: ${asset.publicPath}`);
      if (paths.has(asset.publicPath)) fail(`duplicate reviewed asset path: ${asset.publicPath}`);
      if (hashes.has(asset.sha256)) fail(`duplicate reviewed asset SHA-256: ${asset.sha256}`);
      paths.add(asset.publicPath);
      hashes.add(asset.sha256);
      if (role === SLIDE_ROLE) sourceSlideAssets += 1;
      assets.push({ publicPath: asset.publicPath, segments, bytes: asset.bytes, sha256: asset.sha256 });
    }
  }
  return { assets, sourceSlideAssets };
}

function fetchFixedSource(stage, options) {
  const checkout = join(stage, ".source");
  run("git", ["init", "--quiet", checkout]);
  run("git", ["-C", checkout, "remote", "add", "origin", options.repository]);
  run("git", ["-C", checkout, "fetch", "--no-tags", "--depth=1", "origin", options.ref]);
  const resolvedSHA = run("git", ["-C", checkout, "rev-parse", "FETCH_HEAD"]).toLowerCase();
  if (resolvedSHA !== options.sha) fail(`configured SHA ${options.sha} does not match resolved ${options.ref} (${resolvedSHA})`);
  run("git", ["-C", checkout, "checkout", "--quiet", "--detach", options.sha]);
  if (run("git", ["-C", checkout, "rev-parse", "HEAD"]).toLowerCase() !== options.sha) {
    fail("fixed source checkout did not preserve the configured SHA");
  }
  return checkout;
}

function fsyncDirectory(path) {
  const descriptor = openSync(path, constants.O_RDONLY);
  try {
    fsyncSync(descriptor);
  } finally {
    closeSync(descriptor);
  }
}

function fsyncOutputDirectories(path) {
  for (const name of readdirSync(path).sort(compareBytewise)) {
    const child = join(path, name);
    const metadata = lstatSync(child);
    if (metadata.isSymbolicLink()) fail("sealed output must not contain symbolic links");
    if (metadata.isDirectory()) {
      fsyncOutputDirectories(child);
    } else if (!metadata.isFile()) {
      fail("sealed output contains a non-regular file");
    }
  }
  fsyncDirectory(path);
}

function ensureAuditDirectory(path, label, owner) {
  if (!existsNoFollow(path)) {
    try {
      mkdirSync(path, { mode: 0o700 });
    } catch (error) {
      if (error?.code !== "EEXIST") throw error;
    }
  }
  return assertTrustedDirectory(path, label, owner);
}

function recordAudit(sealedRoot, releaseID, receiptSha256, attempt, owner) {
  const auditRoot = ensureAuditDirectory(join(sealedRoot, ".audit"), "sealed audit root", owner);
  fsyncDirectory(sealedRoot);
  const releaseAuditRoot = ensureAuditDirectory(join(auditRoot, releaseID), "sealed release audit", owner);
  const attemptToken = attempt.slice(".attempt.".length);
  const auditPath = join(releaseAuditRoot, `${attemptToken}.json`);
  const auditBytes = canonicalJSON({
    version: 1,
    release_id: releaseID,
    receipt_sha256: receiptSha256,
    attempt_locator: attempt,
  });
  if (existsNoFollow(auditPath)) {
    const existing = readRegular(auditPath, "sealed attempt audit", { owner, maxBytes: MAX_METADATA_BYTES });
    if (!existing.equals(auditBytes)) fail("sealed attempt audit already exists with different content");
    return;
  }
  writePrivate(auditPath, auditBytes);
  fsyncDirectory(releaseAuditRoot);
  fsyncDirectory(auditRoot);
}

function canonicalJSON(value) {
  return Buffer.from(`${JSON.stringify(value, null, 2)}\n`, "utf8");
}

function digest(buffer) {
  return createHash("sha256").update(buffer).digest("hex");
}

function listTrustedFiles(root, label, owner) {
  assertTrustedDirectory(root, label, owner);
  const files = [];
  function visit(directory, prefix) {
    const names = readdirSync(directory).sort(compareBytewise);
    for (const name of names) {
      if (!name || name.startsWith(".")) fail(`${label} contains an unsafe path`);
      const path = join(directory, name);
      const relativePath = join(prefix, name).split(sep).join("/");
      const metadata = lstatSync(path);
      if (metadata.isSymbolicLink()) fail(`${label} must not contain symbolic links`);
      if (metadata.isDirectory()) {
        assertTrustedMetadata(metadata, `${label} ${relativePath}`, "directory", owner);
        visit(path, relativePath);
      } else if (metadata.isFile()) {
        assertTrustedMetadata(metadata, `${label} ${relativePath}`, "file", owner);
        files.push({ path, relativePath });
      } else {
        fail(`${label} contains a non-regular file`);
      }
    }
  }
  visit(root, "");
  return files.sort((left, right) => compareBytewise(left.relativePath, right.relativePath));
}

function existingRelease(finalPath, expectedReceipt, expectedInventory, expectedEntries, owner) {
  if (!existsNoFollow(finalPath)) return null;
  assertTrustedDirectory(finalPath, "existing sealed release", owner);
  const receiptPath = childPath(finalPath, ["sealed-release.json"], "existing sealed receipt", "file");
  const inventoryPath = childPath(finalPath, ["inventory.json"], "existing sealed inventory", "file");
  const receipt = readRegular(receiptPath, "existing sealed receipt", { owner, maxBytes: MAX_METADATA_BYTES });
  const inventory = readRegular(inventoryPath, "existing sealed inventory", { owner, maxBytes: MAX_METADATA_BYTES });
  if (!receipt.equals(expectedReceipt) || !inventory.equals(expectedInventory)) {
    fail("sealed release ID already exists with a different receipt");
  }
  const files = new Map(listTrustedFiles(finalPath, "existing sealed release", owner).map((item) => [item.relativePath, item.path]));
  const expectedPaths = expectedEntries.map((entry) => entry.path).sort(compareBytewise);
  if (JSON.stringify([...files.keys()]) !== JSON.stringify(expectedPaths)) {
    fail("sealed release ID already exists with a different receipt");
  }
  for (const entry of expectedEntries) {
    const actual = hashRegular(files.get(entry.path), `existing sealed release ${entry.path}`, { owner });
    if (actual.bytes !== entry.bytes || actual.sha256 !== entry.sha256) {
      fail("sealed release ID already exists with a different receipt");
    }
  }
  return digest(receipt);
}

function main() {
  const options = parseOptions(process.argv.slice(2));
  validateOptions(options);
  const sealedRoot = assertTrustedSealedRoot(options.sealedRoot, options.sealedOwner);
  const provisional = mkdtempSync(join(sealedRoot, ".incoming."));
  let finished = false;
  try {
    const sourceCheckout = fetchFixedSource(provisional, options);
    const sourceManifest = readRegular(
      childPath(sourceCheckout, ["manifest.json"], "fixed source manifest", "file"),
      "fixed source manifest",
      { maxBytes: MAX_METADATA_BYTES },
    );
    const manifestSha256 = digest(sourceManifest);
    const expected = parseManifest(sourceManifest);
    const assetInventory = [];
    for (const asset of [...expected.assets].sort((left, right) => compareBytewise(left.publicPath, right.publicPath))) {
      const sourceAsset = childPath(sourceCheckout, asset.segments, `fixed source asset ${asset.publicPath}`, "file");
      const sourceDigest = hashRegular(sourceAsset, `fixed source asset ${asset.publicPath}`);
      if (sourceDigest.bytes !== asset.bytes || sourceDigest.sha256 !== asset.sha256) {
        fail(`fixed source asset ${asset.publicPath} does not match its manifest`);
      }
      copyVerified(sourceAsset, join(provisional, "public", ...asset.segments), asset, `fixed source asset ${asset.publicPath}`);
      assetInventory.push({ public_path: asset.publicPath, bytes: asset.bytes, sha256: asset.sha256 });
    }
    rmSync(sourceCheckout, { recursive: true, force: true });

    const treeEntries = assetInventory
      .map((asset) => ({ path: `public/${asset.public_path}`, bytes: asset.bytes, sha256: asset.sha256 }))
      .sort((left, right) => compareBytewise(left.path, right.path));
    const treeSha256 = digest(canonicalJSON(treeEntries));
    const slides = { status: "deferred", source_slide_assets: expected.sourceSlideAssets };
    const inventory = {
      version: 1,
      source: { repository: options.repository, ref: options.ref, sha: options.sha },
      manifest_sha256: manifestSha256,
      assets: assetInventory,
      slides,
      tree_sha256: treeSha256,
    };
    const inventoryBytes = canonicalJSON(inventory);
    const releaseID = `${options.sha}-${manifestSha256.slice(0, 16)}`;
    const receipt = {
      version: 1,
      release_id: releaseID,
      source: inventory.source,
      manifest_sha256: manifestSha256,
      inventory_sha256: digest(inventoryBytes),
      tree_sha256: treeSha256,
      reviewed_assets: assetInventory.length,
      slides,
    };
    const receiptBytes = canonicalJSON(receipt);
    const receiptSha256 = digest(receiptBytes);
    const sealedEntries = [
      { path: "sealed-release.json", bytes: receiptBytes.length, sha256: receiptSha256 },
      { path: "inventory.json", bytes: inventoryBytes.length, sha256: digest(inventoryBytes) },
      ...treeEntries,
    ].sort((left, right) => compareBytewise(left.path, right.path));
    writePrivate(join(provisional, "inventory.json"), inventoryBytes);
    writePrivate(join(provisional, "sealed-release.json"), receiptBytes);
    fsyncOutputDirectories(provisional);

    const finalPath = join(sealedRoot, releaseID);
    const alreadySealed = existingRelease(finalPath, receiptBytes, inventoryBytes, sealedEntries, options.sealedOwner);
    if (alreadySealed) {
      rmSync(provisional, { recursive: true, force: true });
      recordAudit(sealedRoot, releaseID, alreadySealed, options.attempt, options.sealedOwner);
      finished = true;
      console.log(JSON.stringify({ attempt_locator: options.attempt, release_id: releaseID, receipt_sha256: alreadySealed }));
      return;
    }
    try {
      renameSync(provisional, finalPath);
    } catch (error) {
      if (error?.code !== "EEXIST" && error?.code !== "ENOTEMPTY") throw error;
      const concurrentReceipt = existingRelease(finalPath, receiptBytes, inventoryBytes, sealedEntries, options.sealedOwner);
      if (!concurrentReceipt) throw error;
      rmSync(provisional, { recursive: true, force: true });
      recordAudit(sealedRoot, releaseID, concurrentReceipt, options.attempt, options.sealedOwner);
      finished = true;
      console.log(JSON.stringify({ attempt_locator: options.attempt, release_id: releaseID, receipt_sha256: concurrentReceipt }));
      return;
    }
    fsyncDirectory(sealedRoot);
    recordAudit(sealedRoot, releaseID, receiptSha256, options.attempt, options.sealedOwner);
    finished = true;
    console.log(JSON.stringify({ attempt_locator: options.attempt, release_id: releaseID, receipt_sha256: receiptSha256 }));
  } finally {
    if (!finished && existsSync(provisional)) rmSync(provisional, { recursive: true, force: true });
  }
}

try {
  main();
} catch (error) {
  console.error(`seal-henukit-materials: ${error.message}`);
  process.exit(1);
}
