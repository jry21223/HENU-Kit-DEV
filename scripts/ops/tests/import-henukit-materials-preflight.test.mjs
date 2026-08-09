import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const importer = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "import-henukit-materials.mjs",
);
const dockerAvailable = spawnSync("docker", ["version", "--format", "{{.Server.Version}}"], {
  encoding: "utf8",
}).status === 0;
if (process.env.CI === "true" && !dockerAvailable) {
  test("the CI runner provides Docker for materials psql preflight verification", () => {
    assert.fail("Docker is required to verify the materials psql preflight in CI");
  });
}
const integration = dockerAvailable ? test : test.skip;

function run(command, args, { allowFailure = false } = {}) {
  const result = spawnSync(command, args, { encoding: "utf8" });
  if (!allowFailure) {
    assert.equal(result.status, 0, `${command} ${args.join(" ")}\n${result.stderr}`);
  }
  return result;
}

function waitForPostgres(container) {
  const sleep = (milliseconds) => {
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, milliseconds);
  };
  const readyQuery = () => spawnSync("docker", [
    "exec", container, "psql", "-U", "fixture", "-d", "fixture", "-At", "-c", "SELECT 1",
  ], { encoding: "utf8" });

  for (let attempt = 0; attempt < 60; attempt += 1) {
    // The official image briefly starts a temporary server during initdb, then
    // stops it before launching the final server. Two successful SQL queries
    // separated by that transition make this fixture deterministic.
    const first = readyQuery();
    if (first.status === 0 && first.stdout.trim() === "1") {
      sleep(250);
      const stable = readyQuery();
      if (stable.status === 0 && stable.stdout.trim() === "1") return;
    }
    sleep(100);
  }
  assert.fail("PostgreSQL fixture did not become ready");
}

const PUBLIC_SCHEMA_SQL = `
CREATE TABLE public.schools (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  slug text NOT NULL UNIQUE,
  email_domains text,
  status text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE TABLE public.colleges (
  id uuid PRIMARY KEY,
  school_id uuid NOT NULL,
  name text NOT NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE TABLE public.majors (
  id uuid PRIMARY KEY,
  school_id uuid NOT NULL,
  college_id uuid NOT NULL,
  name text NOT NULL,
  slug text NOT NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE TABLE public.courses (
  id uuid PRIMARY KEY,
  school_id uuid NOT NULL,
  college_id uuid NOT NULL,
  major_id uuid NOT NULL,
  grade text NOT NULL,
  name text NOT NULL,
  slug text NOT NULL,
  description text NOT NULL,
  status text NOT NULL,
  deleted_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE TABLE public.materials (
  id uuid,
  course_id uuid,
  title text,
  type text,
  description text,
  storage_key text NOT NULL,
  file_name text,
  file_size bigint,
  sha256 text,
  slides jsonb,
  access_level text,
  status text,
  reviewed_at timestamptz,
  review_reason text,
  deleted_at timestamptz,
  created_at timestamptz,
  updated_at timestamptz
);`;

const MANIFEST = {
  version: 1,
  subjects: [{
    name: "离散数学",
    assets: [{
      role: "复习讲义",
      title: "离散数学_复习讲义_提纲.pdf",
      publicPath: "materials/outline.pdf",
      bytes: 1,
      sha256: "a".repeat(64),
    }],
  }],
};

function psql(container, sql, { allowFailure = false } = {}) {
  return run("docker", [
    "exec", "-i", container, "psql", "-U", "fixture", "-d", "fixture", "-v", "ON_ERROR_STOP=1", "-c", sql,
  ], { allowFailure });
}

function scalar(container, query) {
  return run("docker", [
    "exec", "-i", container, "psql", "-U", "fixture", "-d", "fixture", "-At", "-c", query,
  ]).stdout.trim();
}

function startPostgres(container) {
  run("docker", [
    "run", "--rm", "-d", "--name", container,
    "-e", "POSTGRES_USER=fixture",
    "-e", "POSTGRES_PASSWORD=fixture",
    "-e", "POSTGRES_DB=fixture",
    "postgres:17-alpine",
  ]);
  waitForPostgres(container);
}

