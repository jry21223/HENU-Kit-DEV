import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { request } from "node:http";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const nginx = readFileSync(join(repositoryRoot, "infra", "nginx", "henukit.conf.example"), "utf8");
const compose = readFileSync(join(repositoryRoot, "docker-compose.henukit.yml"), "utf8");
const nginxConfigPath = join(repositoryRoot, "infra", "nginx", "henukit.conf.example");

function docker(args) {
  return spawnSync("docker", args, { encoding: "utf8" });
}

function get(port, path, host = "henukit.cn") {
  return new Promise((resolve, reject) => {
    const outgoing = request({ hostname: "127.0.0.1", port, path, headers: { Host: host } }, (incoming) => {
      const chunks = [];
      incoming.on("data", (chunk) => chunks.push(chunk));
      incoming.on("end", () =>
        resolve({ status: incoming.statusCode, headers: incoming.headers, body: Buffer.concat(chunks).toString("utf8") }),
      );
    });
    outgoing.on("error", reject);
    outgoing.end();
  });
}

async function waitForNginx(port) {
  let lastError;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      return await get(port, "/materials/not-public");
    } catch (error) {
      lastError = error;
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
  }
  throw lastError;
}

function locationBlocks(start) {
  const blocks = [];
  let offset = 0;
  while ((offset = nginx.indexOf(start, offset)) !== -1) {
    const opening = nginx.indexOf(" {", offset) + 1;
    let depth = 0;
    for (let index = opening; index < nginx.length; index += 1) {
      if (nginx[index] === "{") depth += 1;
      if (nginx[index] === "}") depth -= 1;
      if (depth === 0) {
        blocks.push(nginx.slice(offset, index + 1));
        offset = index + 1;
        break;
      }
    }
  }
  return blocks;
}

test("the edge exposes only immutable release-prefixed material URLs with long-lived caching", () => {
  assert.match(
    compose,
    /source: \$\{HENUKIT_MATERIALS_PUBLIC_ROOT:-\/opt\/henukit-materials\/public\}\n\s+target: \/srv\/materials\n\s+read_only: true/,
  );
  assert.doesNotMatch(compose, /source: .*\/current(?:\}|\s*$)/m);

  const [immutableMaterials] = locationBlocks(
    'location ~ "^/materials/releases/(?<materials_release>[a-f0-9]{40}-[a-f0-9]{16})/(?<materials_asset>.+)$"',
  );
  assert.ok(immutableMaterials, "immutable materials location must exist");
  assert.match(
    immutableMaterials,
    /alias \/srv\/materials\/releases\/\$materials_release\/public\/\$materials_asset;/,
  );
  assert.match(immutableMaterials, /autoindex off;/);
  assert.match(immutableMaterials, /add_header Cache-Control "public, max-age=86400, immutable" always;/);
  assert.match(immutableMaterials, /add_header X-Content-Type-Options "nosniff" always;/);
  assert.match(immutableMaterials, /add_header Content-Disposition "attachment" always;/);
  assert.match(immutableMaterials, /add_header Content-Security-Policy "default-src 'none'; sandbox" always;/);
  assert.doesNotMatch(nginx, /alias \/srv\/materials\/current\//);
  assert.doesNotMatch(nginx, /location \/materials\/\s*\{[^}]*max-age=86400/s);
  assert.match(nginx, /location ~ \^\/materials\/\(\?:\[\^\/\]\+\/\)\*\\\. \{\s*return 404;\s*\}/);
});

test("the activation fence returns public-ready no-store responses without server details", () => {
  assert.match(nginx, /server_tokens off;/);

  const [materials] = locationBlocks(
    'location ~ "^/materials/releases/(?<materials_release>[a-f0-9]{40}-[a-f0-9]{16})/(?<materials_asset>.+)$"',
  );
  assert.match(materials, /default_type text\/plain;/);
  assert.match(materials, /charset utf-8;/);
  assert.match(
    materials,
    /if \(-f \/srv\/materials\/\.maintenance\) \{\s*add_header Cache-Control "no-store" always;\s*return 503 "资料库正在更新，请稍后重试。";\s*\}/,
  );

  const libraryLocations = locationBlocks("location ~ ^/api/v1/library(?:/|$)");
  assert.equal(libraryLocations.length, 2, "henukit.cn and console.henukit.cn must both fence Library API routes");
  for (const location of libraryLocations) {
    assert.match(location, /default_type application\/json;/);
    assert.match(location, /charset utf-8;/);
    assert.match(
      location,
      /if \(-f \/srv\/materials\/\.maintenance\) \{\s*add_header Cache-Control "no-store" always;\s*add_header X-Content-Type-Options "nosniff" always;\s*return 503 '\{"error":\{"code":"LIBRARY_MAINTENANCE","message":"资料库正在更新，请稍后重试。"\},"request_id":"\$request_id"\}';\s*\}/,
    );
  }
  assert.match(libraryLocations[0], /set \$portal_gateway_upstream portal-gateway:8084;/);
  assert.match(libraryLocations[1], /set \$console_gateway_upstream console-gateway:8082;/);
});

