#!/usr/bin/env node
/**
 * 资料库规范化归一化导入器。
 *
 * 把 HENU-Final-Review 仓库的 manifest.json 归一化成对遗留 Study 库
 * (courses/materials 表)的幂等 upsert SQL。SQL 输出到 stdout,可直接
 * 管道给 psql:
 *
 *   node import-henukit-materials.mjs --manifest manifest.json \
 *     --slides-dir ./slides | psql "$STUDY_DATABASE_URL" -v ON_ERROR_STOP=1 -f -
 *
 * 归一化规则:
 *   - 只导入 role 不以 "待复核" 开头的资产,与镜像脚本保持同一条线;
 *   - role -> portal material type: 复习讲义->path 课件PPT->slides
 *     课件资料->note 往年真题->exam 题库练习->mock 笔记总结->note;
 *   - 标题去掉 "{科目}_{类型标记}_" 前缀和扩展名;
 *   - 描述优先取资产的 sourceNote,否则取科目的 note;
 *   - 同一 storage_key 的资产幂等 upsert(局部唯一索引,deleted_at IS NULL);
 *   - 幻灯片:课件PPT 资产若 --slides-dir 下有 <storage_key>.json,写入
 *     materials.slides(jsonb);
 *   - 不再出现在 manifest 中的镜像资料(有 sha256 的行)置为 archived。
 *
 * 依赖仅 Node 内置模块;数据库写入由调用方负责(psql / 驱动脚本)。
 */

import { createHash } from "node:crypto";
import { readFileSync, readdirSync } from "node:fs";
import { join, basename, extname } from "node:path";

const DEFAULT_SCHOOL = "河南大学";

/**
 * 读取 {publicPath: login} 归属表。
 *
 * 这些是别人的笔记、真题和课件，署名应当归实际贡献者，而不是硬编码的
 * "HENU Kit"。归属由 fetch-henukit-contributors.mjs 从仓库提交历史得出；
 * 表缺失或损坏时返回空表，资料不署名而不是让整次导入失败。
 */
function loadContributors(path) {
  if (!path) return {};
  try {
    const parsed = JSON.parse(readFileSync(path, "utf8"));
    return parsed && typeof parsed === "object" ? parsed : {};
  } catch (error) {
    console.error(`contributor attribution unavailable (${error.message}); materials stay uncredited`);
    return {};
  }
}
const DEFAULT_COLLEGE = "软件学院";
const DEFAULT_MAJOR = "软件工程";
const DEFAULT_GRADE = "通用";

// role -> portal MaterialType
// 讲义不是学习路径，课件资料也不是笔记：按资料本身的形态归类，学生筛选才对得上。
const ROLE_TYPE = {
  复习讲义: "note",
  课件PPT: "slides",
  课件资料: "slides",
  往年真题: "exam",
  题库练习: "mock",
  笔记总结: "note",
};

// 标题里可剥离的类型标记,长标记在前。
const TITLE_MARKERS = [
  "复习讲义",
  "课件PPT",
  "课件资料",
  "题库练习",
  "往年真题",
  "笔记总结",
  "课件",
  "真题",
  "笔记",
  "复习",
];

const args = process.argv.slice(2);
const options = {
  manifest: process.env.HENUKIT_MATERIALS_MANIFEST || "",
  slidesDir: "",
  contributors: "",
  school: DEFAULT_SCHOOL,
  college: DEFAULT_COLLEGE,
  major: DEFAULT_MAJOR,
  grade: DEFAULT_GRADE,
};

function usage() {
  console.error(`usage: import-henukit-materials.mjs --manifest <manifest.json> [options]

  --manifest PATH   HENU-Final-Review manifest.json (or HENUKIT_MATERIALS_MANIFEST)
  --slides-dir DIR  幻灯片 JSON 目录(可选,课件PPT 资产按 <storage_key>.json 取)
  --contributors F  贡献者归属 JSON(可选,{publicPath: login});缺省则不署名
  --school NAME     规范学校名(默认: ${DEFAULT_SCHOOL})
  --college NAME    规范学院名(默认: ${DEFAULT_COLLEGE})
  --major NAME      规范专业名(默认: ${DEFAULT_MAJOR})
  --grade NAME      规范年级(默认: ${DEFAULT_GRADE})
  --help

SQL 输出到 stdout,统计摘要输出到 stderr。`);
}

for (let i = 0; i < args.length; i += 1) {
  const arg = args[i];
  if (arg === "--help" || arg === "-h") {
    usage();
    process.exit(0);
  }
  if (arg === "--manifest" || arg === "--slides-dir" || arg === "--contributors" || arg === "--school" || arg === "--college" || arg === "--major" || arg === "--grade") {
    const value = args[i + 1];
    if (value === undefined) {
      console.error(`missing value for ${arg}`);
      usage();
      process.exit(2);
    }
    const key = arg === "--slides-dir" ? "slidesDir" : arg.slice(2);
    options[key] = value;
    i += 1;
    continue;
  }
  console.error(`unknown option: ${arg}`);
  usage();
  process.exit(2);
}

