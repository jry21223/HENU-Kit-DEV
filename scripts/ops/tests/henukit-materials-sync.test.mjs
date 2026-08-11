import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readlinkSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const driver = join(repoRoot, "scripts", "ops", "henukit-materials-sync.sh");
const mirrorHelper = join(repoRoot, "scripts", "ops", "sync-henukit-materials.sh");
const studyMigrationUp = join(
  repoRoot,
  "services",
  "api",
  "migrations",
  "0002_henukit_materials_sync_expand.up.sql",
);
const studyMigrationDown = join(
  repoRoot,
  "services",
  "api",
  "migrations",
  "0002_henukit_materials_sync_expand.down.sql",
);
const currentFile = (mirror, ...segments) =>
  join(mirror, "public", "current", "files", ...segments);
const realPostgresURL = process.env.HENUKIT_TEST_POSTGRES_URL ?? "";

function createMaterialsRepository(
  root,
  {
    includeSlides = false,
    publicPath: requestedPath,
    sha256Override,
    symlinkAsset = false,
    assetContents = "approved course material\n",
    bytesOverride,
  } = {},
) {
  const source = join(root, "source");
  const publicPath = requestedPath ?? "高等数学A（二）/复习讲义/考前复习.pdf";
  const asset = Buffer.from(assetContents, "utf8");

  mkdirSync(source, { recursive: true });
  if (!publicPath.startsWith("..")) {
    mkdirSync(join(source, dirname(publicPath)), { recursive: true });
    if (symlinkAsset) {
      const realPath = join(source, "assets", "real.pdf");
      mkdirSync(dirname(realPath), { recursive: true });
      writeFileSync(realPath, asset);
      symlinkSync(relative(dirname(join(source, publicPath)), realPath), join(source, publicPath));
    } else {
      writeFileSync(join(source, publicPath), asset);
    }
  }
  const assets = [
    {
      role: "复习讲义",
      title: "考前复习.pdf",
      publicPath,
      bytes: bytesOverride ?? asset.byteLength,
      sha256: sha256Override ?? createHash("sha256").update(asset).digest("hex"),
    },
  ];
  if (includeSlides) {
    const slidePublicPath = "高等数学A（二）/课件PPT/二重积分.pptx";
    const slidePath = join(source, slidePublicPath);
    mkdirSync(dirname(slidePath), { recursive: true });
    execFileSync("python3", [
      "-c",
      "from pptx import Presentation; import sys; p=Presentation(); s=p.slides.add_slide(p.slide_layouts[1]); s.shapes.title.text='二重积分'; s.placeholders[1].text='定义与几何意义'; p.save(sys.argv[1])",
      slidePath,
    ]);
    const slide = readFileSync(slidePath);
    assets.push({
      role: "课件PPT",
      title: "二重积分.pptx",
      publicPath: slidePublicPath,
      bytes: slide.byteLength,
      sha256: createHash("sha256").update(slide).digest("hex"),
    });
  }
  writeFileSync(
    join(source, "manifest.json"),
    JSON.stringify({
      version: 1,
      subjects: [
        {
          name: "高等数学A（二）",
          assets,
        },
      ],
    }),
  );
  execFileSync("git", ["init", "-b", "main"], { cwd: source });
  execFileSync("git", ["config", "user.name", "Materials Fixture"], { cwd: source });
  execFileSync("git", ["config", "user.email", "fixture@example.invalid"], { cwd: source });
  execFileSync("git", ["add", "."], { cwd: source });
  execFileSync("git", ["commit", "-m", "fixture"], { cwd: source });
  const sha = execFileSync("git", ["rev-parse", "HEAD"], {
    cwd: source,
    encoding: "utf8",
  }).trim();
  return { source, publicPath, sha };
}

function createFakePsql(root) {
  const bin = join(root, "bin");
  const psql = join(bin, "psql");
  mkdirSync(bin, { recursive: true });
  writeFileSync(
    psql,
    `#!/usr/bin/env bash
set -euo pipefail
printf 'run\n' >> "$HENUKIT_TEST_PSQL_RUNS"
printf '%s' "$*" | tr '\n' ' ' >> "$HENUKIT_TEST_PSQL_ARGS"
printf '\n' >> "$HENUKIT_TEST_PSQL_ARGS"
printf '%s\n' "\${PGDATABASE-}" >> "$HENUKIT_TEST_PSQL_DATABASES"
if [[ "$*" == *"materials_storage_key_active_idx"* ]]; then
  printf '%s\n' "\${HENUKIT_TEST_SCHEMA_STATUS:-ready}"
  exit 0
fi
if [[ "$*" == *"to_regclass('public.henukit_materials_sync_state')"* ]]; then
  if [[ -s "$HENUKIT_TEST_DB_MARKER" ]]; then
    printf 'henukit_materials_sync_state\n'
  fi
  exit 0
fi
if [[ "$*" == *"SELECT synced_sha || ':' || delivery"* ]]; then
  cat "$HENUKIT_TEST_DB_MARKER"
  exit 0
fi
sql_file=""
while (( "$#" )); do
  if [[ "$1" == "-f" ]]; then
    sql_file="\${2:?}"
    shift 2
  else
    shift
  fi
done
if [[ -n "$sql_file" && "$sql_file" != "-" ]]; then
  cp "$sql_file" "$HENUKIT_TEST_SQL_LOG"
else
  cat > "$HENUKIT_TEST_SQL_LOG"
fi
exit "\${HENUKIT_TEST_PSQL_EXIT:-0}"
`,
  );
  chmodSync(psql, 0o755);
  return bin;
}

