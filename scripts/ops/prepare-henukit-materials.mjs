#!/usr/bin/env node
/**
 * Prepare one immutable materials candidate without publishing it.
 *
 * The command deliberately has no public-root, database, or activation option.
 * It fetches one accepted ref/SHA and validates the reviewed manifest assets
 * into a newly-created candidate directory. Online preview derivation is not
 * part of the OSS-only release boundary.
 */

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  chmodSync,
  closeSync,
  copyFileSync,
  existsSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  readSync,
  realpathSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { basename, dirname, isAbsolute, join, posix, resolve, relative, sep } from "node:path";
const REVIEW_MARKER = "待复核";
const SHA_PATTERN = /^[a-f0-9]{40}$/;
const FILE_HASH_PATTERN = /^[a-f0-9]{64}$/;
const DEFAULT_SERVED_PUBLIC_ROOT = "/opt/henukit-materials/public";

function usage() {
  console.error(`usage: prepare-henukit-materials.mjs \\
  --repository <source-repository> \\
  --ref <refs/heads/name> \\
  --sha <40-lowercase-hex> \\
  --candidate-dir <new-absolute-directory>

Prepares a detached, validated materials candidate only. It never changes a
served public tree, the Study catalog, or a production configuration.`);
}

function fail(message) {
  throw new Error(message);
}

function parseOptions(argv) {
  const options = {
    repository: "",
    ref: "",
    sha: "",
    candidateDir: "",
  };
  const names = new Map([
    ["--repository", "repository"],
    ["--ref", "ref"],
    ["--sha", "sha"],
    ["--candidate-dir", "candidateDir"],
  ]);
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--help" || argument === "-h") {
      usage();
      process.exit(0);
    }
    const name = names.get(argument);
    if (!name) {
      fail(`unknown option: ${argument}`);
    }
    const value = argv[index + 1];
    if (!value || value.startsWith("--")) {
      fail(`missing value for ${argument}`);
    }
    options[name] = value;
    index += 1;
  }
  return options;
}

function resolveThroughExistingAncestor(path) {
  const suffix = [];
  let cursor = resolve(path);
  while (!existsSync(cursor)) {
    const parent = dirname(cursor);
    if (parent === cursor) return cursor;
    suffix.unshift(basename(cursor));
    cursor = parent;
  }
  return join(realpathSync(cursor), ...suffix);
}

function servedPublicRoots() {
  const roots = [DEFAULT_SERVED_PUBLIC_ROOT];
  const configured = process.env.HENUKIT_MATERIALS_PUBLIC_ROOT?.trim();
  if (configured) {
    if (!isAbsolute(configured)) {
      fail("HENUKIT_MATERIALS_PUBLIC_ROOT must be an absolute path");
    }
    roots.push(configured);
  }
  return [...new Set(roots.map((root) => resolveThroughExistingAncestor(root)))];
}

function validateSource(options) {
  if (!options.repository.trim()) fail("--repository is required");
  if (!SHA_PATTERN.test(options.sha)) {
    fail("--sha must be a lowercase 40-character Git SHA");
  }
  if (!options.ref.startsWith("refs/heads/") || options.ref.includes("..") || options.ref.includes("//") || /[^A-Za-z0-9._/-]/.test(options.ref)) {
    fail("--ref must be a full, safe refs/heads/... branch ref");
  }
  if (options.ref === "refs/heads/") fail("--ref must name a branch");
  if (!options.candidateDir || !isAbsolute(options.candidateDir)) {
    fail("--candidate-dir must be a new absolute directory");
  }
  if (resolve(options.candidateDir) === sep) {
    fail("--candidate-dir must not be the filesystem root");
  }
  if (existsSync(options.candidateDir)) {
    fail("--candidate-dir must not already exist");
  }
  const candidate = resolveThroughExistingAncestor(options.candidateDir);
  for (const servedRoot of servedPublicRoots()) {
    if (isWithin(servedRoot, candidate)) {
      fail("--candidate-dir must be outside every served public tree");
    }
  }
}

function run(command, args, { cwd } = {}) {
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const detail = [result.stderr, result.stdout].filter(Boolean).join("\n").trim();
    fail(`${command} ${args.join(" ")} failed${detail ? `: ${detail}` : ""}`);
  }
  return result.stdout.trim();
}

function hashFile(path) {
  const digest = createHash("sha256");
  const descriptor = openSync(path, "r");
  const buffer = Buffer.allocUnsafe(128 * 1024);
  try {
    for (;;) {
      const read = readSync(descriptor, buffer, 0, buffer.length, null);
      if (read === 0) return digest.digest("hex");
      digest.update(buffer.subarray(0, read));
    }
  } finally {
    closeSync(descriptor);
  }
}

function isWithin(root, target) {
  const path = relative(root, target);
  return path === "" || (!path.startsWith(`..${sep}`) && path !== ".." && !isAbsolute(path));
}

function validatePublicPath(value) {
  if (typeof value !== "string" || value.length === 0 || value.includes("\\") || value.includes("\0")) {
    fail("reviewed asset publicPath must be a non-empty POSIX relative path");
  }
  if (posix.isAbsolute(value)) fail(`reviewed asset path must be relative: ${value}`);
  const segments = value.split("/");
  if (segments.some((segment) => !segment || segment === "." || segment === ".." || segment.startsWith("."))) {
    fail(`reviewed asset path is unsafe: ${value}`);
  }
  return segments;
}