if (!options.manifest) {
  console.error("--manifest is required");
  usage();
  process.exit(2);
}

const sqlEscape = (value) => String(value).replace(/'/g, "''");
const q = (value) => `'${sqlEscape(value)}'`;
const slugOf = (name) =>
  `subject-${createHash("sha256").update(name).digest("hex").slice(0, 12)}`;

function normalizeTitle(courseName, role, raw) {
  let title = raw;
  if (title.startsWith(`${courseName}_`)) {
    title = title.slice(courseName.length + 1);
  }
  if (title.startsWith(`${role}_`)) {
    title = title.slice(role.length + 1);
  } else {
    for (const marker of TITLE_MARKERS) {
      if (title.startsWith(`${marker}_`)) {
        title = title.slice(marker.length + 1);
        break;
      }
    }
  }
  if (extname(title)) {
    title = title.slice(0, -extname(title).length);
  }
  return title.trim();
}

function loadManifest(path) {
  const raw = readFileSync(path, "utf8");
  const manifest = JSON.parse(raw);
  if (!Array.isArray(manifest.subjects)) {
    throw new Error("manifest.subjects must be an array");
  }
  for (const subject of manifest.subjects) {
    if (!subject.name || !Array.isArray(subject.assets)) {
      throw new Error(`subject ${subject.name || "<unnamed>"} has no assets array`);
    }
  }
  return manifest;
}

function loadSlidesIndex(slidesDir) {
  const index = new Map();
  if (!slidesDir) return index;
  const walk = (dir) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(path);
      } else if (entry.name.endsWith(".json")) {
        try {
          // 文件名是 <storage_key>.json,内容键按 publicPath 命中。
          const key = path.slice(slidesDir.length + 1, -5);
          index.set(key, JSON.parse(readFileSync(path, "utf8")));
        } catch {
          // 单个幻灯片文件损坏不影响整体导入。
        }
      }
    }
  };
  walk(slidesDir);
  return index;
}

