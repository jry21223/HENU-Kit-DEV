// libraryctl — materials.csv read/write

import { randomUUID } from 'node:crypto';
import {
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  unlinkSync,
  writeFileSync,
} from 'node:fs';
import { basename, resolve, join } from 'node:path';

/** CSV header fields for materials.csv */
export const MATERIALS_HEADER = [
  'local_id',
  'title',
  'type',
  'status',
  'year',
  'path',
  'source_note',
  'sha256',
  'web_id',
  'notes',
];

/** Valid material types */
export const VALID_TYPES = new Set([
  '真题',
  '样卷',
  '讲义',
  '答案解析',
  '课件',
  '笔记',
  '题库',
  '实验资料',
  '其他',
]);

/** Valid material statuses */
export const VALID_STATUSES = new Set([
  'raw',
  'pending',
  'reviewed',
  'published',
  'archived',
]);

/**
 * Parse a CSV string into an array of row objects.
 * Handles quoted fields with embedded commas and newlines.
 *
 * @param {string} csv
 * @returns {Array<Record<string, string>>}
 */
export function parseCsv(csv) {
  const lines = [];
  let current = '';
  let inQuotes = false;

  for (let i = 0; i < csv.length; i++) {
    const ch = csv[i];
    if (ch === '"') {
      // Keep the quote so parseCsvLine can still see the field boundaries.
      inQuotes = !inQuotes;
      current += ch;
    } else if (ch === '\n' && !inQuotes) {
      lines.push(current);
      current = '';
    } else {
      current += ch;
    }
  }
  if (current) lines.push(current);

  if (lines.length === 0) return [];

  const headers = parseCsvLine(lines[0]);
  const rows = [];

  for (let i = 1; i < lines.length; i++) {
    const values = parseCsvLine(lines[i]);
    if (values.length === 0 || values.every(v => !v)) continue;
    const row = {};
    for (let j = 0; j < headers.length; j++) {
      row[headers[j]] = values[j] ?? '';
    }
    rows.push(row);
  }

  return rows;
}

/**
 * Parse a single CSV line into an array of values.
 */
function parseCsvLine(line) {
  const values = [];
  let current = '';
  let inQuotes = false;

  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (ch === '"') {
      if (inQuotes && i + 1 < line.length && line[i + 1] === '"') {
        current += '"';
        i++;
      } else {
        inQuotes = !inQuotes;
      }
    } else if (ch === ',' && !inQuotes) {
      values.push(current.trim());
      current = '';
    } else {
      current += ch;
    }
  }
  values.push(current.trim());
  return values;
}

/**
 * Escape a value for CSV output.
 */
function csvEscape(value) {
  const str = String(value ?? '');
  if (str.includes(',') || str.includes('"') || str.includes('\n')) {
    return `"${str.replace(/"/g, '""')}"`;
  }
  return str;
}

/**
 * Convert rows to CSV string.
 * @param {Array<Record<string, string>>} rows
 * @returns {string}
 */
export function rowsToCsv(rows) {
  const headerLine = MATERIALS_HEADER.join(',');
  const dataLines = rows.map(row =>
    MATERIALS_HEADER.map(h => csvEscape(row[h] ?? '')).join(','),
  );
  return [headerLine, ...dataLines].join('\n') + '\n';
}

/**
 * Read materials.csv from a course directory.
 * @param {string} courseDir
 * @returns {{ rows: Array<Record<string, string>>, path: string } | null}
 */
export function readMaterialsCsv(courseDir) {
  const csvPath = resolve(courseDir, '00_课程档案', 'materials.csv');
  if (!existsSync(csvPath)) return null;
  const content = readFileSync(csvPath, 'utf-8');
  return {
    rows: parseCsv(content),
    path: csvPath,
  };
}

/**
 * Write materials.csv to a course directory.
 * @param {string} courseDir
 * @param {Array<Record<string, string>>} rows
 */
export function writeMaterialsCsv(courseDir, rows) {
  const archiveDir = resolve(courseDir, '00_课程档案');
  const csvPath = join(archiveDir, 'materials.csv');
  mkdirSync(archiveDir, { recursive: true });

  // Write to a sibling temp file and rename, so a crash mid-write cannot leave
  // the course index truncated.
  const tempPath = join(
    archiveDir,
    `.${basename(csvPath)}.${process.pid}.${randomUUID()}.tmp`,
  );
  try {
    writeFileSync(tempPath, rowsToCsv(rows), { encoding: 'utf-8', flag: 'wx' });
    renameSync(tempPath, csvPath);
  } finally {
    if (existsSync(tempPath)) unlinkSync(tempPath);
  }
}
