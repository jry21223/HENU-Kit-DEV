import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

// fileURLToPath decodes percent-escaped characters, so a workspace path
// containing non-ASCII characters (e.g. 中文目录名) stays readable.
const assetsDirectory = fileURLToPath(new URL('../dist/assets/', import.meta.url));
const javascriptAssets = readdirSync(assetsDirectory).filter((name) => name.endsWith('.js'));

if (javascriptAssets.length === 0) {
  throw new Error('cutover bundle has no JavaScript assets');
}

const forbiddenFixtures = [
  'mock-ch1',
  'preview-${',
  '以下 Java 代码的输出是什么？',
  'Java 题库',
  'Web 前端',
];

for (const asset of javascriptAssets) {
  const source = readFileSync(join(assetsDirectory, asset), 'utf8');
  for (const fixture of forbiddenFixtures) {
    if (source.includes(fixture)) {
      throw new Error(`cutover bundle ${asset} contains forbidden preview fixture: ${fixture}`);
    }
  }
}
