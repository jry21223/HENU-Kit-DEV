#!/usr/bin/env node
/**
 * 资料库规范化归一化导入器。
 *
 * 把 HENU-Final-Review 仓库的 manifest.json 归一化成对遗留 Study 库
 * (courses/materials 表)的幂等 upsert SQL。SQL 输出到 stdout,可直接
 * 管道给 psql:
 *
 *   node import-henukit-materials.mjs --manifest manifest.json \
 *     --release-id <approved-release-id> | \
 *     PGSERVICEFILE=/etc/henukit-deploy/materials-postgresql.conf \
 *     PGSERVICE=henukit-materials psql -v ON_ERROR_STOP=1 -f -
 *
 * 归一化规则:
 *   - 只导入 role 不以 "待复核" 开头的资产,与镜像脚本保持同一条线;
 *   - role -> portal material type: 复习讲义->path 课件PPT->slides
 *     课件资料->note 往年真题->exam 题库练习->mock 笔记总结->note;
 *   - 标题去掉 "{科目}_{类型标记}_" 前缀和扩展名;
 *   - 描述优先取资产的 sourceNote,否则取科目的 note;
 *   - 同一 storage_key 的资产幂等 upsert(局部唯一索引,deleted_at IS NULL);
 *   - 在线预览关闭:所有资料（包括课件PPT）的 materials.slides 写为 NULL;
 *   - 受审 legacy inventory 与不再出现在新 release 中的镜像资料置为 archived。
 *
 * 依赖仅 Node 内置模块;数据库写入由调用方负责(psql / 驱动脚本)。
 */

import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { basename, extname } from "node:path";

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
  releaseId: "",
  legacyInventory: "",
  school: DEFAULT_SCHOOL,
  college: DEFAULT_COLLEGE,
  major: DEFAULT_MAJOR,
  grade: DEFAULT_GRADE,
};