function writeGeneratedSql(root, sourceManifest = MANIFEST, legacyKeys = []) {
  const manifest = join(root, "manifest.json");
  const sql = join(root, "import.sql");
  const legacyInventory = join(root, "legacy-inventory.json");
  writeFileSync(manifest, JSON.stringify(sourceManifest));
  writeFileSync(legacyInventory, JSON.stringify({ version: 1, storage_keys: legacyKeys }));
  const generated = run(process.execPath, [
    importer,
    "--manifest", manifest,
    "--release-id", `${"a".repeat(40)}-${"b".repeat(16)}`,
    "--legacy-inventory", legacyInventory,
  ]);
  assert.match(generated.stdout, /SET search_path TO pg_catalog, public;/);
  assert.match(generated.stdout, /material_index\.indisvalid/);
  assert.match(generated.stdout, /material_index\.indisready/);
  assert.match(generated.stdout, /material_index\.indislive/);
  writeFileSync(sql, generated.stdout);
  return sql;
}

integration("an empty reviewed release archives only prior release-owned catalog rows", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-empty-release-"));
  const container = `henukit-materials-empty-release-${process.pid}-${Date.now()}`;
  try {
    const sql = writeGeneratedSql(root, {
      subjects: [{
        name: "离散数学",
        assets: [{
          role: "待复核资料",
          publicPath: "materials/pending.pdf",
          reviewStatus: "needs_review",
          bytes: 1,
          sha256: "c".repeat(64),
        }],
      }],
    });
    startPostgres(container);
    psql(container, `${PUBLIC_SCHEMA_SQL} CREATE UNIQUE INDEX materials_storage_key_active_idx ON public.materials(storage_key) WHERE deleted_at IS NULL;`);
    psql(container, `
      INSERT INTO public.materials(storage_key, sha256, status, created_at, updated_at) VALUES
        ('releases/${"c".repeat(40)}-${"d".repeat(16)}/materials/old.pdf', '${"e".repeat(64)}', 'published', now(), now()),
        ('manual/checksummed.pdf', '${"f".repeat(64)}', 'published', now(), now());
    `);
    const imported = runGeneratedSql(container, sql);
    assert.equal(imported.status, 0, imported.stderr);
    assert.equal(scalar(container, `SELECT status FROM public.materials WHERE storage_key LIKE 'releases/%';`), "archived");
    assert.equal(scalar(container, `SELECT status FROM public.materials WHERE storage_key = 'manual/checksummed.pdf';`), "published");
  } finally {
    if (existsSync(root)) rmSync(root, { recursive: true, force: true });
    spawnSync("docker", ["rm", "-f", container], { encoding: "utf8" });
  }
});

integration("the first immutable import archives only legacy keys named by the reviewed manifest", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-first-cut-"));
  const container = `henukit-materials-first-cut-${process.pid}-${Date.now()}`;
  try {
    const sql = writeGeneratedSql(root, MANIFEST, ["materials/outline.pdf", "materials/removed.pdf"]);
    startPostgres(container);
    psql(container, `${PUBLIC_SCHEMA_SQL} CREATE UNIQUE INDEX materials_storage_key_active_idx ON public.materials(storage_key) WHERE deleted_at IS NULL;`);
    psql(container, `
      INSERT INTO public.materials(storage_key, sha256, status, created_at, updated_at) VALUES
        ('materials/outline.pdf', '${"e".repeat(64)}', 'published', now(), now()),
        ('materials/removed.pdf', '${"d".repeat(64)}', 'published', now(), now()),
        ('manual/checksummed.pdf', '${"f".repeat(64)}', 'published', now(), now());
    `);
    const imported = runGeneratedSql(container, sql);
    assert.equal(imported.status, 0, imported.stderr);
    assert.equal(scalar(container, `SELECT status FROM public.materials WHERE storage_key = 'materials/outline.pdf';`), "archived");
    assert.equal(scalar(container, `SELECT status FROM public.materials WHERE storage_key = 'materials/removed.pdf';`), "archived");
    assert.equal(scalar(container, `SELECT status FROM public.materials WHERE storage_key = 'manual/checksummed.pdf';`), "published");
    assert.equal(scalar(container, `SELECT count(*) FROM public.materials WHERE storage_key LIKE 'releases/%' AND status = 'published';`), "1");
  } finally {
    if (existsSync(root)) rmSync(root, { recursive: true, force: true });
    spawnSync("docker", ["rm", "-f", container], { encoding: "utf8" });
  }
});

function runGeneratedSql(container, sql) {
  run("docker", ["cp", sql, `${container}:/tmp/import.sql`]);
  return run("docker", [
    "exec", "-i", container, "psql", "-U", "fixture", "-d", "fixture", "-v", "ON_ERROR_STOP=1", "-f", "/tmp/import.sql",
  ], { allowFailure: true });
}

