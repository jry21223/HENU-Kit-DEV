import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const files = [
  path.join(root, 'src/pages/Quiz.tsx'),
  path.join(root, 'src/stores/quizStore.ts'),
];

for (const file of files) {
  const source = fs.readFileSync(file, 'utf8');
  for (const forbidden of ['starredQuestions', 'toggleStar', 'isStarred']) {
    if (source.includes(forbidden)) {
      throw new Error(`${path.relative(root, file)} must not retain browser-owned favorite fallback: ${forbidden}`);
    }
  }
}
