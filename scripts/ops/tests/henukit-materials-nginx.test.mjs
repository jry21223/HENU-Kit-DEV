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

test("the edge never serves local material bytes when OSS is the sole download owner", () => {
  assert.doesNotMatch(nginx, /alias \/srv\/materials\//);
  assert.doesNotMatch(nginx, /Cache-Control "public, max-age=86400, immutable"/);
  assert.doesNotMatch(nginx, /@materials_missing/);
  const [materials] = locationBlocks("location /materials/");
  assert.ok(materials, "one fail-closed local materials location must exist");
  assert.match(materials, /add_header Cache-Control "no-store" always;/);
  assert.match(materials, /return 404 "资料文件只通过资料库的 OSS 下载入口提供。";/);
});

test("the local mount remains read-only solely for the activation maintenance fence", () => {
  assert.match(
    compose,
    /source: \$\{HENUKIT_MATERIALS_PUBLIC_ROOT:-\/opt\/henukit-materials\/public\}\n\s+target: \/srv\/materials\n\s+read_only: true/,
  );
  assert.doesNotMatch(compose, /source: .*\/current(?:\}|\s*$)/m);
  assert.match(nginx, /if \(-f \/srv\/materials\/\.maintenance\)/);
});

test("the activation fence returns public-ready no-store responses without server details", () => {
  assert.match(nginx, /server_tokens off;/);

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

test("all local material links return one actionable no-store response", () => {
  assert.match(nginx, /资料文件只通过资料库的 OSS 下载入口提供。/);
  assert.doesNotMatch(nginx, /location @materials_missing/);
  assert.doesNotMatch(nginx, /alias \/srv\/materials\//);
});

test("the real nginx edge rejects all local material URLs and fences the Library API", async (context) => {
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

    for (const path of [
      `/materials/releases/${releaseID}/guide.txt`,
      "/materials/current/guide.txt",
      `/materials/releases/${releaseID}/missing.pdf`,
      `/materials/releases/${releaseID}/.internal`,
    ]) {
      const response = await get(port, path);
      assert.equal(response.status, 404);
      assert.equal(response.body, "资料文件只通过资料库的 OSS 下载入口提供。");
      assert.match(response.headers["content-type"] || "", /^text\/plain;.*charset=utf-8/i);
      assert.equal(response.headers["cache-control"], "no-store");
      assert.equal(response.headers["content-disposition"], undefined);
      assert.equal(response.headers["x-content-type-options"], "nosniff");
      assert.doesNotMatch(response.headers.server || "", /nginx\/\d/);
    }

    writeFileSync(join(materialsRoot, ".maintenance"), "");
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
