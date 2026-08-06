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

function runImport({ slidesDir } = {}) {
  const dir = mkdtempSync(join(tmpdir(), "henukit-import-"));
  const manifestPath = join(dir, "manifest.json");
  writeFileSync(manifestPath, JSON.stringify(MANIFEST));
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
  const result = spawnSync(process.execPath, [script, ...args], {
    encoding: "utf8",
  });
  assert.equal(result.status, 0, result.stderr);
  rmSync(dir, { recursive: true, force: true });
  return { stdout: result.stdout, stderr: result.stderr };
}

test("import emits idempotent transaction SQL", () => {
  const { stdout } = runImport();
  assert.match(stdout, /^BEGIN;/m);
  assert.match(stdout, /COMMIT;$/m);
  assert.match(stdout, /ALTER TABLE materials ADD COLUMN IF NOT EXISTS sha256 text;/);
  assert.match(stdout, /ALTER TABLE materials ADD COLUMN IF NOT EXISTS slides jsonb;/);
  assert.match(stdout, /ON CONFLICT \(storage_key\) WHERE deleted_at IS NULL DO UPDATE/);
});

test("normalizes titles: strips course prefix, role marker and extension", () => {
  const { stdout } = runImport();
  // 标题列已归一化(去掉科目前缀/类型标记/扩展名),type 紧随其后。
  assert.match(stdout, /'考前复习知识点讲义', 'note'/);
  assert.match(stdout, /'D10-1二重积分概念', 'slides'/);
  // 原始文件名不得作为标题出现(它仍会以 storage_key 形式出现)。
  assert.doesNotMatch(stdout, /'高等数学A（二）_考前复习知识点讲义\.pdf', 'note'/);
});

test("maps roles to portal types", () => {
  const { stdout } = runImport();
  // VALUES 列序:id, course_id, title, type, description, ...
  // 讲义归 note 而不是学习路径，课件资料归 slides 而不是笔记：
  // 按资料形态归类，学生的类型筛选才对得上。
  assert.match(stdout, /'考前复习知识点讲义', 'note', '科目简介'/);
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
  assert.match(stdout, /storage_key NOT IN \('高等数学A（二）\/复习讲义\//);
  assert.match(stdout, /AND sha256 IS NOT NULL/);
});

test("rejects a manifest without subjects", () => {
  const dir = mkdtempSync(join(tmpdir(), "henukit-bad-"));
  const manifestPath = join(dir, "manifest.json");
  writeFileSync(manifestPath, JSON.stringify({ version: 1 }));
  const result = spawnSync(process.execPath, [script, "--manifest", manifestPath]);
  assert.notEqual(result.status, 0);
  assert.match(String(result.stderr), /manifest\.subjects must be an array/);
  rmSync(dir, { recursive: true, force: true });
});
