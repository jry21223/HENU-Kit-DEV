// libraryctl — SHA256 hasher (V2)
// Computes SHA256 hashes for materials.csv entries missing sha256,
// and generates a possible-duplicates list.

import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';

/**
 * Compute SHA256 hash of a file.
 * @param {string} filePath - absolute path
 * @returns {string} hex digest
 */
export function hashFile(filePath) {
  const content = readFileSync(filePath);
  return createHash('sha256').update(content).digest('hex');
}

export function hashAll(root) {
  throw new Error('[libraryctl] hash 命令尚未实现（V2 规划中）');
}
