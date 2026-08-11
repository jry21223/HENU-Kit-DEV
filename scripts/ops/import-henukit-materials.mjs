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
const DEFAULT_COLLEGE = "软件学院";
const DEFAULT_MAJOR = "软件工程";
const DEFAULT_GRADE = "通用";

// role -> portal MaterialType
const ROLE_TYPE = {
  复习讲义: "path",
  课件PPT: "slides",
  课件资料: "note",
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
  school: DEFAULT_SCHOOL,
  college: DEFAULT_COLLEGE,
  major: DEFAULT_MAJOR,
  grade: DEFAULT_GRADE,
  syncSha: "",
  delivery: "",
};

function usage() {
  console.error(`usage: import-henukit-materials.mjs --manifest <manifest.json> [options]

  --manifest PATH   HENU-Final-Review manifest.json (or HENUKIT_MATERIALS_MANIFEST)
  --slides-dir DIR  幻灯片 JSON 目录(可选,课件PPT 资产按 <storage_key>.json 取)
  --school NAME     规范学校名(默认: ${DEFAULT_SCHOOL})
  --college NAME    规范学院名(默认: ${DEFAULT_COLLEGE})
  --major NAME      规范专业名(默认: ${DEFAULT_MAJOR})
  --grade NAME      规范年级(默认: ${DEFAULT_GRADE})
  --sync-sha SHA    本次镜像的完整 lowercase Git SHA(必填)
  --delivery ID     本次 webhook delivery ID 或显式 manual ID(必填)
  --help

SQL 输出到 stdout,统计摘要输出到 stderr。`);
}

for (let i = 0; i < args.length; i += 1) {
  const arg = args[i];
  if (arg === "--help" || arg === "-h") {
    usage();
    process.exit(0);
  }
  if (arg === "--manifest" || arg === "--slides-dir" || arg === "--school" || arg === "--college" || arg === "--major" || arg === "--grade" || arg === "--sync-sha" || arg === "--delivery") {
    const value = args[i + 1];
    if (value === undefined) {
      console.error(`missing value for ${arg}`);
      usage();
      process.exit(2);
    }
    const key = arg === "--slides-dir" ? "slidesDir" : arg === "--sync-sha" ? "syncSha" : arg.slice(2);
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
if (!/^[0-9a-f]{40}$/.test(options.syncSha)) {
  console.error("--sync-sha must be a full lowercase Git SHA");
  process.exit(2);
}
if (!/^[A-Za-z0-9][A-Za-z0-9-]{0,127}$/.test(options.delivery)) {
  console.error("--delivery is invalid");
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

function validateSlidesPayload(payload, path) {
  if (payload === null || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error(`derived slides ${path} must be an object`);
  }
  if (!Array.isArray(payload.slides)) {
    throw new Error(`derived slides ${path}: slides must be an array`);
  }
  for (let index = 0; index < payload.slides.length; index += 1) {
    const slide = payload.slides[index];
    if (slide === null || typeof slide !== "object" || Array.isArray(slide)) {
      throw new Error(`derived slides ${path}: slide ${index} must be an object`);
    }
    if (typeof slide.title !== "string") {
      throw new Error(`derived slides ${path}: slide ${index} title must be a string`);
    }
    if (!Array.isArray(slide.blocks)) {
      throw new Error(`derived slides ${path}: slide ${index} blocks must be an array`);
    }
    if (!slide.blocks.every((block) => typeof block === "string")) {
      throw new Error(`derived slides ${path}: slide ${index} blocks must contain only strings`);
    }
  }
  return payload.slides;
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
        // 文件名是 <storage_key>.json,内容键按 publicPath 命中。任何损坏的
        // 派生文件都使整批导入失败，避免 public 文件与 catalogue 部分发布。
        const key = path.slice(slidesDir.length + 1, -5);
        const payload = JSON.parse(readFileSync(path, "utf8"));
        index.set(key, validateSlidesPayload(payload, path));
      }
    }
  };
  walk(slidesDir);
  return index;
}

