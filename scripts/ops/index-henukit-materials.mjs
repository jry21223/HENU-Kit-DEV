#!/usr/bin/env node
// Index the mirrored course materials into the Study catalogue.
//
// The mirror puts files on disk; without this the Library page still lists
// nothing, because it reads courses and materials from the Study database.
// Both halves are derived from the same manifest.json, so the catalogue can be
// rebuilt from the repository at any time.
//
// Idempotent: rows are keyed by a UUID derived from the subject or the asset's
// public path, so re-running updates in place. Materials that disappear from
// the manifest are soft-deleted rather than dropped, keeping the audit trail
// the Study schema expects.
//
// Environment:
//   STUDY_DATABASE_URL  required, the Study database to index into
//   HENUKIT_MATERIALS_ROOT  default /opt/henukit-materials

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

const ROOT = process.env.HENUKIT_MATERIALS_ROOT ?? "/opt/henukit-materials";
const MANIFEST = path.join(ROOT, "repo", "manifest.json");

/**
 * A stable UUIDv5-style identifier derived from a name.
 *
 * The Study schema keys everything by uuid, and the manifest has no ids of its
 * own. Deriving them from the public path makes re-indexing idempotent without
 * needing to store a mapping anywhere.
 */
export function stableUUID(namespace, name) {
  const hash = createHash("sha1").update(`${namespace}:${name}`).digest();
  const bytes = Buffer.from(hash.subarray(0, 16));
  bytes[6] = (bytes[6] & 0x0f) | 0x50; // version 5
  bytes[8] = (bytes[8] & 0x3f) | 0x80; // RFC 4122 variant
  const hex = bytes.toString("hex");
  return [
    hex.slice(0, 8),
    hex.slice(8, 12),
    hex.slice(12, 16),
    hex.slice(16, 20),
    hex.slice(20, 32),
  ].join("-");
}

/**
 * Maps a repository role onto a Study material type.
 *
 * The five legacy types had no home for courseware, and 127 of the 182
 * published assets are lecture slides — filing them as "学长笔记" would make
 * the type filter actively misleading, so `courseware` was added.
 */
export function materialTypeForRole(role) {
  switch (role) {
    case "往年真题":
      return "past_exam";
    case "题库练习":
      return "mock_paper";
    case "课件PPT":
    case "课件资料":
      return "courseware";
    case "复习讲义":
    case "笔记总结":
      return "knowledge_note";
    default:
      return "knowledge_note";
  }
}

/** Assets awaiting provenance review are never published. */
export function isPublishable(asset) {
  return !String(asset?.role ?? "").startsWith("待复核");
}

/**
 * Flattens the manifest into the rows the Study catalogue needs.
 *
 * `storageKey` stays a repository-relative path; portal-api turns it into the
 * URL nginx serves, so moving the mirror never requires rewriting rows.
 */
export function planCatalogue(manifest) {
  const courses = [];
  const materials = [];

  for (const subject of manifest.subjects ?? []) {
    const publishable = (subject.assets ?? []).filter(isPublishable);
    if (publishable.length === 0) continue;

    const courseId = stableUUID("henukit.course", subject.name);
    courses.push({
      id: courseId,
      name: subject.name,
      slug: subject.name,
      description: subject.note ?? "",
    });

    for (const asset of publishable) {
      materials.push({
        id: stableUUID("henukit.material", asset.publicPath),
        courseId,
        title: asset.title,
        type: materialTypeForRole(asset.role),
        description: asset.role ?? "",
        storageKey: asset.publicPath,
        fileName: path.basename(asset.publicPath),
        fileSize: Number(asset.bytes ?? 0),
      });
    }
  }

  return { courses, materials };
}

/** Builds the SQL that reconciles the catalogue with the manifest. */
export function buildSQL({ courses, materials }) {
  const quote = (value) => `'${String(value).replace(/'/g, "''")}'`;
  const statements = ["BEGIN;"];

  // Courses and materials predate this indexer and carry NOT NULL identifiers
  // for school, college and major with no lookup to populate them. They are
  // derived from a fixed namespace so re-runs stay stable, and #250 will decide
  // whether the Library keeps living in this schema at all.
  const placeholder = stableUUID("henukit.org", "henu");

  for (const course of courses) {
    statements.push(
      `INSERT INTO courses (id, created_at, updated_at, school_id, college_id, major_id, grade, name, slug, description, status) VALUES (` +
        [
          quote(course.id),
          "now()",
          "now()",
          quote(placeholder),
          quote(placeholder),
          quote(placeholder),
          quote(""),
          quote(course.name),
          quote(course.slug),
          quote(course.description),
          quote("published"),
        ].join(", ") +
        `) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, slug = EXCLUDED.slug, ` +
        `description = EXCLUDED.description, status = 'published', deleted_at = NULL, updated_at = now();`
    );
  }

  for (const material of materials) {
    statements.push(
      `INSERT INTO materials (id, created_at, updated_at, course_id, title, type, description, storage_key, file_name, file_size, access_level, status) VALUES (` +
        [
          quote(material.id),
          "now()",
          "now()",
          quote(material.courseId),
          quote(material.title),
          quote(material.type),
          quote(material.description),
          quote(material.storageKey),
          quote(material.fileName),
          String(material.fileSize),
          quote("free"),
          quote("published"),
        ].join(", ") +
        `) ON CONFLICT (id) DO UPDATE SET course_id = EXCLUDED.course_id, title = EXCLUDED.title, ` +
        `type = EXCLUDED.type, description = EXCLUDED.description, storage_key = EXCLUDED.storage_key, ` +
        `file_name = EXCLUDED.file_name, file_size = EXCLUDED.file_size, access_level = 'free', ` +
        `status = 'published', deleted_at = NULL, updated_at = now();`
    );
  }

  // Anything this indexer published before but the manifest no longer lists has
  // been withdrawn upstream — most likely a takedown. Soft-delete so it leaves
  // the Library immediately without destroying the record.
  const keptMaterials = materials.map((m) => quote(m.id)).join(", ") || "NULL";
  statements.push(
    `UPDATE materials SET status = 'withdrawn', deleted_at = now(), updated_at = now() ` +
      `WHERE deleted_at IS NULL AND storage_key <> '' AND storage_key NOT LIKE '/%' ` +
      `AND storage_key NOT LIKE 'http%' AND id NOT IN (${keptMaterials});`
  );

  statements.push("COMMIT;");
  return statements.join("\n") + "\n";
}

function main() {
  if (!fs.existsSync(MANIFEST)) {
    console.error(`manifest not found at ${MANIFEST}; run the mirror sync first`);
    process.exit(66);
  }

  const manifest = JSON.parse(fs.readFileSync(MANIFEST, "utf8"));
  const plan = planCatalogue(manifest);
  process.stdout.write(buildSQL(plan));
  console.error(
    `indexed ${plan.courses.length} courses and ${plan.materials.length} materials`
  );
}

if (process.argv[1] && import.meta.url.endsWith(path.basename(process.argv[1]))) {
  main();
}