function main() {
  const manifest = loadManifest(options.manifest);
  const slidesIndex = loadSlidesIndex(options.slidesDir);
  const contributors = loadContributors(options.contributors);

  const schoolSlug = slugOf(`school:${options.school}`);
  const collegeKey = `${schoolSlug}:${options.college}`;
  const majorKey = `${collegeKey}:${options.major}`;
  const collegeSlug = slugOf(collegeKey);
  const majorSlug = slugOf(majorKey);

  const subjects = manifest.subjects;
  const rows = [];
  const duplicateSha = new Map();

  let totalAssets = 0;
  let skippedPending = 0;

  for (const subject of subjects) {
    for (const asset of subject.assets) {
      totalAssets += 1;
      const role = asset.role || "";
      if (role.startsWith("待复核")) {
        skippedPending += 1;
        continue;
      }
      const publicPath = asset.publicPath || "";
      if (!publicPath) continue;
      if (!asset.sha256 || !Number.isFinite(asset.bytes)) {
        console.error(`WARNING: skipping asset without sha256/bytes: ${publicPath}`);
        continue;
      }
      const existing = duplicateSha.get(asset.sha256);
      if (existing) {
        existing.push(publicPath);
      } else {
        duplicateSha.set(asset.sha256, [publicPath]);
      }

      const slides = slidesIndex.get(publicPath);
      rows.push({
        subject: subject.name,
        courseDescription: subject.note || "",
        role,
        type: ROLE_TYPE[role] || "note",
        title: normalizeTitle(subject.name, role, asset.title || basename(publicPath)),
        description: (asset.sourceNote || subject.note || "").slice(0, 1000),
        storageKey: publicPath,
        fileName: basename(publicPath),
        fileSize: asset.bytes,
        sha256: asset.sha256,
        slides: slides || null,
        contributor: contributors[publicPath] || "",
        reviewReason: asset.reviewStatus || "mirrored from HENU-Final-Review manifest",
      });
    }
  }

  const dupes = [...duplicateSha.entries()].filter(([, paths]) => paths.length > 1);

  const lines = [];
  lines.push("BEGIN;");
  lines.push("");
  lines.push("-- 归一化所需的补充列与幂等索引(重复执行安全)");
  lines.push("ALTER TABLE materials ADD COLUMN IF NOT EXISTS sha256 text;");
  lines.push("ALTER TABLE materials ADD COLUMN IF NOT EXISTS slides jsonb;");
  lines.push("ALTER TABLE materials ADD COLUMN IF NOT EXISTS contributor text;");
  lines.push(
    "CREATE UNIQUE INDEX IF NOT EXISTS materials_storage_key_active_idx ON materials (storage_key) WHERE deleted_at IS NULL;",
  );
  lines.push("");

  lines.push("-- 规范组织行(学校/学院/专业)");
  lines.push(
    `INSERT INTO schools (id, name, slug, email_domains, status, created_at, updated_at) VALUES (gen_random_uuid(), ${q(options.school)}, ${q(schoolSlug)}, NULL, 'published', now(), now()) ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, status = EXCLUDED.status, updated_at = now();`,
  );
  lines.push(
    `INSERT INTO colleges (id, school_id, name, status, created_at, updated_at) SELECT gen_random_uuid(), s.id, ${q(options.college)}, 'published', now(), now() FROM schools s WHERE s.slug = ${q(schoolSlug)} AND NOT EXISTS (SELECT 1 FROM colleges c WHERE c.school_id = s.id AND c.name = ${q(options.college)});`,
  );
  lines.push(
    `INSERT INTO majors (id, school_id, college_id, name, slug, status, created_at, updated_at) SELECT gen_random_uuid(), s.id, c.id, ${q(options.major)}, ${q(majorSlug)}, 'published', now(), now() FROM schools s JOIN colleges c ON c.school_id = s.id AND c.name = ${q(options.college)} WHERE s.slug = ${q(schoolSlug)} AND NOT EXISTS (SELECT 1 FROM majors m WHERE m.school_id = s.id AND m.college_id = c.id AND m.name = ${q(options.major)});`,
  );
  lines.push("");

  const courseNames = [...new Set(rows.map((row) => row.subject))];
  for (const name of courseNames) {
    const description = rows.find((row) => row.subject === name).courseDescription;
    lines.push(
      `INSERT INTO courses (id, school_id, college_id, major_id, grade, name, slug, description, status, created_at, updated_at) SELECT gen_random_uuid(), s.id, c.id, m.id, ${q(options.grade)}, ${q(name)}, ${q(slugOf(name))}, ${q(description.slice(0, 1000))}, 'published', now(), now() FROM schools s JOIN colleges c ON c.school_id = s.id AND c.name = ${q(options.college)} JOIN majors m ON m.school_id = s.id AND m.college_id = c.id AND m.name = ${q(options.major)} WHERE s.slug = ${q(schoolSlug)} AND NOT EXISTS (SELECT 1 FROM courses x WHERE x.name = ${q(name)} AND x.deleted_at IS NULL);`,
    );
    lines.push(
      `UPDATE courses SET status = 'published', updated_at = now() WHERE name = ${q(name)} AND deleted_at IS NULL AND status = 'archived';`,
    );
  }
  lines.push("");

  for (const row of rows) {
    const slidesSql = row.slides ? q(JSON.stringify(row.slides)) : "NULL";
    lines.push(
      `INSERT INTO materials (id, course_id, title, type, description, storage_key, file_name, file_size, sha256, slides, contributor, access_level, status, reviewed_at, review_reason, created_at, updated_at) SELECT gen_random_uuid(), c.id, ${q(row.title)}, ${q(row.type)}, ${q(row.description)}, ${q(row.storageKey)}, ${q(row.fileName)}, ${row.fileSize}, ${q(row.sha256)}, ${slidesSql}::jsonb, ${q(row.contributor)}, 'free', 'published', now(), ${q(row.reviewReason)}, now(), now() FROM courses c WHERE c.name = ${q(row.subject)} AND c.deleted_at IS NULL ON CONFLICT (storage_key) WHERE deleted_at IS NULL DO UPDATE SET title = EXCLUDED.title, type = EXCLUDED.type, description = EXCLUDED.description, file_name = EXCLUDED.file_name, file_size = EXCLUDED.file_size, sha256 = EXCLUDED.sha256, slides = EXCLUDED.slides, contributor = EXCLUDED.contributor, access_level = EXCLUDED.access_level, status = EXCLUDED.status, updated_at = now();`,
    );
  }
  lines.push("");

  if (rows.length > 0) {
    const keys = rows.map((row) => q(row.storageKey)).join(", ");
    lines.push("-- 不再出现在 manifest 中的镜像资料(有 sha256 标记的行)下线");
    lines.push(
      `UPDATE materials SET status = 'archived', updated_at = now() WHERE status = 'published' AND deleted_at IS NULL AND sha256 IS NOT NULL AND storage_key NOT IN (${keys});`,
    );
  }
  lines.push("");
  lines.push("COMMIT;");
  lines.push("");

  process.stdout.write(lines.join("\n"));

  console.error(
    JSON.stringify({
      subjects: subjects.length,
      total_assets: totalAssets,
      skipped_pending_review: skippedPending,
      imported: rows.length,
      slides_converted: rows.filter((row) => row.slides).length,
      duplicate_sha256: dupes.map(([sha, paths]) => ({ sha256: sha, paths })),
    }),
  );
}

try {
  main();
} catch (error) {
  console.error(`import-henukit-materials: ${error.message}`);
  process.exit(1);
}