function main() {
  const manifest = loadManifest(options.manifest);
  const slidesIndex = loadSlidesIndex(options.slidesDir);

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
      const publicPath = asset.publicPath || "";
      if (!Number.isSafeInteger(asset.bytes) || asset.bytes < 0) {
        throw new Error(`asset.bytes must be a non-negative safe integer: ${publicPath || "<missing-publicPath>"}`);
      }
      const role = asset.role || "";
      if (role.startsWith("待复核")) {
        skippedPending += 1;
        continue;
      }
      if (!publicPath) continue;
      if (!asset.sha256) {
        console.error(`WARNING: skipping asset without sha256: ${publicPath}`);
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
        reviewReason: asset.reviewStatus || "mirrored from HENU-Final-Review manifest",
      });
    }
  }

  const dupes = [...duplicateSha.entries()].filter(([, paths]) => paths.length > 1);

  const lines = [];
  lines.push("BEGIN;");
  lines.push("");
  lines.push("-- 规范组织行(学校/学院/专业)");
  lines.push(
    `INSERT INTO public.schools (id, name, slug, email_domains, status, created_at, updated_at) VALUES (gen_random_uuid(), ${q(options.school)}, ${q(schoolSlug)}, NULL, 'published', now(), now()) ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, status = EXCLUDED.status, updated_at = now();`,
  );
  lines.push(
    `INSERT INTO public.colleges (id, school_id, name, status, created_at, updated_at) SELECT gen_random_uuid(), s.id, ${q(options.college)}, 'published', now(), now() FROM public.schools s WHERE s.slug = ${q(schoolSlug)} AND NOT EXISTS (SELECT 1 FROM public.colleges c WHERE c.school_id = s.id AND c.name = ${q(options.college)});`,
  );
  lines.push(
    `INSERT INTO public.majors (id, school_id, college_id, name, slug, status, created_at, updated_at) SELECT gen_random_uuid(), s.id, c.id, ${q(options.major)}, ${q(majorSlug)}, 'published', now(), now() FROM public.schools s JOIN public.colleges c ON c.school_id = s.id AND c.name = ${q(options.college)} WHERE s.slug = ${q(schoolSlug)} AND NOT EXISTS (SELECT 1 FROM public.majors m WHERE m.school_id = s.id AND m.college_id = c.id AND m.name = ${q(options.major)});`,
  );
  lines.push("");

  const courseNames = [...new Set(rows.map((row) => row.subject))];
  for (const name of courseNames) {
    const description = rows.find((row) => row.subject === name).courseDescription;
    lines.push(
      `INSERT INTO public.courses (id, school_id, college_id, major_id, grade, name, slug, description, status, created_at, updated_at) SELECT gen_random_uuid(), s.id, c.id, m.id, ${q(options.grade)}, ${q(name)}, ${q(slugOf(name))}, ${q(description.slice(0, 1000))}, 'published', now(), now() FROM public.schools s JOIN public.colleges c ON c.school_id = s.id AND c.name = ${q(options.college)} JOIN public.majors m ON m.school_id = s.id AND m.college_id = c.id AND m.name = ${q(options.major)} WHERE s.slug = ${q(schoolSlug)} AND NOT EXISTS (SELECT 1 FROM public.courses x WHERE x.school_id = s.id AND x.college_id = c.id AND x.major_id = m.id AND x.grade = ${q(options.grade)} AND x.name = ${q(name)} AND x.deleted_at IS NULL);`,
    );
    lines.push(
      `UPDATE public.courses x SET status = 'published', updated_at = now() FROM public.schools s JOIN public.colleges c ON c.school_id = s.id AND c.name = ${q(options.college)} JOIN public.majors m ON m.school_id = s.id AND m.college_id = c.id AND m.name = ${q(options.major)} WHERE s.slug = ${q(schoolSlug)} AND x.school_id = s.id AND x.college_id = c.id AND x.major_id = m.id AND x.grade = ${q(options.grade)} AND x.name = ${q(name)} AND x.deleted_at IS NULL AND x.status = 'archived';`,
    );
  }
  lines.push("");

  for (const row of rows) {
    const slidesSql = row.slides ? q(JSON.stringify(row.slides)) : "NULL";
    lines.push(
      `INSERT INTO public.materials (id, course_id, title, type, description, storage_key, file_name, file_size, sha256, slides, access_level, status, reviewed_at, review_reason, created_at, updated_at) SELECT gen_random_uuid(), x.id, ${q(row.title)}, ${q(row.type)}, ${q(row.description)}, ${q(row.storageKey)}, ${q(row.fileName)}, ${row.fileSize}, ${q(row.sha256)}, ${slidesSql}::jsonb, 'free', 'published', now(), ${q(row.reviewReason)}, now(), now() FROM public.schools s JOIN public.colleges c ON c.school_id = s.id AND c.name = ${q(options.college)} JOIN public.majors m ON m.school_id = s.id AND m.college_id = c.id AND m.name = ${q(options.major)} JOIN public.courses x ON x.school_id = s.id AND x.college_id = c.id AND x.major_id = m.id AND x.grade = ${q(options.grade)} AND x.name = ${q(row.subject)} AND x.deleted_at IS NULL WHERE s.slug = ${q(schoolSlug)} ON CONFLICT (storage_key) WHERE deleted_at IS NULL DO UPDATE SET course_id = EXCLUDED.course_id, title = EXCLUDED.title, type = EXCLUDED.type, description = EXCLUDED.description, file_name = EXCLUDED.file_name, file_size = EXCLUDED.file_size, sha256 = EXCLUDED.sha256, slides = EXCLUDED.slides, access_level = EXCLUDED.access_level, status = EXCLUDED.status, updated_at = now();`,
    );
  }
  lines.push("");

  if (rows.length > 0) {
    const keys = rows.map((row) => q(row.storageKey)).join(", ");
    lines.push("-- 不再出现在 manifest 中的镜像资料(有 sha256 标记的行)下线");
    lines.push(
      `UPDATE public.materials SET status = 'archived', updated_at = now() WHERE status = 'published' AND deleted_at IS NULL AND sha256 IS NOT NULL AND storage_key NOT IN (${keys});`,
    );
  } else {
    lines.push("-- manifest 当前没有可发布资产;下线全部镜像资料");
    lines.push(
      "UPDATE public.materials SET status = 'archived', updated_at = now() WHERE status = 'published' AND deleted_at IS NULL AND sha256 IS NOT NULL;",
    );
  }
  lines.push("");
  lines.push("-- 与资料行同事务提交的恢复判据;文件发布 journal 以此消除 COMMIT 歧义");
  lines.push(
    `INSERT INTO public.henukit_materials_sync_state (singleton, synced_sha, delivery, updated_at) VALUES (1, ${q(options.syncSha)}, ${q(options.delivery)}, now()) ON CONFLICT (singleton) DO UPDATE SET synced_sha = EXCLUDED.synced_sha, delivery = EXCLUDED.delivery, updated_at = EXCLUDED.updated_at;`,
  );
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
