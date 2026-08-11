import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const script = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "import-henukit-materials.mjs",
);

const MANIFEST = {
  version: 1,
  generatedAt: "2026-08-05T00:00:00.000Z",
  subjects: [
    {
      name: "高等数学A（二）",
      note: "科目简介",
      assets: [
        {
          subject: "高等数学A（二）",
          role: "复习讲义",
          title: "高等数学A（二）_考前复习知识点讲义.pdf",
          publicPath: "高等数学A（二）/复习讲义/高等数学A（二）_考前复习知识点讲义.pdf",
          bytes: 237837,
          sha256: "8d78ba648b48cf8b36d306822a16f0bdbed08f74e3507bbaf65690f7a0eb93a6",
        },
        {
          subject: "高等数学A（二）",
          role: "课件PPT",
          title: "高等数学A（二）_课件_D10-1二重积分概念.ppt",
          publicPath: "高等数学A（二）/课件PPT/高等数学A（二）_课件_D10-1二重积分概念.ppt",
          bytes: 2318336,
          sha256: "66aa36fe4983fefddbd31eb9eb48a1730467fdf7af054b74b6f950e9ff7631f2",
          sourceNote: "老师课件,含引号 ' 与单引号''",
        },
        {
          subject: "高等数学A（二）",
          role: "待复核资料",
          title: "高等数学A（二）_待复核_第八章自测.docx",
          publicPath: "高等数学A（二）/待复核资料/高等数学A（二）_待复核_第八章自测.docx",
          reviewStatus: "needs_review",
          bytes: 33967,
          sha256: "47b88cdceab421b479ed2d2d28b6daa86e60c37fd9b5ae967576fc25094e8220",
        },
      ],
    },
  ],
};

function runImport({ slidesDir, releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`, manifest = MANIFEST, legacyKeys = [] } = {}) {
  const dir = mkdtempSync(join(tmpdir(), "henukit-import-"));
  const manifestPath = join(dir, "manifest.json");
  const legacyInventoryPath = join(dir, "legacy-inventory.json");
  writeFileSync(manifestPath, JSON.stringify(manifest));
  writeFileSync(legacyInventoryPath, JSON.stringify({ version: 1, storage_keys: legacyKeys }));
  const args = ["--manifest", manifestPath];
  if (slidesDir) {
    const slidesPath = join(
      slidesDir,
      "高等数学A（二）/课件PPT/高等数学A（二）_课件_D10-1二重积分概念.ppt.json",
    );
    mkdirSync(dirname(slidesPath), { recursive: true });
    writeFileSync(
      slidesPath,
      JSON.stringify({
        slides: [
          { title: "二重积分概念", blocks: ["定义", "几何意义"] },
        ],
      }),
    );
    args.push("--slides-dir", slidesDir);
  }
  args.push("--release-id", releaseID);
  args.push("--legacy-inventory", legacyInventoryPath);
  const result = spawnSync(process.execPath, [script, ...args], {
    encoding: "utf8",
  });
  assert.equal(result.status, 0, result.stderr);
  rmSync(dir, { recursive: true, force: true });
  return { stdout: result.stdout, stderr: result.stderr };
}

test("prefixes storage keys with the immutable published release path", () => {
  const releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`;
  const { stdout } = runImport({ releaseID });
  assert.match(stdout, new RegExp(`releases/${releaseID}/高等数学A（二）/复习讲义/`));
  assert.match(stdout, new RegExp(`storage_key NOT IN \\('releases/${releaseID}/`));
});

test("rejects an invalid or mutable release ID", () => {
  const dir = mkdtempSync(join(tmpdir(), "henukit-prefix-"));
  const manifestPath = join(dir, "manifest.json");
  writeFileSync(manifestPath, JSON.stringify(MANIFEST));
  for (const releaseID of ["current", "../releases", "/absolute"]) {
    const result = spawnSync(process.execPath, [script, "--manifest", manifestPath, "--release-id", releaseID], { encoding: "utf8" });
    assert.notEqual(result.status, 0, releaseID);
    assert.match(result.stderr, /release ID/i);
  }
  rmSync(dir, { recursive: true, force: true });
});

test("import requires the reviewed schema and emits only transactional DML", () => {
  const { stdout } = runImport();
  assert.match(stdout, /henukit_materials_schema_ready/);
  assert.match(stdout, /materials_storage_key_active_idx/);
  assert.match(stdout, /material_index\.indnkeyatts = 1/);
  assert.match(stdout, /unnest\(material_index\.indkey\)/);
  assert.match(stdout, /apply the reviewed materials schema prerequisite/i);
  assert.doesNotMatch(stdout, /\b(?:ALTER|CREATE|DROP)\b/i);
  assert.match(stdout, /^BEGIN;/m);
  assert.match(stdout, /COMMIT;$/m);
  assert.match(stdout, /ON CONFLICT \(storage_key\) WHERE deleted_at IS NULL DO UPDATE/);
});