function psqlEnv(databaseURL) {
  const parsed = new URL(databaseURL);
  return {
    ...process.env,
    PGHOST: parsed.hostname,
    PGPORT: parsed.port || "5432",
    PGUSER: decodeURIComponent(parsed.username),
    PGPASSWORD: decodeURIComponent(parsed.password),
    PGDATABASE: decodeURIComponent(parsed.pathname.slice(1)),
    PGSSLMODE: parsed.searchParams.get("sslmode") ?? "prefer",
  };
}

function runPsql(databaseURL, sql) {
  return execFileSync("psql", ["-v", "ON_ERROR_STOP=1", "-f", "-"], {
    encoding: "utf8",
    input: sql,
    env: psqlEnv(databaseURL),
  });
}

function queryPsql(databaseURL, sql) {
  return execFileSync("psql", ["-v", "ON_ERROR_STOP=1", "-tAc", sql], {
    encoding: "utf8",
    env: psqlEnv(databaseURL),
  }).trim();
}

function createRealStudyDatabase() {
  const databaseName = `hc306_${process.pid}_${Date.now()}`;
  runPsql(realPostgresURL, `CREATE DATABASE ${databaseName};\n`);
  const databaseURL = new URL(realPostgresURL);
  databaseURL.pathname = `/${databaseName}`;
  runPsql(
    databaseURL.toString(),
    `
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE schools (
  id uuid PRIMARY KEY, name text NOT NULL, slug text UNIQUE NOT NULL,
  email_domains text, status text NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE colleges (
  id uuid PRIMARY KEY, school_id uuid NOT NULL, name text NOT NULL,
  status text NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE majors (
  id uuid PRIMARY KEY, school_id uuid NOT NULL, college_id uuid NOT NULL,
  name text NOT NULL, slug text NOT NULL, status text NOT NULL,
  created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
);
CREATE TABLE courses (
  id uuid PRIMARY KEY, school_id uuid NOT NULL, college_id uuid NOT NULL, major_id uuid NOT NULL,
  grade text NOT NULL, name text NOT NULL, slug text NOT NULL, description text NOT NULL,
  status text NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL,
  deleted_at timestamptz
);
CREATE TABLE materials (
  id uuid PRIMARY KEY, course_id uuid NOT NULL, title text NOT NULL, type text NOT NULL,
  description text NOT NULL, storage_key text NOT NULL, file_name text NOT NULL,
  file_size bigint NOT NULL, access_level text NOT NULL, status text NOT NULL,
  reviewed_at timestamptz, review_reason text, created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL, deleted_at timestamptz
);
`,
  );
  const migration = readFileSync(studyMigrationUp, "utf8");
  runPsql(databaseURL.toString(), migration);
  runPsql(databaseURL.toString(), migration);
  runPsql(
    databaseURL.toString(),
    `
DO $decoy$
BEGIN
  EXECUTE format('CREATE SCHEMA IF NOT EXISTS %I AUTHORIZATION %I', current_user, current_user);
  EXECUTE format(
    'CREATE TABLE IF NOT EXISTS %I.henukit_materials_sync_state (LIKE public.henukit_materials_sync_state INCLUDING ALL)',
    current_user
  );
  EXECUTE format(
    'INSERT INTO %I.henukit_materials_sync_state (singleton, synced_sha, delivery, updated_at) VALUES (1, %L, %L, now()) ON CONFLICT (singleton) DO UPDATE SET synced_sha = EXCLUDED.synced_sha, delivery = EXCLUDED.delivery, updated_at = EXCLUDED.updated_at',
    current_user,
    '${"d".repeat(40)}',
    'decoy-user-schema'
  );
END;
$decoy$;
`,
  );
  return {
    databaseName,
    databaseURL: databaseURL.toString(),
    cleanup() {
      runPsql(realPostgresURL, `DROP DATABASE IF EXISTS ${databaseName} WITH (FORCE);\n`);
    },
  };
}

