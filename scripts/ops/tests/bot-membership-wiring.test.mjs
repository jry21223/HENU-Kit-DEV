import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

for (const file of ['docker-compose.henukit.yml', 'docker-compose.henukit.prebuilt.yml']) {
  test(`${file} forwards only the dedicated Bot reader credential to its owner`, () => {
    const source = readFileSync(new URL(`../../../${file}`, import.meta.url), 'utf8');
    const account = source.match(/^  account-portfolio:\n([\s\S]*?)(?=^  [a-z]|$(?![\s\S]))/m)?.[1];
    assert.ok(account);
    for (const name of ['CLIENT_ID', 'KEY_ID', 'SECRET']) {
      const key = `ACCOUNT_PORTFOLIO_BOT_${name}`;
      assert.ok(account.includes(`${key}: \u0024{${key}:-}`), `${key} must remain opt-in and pass through`);
      assert.equal((source.match(new RegExp(`^      ${key}:`, "gm")) || []).length, 1, `${key} must only reach Account Portfolio`);
    }
  });
}

test('the exact Bot entitlement edge route cannot expose other owner routes', () => {
  const nginx = readFileSync(new URL('../../../infra/nginx/henukit.conf.example', import.meta.url), 'utf8');
  const block = nginx.match(/location = \/api\/v1\/bot\/membership-entitlement \{([\s\S]*?)\n  \}/)?.[1];
  assert.ok(block, 'an exact authenticated owner route is required');
  assert.match(block, /set \$account_portfolio_upstream account-portfolio:8097;/);
  assert.match(block, /proxy_pass http:\/\/\$account_portfolio_upstream;/);
  assert.match(block, /Cache-Control "no-store" always/);
  assert.doesNotMatch(nginx, /location (?:\^~ )?\/api\/v1\/bot\//);
});