test("missing and legacy material links return actionable no-store text instead of cached attachments", () => {
  assert.match(nginx, /这份资料暂时无法下载，请返回资料库刷新后重试。/);
  assert.match(nginx, /这个资料链接已失效，请返回资料库重新打开。/);
  assert.match(nginx, /if \(!-f \/srv\/materials\/releases\/\$materials_release\/public\/\$materials_asset\) \{ return 418; \}/);
  assert.match(nginx, /location @materials_missing \{[\s\S]*types \{ \}[\s\S]*Cache-Control "no-store"[\s\S]*return 404 "这份资料暂时无法下载，请返回资料库刷新后重试。";/);
});

test("the real nginx edge enforces immutable URLs and public-ready maintenance responses", async (context) => {
  const dockerInfo = docker(["info", "--format", "{{.ServerVersion}}"]).status;
  if (dockerInfo !== 0) {
    context.skip("Docker daemon is unavailable; CI runs this seam with Docker");
    return;
  }

  const fixture = mkdtempSync(join(tmpdir(), "henukit-materials-nginx-"));
  const materialsRoot = join(fixture, "materials");
  const releaseID = `${"a".repeat(40)}-${"b".repeat(16)}`;
  const publicRelease = join(materialsRoot, "releases", releaseID, "public");
  const container = `henukit-materials-nginx-${process.pid}-${Date.now()}`;
  mkdirSync(publicRelease, { recursive: true });
  writeFileSync(join(publicRelease, "guide.txt"), "reviewed material\n");
  writeFileSync(join(publicRelease, ".internal"), "must not be public\n");

  try {
    const started = docker([
      "run",
      "--detach",
      "--name",
      container,
      "--publish",
      "127.0.0.1::80",
      "--volume",
      `${nginxConfigPath}:/etc/nginx/conf.d/default.conf:ro`,
      "--volume",
      `${materialsRoot}:/srv/materials:ro`,
      "nginx:1.27-alpine",
    ]);
    assert.equal(started.status, 0, `nginx container must start: ${started.stderr}`);

    const published = docker(["port", container, "80/tcp"]);
    assert.equal(published.status, 0, published.stderr);
    const port = Number(published.stdout.trim().split(":").at(-1));
    assert.ok(Number.isInteger(port) && port > 0, `published port is invalid: ${published.stdout}`);
    await waitForNginx(port);

    const immutable = await get(port, `/materials/releases/${releaseID}/guide.txt`);
    assert.equal(immutable.status, 200);
    assert.equal(immutable.body, "reviewed material\n");
    assert.equal(immutable.headers["cache-control"], "public, max-age=86400, immutable");
    assert.equal(immutable.headers["content-disposition"], "attachment");
    assert.doesNotMatch(immutable.headers.server || "", /nginx\/\d/);

    const mutable = await get(port, "/materials/current/guide.txt");
    assert.equal(mutable.status, 404);
    assert.equal(mutable.body, "这个资料链接已失效，请返回资料库重新打开。");
    assert.match(mutable.headers["content-type"] || "", /^text\/plain;.*charset=utf-8/i);
    assert.equal(mutable.headers["cache-control"], "no-store");
    assert.equal(mutable.headers["content-disposition"], undefined);

    const missing = await get(port, `/materials/releases/${releaseID}/missing.pdf`);
    assert.equal(missing.status, 404);
    assert.equal(missing.body, "这份资料暂时无法下载，请返回资料库刷新后重试。");
    assert.match(missing.headers["content-type"] || "", /^text\/plain;.*charset=utf-8/i);
    assert.equal(missing.headers["cache-control"], "no-store");
    assert.equal(missing.headers["content-disposition"], undefined);

    const hidden = await get(port, `/materials/releases/${releaseID}/.internal`);
    assert.equal(hidden.status, 404);
    assert.notEqual(hidden.body, "must not be public\n");

    writeFileSync(join(materialsRoot, ".maintenance"), "");
    const fencedMaterial = await get(port, `/materials/releases/${releaseID}/guide.txt`);
    assert.equal(fencedMaterial.status, 503);
    assert.equal(fencedMaterial.body, "资料库正在更新，请稍后重试。");
    assert.match(fencedMaterial.headers["content-type"] || "", /^text\/plain;.*charset=utf-8/i);
    assert.equal(fencedMaterial.headers["cache-control"], "no-store");
    assert.doesNotMatch(fencedMaterial.headers.server || "", /nginx\/\d/);

    for (const host of ["henukit.cn", "console.henukit.cn"]) {
      const fencedAPI = await get(port, "/api/v1/library/workspace", host);
      assert.equal(fencedAPI.status, 503);
      assert.match(fencedAPI.headers["content-type"] || "", /^application\/json;.*charset=utf-8/i);
      assert.equal(fencedAPI.headers["cache-control"], "no-store");
      assert.equal(fencedAPI.headers["x-content-type-options"], "nosniff");
      const payload = JSON.parse(fencedAPI.body);
      assert.equal(payload.error.code, "LIBRARY_MAINTENANCE");
      assert.equal(payload.error.message, "资料库正在更新，请稍后重试。");
      assert.match(payload.request_id, /^[a-f0-9]{32}$/);
    }
  } finally {
    docker(["rm", "--force", container]);
    rmSync(fixture, { recursive: true, force: true });
  }
});
