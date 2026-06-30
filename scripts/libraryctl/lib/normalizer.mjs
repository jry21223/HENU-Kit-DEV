// libraryctl — file name normalization

import { extname } from 'node:path';

/** Characters to replace with underscores */
const SPACE_REPLACE_RE = /\s+/g;

/** Illegal filename characters (Windows + Unix common) */
const ILLEGAL_CHAR_RE = /[\/\\:*?"<>|]/g;

/** Characters to strip: version suffixes, editorial noise */
const STRIP_PATTERNS = [
  /[（(]最终版[）)]/g,
  /[（(]最新版[）)]/g,
  /[（(]改完版[）)]/g,
  /[（(]修订版[）)]/g,
  /_final/gi,
  /_v\d+(\.\d+)*/gi,
  /_copy/gi,
  /[（(]\d+[）)]/g,   // parenthetical numbers like (1)
  /^\d+_/,             // leading numeric prefix
];

/**
 * Normalize a string for use in a filename.
 * - Replaces spaces with underscores
 * - Replaces illegal characters with hyphens
 * - Collapses consecutive underscores
 * - Lowercases file extension only
 *
 * @param {string} input
 * @returns {string}
 */
export function normalizeString(input) {
  let result = input;

  // Strip version/editorial noise
  for (const re of STRIP_PATTERNS) {
    result = result.replace(re, '');
  }

  // Replace spaces
  result = result.replace(SPACE_REPLACE_RE, '_');

  // Replace illegal chars
  result = result.replace(ILLEGAL_CHAR_RE, '-');

  // Collapse consecutive underscores/hyphens
  result = result.replace(/_{2,}/g, '_');
  result = result.replace(/-{2,}/g, '-');

  // Remove leading/trailing underscores and hyphens
  result = result.replace(/^[_\-]+/, '').replace(/[_\-]+$/, '');

  return result;
}

/**
 * Build a canonical material filename.
 *
 * Pattern: 课程名_资料类型_年份或主题_标题.扩展名
 *
 * Rules:
 * 1. Chinese characters preserved
 * 2. Spaces → _
 * 3. Illegal chars → -
 * 4. Consecutive underscores collapsed
 * 5. Extension lowercased
 * 6. No hardcoded year
 * 7. No editorial suffixes like "最终版"
 *
 * @param {{courseName: string, type: string, year?: string, title?: string, topic?: string, ext: string}} input
 * @returns {string}
 */
export function buildMaterialFileName(input) {
  const { courseName, type, year, title, topic, ext } = input;

  const parts = [normalizeString(courseName), normalizeString(type)];

  // Year or topic (at least one should be present)
  if (year) {
    parts.push(normalizeString(year));
  } else {
    parts.push('年份待确认');
  }

  // Title/topic for disambiguation
  const desc = title || topic || '';
  if (desc) {
    parts.push(normalizeString(desc));
  }

  // Extension
  const cleanExt = ext.toLowerCase().replace(/^\./, '');

  return parts.join('_') + '.' + cleanExt;
}

/**
 * Check if a filename contains illegal characters.
 * @param {string} filename
 * @returns {{ok: boolean, illegalChars: string[]}}
 */
export function checkFilename(filename) {
  const illegal = [];
  const matches = filename.match(ILLEGAL_CHAR_RE);
  if (matches) {
    illegal.push(...matches);
  }
  return {
    ok: illegal.length === 0,
    illegalChars: [...new Set(illegal)],
  };
}
