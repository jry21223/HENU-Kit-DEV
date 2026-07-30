import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const config = readFileSync(
  new URL("../../../infra/nginx/henukit-host.conf.example", import.meta.url),
  "utf8",
);
const runbook = readFileSync(
  new URL("../../../docs/operations/henukit-domain-cutover.md", import.meta.url),
  "utf8",
);

test("henukit host ingress is canonical, hardened, and rollback-safe", () => {
  assert.match(config, /server_name henukit\.cn www\.henukit\.cn/);
  assert.match(config, /server_name www\.henukit\.cn/);
  assert.match(config, /return 308 https:\/\/henukit\.cn\$request_uri/);
  assert.match(config, /live\/henukit\.cn\/fullchain\.pem/);
  assert.match(config, /proxy_pass http:\/\/127\.0\.0\.1:8088/);
  assert.match(config, /Strict-Transport-Security/);
  assert.match(config, /X-Forwarded-For \$remote_addr/);
  assert.doesNotMatch(config, /X-Forwarded-For \$proxy_add_x_forwarded_for/);
  assert.doesNotMatch(config, /server_name _/);
  assert.match(runbook, /8\.146\.200\.82/);
  assert.match(runbook, /:22222/);
  assert.match(runbook, /dns23\.hichina\.com/);
  assert.match(runbook, /oauth_clients/);
  assert.match(runbook, /Full \(strict\)/);
  assert.match(runbook, /MX, SPF,\s+DKIM, DMARC and ownership records remain DNS-only/);
  assert.match(runbook, /superhuazai\.me/);
});