test("normalizes titles: strips course prefix, role marker and extension", () => {
  const { stdout } = runImport();
  // 标题列已归一化(去掉科目前缀/类型标记/扩展名),type 紧随其后。
  assert.match(stdout, /'考前复习知识点讲义', 'path'/);
  assert.match(stdout, /'D10-1二重积分概念', 'slides'/);
  // 原始文件名不得作为标题出现(它仍会以 storage_key 形式出现)。
  assert.doesNotMatch(stdout, /'高等数学A（二）_考前复习知识点讲义\.pdf', 'path'/);
});

test("maps roles to portal types", () => {
  const { stdout } = runImport();
  // VALUES 列序:id, course_id, title, type, description, ...
  assert.match(stdout, /'考前复习知识点讲义', 'path', '科目简介'/);
  assert.match(stdout, /'D10-1二重积分概念', 'slides', '老师课件/);
});

test("skips assets whose role starts with 待复核", () => {
  const { stdout, stderr } = runImport();
  assert.doesNotMatch(stdout, /第八章自测/);
  assert.match(stderr, /"skipped_pending_review":1/);
});

test("quotes single quotes in text values", () => {
  const { stdout } = runImport();
  assert.match(stdout, /含引号 '' 与单引号''''/);
});

test("attaches converted slides jsonb for slides assets", () => {
  const dir = mkdtempSync(join(tmpdir(), "henukit-slides-"));
  const { stdout } = runImport({ slidesDir: dir });
  assert.match(stdout, /"slides":\[\{"title":"二重积分概念".*\]\}'::jsonb/);
  assert.match(stdout, /slides = EXCLUDED\.slides/);
  rmSync(dir, { recursive: true, force: true });
});

test("deactivates mirror rows no longer present in the manifest", () => {
  const { stdout } = runImport();
  assert.match(stdout, /storage_key !~ '\^releases\/\[a-f0-9\]\{40\}-\[a-f0-9\]\{16\}\/' AND storage_key IN \('高等数学A（二）\/复习讲义\//);
  assert.match(stdout, /storage_key NOT IN \('releases\/[a-f0-9-]+\/高等数学A（二）\/复习讲义\//);
  assert.match(stdout, /storage_key ~ '\^releases\/\[a-f0-9\]\{40\}-\[a-f0-9\]\{16\}\/'/);
  assert.doesNotMatch(stdout, /AND sha256 IS NOT NULL/);
});

test("an empty reviewed release archives prior mirrored releases without touching unrelated checksummed rows", () => {
  const manifest = {
    subjects: [{
      name: "软件工程",
      assets: [{
        role: "待复核资料",
        publicPath: "软件工程/待复核资料/草稿.pdf",
        reviewStatus: "needs_review",
        bytes: 1,
        sha256: "a".repeat(64),
      }],
    }],
  };
  const { stdout } = runImport({ manifest });
  assert.match(stdout, /UPDATE materials SET status = 'archived'/);
  assert.match(stdout, /storage_key ~ '\^releases\/\[a-f0-9\]\{40\}-\[a-f0-9\]\{16\}\/'/);
  assert.doesNotMatch(stdout, /sha256 IS NOT NULL/);
});

test("archives removed legacy mirror keys only when named by the reviewed legacy inventory", () => {
  const { stdout } = runImport({ legacyKeys: ["软件工程/已删除讲义.pdf"] });
  assert.match(stdout, /storage_key IN \([^;]*'软件工程\/已删除讲义\.pdf'/);
  assert.doesNotMatch(stdout, /sha256 IS NOT NULL/);
});

test("rejects a manifest without subjects", () => {
  const dir = mkdtempSync(join(tmpdir(), "henukit-bad-"));
  const manifestPath = join(dir, "manifest.json");
  writeFileSync(manifestPath, JSON.stringify({ version: 1 }));
  const legacyInventoryPath = join(dir, "legacy-inventory.json");
  writeFileSync(legacyInventoryPath, JSON.stringify({ version: 1, storage_keys: [] }));
  const result = spawnSync(process.execPath, [script, "--manifest", manifestPath, "--release-id", `${"a".repeat(40)}-${"b".repeat(16)}`, "--legacy-inventory", legacyInventoryPath]);
  assert.notEqual(result.status, 0);
  assert.match(String(result.stderr), /manifest\.subjects must be an array/);
  rmSync(dir, { recursive: true, force: true });
});
