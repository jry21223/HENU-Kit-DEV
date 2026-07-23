import fs from 'node:fs';

const client = fs.readFileSync(new URL('../src/api/client.ts', import.meta.url), 'utf8');
const rollout = fs.readFileSync(new URL('../src/api/quizcraftRollout.ts', import.meta.url), 'utf8');

const requireText = (source, text, message) => {
  if (!source.includes(text)) throw new Error(message);
};

requireText(client, 'if (QUIZCRAFT_GO_READ_ENABLED)', 'bank reads must have an independent rollout gate');
if ((client.match(/if \(QUIZCRAFT_GO_WRITES_ENABLED\)/g) || []).length < 2) {
  throw new Error('practice session and answer writes must share the explicit write gate');
}
requireText(rollout, "VITE_QUIZCRAFT_GO_WRITES", 'explicit write cutover is missing');
requireText(rollout, 'QUIZCRAFT_GO_READ_ENABLED = QUIZCRAFT_GO_WRITES_ENABLED', 'reads and writes must switch atomically');
for (const forbidden of ['READ_PERCENT', 'read_cohort', 'hashPercent', 'legacyAllTraffic']) {
  if (rollout.includes(forbidden)) throw new Error(`percentage or cohort rollout remains: ${forbidden}`);
}