function usage() {
  console.error(`usage: import-henukit-materials.mjs --manifest <manifest.json> [options]

  --manifest PATH   HENU-Final-Review manifest.json (or HENUKIT_MATERIALS_MANIFEST)
  --release-id ID   已批准的不可变发布 ID（写入公开 storage_key 时必需）
  --legacy-inventory PATH  首切前审核的旧镜像 storage_key inventory
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
  if (arg === "--manifest" || arg === "--release-id" || arg === "--legacy-inventory" || arg === "--school" || arg === "--college" || arg === "--major" || arg === "--grade") {
    const value = args[i + 1];
    if (value === undefined) {
      console.error(`missing value for ${arg}`);
      usage();
      process.exit(2);
    }
    const key = arg === "--release-id" ? "releaseId" : arg === "--legacy-inventory" ? "legacyInventory" : arg.slice(2);
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

if (!/^[a-f0-9]{40}-[a-f0-9]{16}$/.test(options.releaseId)) {
  console.error("release ID must be an approved immutable release identifier");
  process.exit(2);
}
if (!options.legacyInventory) {
  console.error("--legacy-inventory is required");
  process.exit(2);
}
let legacyInventory;
try {
  legacyInventory = JSON.parse(readFileSync(options.legacyInventory, "utf8"));
} catch {
  console.error("legacy inventory is not readable JSON");
  process.exit(2);
}
if (legacyInventory?.version !== 1 || !Array.isArray(legacyInventory.storage_keys)) {
  throw new Error("legacy inventory must contain version 1 storage_keys");
}
const legacyStorageKeys = [...new Set(legacyInventory.storage_keys)];
if (legacyStorageKeys.length !== legacyInventory.storage_keys.length || legacyStorageKeys.some((key) => {
  if (typeof key !== "string" || !key || key.startsWith("releases/") || key.includes("\\") || key.includes("\0")) return true;
  return key.split("/").some((segment) => !segment || segment === "." || segment === ".." || segment.startsWith("."));
})) {
  throw new Error("legacy inventory contains an unsafe or duplicate storage key");
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


function appendReviewedSchemaPreflight(lines) {
  // This is deliberately outside BEGIN: a missing prerequisite must stop psql
  // before any catalog row can change. It is read-only; schema installation is
  // a separately reviewed owner operation.
  // Keep pg_catalog first so unqualified built-ins cannot be shadowed by a
  // writable application schema; public remains the only application schema
  // used by the preflight and generated DML below.
  lines.push("SET search_path TO pg_catalog, public;");
  lines.push("");
  lines.push("-- Required reviewed schema preflight; runtime import performs no DDL.");
  lines.push("SELECT (");
  lines.push("  to_regclass('public.materials') IS NOT NULL");
  lines.push("  AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'materials' AND column_name = 'storage_key')");
  lines.push("  AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'materials' AND column_name = 'deleted_at')");
  lines.push("  AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'materials' AND column_name = 'sha256' AND data_type = 'text')");
  lines.push("  AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'materials' AND column_name = 'slides' AND udt_name = 'jsonb')");
  lines.push("  AND EXISTS (SELECT 1 FROM pg_class material_table JOIN pg_namespace material_namespace ON material_namespace.oid = material_table.relnamespace JOIN pg_index material_index ON material_index.indrelid = material_table.oid JOIN pg_class index_class ON index_class.oid = material_index.indexrelid WHERE material_namespace.nspname = 'public' AND material_table.relname = 'materials' AND index_class.relname = 'materials_storage_key_active_idx' AND material_index.indisunique AND material_index.indisvalid AND material_index.indisready AND material_index.indislive AND material_index.indnkeyatts = 1 AND EXISTS (SELECT 1 FROM unnest(material_index.indkey) WITH ORDINALITY AS indexed_key(attnum, key_position) JOIN pg_attribute indexed_attribute ON indexed_attribute.attrelid = material_table.oid AND indexed_attribute.attnum = indexed_key.attnum WHERE indexed_key.key_position = 1 AND indexed_attribute.attname = 'storage_key' AND NOT indexed_attribute.attisdropped) AND pg_get_expr(material_index.indpred, material_index.indrelid) = '(deleted_at IS NULL)')");
  lines.push(") AS henukit_materials_schema_ready;");
  lines.push("\\gset");
  lines.push("\\if :henukit_materials_schema_ready");
  lines.push("\\else");
  lines.push("\\echo 'Materials import refused: apply the reviewed materials schema prerequisite, then rerun this command.'");
  lines.push("\\set ON_ERROR_STOP on");
  lines.push("SELECT 1 / 0 AS henukit_materials_schema_preflight_failed;");
  lines.push("\\endif");
  lines.push("");
}

function main() {
  const manifest = loadManifest(options.manifest);

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

      rows.push({
        subject: subject.name,
        courseDescription: subject.note || "",
        role,
        type: ROLE_TYPE[role] || "note",
        title: normalizeTitle(subject.name, role, asset.title || basename(publicPath)),
        description: (asset.sourceNote || subject.note || "").slice(0, 1000),
        storageKey: `releases/${options.releaseId}/${publicPath}`,
        legacyStorageKey: publicPath,
        fileName: basename(publicPath),
        fileSize: asset.bytes,
        sha256: asset.sha256,
        reviewReason: asset.reviewStatus || "mirrored from HENU-Final-Review manifest",
      });
    }
  }

  const dupes = [...duplicateSha.entries()].filter(([, paths]) => paths.length > 1);

  const lines = [];
  appendReviewedSchemaPreflight(lines);
  lines.push("BEGIN;");
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
    lines.push(
      `INSERT INTO materials (id, course_id, title, type, description, storage_key, file_name, file_size, sha256, slides, access_level, status, reviewed_at, review_reason, created_at, updated_at) SELECT gen_random_uuid(), c.id, ${q(row.title)}, ${q(row.type)}, ${q(row.description)}, ${q(row.storageKey)}, ${q(row.fileName)}, ${row.fileSize}, ${q(row.sha256)}, NULL::jsonb, 'free', 'published', now(), ${q(row.reviewReason)}, now(), now() FROM courses c WHERE c.name = ${q(row.subject)} AND c.deleted_at IS NULL ON CONFLICT (storage_key) WHERE deleted_at IS NULL DO UPDATE SET title = EXCLUDED.title, type = EXCLUDED.type, description = EXCLUDED.description, file_name = EXCLUDED.file_name, file_size = EXCLUDED.file_size, sha256 = EXCLUDED.sha256, slides = EXCLUDED.slides, access_level = EXCLUDED.access_level, status = EXCLUDED.status, updated_at = now();`,
    );
  }
  lines.push("");

  const reviewedLegacyKeys = [...new Set([...legacyStorageKeys, ...rows.map((row) => row.legacyStorageKey)])];
  if (reviewedLegacyKeys.length > 0) {
    const legacyKeys = reviewedLegacyKeys.map(q).join(", ");
    lines.push("-- 首切迁移：只归档受审 legacy inventory 与本次 manifest 精确列出的旧裸路径");
    lines.push(
      `UPDATE materials SET status = 'archived', updated_at = now() WHERE status = 'published' AND deleted_at IS NULL AND storage_key !~ '^releases/[a-f0-9]{40}-[a-f0-9]{16}/' AND storage_key IN (${legacyKeys});`,
    );
  }
  lines.push("");

  const retainedKeys = rows.length > 0
    ? ` AND storage_key NOT IN (${rows.map((row) => q(row.storageKey)).join(", ")})`
    : "";
  lines.push("-- 只下线由不可变 release storage_key 明确标记、且不属于本次 release 的镜像资料");
  lines.push(
    `UPDATE materials SET status = 'archived', updated_at = now() WHERE status = 'published' AND deleted_at IS NULL AND storage_key ~ '^releases/[a-f0-9]{40}-[a-f0-9]{16}/'${retainedKeys};`,
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
      online_preview: "disabled",
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
