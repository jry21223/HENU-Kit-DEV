import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { createHash } from "node:crypto";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const command = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "prepare-henukit-materials.mjs",
);

function sha256(content) {
  return createHash("sha256").update(content).digest("hex");
}

function git(repository, args) {
  return execFileSync("git", args, { cwd: repository, encoding: "utf8" }).trim();
}

function gitCommit(repository, message) {
  git(repository, ["add", "."]);
  git(repository, ["-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-m", message]);
  return git(repository, ["rev-parse", "HEAD"]);
}

function runPreparation(setup, { sha = setup.commit, candidate = setup.candidate } = {}) {
  const args = [
    command,
    "--repository", setup.repository,
    "--ref", "refs/heads/main",
    "--sha", sha,
    "--candidate-dir", candidate,
  ];
  return spawnSync(process.execPath, args, {
    encoding: "utf8",
    env: { ...process.env, ...(setup.env || {}) },
  });
}

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-prepare-"));
  const repository = join(root, "source");
  const candidate = join(root, "candidate");
  mkdirSync(join(repository, "materials"), { recursive: true });
  const bytes = Buffer.from("%PDF-1.4\nfixture material\n");
  const publicPath = "materials/outline.pdf";
  writeFileSync(join(repository, publicPath), bytes);
  writeFileSync(
    join(repository, "manifest.json"),
    JSON.stringify({
      version: 1,
      subjects: [
        {
          name: "离散数学",
          assets: [
            {
              role: "复习讲义",
              title: "离散数学_复习讲义_提纲.pdf",
              publicPath,
              bytes: bytes.length,
              sha256: sha256(bytes),
            },
          ],
        },
      ],
    }),
  );
  git(root, ["init", "--initial-branch=main", repository]);
  const commit = gitCommit(repository, "fixture");
  return { root, repository, candidate, commit, bytes };
}

test("prepares the exact accepted commit into an isolated candidate", () => {
  const setup = fixture();
  try {
    const result = runPreparation(setup);

    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(
      readFileSync(join(setup.candidate, "public", "materials", "outline.pdf")),
      setup.bytes,
    );
    assert.equal(existsSync(join(setup.candidate, "READY")), true);
    const release = JSON.parse(readFileSync(join(setup.candidate, "release.json"), "utf8"));
    assert.equal(release.source.sha, setup.commit);
    assert.equal(release.source.ref, "refs/heads/main");
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("rejects a ref that moved after acceptance without changing served files or the catalog", () => {
  const setup = fixture();
  try {
    const served = join(setup.root, "served-public", "keep.txt");
    const catalog = join(setup.root, "study-catalog.json");
    mkdirSync(dirname(served), { recursive: true });
    writeFileSync(served, "previous public tree");
    writeFileSync(catalog, "previous catalog");

    writeFileSync(join(setup.repository, "advance.txt"), "newer source ref");
    const newerCommit = gitCommit(setup.repository, "advance source ref");
    assert.notEqual(newerCommit, setup.commit);

    const result = runPreparation(setup);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /does not match resolved refs\/heads\/main/);
    assert.equal(readFileSync(served, "utf8"), "previous public tree");
    assert.equal(readFileSync(catalog, "utf8"), "previous catalog");
    assert.equal(existsSync(join(setup.candidate, "READY")), false);
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("rejects unsafe paths, incorrect hashes, and duplicate reviewed content at the CLI boundary", () => {
  const cases = [
    {
      name: "unsafe path",
      mutate({ repository }) {
        const manifest = JSON.parse(readFileSync(join(repository, "manifest.json"), "utf8"));
        manifest.subjects[0].assets[0].publicPath = "../outside.pdf";
        writeFileSync(join(repository, "manifest.json"), JSON.stringify(manifest));
      },
      expected: /path is unsafe/,
    },
    {
      name: "incorrect hash",
      mutate({ repository }) {
        const manifest = JSON.parse(readFileSync(join(repository, "manifest.json"), "utf8"));
        manifest.subjects[0].assets[0].sha256 = "0".repeat(64);
        writeFileSync(join(repository, "manifest.json"), JSON.stringify(manifest));
      },
      expected: /SHA-256 does not match manifest/,
    },
    {
      name: "duplicate reviewed hash",
      mutate({ repository, bytes }) {
        const manifestPath = join(repository, "manifest.json");
        const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
        const duplicatePath = "materials/outline-copy.pdf";
        writeFileSync(join(repository, duplicatePath), bytes);
        manifest.subjects[0].assets.push({
          ...manifest.subjects[0].assets[0],
          publicPath: duplicatePath,
          title: "离散数学_复习讲义_提纲副本.pdf",
        });
        writeFileSync(manifestPath, JSON.stringify(manifest));
      },
      expected: /duplicate reviewed asset SHA-256/,
    },
  ];

  for (const item of cases) {
    const setup = fixture();
    try {
      item.mutate(setup);
      const accepted = gitCommit(setup.repository, item.name);
      const result = runPreparation(setup, { sha: accepted });
      assert.notEqual(result.status, 0, item.name);
      assert.match(result.stderr, item.expected, item.name);
      assert.equal(existsSync(join(setup.candidate, "READY")), false, item.name);
      assert.equal(existsSync(join(setup.candidate, "public")), false, item.name);
    } finally {
      rmSync(setup.root, { recursive: true, force: true });
    }
  }
});

test("prepares raw slide assets without deriving an online preview", () => {
  const setup = fixture();
  try {
    const originalPath = join(setup.repository, "materials", "outline.pdf");
    const publicPath = "materials/lecture.pptx";
    const bytes = Buffer.from("reviewed raw slide asset\n");
    rmSync(originalPath);
    writeFileSync(join(setup.repository, publicPath), bytes);
    const manifestPath = join(setup.repository, "manifest.json");
    const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
    manifest.subjects[0].assets[0] = {
      role: "课件PPT",
      title: "离散数学_课件_第一章.pptx",
      publicPath,
      bytes: bytes.length,
      sha256: sha256(bytes),
    };
    writeFileSync(manifestPath, JSON.stringify(manifest));
    const accepted = gitCommit(setup.repository, "raw slide asset");

    const result = runPreparation(setup, { sha: accepted });
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(
      readFileSync(join(setup.candidate, "public", publicPath)),
      bytes,
    );
    assert.equal(existsSync(join(setup.candidate, "slides")), false);
    const release = JSON.parse(readFileSync(join(setup.candidate, "release.json"), "utf8"));
    assert.equal(Object.hasOwn(release, "slides_root"), false);
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});

test("refuses a candidate directory below the served public root, including through a symlink", () => {
  const setup = fixture();
  try {
    const servedRoot = join(setup.root, "served-public");
    const servedAlias = join(setup.root, "served-alias");
    mkdirSync(servedRoot);
    symlinkSync(servedRoot, servedAlias, "dir");
    setup.env = { HENUKIT_MATERIALS_PUBLIC_ROOT: servedRoot };

    for (const candidate of [
      join(servedRoot, "candidate-direct"),
      join(servedAlias, "candidate-through-alias"),
    ]) {
      const result = runPreparation(setup, { candidate });
      assert.notEqual(result.status, 0, candidate);
      assert.match(result.stderr, /served public tree/, candidate);
      assert.equal(existsSync(join(candidate, "checkout")), false, candidate);
      assert.equal(existsSync(join(candidate, "READY")), false, candidate);
    }
  } finally {
    rmSync(setup.root, { recursive: true, force: true });
  }
});
