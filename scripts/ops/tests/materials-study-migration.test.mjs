import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const migration = join(
  dirname(fileURLToPath(import.meta.url)),
  "..", "..", "..", "services", "library", "db", "legacy-study-migrations",
  "000001_materials_oss_release.up.sql",
);
const dockerAvailable = spawnSync("docker", ["version", "--format", "{{.Server.Version}}"], {
  encoding: "utf8",
}).status === 0;
if (process.env.CI === "true" && !dockerAvailable) {
  test("CI provides Docker for the legacy Study migration seam", () => {
    assert.fail("Docker is required to verify the legacy Study migration in CI");
  });
}
const integration = dockerAvailable ? test : test.skip;

function run(args, { allowFailure = false, input } = {}) {
  const result = spawnSync("docker", args, { encoding: "utf8", input });
  if (!allowFailure) assert.equal(result.status, 0, result.stderr);
  return result;
}

function waitForPostgres(container) {
  const sleep = (milliseconds) => Atomics.wait(
    new Int32Array(new SharedArrayBuffer(4)), 0, 0, milliseconds,
  );
  for (let attempt = 0; attempt < 60; attempt += 1) {
    const result = run([
      "exec", container, "psql", "-U", "fixture", "-d", "fixture", "-Atqc", "SELECT 1",
    ], { allowFailure: true });
    if (result.status === 0 && result.stdout.trim() === "1") return;
    sleep(100);
  }
  assert.fail("PostgreSQL fixture did not become ready");
}

function psql(container, sql, { allowFailure = false } = {}) {
  return run([
    "exec", "-i", container, "psql", "-U", "fixture", "-d", "fixture",
    "-v", "ON_ERROR_STOP=1", "-f", "-",
  ], { allowFailure, input: sql });
}

integration("legacy Study migration is idempotent and creates the exact OSS catalog contract", () => {
  const container = `henukit-study-migration-${process.pid}-${Date.now()}`;
  try {
    run([
      "run", "--rm", "-d", "--name", container,
      "-e", "POSTGRES_USER=fixture", "-e", "POSTGRES_PASSWORD=fixture",
      "-e", "POSTGRES_DB=fixture", "postgres:17-alpine",
    ]);
    waitForPostgres(container);
    psql(container, `
      CREATE TABLE public.materials (
        id uuid PRIMARY KEY,
        storage_key text NOT NULL,
        deleted_at timestamptz
      );
      INSERT INTO public.materials(id, storage_key) VALUES
        ('00000000-0000-0000-0000-000000000001', 'materials/existing.pdf');
    `);
    const source = spawnSync("sh", ["-c", `cat "$1"`, "sh", migration], { encoding: "utf8" });
    assert.equal(source.status, 0, source.stderr);
    psql(container, source.stdout);
    psql(container, source.stdout);
    const contract = run([
      "exec", container, "psql", "-U", "fixture", "-d", "fixture", "-Atqc",
      `SELECT
        (SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='materials' AND column_name='sha256') || ',' ||
        (SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='materials' AND column_name='slides') || ',' ||
        (SELECT pg_get_indexdef(indexrelid) FROM pg_index WHERE indexrelid='public.materials_storage_key_active_idx'::regclass) || ',' ||
        (SELECT count(*) FROM public.materials);`,
    ]).stdout.trim();
    assert.equal(
      contract,
      "text,jsonb,CREATE UNIQUE INDEX materials_storage_key_active_idx ON public.materials USING btree (storage_key) WHERE (deleted_at IS NULL),1",
    );
  } finally {
    run(["rm", "-f", container], { allowFailure: true });
  }
});

integration("legacy Study migration fails closed on active storage-key duplicates", () => {
  const container = `henukit-study-migration-duplicate-${process.pid}-${Date.now()}`;
  try {
    run([
      "run", "--rm", "-d", "--name", container,
      "-e", "POSTGRES_USER=fixture", "-e", "POSTGRES_PASSWORD=fixture",
      "-e", "POSTGRES_DB=fixture", "postgres:17-alpine",
    ]);
    waitForPostgres(container);
    psql(container, `
      CREATE TABLE public.materials (id uuid, storage_key text NOT NULL, deleted_at timestamptz);
      INSERT INTO public.materials VALUES
        ('00000000-0000-0000-0000-000000000001', 'materials/duplicate.pdf', NULL),
        ('00000000-0000-0000-0000-000000000002', 'materials/duplicate.pdf', NULL);
    `);
    const source = spawnSync("sh", ["-c", `cat "$1"`, "sh", migration], { encoding: "utf8" });
    const result = psql(container, source.stdout, { allowFailure: true });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /duplicate storage_key/);
    const columns = run([
      "exec", container, "psql", "-U", "fixture", "-d", "fixture", "-Atqc",
      "SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='materials' AND column_name IN ('sha256','slides');",
    ]).stdout.trim();
    assert.equal(columns, "0");
  } finally {
    run(["rm", "-f", container], { allowFailure: true });
  }
});