function addMaterialCommit(source, publicPath, contents) {
  const manifestPath = join(source, "manifest.json");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  const asset = Buffer.from(contents, "utf8");
  mkdirSync(dirname(join(source, publicPath)), { recursive: true });
  writeFileSync(join(source, publicPath), asset);
  manifest.subjects[0].assets.push({
    role: "课件资料",
    title: publicPath.split("/").at(-1),
    publicPath,
    bytes: asset.byteLength,
    sha256: createHash("sha256").update(asset).digest("hex"),
  });
  writeFileSync(manifestPath, JSON.stringify(manifest));
  execFileSync("git", ["add", "."], { cwd: source });
  execFileSync("git", ["commit", "-m", `add ${publicPath}`], { cwd: source });
  return execFileSync("git", ["rev-parse", "HEAD"], { cwd: source, encoding: "utf8" }).trim();
}

function markAllMaterialsPending(source) {
  const manifestPath = join(source, "manifest.json");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  for (const subject of manifest.subjects) {
    for (const asset of subject.assets) {
      asset.role = "待复核资料";
    }
  }
  writeFileSync(manifestPath, JSON.stringify(manifest));
  execFileSync("git", ["add", "manifest.json"], { cwd: source });
  execFileSync("git", ["commit", "-m", "mark all materials pending"], { cwd: source });
  return execFileSync("git", ["rev-parse", "HEAD"], { cwd: source, encoding: "utf8" }).trim();
}

function runDriver({
  root,
  source,
  sha,
  delivery = "delivery-materials-1",
  psqlExit = 0,
  databaseURL = "postgres://fixture:fixture@fixture.invalid/study?sslmode=disable",
  psqlBin,
  extraEnv = {},
}) {
  const bin = psqlBin ?? createFakePsql(root);
  return spawnSync(
    "bash",
    [
      "-c",
      'umask 077; exec /bin/bash "$@"',
      "henukit-materials-test",
      driver,
      "--sha",
      sha,
      "--delivery",
      delivery,
      "--repository",
      "jry21223/HENU-Final-Review",
      "--ref",
      "refs/heads/main",
    ],
    {
      cwd: repoRoot,
      encoding: "utf8",
      env: {
        ...process.env,
        PATH: `${bin}:${process.env.PATH}`,
        HENUKIT_MATERIALS_ROOT: join(root, "mirror"),
        HENUKIT_MATERIALS_REPO_URL: source,
        HENUKIT_MATERIALS_REPO_REF: "main",
        HENUKIT_MATERIALS_DATABASE_URL: databaseURL,
        HENUKIT_TEST_PSQL_EXIT: String(psqlExit),
        HENUKIT_TEST_PSQL_ARGS: join(root, "psql-args"),
        HENUKIT_TEST_PSQL_DATABASES: join(root, "psql-databases"),
        HENUKIT_TEST_DB_MARKER: join(root, "database-marker"),
        ...extraEnv,
        HENUKIT_TEST_SQL_LOG: join(root, "import.sql"),
        HENUKIT_TEST_PSQL_RUNS: join(root, "psql-runs"),
      },
    },
  );
}

