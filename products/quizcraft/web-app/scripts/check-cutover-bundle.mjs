import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const assetsDirectory = new URL('../dist/assets/', import.meta.url);
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
  const source = readFileSync(join(assetsDirectory.pathname, asset), 'utf8');
  for (const fixture of forbiddenFixtures) {
    if (source.includes(fixture)) {
      throw new Error(`cutover bundle ${asset} contains forbidden preview fixture: ${fixture}`);
    }
  }
}