function checkedAsset(checkout, asset, seenPaths, seenHashes) {
  const publicPath = asset?.publicPath;
  const segments = validatePublicPath(publicPath);
  if (!FILE_HASH_PATTERN.test(asset.sha256 || "")) {
    fail(`reviewed asset has an invalid SHA-256: ${publicPath}`);
  }
  if (!Number.isSafeInteger(asset.bytes) || asset.bytes < 0) {
    fail(`reviewed asset has an invalid byte count: ${publicPath}`);
  }
  if (seenPaths.has(publicPath)) fail(`duplicate reviewed asset path: ${publicPath}`);
  if (seenHashes.has(asset.sha256)) fail(`duplicate reviewed asset SHA-256: ${asset.sha256}`);

  const source = join(checkout, ...segments);
  const checkoutRealPath = realpathSync(checkout);
  const sourceRealPath = realpathSync(source);
  if (!isWithin(checkoutRealPath, sourceRealPath)) {
    fail(`reviewed asset escapes the checkout: ${publicPath}`);
  }
  const metadata = lstatSync(source);
  if (!metadata.isFile()) fail(`reviewed asset is not a regular file: ${publicPath}`);
  if (statSync(source).size !== asset.bytes) {
    fail(`reviewed asset byte count does not match manifest: ${publicPath}`);
  }
  if (hashFile(source) !== asset.sha256) {
    fail(`reviewed asset SHA-256 does not match manifest: ${publicPath}`);
  }

  seenPaths.add(publicPath);
  seenHashes.add(asset.sha256);
  return { publicPath, source };
}

function loadReviewedAssets(checkout) {
  const manifestPath = join(checkout, "manifest.json");
  const manifestBytes = readFileSync(manifestPath);
  let manifest;
  try {
    manifest = JSON.parse(manifestBytes.toString("utf8"));
  } catch {
    fail("manifest.json is not valid JSON");
  }
  if (!Array.isArray(manifest.subjects)) fail("manifest.subjects must be an array");

  const seenPaths = new Set();
  const seenHashes = new Set();
  const assets = [];
  let skippedPending = 0;
  for (const subject of manifest.subjects) {
    if (!subject || !Array.isArray(subject.assets)) {
      fail("every manifest subject must have an assets array");
    }
    for (const asset of subject.assets) {
      const role = typeof asset?.role === "string" ? asset.role : "";
      if (role.startsWith(REVIEW_MARKER)) {
        skippedPending += 1;
        continue;
      }
      assets.push(checkedAsset(checkout, asset, seenPaths, seenHashes));
    }
  }
  return {
    manifestPath,
    manifestSha256: createHash("sha256").update(manifestBytes).digest("hex"),
    manifest,
    assets,
    skippedPending,
  };
}

function copyAssets(assets, publicRoot) {
  mkdirSync(publicRoot, { recursive: true, mode: 0o755 });
  for (const asset of assets) {
    const target = join(publicRoot, ...asset.publicPath.split("/"));
    mkdirSync(dirname(target), { recursive: true, mode: 0o755 });
    copyFileSync(asset.source, target);
    chmodSync(target, 0o444);
  }
}

function main() {
  const options = parseOptions(process.argv.slice(2));
  if (typeof process.getuid === "function" && process.getuid() === 0) {
    fail("materials candidate preparation must run as an unprivileged user");
  }
  validateSource(options);

  const candidate = resolve(options.candidateDir);
  const checkout = join(candidate, "checkout");
  const publicRoot = join(candidate, "public");
  mkdirSync(candidate, { recursive: false, mode: 0o700 });

  run("git", ["init", "--quiet", checkout]);
  run("git", ["-C", checkout, "remote", "add", "origin", options.repository]);
  run("git", ["-C", checkout, "fetch", "--no-tags", "--depth=1", "origin", options.ref]);
  const resolvedRefSHA = run("git", ["-C", checkout, "rev-parse", "FETCH_HEAD"]).toLowerCase();
  if (resolvedRefSHA !== options.sha) {
    fail(`accepted SHA ${options.sha} does not match resolved ${options.ref} (${resolvedRefSHA})`);
  }
  run("git", ["-C", checkout, "checkout", "--quiet", "--detach", options.sha]);
  const detachedSHA = run("git", ["-C", checkout, "rev-parse", "HEAD"]).toLowerCase();
  if (detachedSHA !== options.sha) fail("detached checkout did not preserve the accepted SHA");

  const prepared = loadReviewedAssets(checkout);
  copyAssets(prepared.assets, publicRoot);

  const release = {
    version: 1,
    source: {
      repository: options.repository,
      ref: options.ref,
      sha: options.sha,
    },
    manifest_sha256: prepared.manifestSha256,
    reviewed_assets: prepared.assets.length,
    skipped_pending_review: prepared.skippedPending,
    public_root: "public",
  };
  writeFileSync(join(candidate, "release.json"), `${JSON.stringify(release, null, 2)}\n`, { mode: 0o600 });
  writeFileSync(join(candidate, "READY"), `${options.sha}\n`, { mode: 0o600 });
  console.log(JSON.stringify(release));
}

try {
  main();
} catch (error) {
  console.error(`prepare-henukit-materials: ${error.message}`);
  process.exit(1);
}
