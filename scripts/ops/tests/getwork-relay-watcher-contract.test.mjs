import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const watcher = readFileSync(
  new URL("../watch-henukit-actions.sh", import.meta.url),
  "utf8",
);
const start = watcher.indexOf("getwork_relay_matches() {");
const end = watcher.indexOf("release_uses_account_portfolio() {");
assert.notEqual(start, -1);
assert.notEqual(end, -1);
const functions = watcher.slice(start, end);

function executable(path, body) {
  writeFileSync(path, `#!/usr/bin/env bash\nset -Eeuo pipefail\n${body}`);
  chmodSync(path, 0o755);
}

function runContract(overrides = {}) {
  const root = mkdtempSync(join(tmpdir(), "henukit-relay-watcher-"));
  executable(
    join(root, "docker"),
    `
case "$1" in
  inspect)
    case "$*" in
      *Config.Image*) printf 'henukit-career-opportunities:%s\\n' "$TEST_SHA" ;;
      *HostConfig.NetworkMode*) printf '%s\\n' "\${TEST_NETWORK_MODE:-host}" ;;
      *Config.Env*) printf 'GETWORK_RELAY_ADDR=%s:18101\\nGETWORK_RELAY_UPSTREAM_URL=http://127.0.0.1:18100\\n' "\${TEST_RELAY_HOST:-172.17.0.1}" ;;
      *) printf 'healthy\\n' ;;
    esac
    ;;
  network)
    if [[ "$*" == *henukit_default* ]]; then printf '172.19.0.0/16\\n'; else printf '172.17.0.1\\n'; fi
    ;;
  ps) [[ "\${TEST_LOCAL_CRAWLER:-0}" == 0 ]] || printf 'henukit-getwork-mcp-1\\n' ;;
  *) exit 64 ;;
esac
`,
  );
  executable(
    join(root, "ss"),
    `printf 'LISTEN 0 4096 %s:18101 0.0.0.0:*\\n' "\${TEST_LISTEN_HOST:-172.17.0.1}"`,
  );
  executable(
    join(root, "iptables"),
    `
if [[ "$1 $2" == "-S INPUT" ]]; then
  if [[ "\${TEST_FIREWALL_UNSAFE:-0}" == 1 ]]; then printf '%s\\n' '-A INPUT -j ACCEPT'; fi
  printf '%s\\n' '-A INPUT -d 172.17.0.1/32 -p tcp -m tcp --dport 18101 -j HENUKIT-GETWORK-INGRESS'
elif [[ "$1 $2" == "-S OUTPUT" ]]; then
  printf '%s\\n' '-A OUTPUT -d 172.17.0.1/32 -p tcp -m tcp --dport 18101 -j HENUKIT-GETWORK-OUTPUT'
elif [[ "$1 $2" == "-S HENUKIT-GETWORK-INGRESS" ]]; then
  printf '%s\\n' '-A HENUKIT-GETWORK-INGRESS -s 172.19.0.0/16 -j ACCEPT' '-A HENUKIT-GETWORK-INGRESS -i lo -j ACCEPT' '-A HENUKIT-GETWORK-INGRESS -j REJECT'
elif [[ "$1 $2" == "-S HENUKIT-GETWORK-OUTPUT" ]]; then
  printf '%s\\n' '-A HENUKIT-GETWORK-OUTPUT -m owner --uid-owner 0 -j ACCEPT' '-A HENUKIT-GETWORK-OUTPUT -j REJECT'
elif [[ "$1" == "-C" ]]; then
  case "$*" in
    "-C HENUKIT-GETWORK-INGRESS -s 172.19.0.0/16 -j ACCEPT" | \
    "-C HENUKIT-GETWORK-INGRESS -i lo -j ACCEPT" | \
    "-C HENUKIT-GETWORK-INGRESS -j REJECT" | \
    "-C HENUKIT-GETWORK-OUTPUT -m owner --uid-owner 0 -j ACCEPT" | \
    "-C HENUKIT-GETWORK-OUTPUT -j REJECT") exit 0 ;;
    *) exit 1 ;;
  esac
else
  exit 64
fi
`,
  );
  executable(
    join(root, "curl"),
    String.raw`
arguments="$*"
if [[ "$arguments" == *"--write-out"* ]]; then
  printf '401'
  exit 0
fi
header="$(cat)"
[[ "$header" == "Authorization: Bearer fixture-getwork-token-000000000000000000" ]]
output=
previous=
for argument in "$@"; do
  if [[ "$previous" == --output ]]; then output="$argument"; fi
  previous="$argument"
done
[[ -n "$output" ]]
if [[ "$arguments" == *"tools/list"* ]]; then
  printf '{"result":{"tools":[{"name":"list_sources"},{"name":"crawl_jobs"}]}}' > "$output"
else
  printf '%s' '{"result":{"content":[{"type":"text","text":"{\"sources\":[{\"key\":\"alibaba\"},{\"key\":\"baidu\"},{\"key\":\"beike\"},{\"key\":\"bytedance\"},{\"key\":\"ctrip\"},{\"key\":\"dewu\"},{\"key\":\"didi\"},{\"key\":\"jd\"},{\"key\":\"kuaishou\"},{\"key\":\"meituan\"},{\"key\":\"netease\"},{\"key\":\"pdd\"},{\"key\":\"tencent\"},{\"key\":\"tencentmusic\"},{\"key\":\"tongcheng\"},{\"key\":\"vipshop\"},{\"key\":\"xfusion\"},{\"key\":\"xiaohongshu\"}]}"}]}}' > "$output"
fi
`,
  );
  const script = `
release_has_service() { [[ "$2" == getwork-mcp-relay ]]; }
environment_value() { printf 'fixture-getwork-token-000000000000000000'; }
container_is_healthy() { [[ "$1" == henukit-getwork-mcp-relay-1 ]]; }
${functions}
getwork_relay_matches "$TEST_SHA"
getwork_relay_is_healthy "$TEST_SHA"
getwork_relay_contract_is_live "$TEST_SHA"
`;
  return spawnSync("bash", ["-ceu", script], {
    encoding: "utf8",
    env: {
      ...process.env,
      PATH: `${root}:${process.env.PATH}`,
      state_root: root,
      TEST_SHA: "a".repeat(40),
      ...overrides,
    },
  });
}

test("watcher proves the exact relay configuration and authenticated MCP surface", () => {
  const result = runContract();
  assert.equal(result.status, 0, result.stderr);
});

test("watcher rejects a relay on a LAN address", () => {
  const result = runContract({ TEST_RELAY_HOST: "192.168.1.20" });
  assert.notEqual(result.status, 0);
});

test("watcher rejects a production-local browser crawler", () => {
  const result = runContract({ TEST_LOCAL_CRAWLER: "1" });
  assert.notEqual(result.status, 0);
});

test("watcher rejects an earlier firewall bypass rule", () => {
  const result = runContract({ TEST_FIREWALL_UNSAFE: "1" });
  assert.notEqual(result.status, 0);
});
