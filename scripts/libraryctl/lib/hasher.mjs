// libraryctl — SHA256 hasher (V2)
// Computes SHA256 hashes for materials.csv entries missing sha256,
// and generates a possible-duplicates list.

import { createHash } from 'node:crypto';
import { closeSync, openSync, readSync, statSync } from 'node:fs';
import { basename } from 'node:path';

import { discoverCourses } from './paths.mjs';
import { MATERIALS_HEADER, readMaterialsCsv, writeMaterialsCsv } from './materials.mjs';
import { SafePathError, resolveWithinRoot } from './safe-path.mjs';

/** Files larger than this are skipped instead of hashed (500MB). */
export const MAX_HASH_FILE_SIZE = 500 * 1024 * 1024;

/** Read size of a single chunk in the hash loop (1MB). */
const HASH_CHUNK_SIZE = 1024 * 1024;

/** A recorded sha256 has to look like one before we group by it. */
const SHA256_RE = /^[0-9a-f]{64}$/i;

/**
 * Compute SHA256 hash of a file.
 * Reads in fixed-size chunks so a large file never lands in memory at once.
 *
 * @param {string} filePath - absolute path
 * @returns {string} hex digest
 */
export function hashFile(filePath) {
  const hash = createHash('sha256');
  const buffer = Buffer.allocUnsafe(HASH_CHUNK_SIZE);
  const fd = openSync(filePath, 'r');

  try {
    let bytesRead = readSync(fd, buffer, 0, HASH_CHUNK_SIZE, null);
    while (bytesRead > 0) {
      hash.update(buffer.subarray(0, bytesRead));
      bytesRead = readSync(fd, buffer, 0, HASH_CHUNK_SIZE, null);
    }
  } finally {
    closeSync(fd);
  }

  return hash.digest('hex');
}

/**
 * Match a discovered course against the --course filter.
 * Accepts either the bare course name or the numbered directory name.
 */
function matchesCourse(course, filter) {
  return course.course === filter || basename(course.relPath) === filter;
}

/**
 * Columns present in the parsed rows that materials.csv does not define.
 * Writing such a course back would silently drop them.
 */
function unknownColumns(rows) {
  const extras = new Set();
  for (const row of rows) {
    for (const key of Object.keys(row)) {
      if (!MATERIALS_HEADER.includes(key)) extras.add(key);
    }
  }
  return [...extras];
}

function formatSize(bytes) {
  return `${(bytes / 1024 / 1024).toFixed(1)}MB`;
}

/**
 * Compute the missing SHA256 of every registered material and group all known
 * digests so identical files surface as possible duplicates.
 *
 * Conservative by default: the report is computed but nothing is written back
 * unless `apply` is set.
 *
 * @param {string} root - absolute library root
 * @param {{apply?: boolean, course?: string | null, maxFileSize?: number}} [options]
 * @returns {{ok: boolean, root: string, applied: boolean, summary: object, courses: Array, possibleDuplicates: Array, warnings: Array, errors: Array, message: string}}
 */