integration("an invalid materials uniqueness index fails the public psql preflight before DML", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-preflight-"));
  const container = `henukit-materials-preflight-${process.pid}-${Date.now()}`;
  try {
    const sql = writeGeneratedSql(root);
    startPostgres(container);
    psql(container, `${PUBLIC_SCHEMA_SQL} INSERT INTO public.materials(storage_key) VALUES ('duplicate'), ('duplicate');`);
    const invalidIndex = psql(container,
      "CREATE UNIQUE INDEX CONCURRENTLY materials_storage_key_active_idx ON public.materials(storage_key) WHERE deleted_at IS NULL;",
      { allowFailure: true },
    );
    assert.notEqual(invalidIndex.status, 0, "fixture must leave an invalid concurrent unique index");
    const indexState = scalar(container,
      "SELECT indisvalid::text || ',' || indisready::text || ',' || indislive::text FROM pg_index WHERE indexrelid = 'public.materials_storage_key_active_idx'::regclass;",
    );
    assert.notEqual(indexState, "t,t,t", `fixture index unexpectedly active: ${indexState}`);

    const imported = runGeneratedSql(container, sql);
    assert.notEqual(imported.status, 0);
    assert.match(imported.stdout + imported.stderr, /Materials import refused/);
    assert.match(imported.stdout + imported.stderr, /division by zero/);
    assert.equal(scalar(container, "SELECT count(*) FROM public.schools;"), "0");
    assert.equal(scalar(container, "SELECT count(*) FROM public.materials;"), "2");
  } finally {
    if (existsSync(root)) rmSync(root, { recursive: true, force: true });
    spawnSync("docker", ["rm", "-f", container], { encoding: "utf8" });
  }
});

integration("an active expression index cannot satisfy the storage_key conflict-arbiter preflight", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-expression-index-"));
  const container = `henukit-materials-expression-index-${process.pid}-${Date.now()}`;
  try {
    const sql = writeGeneratedSql(root);
    startPostgres(container);
    psql(container, `${PUBLIC_SCHEMA_SQL} CREATE UNIQUE INDEX materials_storage_key_active_idx ON public.materials(lower(storage_key)) WHERE deleted_at IS NULL;`);
    assert.equal(scalar(container,
      "SELECT indisvalid::text || ',' || indisready::text || ',' || indislive::text FROM pg_index WHERE indexrelid = 'public.materials_storage_key_active_idx'::regclass;",
    ), "true,true,true");

    const imported = runGeneratedSql(container, sql);
    assert.notEqual(imported.status, 0);
    assert.match(imported.stdout + imported.stderr, /Materials import refused/);
    assert.equal(scalar(container, "SELECT count(*) FROM public.schools;"), "0");
    assert.equal(scalar(container, "SELECT count(*) FROM public.materials;"), "0");
  } finally {
    if (existsSync(root)) rmSync(root, { recursive: true, force: true });
    spawnSync("docker", ["rm", "-f", container], { encoding: "utf8" });
  }
});

integration("a hostile connection search_path cannot divert DML away from public", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-search-path-"));
  const container = `henukit-materials-search-path-${process.pid}-${Date.now()}`;
  try {
    const sql = writeGeneratedSql(root);
    startPostgres(container);
    psql(container, `${PUBLIC_SCHEMA_SQL} CREATE UNIQUE INDEX materials_storage_key_active_idx ON public.materials(storage_key) WHERE deleted_at IS NULL;`);
    psql(container, `
      CREATE SCHEMA hostile;
      CREATE TABLE hostile.schools (LIKE public.schools INCLUDING ALL);
      CREATE TABLE hostile.colleges (LIKE public.colleges INCLUDING ALL);
      CREATE TABLE hostile.majors (LIKE public.majors INCLUDING ALL);
      CREATE TABLE hostile.courses (LIKE public.courses INCLUDING ALL);
      CREATE TABLE hostile.materials (LIKE public.materials INCLUDING ALL);
      ALTER ROLE fixture SET search_path = hostile, public;
    `);
    assert.match(scalar(container, "SHOW search_path;"), /^hostile, public$/);

    const imported = runGeneratedSql(container, sql);
    assert.equal(imported.status, 0, imported.stderr);
    assert.equal(scalar(container, `SELECT count(*) FROM public.materials WHERE storage_key = 'releases/${"a".repeat(40)}-${"b".repeat(16)}/materials/outline.pdf';`), "1");
    assert.equal(scalar(container, "SELECT count(*) FROM hostile.materials WHERE storage_key = 'materials/outline.pdf';"), "0");
  } finally {
    if (existsSync(root)) rmSync(root, { recursive: true, force: true });
    spawnSync("docker", ["rm", "-f", container], { encoding: "utf8" });
  }
});
