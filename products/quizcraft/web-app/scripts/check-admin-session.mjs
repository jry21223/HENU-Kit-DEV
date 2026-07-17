import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { extname, join } from 'node:path';

const collectFiles = (root) => {
  if (!existsSync(root)) return [];
  const files = [];
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const path = join(root, entry.name);
    if (entry.isDirectory()) {
      files.push(...collectFiles(path));
    } else {
      files.push(path);
    }
  }
  return files;
};

const readText = (path) => readFileSync(path, 'utf8');

if (process.argv.includes('--dist')) {
  const distFiles = collectFiles('dist');
  assert.ok(distFiles.length > 0, 'dist must exist before the admin-session bundle check');
  for (const path of distFiles) {
    const source = readText(path);
    assert.doesNotMatch(source, /VITE_ADMIN_TOKEN/);
    assert.doesNotMatch(source, /[?&]admin_token=/);
  }
} else {
  const sourceFiles = collectFiles('src').filter((path) =>
    ['.ts', '.tsx', '.js', '.jsx'].includes(extname(path)),
  );
  for (const path of sourceFiles) {
    const source = readText(path);
    assert.doesNotMatch(
      source,
      /VITE_ADMIN_TOKEN/,
      `${path} must not read an admin secret from Vite environment variables`,
    );
    assert.doesNotMatch(
      source,
      /[?&]admin_token=/,
      `${path} must not place an admin secret in a browser URL`,
    );
  }

  const envFiles = readdirSync('.').filter((name) => name.startsWith('.env'));
  for (const path of envFiles) {
    assert.doesNotMatch(
      readText(path),
      /^\s*VITE_ADMIN_TOKEN\s*=/m,
      `${path} must not assign VITE_ADMIN_TOKEN`,
    );
  }
}