export function hashAll(root, options = {}) {
  const apply = options.apply ?? false;
  const maxFileSize = options.maxFileSize ?? MAX_HASH_FILE_SIZE;
  const courseFilter = options.course ?? null;

  const warnings = [];
  const errors = [];
  const courseReports = [];
  const byDigest = new Map();

  let materialCount = 0;
  let hashedCount = 0;
  let alreadyHashedCount = 0;
  let skippedCount = 0;
  let updatedCount = 0;

  const courses = discoverCourses(root).filter(
    c => !courseFilter || matchesCourse(c, courseFilter),
  );

  if (courseFilter && courses.length === 0) {
    errors.push({ message: `未找到匹配 --course "${courseFilter}" 的课程` });
  }

  for (const c of courses) {
    const report = {
      course: c.course,
      relPath: c.relPath,
      materials: 0,
      hashed: 0,
      alreadyHashed: 0,
      skipped: 0,
      errors: 0,
      updated: false,
    };

    const csv = readMaterialsCsv(c.absPath);
    if (!csv) {
      warnings.push({
        course: c.course,
        file: '00_课程档案/materials.csv',
        message: 'materials.csv 不存在（课程无已登记资料）',
      });
      courseReports.push(report);
      continue;
    }

    let changed = false;

    for (let i = 0; i < csv.rows.length; i++) {
      const row = csv.rows[i];
      const rowLabel = `00_课程档案/materials.csv 第 ${i + 2} 行`; // +2 for header and 0-index
      report.materials++;
      materialCount++;

      const recorded = (row.sha256 ?? '').trim();
      if (recorded) {
        report.alreadyHashed++;
        alreadyHashedCount++;
        if (!SHA256_RE.test(recorded)) {
          warnings.push({
            course: c.course,
            file: rowLabel,
            message: `已登记的 sha256 不是合法的 64 位十六进制值，本次不参与去重比对: "${recorded}"`,
          });
          continue;
        }
        recordDigest(byDigest, recorded.toLowerCase(), c, row, row.path ?? '');
        continue;
      }

      let safePath;
      try {
        safePath = resolveWithinRoot(c.absPath, row.path);
      } catch (error) {
        if (!(error instanceof SafePathError)) throw error;
        report.errors++;
        errors.push({
          course: c.course,
          file: rowLabel,
          code: error.code,
          path: row.path ?? '',
          message: `资料路径校验失败 [${error.code}]: ${error.message}`,
        });
        continue;
      }

      let size;
      try {
        size = statSync(safePath.absolutePath).size;
      } catch (error) {
        report.errors++;
        errors.push({
          course: c.course,
          file: rowLabel,
          path: safePath.relativePath,
          message: `读取文件信息失败: ${error.message}`,
        });
        continue;
      }

      if (size > maxFileSize) {
        report.skipped++;
        skippedCount++;
        warnings.push({
          course: c.course,
          file: rowLabel,
          path: safePath.relativePath,
          message: `文件体积 ${formatSize(size)} 超过 ${formatSize(maxFileSize)} 上限，已跳过哈希计算`,
        });
        continue;
      }

      let digest;
      try {
        digest = hashFile(safePath.absolutePath);
      } catch (error) {
        report.errors++;
        errors.push({
          course: c.course,
          file: rowLabel,
          path: safePath.relativePath,
          message: `计算 SHA256 失败: ${error.message}`,
        });
        continue;
      }

      row.sha256 = digest;
      changed = true;
      report.hashed++;
      hashedCount++;
      recordDigest(byDigest, digest, c, row, safePath.relativePath);
    }

    if (changed && apply) {
      const extras = unknownColumns(csv.rows);
      if (extras.length > 0) {
        report.errors++;
        errors.push({
          course: c.course,
          file: '00_课程档案/materials.csv',
          message: `表头包含 materials.csv 未定义的列，写回会丢失数据，已跳过写回: ${extras.join(', ')}`,
        });
      } else {
        writeMaterialsCsv(c.absPath, csv.rows);
        report.updated = true;
        updatedCount++;
      }
    }

    courseReports.push(report);
  }

  const possibleDuplicates = [...byDigest.entries()]
    .filter(([, entries]) => entries.length > 1)
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
    .map(([sha256, entries]) => ({ sha256, count: entries.length, entries }));

  const duplicateFiles = possibleDuplicates.reduce((sum, g) => sum + g.count, 0);

  return {
    ok: errors.length === 0,
    root,
    applied: apply,
    summary: {
      courses: courses.length,
      materials: materialCount,
      hashed: hashedCount,
      alreadyHashed: alreadyHashedCount,
      skipped: skippedCount,
      updatedCourses: updatedCount,
      duplicateGroups: possibleDuplicates.length,
      duplicateFiles,
      warnings: warnings.length,
      errors: errors.length,
    },
    courses: courseReports,
    possibleDuplicates,
    warnings,
    errors,
    message: apply
      ? `已计算 ${hashedCount} 个 SHA256 并写回 ${updatedCount} 门课程的 materials.csv，发现 ${possibleDuplicates.length} 组可能重复`
      : `已计算 ${hashedCount} 个 SHA256（预览模式，未写回），发现 ${possibleDuplicates.length} 组可能重复；加 --apply 写回 materials.csv`,
  };
}

/**
 * Append one material to the digest group it belongs to.
 */
function recordDigest(byDigest, sha256, course, row, path) {
  const entry = {
    course: course.course,
    courseRelPath: course.relPath,
    localId: row.local_id ?? '',
    title: row.title ?? '',
    path,
  };
  const group = byDigest.get(sha256);
  if (group) {
    group.push(entry);
  } else {
    byDigest.set(sha256, [entry]);
  }
}