test("the privileged driver requires explicit manual mode or one complete event tuple", () => {
  const result = spawnSync("bash", [driver], {
    cwd: repoRoot,
    encoding: "utf8",
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /use --manual or provide one complete webhook event tuple/);
});

test("the low-level mirror rejects boolean bytes even for a one-byte file", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-mirror-bytes-"));
  try {
    const fixture = createMaterialsRepository(root, {
      assetContents: "x",
      bytesOverride: true,
    });
    const mirror = join(root, "mirror");
    const result = spawnSync("bash", [mirrorHelper], {
      cwd: repoRoot,
      encoding: "utf8",
      env: {
        ...process.env,
        HENUKIT_MATERIALS_ROOT: mirror,
        HENUKIT_MATERIALS_REPO_URL: fixture.source,
        HENUKIT_MATERIALS_REPO_REF: "main",
        HENUKIT_MATERIALS_EXPECTED_SHA: fixture.sha,
      },
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /manifest asset has invalid byte count/);
    assert.equal(existsSync(join(mirror, ".staging", "public", fixture.publicPath)), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("an exact committed event marker is a no-op after Store recovery", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root);
    const mirror = join(root, "mirror");
    const target = ".snapshots/already-committed";
    const snapshot = join(mirror, "public", target);
    const delivery = "delivery-already-committed";
    mkdirSync(join(snapshot, "files"), { recursive: true });
    mkdirSync(join(snapshot, "slides"), { recursive: true });
    writeFileSync(join(snapshot, "files", "keep.pdf"), "already published\n");
    writeFileSync(join(snapshot, "SYNCED_SHA"), `${fixture.sha}\n`);
    writeFileSync(join(snapshot, "DELIVERY"), `${delivery}\n`);
    symlinkSync(target, join(mirror, "public", "current"));
    writeFileSync(join(root, "database-marker"), `${fixture.sha}:${delivery}\n`);

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      delivery,
    });

    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stderr, /already committed and published/);
    assert.equal(readlinkSync(join(mirror, "public", "current")), target);
    assert.equal(existsSync(currentFile(mirror, "keep.pdf")), true);
    assert.equal(existsSync(join(root, "import.sql")), false);
    assert.equal(existsSync(join(mirror, "repo")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("an exact database marker fails closed when the published snapshot metadata disagrees", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root);
    const mirror = join(root, "mirror");
    const target = ".snapshots/inconsistent";
    const snapshot = join(mirror, "public", target);
    const delivery = "delivery-consistency-check";
    mkdirSync(join(snapshot, "files"), { recursive: true });
    mkdirSync(join(snapshot, "slides"), { recursive: true });
    writeFileSync(join(snapshot, "SYNCED_SHA"), `${fixture.sha}\n`);
    writeFileSync(join(snapshot, "DELIVERY"), "different-delivery\n");
    symlinkSync(target, join(mirror, "public", "current"));
    writeFileSync(join(root, "database-marker"), `${fixture.sha}:${delivery}\n`);

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      delivery,
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /database marker matches event but published snapshot metadata is inconsistent/);
    assert.equal(readlinkSync(join(mirror, "public", "current")), target);
    assert.equal(existsSync(join(root, "import.sql")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("database failure restores the previously published materials snapshot", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root);
    const mirror = join(root, "mirror");
    mkdirSync(join(mirror, "public"), { recursive: true });
    mkdirSync(join(mirror, "slides"), { recursive: true });
    writeFileSync(join(mirror, "public", "previous.pdf"), "previous snapshot\n");
    writeFileSync(join(mirror, "SYNCED_SHA"), `${"f".repeat(40)}\n`);

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      psqlExit: 9,
    });

    assert.notEqual(result.status, 0, result.stderr);
    assert.equal(existsSync(currentFile(mirror, "previous.pdf")), true);
    assert.equal(existsSync(currentFile(mirror, fixture.publicPath)), false);
    assert.equal(readFileSync(join(mirror, "SYNCED_SHA"), "utf8").trim(), "f".repeat(40));
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a legacy snapshot stays traversable under the privileged umask", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root);
    const mirror = join(root, "mirror");
    const legacyFiles = join(mirror, "public", "course", "week-1");
    const legacySlides = join(mirror, "slides", "course");
    mkdirSync(legacyFiles, { recursive: true });
    mkdirSync(legacySlides, { recursive: true });
    chmodSync(join(mirror, "public", "course"), 0o700);
    chmodSync(legacyFiles, 0o700);
    chmodSync(join(mirror, "slides"), 0o700);
    chmodSync(legacySlides, 0o700);
    writeFileSync(join(legacyFiles, "legacy.pdf"), "previous snapshot\n");
    writeFileSync(join(legacySlides, "legacy.json"), "{}\n");
    writeFileSync(join(mirror, "SYNCED_SHA"), `${"f".repeat(40)}\n`);

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      psqlExit: 9,
    });

    assert.notEqual(result.status, 0, result.stderr);
    const currentTarget = readlinkSync(join(mirror, "public", "current"));
    for (const directory of [
      join(mirror, "public"),
      join(mirror, "public", ".snapshots"),
      join(mirror, "public", currentTarget),
      join(mirror, "public", currentTarget, "files"),
      join(mirror, "public", currentTarget, "files", "course"),
      join(mirror, "public", currentTarget, "files", "course", "week-1"),
      join(mirror, "public", currentTarget, "slides"),
      join(mirror, "public", currentTarget, "slides", "course"),
    ]) {
      assert.equal(statSync(directory).mode & 0o777, 0o755, directory);
    }
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a missing Study expand migration fails before public layout changes", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root);
    const mirror = join(root, "mirror");
    mkdirSync(join(mirror, "public"), { recursive: true });
    writeFileSync(join(mirror, "public", "previous.pdf"), "previous snapshot\n");

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      extraEnv: { HENUKIT_TEST_SCHEMA_STATUS: "missing" },
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /Study materials expand migration 0002 is required/);
    assert.equal(existsSync(join(mirror, "public", "previous.pdf")), true);
    assert.equal(existsSync(join(mirror, "public", "current")), false);
    assert.equal(existsSync(join(root, "import.sql")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("recovery finalizes files when the same database transaction marker committed", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root);
    const mirror = join(root, "mirror");
    const snapshots = join(mirror, "public", ".snapshots");
    const oldTarget = ".snapshots/old";
    const newTarget = ".snapshots/new";
    const committedSha = "b".repeat(40);
    mkdirSync(join(snapshots, "old", "files"), { recursive: true });
    mkdirSync(join(snapshots, "new", "files"), { recursive: true });
    mkdirSync(join(snapshots, "new", "slides"), { recursive: true });
    writeFileSync(join(snapshots, "old", "files", "old.pdf"), "old\n");
    writeFileSync(join(snapshots, "new", "files", "committed.pdf"), "committed\n");
    writeFileSync(join(snapshots, "new", "SYNCED_SHA"), `${committedSha}\n`);
    symlinkSync(newTarget, join(mirror, "public", "current"));
    mkdirSync(join(mirror, ".sync-transaction"), { recursive: true });
    writeFileSync(join(mirror, ".sync-transaction", "phase"), "published\n");
    writeFileSync(join(mirror, ".sync-transaction", "previous"), `${oldTarget}\n`);
    writeFileSync(join(mirror, ".sync-transaction", "new"), `${newTarget}\n`);
    writeFileSync(join(mirror, ".sync-transaction", "sha"), `${committedSha}\n`);
    writeFileSync(join(mirror, ".sync-transaction", "delivery"), "delivery-committed\n");
    writeFileSync(join(root, "database-marker"), `${committedSha}:delivery-committed\n`);

    const result = runDriver({
      root,
      source: fixture.source,
      sha: "a".repeat(40),
      psqlExit: 0,
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /database marker confirms interrupted COMMIT/);
    assert.equal(existsSync(currentFile(mirror, "committed.pdf")), true);
    assert.equal(existsSync(join(snapshots, "old")), false);
    assert.equal(existsSync(join(mirror, ".sync-transaction")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("corrupt snapshot metadata cannot recursively delete the public root", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root);
    const mirror = join(root, "mirror");
    const snapshots = join(mirror, "public", ".snapshots");
    mkdirSync(join(snapshots, "old", "files"), { recursive: true });
    mkdirSync(join(snapshots, "new", "files"), { recursive: true });
    writeFileSync(join(snapshots, "old", "files", "keep.pdf"), "keep\n");
    symlinkSync(".snapshots/old", join(mirror, "public", "current"));
    mkdirSync(join(mirror, ".sync-transaction"), { recursive: true });
    writeFileSync(join(mirror, ".sync-transaction", "phase"), "published\n");
    writeFileSync(join(mirror, ".sync-transaction", "previous"), ".snapshots/..\n");
    writeFileSync(join(mirror, ".sync-transaction", "new"), ".snapshots/new\n");
    writeFileSync(join(mirror, ".sync-transaction", "sha"), `${"b".repeat(40)}\n`);
    writeFileSync(join(mirror, ".sync-transaction", "delivery"), "delivery-corrupt\n");

    const result = runDriver({
      root,
      source: fixture.source,
      sha: "a".repeat(40),
      psqlExit: 0,
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /invalid materials snapshot target/);
    assert.equal(existsSync(join(mirror, "public")), true);
    assert.equal(existsSync(currentFile(mirror, "keep.pdf")), true);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("an interrupted first-layout migration resumes without moving live sources", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root);
    const mirror = join(root, "mirror");
    const snapshot = join(mirror, "public", ".snapshots", "legacy-recovery");
    mkdirSync(join(snapshot, "files"), { recursive: true });
    mkdirSync(join(snapshot, "slides"), { recursive: true });
    writeFileSync(join(snapshot, "files", "legacy.pdf"), "copied before switch\n");
    writeFileSync(join(snapshot, ".legacy-migration"), "");
    writeFileSync(join(snapshot, ".legacy-sources"), Buffer.from("legacy.pdf\0"));
    writeFileSync(join(snapshot, ".legacy-slides"), "");
    writeFileSync(join(snapshot, ".legacy-synced-sha"), "");
    writeFileSync(join(mirror, "public", "legacy.pdf"), "source remains after switch\n");
    mkdirSync(join(mirror, "slides"), { recursive: true });
    writeFileSync(join(mirror, "SYNCED_SHA"), `${"f".repeat(40)}\n`);
    symlinkSync(".snapshots/legacy-recovery", join(mirror, "public", "current"));

    const result = runDriver({
      root,
      source: fixture.source,
      sha: "a".repeat(40),
      psqlExit: 0,
    });

    assert.notEqual(result.status, 0);
    assert.equal(existsSync(join(mirror, "public", "legacy.pdf")), false);
    assert.equal(existsSync(currentFile(mirror, "legacy.pdf")), true);
    assert.equal(lstatSync(join(mirror, "slides")).isSymbolicLink(), true);
    assert.equal(lstatSync(join(mirror, "SYNCED_SHA")).isSymbolicLink(), true);
    assert.equal(existsSync(join(snapshot, ".legacy-migration")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("one successful run publishes the verified snapshot, derived slides, and catalogue once", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root, { includeSlides: true });
    const mirror = join(root, "mirror");
    mkdirSync(join(mirror, "public"), { recursive: true });
    const publicInode = statSync(join(mirror, "public")).ino;
    writeFileSync(join(mirror, "public", "previous.pdf"), "previous snapshot\n");
    writeFileSync(join(mirror, "SYNCED_SHA"), `${"f".repeat(40)}\n`);

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      psqlExit: 0,
    });

    assert.equal(result.status, 0, result.stderr);
    assert.equal(statSync(join(mirror, "public")).ino, publicInode);
    assert.equal(statSync(join(mirror, "public")).mode & 0o777, 0o755);
    assert.equal(statSync(join(mirror, "public", ".snapshots")).mode & 0o777, 0o755);
    const currentTarget = readlinkSync(join(mirror, "public", "current"));
    assert.equal(
      statSync(join(mirror, "public", currentTarget)).mode & 0o777,
      0o755,
    );
    assert.equal(
      statSync(join(mirror, "public", currentTarget, "files")).mode & 0o777,
      0o755,
    );
    assert.equal(existsSync(currentFile(mirror, "previous.pdf")), false);
    assert.equal(existsSync(currentFile(mirror, fixture.publicPath)), true);
    const derivedSlidesPath = join(
      mirror,
      "slides",
      "高等数学A（二）/课件PPT/二重积分.pptx.json",
    );
    assert.equal(existsSync(derivedSlidesPath), true);
    const derivedSlides = JSON.parse(readFileSync(derivedSlidesPath, "utf8"));
    assert.deepEqual(Object.keys(derivedSlides), ["slides"]);
    assert.equal(Array.isArray(derivedSlides.slides), true);
    assert.equal(readFileSync(join(mirror, "SYNCED_SHA"), "utf8").trim(), fixture.sha);
    assert.equal(readFileSync(join(root, "psql-runs"), "utf8").trim(), "run\nrun\nrun");
    const psqlArguments = readFileSync(join(root, "psql-args"), "utf8")
      .trim()
      .split("\n");
    for (const argumentsLine of psqlArguments) {
      assert.match(argumentsLine, /(?:^| )-X(?: |$)/, argumentsLine);
      assert.match(argumentsLine, /(?:^| )-v ON_ERROR_STOP=1(?: |$)/, argumentsLine);
      assert.doesNotMatch(argumentsLine, /postgres:/);
    }
    assert.deepEqual(
      readFileSync(join(root, "psql-databases"), "utf8").trim().split("\n"),
      ["study", "study", "study"],
    );
    const sql = readFileSync(join(root, "import.sql"), "utf8");
    assert.match(sql, /^BEGIN;/m);
    assert.match(sql, /'\[\{"title":"二重积分"/);
    assert.doesNotMatch(sql, /'\{"slides":/);
    assert.match(sql, /COMMIT;$/m);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a stale webhook SHA cannot publish the current target branch", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root);
    const mirror = join(root, "mirror");
    mkdirSync(join(mirror, "public"), { recursive: true });
    writeFileSync(join(mirror, "public", "previous.pdf"), "previous snapshot\n");
    writeFileSync(join(mirror, "SYNCED_SHA"), `${"f".repeat(40)}\n`);

    const result = runDriver({
      root,
      source: fixture.source,
      sha: "a".repeat(40),
      psqlExit: 0,
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /expected webhook SHA/);
    assert.equal(existsSync(currentFile(mirror, "previous.pdf")), true);
    assert.equal(existsSync(join(root, "import.sql")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("an escaping manifest path is rejected before publication or database sync", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root, { publicPath: "../outside.pdf" });
    const mirror = join(root, "mirror");
    mkdirSync(join(mirror, "public"), { recursive: true });
    writeFileSync(join(mirror, "public", "previous.pdf"), "previous snapshot\n");
    writeFileSync(join(mirror, "SYNCED_SHA"), `${"f".repeat(40)}\n`);

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      psqlExit: 0,
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /manifest path escapes the checkout/);
    assert.equal(existsSync(currentFile(mirror, "previous.pdf")), true);
    assert.equal(existsSync(join(root, "import.sql")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a manifest symlink is rejected even when it resolves inside the checkout", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root, { symlinkAsset: true });
    const mirror = join(root, "mirror");
    mkdirSync(join(mirror, "public"), { recursive: true });
    writeFileSync(join(mirror, "public", "previous.pdf"), "previous snapshot\n");

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      psqlExit: 0,
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /not a regular file/);
    assert.equal(existsSync(currentFile(mirror, "previous.pdf")), true);
    assert.equal(existsSync(join(root, "import.sql")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a dotfile named by the manifest is rejected before publication", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root, { publicPath: "approved/.hidden.pdf" });
    const mirror = join(root, "mirror");
    mkdirSync(join(mirror, "public"), { recursive: true });
    writeFileSync(join(mirror, "public", "previous.pdf"), "previous snapshot\n");

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      psqlExit: 0,
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /mirror contains dotfiles/);
    assert.equal(existsSync(currentFile(mirror, "previous.pdf")), true);
    assert.equal(existsSync(join(root, "import.sql")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("a manifest hash mismatch is rejected before publication", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-materials-sync-"));
  try {
    const fixture = createMaterialsRepository(root, { sha256Override: "0".repeat(64) });
    const mirror = join(root, "mirror");
    mkdirSync(join(mirror, "public"), { recursive: true });
    writeFileSync(join(mirror, "public", "previous.pdf"), "previous snapshot\n");

    const result = runDriver({
      root,
      source: fixture.source,
      sha: fixture.sha,
      psqlExit: 0,
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /manifest sha256 mismatch/);
    assert.equal(existsSync(currentFile(mirror, "previous.pdf")), true);
    assert.equal(existsSync(join(root, "import.sql")), false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test(
  "real PostgreSQL migration 0002 is repeatable and has a compatibility-safe Down",
  { skip: !realPostgresURL },
  () => {
    const database = createRealStudyDatabase();
    try {
      assert.equal(
        queryPsql(
          database.databaseURL,
          "SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'materials' AND column_name IN ('sha256', 'slides');",
        ),
        "2",
      );
      assert.equal(
        queryPsql(
          database.databaseURL,
          "SELECT to_regclass('public.materials_storage_key_active_idx');",
        ),
        "materials_storage_key_active_idx",
      );

      runPsql(database.databaseURL, readFileSync(studyMigrationDown, "utf8"));

      assert.equal(
        queryPsql(
          database.databaseURL,
          "SELECT to_regclass('public.henukit_materials_sync_state');",
        ),
        "",
      );
      assert.equal(
        queryPsql(
          database.databaseURL,
          "SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'materials' AND column_name IN ('sha256', 'slides');",
        ),
        "2",
      );
    } finally {
      database.cleanup();
    }
  },
);

test(
  "an invalid public marker schema fails before any public layout switch",
  { skip: !realPostgresURL },
  () => {
    const invalidSchemas = [
      {
        name: "wrong singleton type",
        definition: `
CREATE TABLE public.henukit_materials_sync_state (
  singleton integer PRIMARY KEY CHECK (singleton = 1),
  synced_sha text NOT NULL,
  delivery text NOT NULL,
  updated_at timestamptz NOT NULL
);`,
      },
      {
        name: "missing primary key and singleton check",
        definition: `
CREATE TABLE public.henukit_materials_sync_state (
  singleton smallint NOT NULL,
  synced_sha text NOT NULL,
  delivery text NOT NULL,
  updated_at timestamptz NOT NULL
);`,
      },
      {
        name: "nullable delivery",
        definition: `
CREATE TABLE public.henukit_materials_sync_state (
  singleton smallint PRIMARY KEY CHECK (singleton = 1),
  synced_sha text NOT NULL,
  delivery text,
  updated_at timestamptz NOT NULL
);`,
      },
    ];

    for (const invalidSchema of invalidSchemas) {
      const root = mkdtempSync(join(tmpdir(), "henukit-materials-marker-schema-"));
      const database = createRealStudyDatabase();
      try {
        runPsql(
          database.databaseURL,
          `
DROP TABLE public.henukit_materials_sync_state;
${invalidSchema.definition}
`,
        );
        const fixture = createMaterialsRepository(root);
        const mirror = join(root, "mirror");
        const legacyPath = join(mirror, "public", "legacy-material.pdf");
        mkdirSync(dirname(legacyPath), { recursive: true });
        writeFileSync(legacyPath, "must remain in the untouched legacy layout\n");
        const realPsql = execFileSync("sh", ["-c", "command -v psql"], { encoding: "utf8" }).trim();

        const result = runDriver({
          root,
          source: fixture.source,
          sha: fixture.sha,
          databaseURL: database.databaseURL,
          psqlBin: dirname(realPsql),
        });

        assert.notEqual(result.status, 0, invalidSchema.name);
        assert.equal(existsSync(join(mirror, "public", "current")), false, invalidSchema.name);
        assert.equal(
          readFileSync(legacyPath, "utf8"),
          "must remain in the untouched legacy layout\n",
          invalidSchema.name,
        );
        assert.match(result.stderr, /Study materials expand migration 0002 is required/, invalidSchema.name);
      } finally {
        database.cleanup();
        rmSync(root, { recursive: true, force: true });
      }
    }
  },
);

test(
  "real PostgreSQL keeps files and the committed catalogue marker converged across success, interruption, and SQL failure",
  { skip: !realPostgresURL },
  () => {
    const root = mkdtempSync(join(tmpdir(), "henukit-materials-postgres-"));
    const database = createRealStudyDatabase();
    try {
      const fixture = createMaterialsRepository(root);
      const mirror = join(root, "mirror");
      const realPsql = execFileSync("sh", ["-c", "command -v psql"], { encoding: "utf8" }).trim();
      const realPsqlBin = dirname(realPsql);

      const initial = runDriver({
        root,
        source: fixture.source,
        sha: fixture.sha,
        databaseURL: database.databaseURL,
        psqlBin: realPsqlBin,
      });
      assert.equal(initial.status, 0, initial.stderr);
      assert.equal(
        queryPsql(database.databaseURL, "SELECT synced_sha FROM public.henukit_materials_sync_state WHERE singleton = 1;"),
        fixture.sha,
      );
      const roleSchema = queryPsql(database.databaseURL, "SELECT current_user;");
      const quotedRoleSchema = `"${roleSchema.replaceAll('"', '""')}"`;
      assert.equal(
        queryPsql(
          database.databaseURL,
          `SELECT synced_sha FROM ${quotedRoleSchema}.henukit_materials_sync_state WHERE singleton = 1;`,
        ),
        "d".repeat(40),
      );

      const emptySha = markAllMaterialsPending(fixture.source);
      const emptied = runDriver({
        root,
        source: fixture.source,
        sha: emptySha,
        databaseURL: database.databaseURL,
        psqlBin: realPsqlBin,
      });
      assert.equal(emptied.status, 0, emptied.stderr);
      assert.equal(existsSync(currentFile(mirror, fixture.publicPath)), false);
      assert.equal(
        queryPsql(
          database.databaseURL,
          `SELECT status FROM public.materials WHERE storage_key = '${fixture.publicPath}';`,
        ),
        "archived",
      );

      const interruptedPath = "增量资料/commit-interrupted.pdf";
      const interruptedSha = addMaterialCommit(
        fixture.source,
        interruptedPath,
        "committed before client acknowledgement\n",
      );
      const killBin = join(root, "kill-bin");
      const killPsql = join(killBin, "psql");
      const killSentinel = join(root, "killed-after-commit");
      mkdirSync(killBin, { recursive: true });
      writeFileSync(
        killPsql,
        `#!/usr/bin/env bash
set -euo pipefail
"$HENUKIT_REAL_PSQL" "$@"
status=$?
if (( status == 0 )) && [[ "$*" == *"-f"* ]] && [[ ! -e "$HENUKIT_KILL_SENTINEL" ]]; then
  : > "$HENUKIT_KILL_SENTINEL"
  kill -KILL "$PPID"
fi
exit "$status"
`,
      );
      chmodSync(killPsql, 0o755);

      const interrupted = runDriver({
        root,
        source: fixture.source,
        sha: interruptedSha,
        databaseURL: database.databaseURL,
        psqlBin: killBin,
        extraEnv: {
          HENUKIT_REAL_PSQL: realPsql,
          HENUKIT_KILL_SENTINEL: killSentinel,
        },
      });
      assert.notEqual(interrupted.status, 0);
      assert.equal(existsSync(join(mirror, ".sync-transaction")), true);
      assert.equal(
        queryPsql(database.databaseURL, "SELECT synced_sha FROM public.henukit_materials_sync_state WHERE singleton = 1;"),
        interruptedSha,
      );
      const interruptedTarget = readlinkSync(join(mirror, "public", "current"));

      const recovered = runDriver({
        root,
        source: fixture.source,
        sha: interruptedSha,
        databaseURL: database.databaseURL,
        psqlBin: realPsqlBin,
      });
      assert.equal(recovered.status, 0, recovered.stderr);
      assert.match(recovered.stderr, /database marker confirms interrupted COMMIT/);
      assert.match(recovered.stderr, /event already committed and published/);
      assert.equal(readlinkSync(join(mirror, "public", "current")), interruptedTarget);
      assert.equal(readFileSync(join(mirror, "SYNCED_SHA"), "utf8").trim(), interruptedSha);
      assert.equal(existsSync(currentFile(mirror, interruptedPath)), true);

      const rejectedPath = "increment/failure.pdf";
      runPsql(
        database.databaseURL,
        `ALTER TABLE materials ADD CONSTRAINT reject_failure_material CHECK (storage_key <> '${rejectedPath}');\n`,
      );
      const rejectedSha = addMaterialCommit(fixture.source, rejectedPath, "must roll back\n");
      const rejected = runDriver({
        root,
        source: fixture.source,
        sha: rejectedSha,
        databaseURL: database.databaseURL,
        psqlBin: realPsqlBin,
      });
      assert.notEqual(rejected.status, 0);
      assert.equal(readFileSync(join(mirror, "SYNCED_SHA"), "utf8").trim(), interruptedSha);
      assert.equal(existsSync(currentFile(mirror, rejectedPath)), false);
      assert.equal(
        queryPsql(database.databaseURL, "SELECT synced_sha FROM public.henukit_materials_sync_state WHERE singleton = 1;"),
        interruptedSha,
      );
    } finally {
      database.cleanup();
      rmSync(root, { recursive: true, force: true });
    }
  },
);
