import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const script = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "import-henukit-materials.mjs",
);
const converter = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "convert-henukit-slides.py",
);
const studyMigrationUp = fileURLToPath(
  new URL("../../../services/api/migrations/0002_henukit_materials_sync_expand.up.sql", import.meta.url),
);
const realPostgresURL = process.env.HENUKIT_TEST_POSTGRES_URL ?? "";

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
  return execFileSync("psql", ["-X", "-v", "ON_ERROR_STOP=1", "-f", "-"], {
    encoding: "utf8",
    input: sql,
    env: psqlEnv(databaseURL),
  });
}

function queryPsql(databaseURL, sql) {
  return execFileSync("psql", ["-X", "-v", "ON_ERROR_STOP=1", "-tAc", sql], {
    encoding: "utf8",
    env: psqlEnv(databaseURL),
  }).trim();
}

function createRealImporterDatabase() {
  const databaseName = `hc306_import_${process.pid}_${Date.now()}`;
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
  runPsql(databaseURL.toString(), readFileSync(studyMigrationUp, "utf8"));
  return {
    databaseName,
    databaseURL: databaseURL.toString(),
    cleanup() {
      runPsql(realPostgresURL, `DROP DATABASE IF EXISTS ${databaseName} WITH (FORCE);\n`);
    },
  };
}

const MANIFEST = {
  version: 1,
  generatedAt: "2026-08-05T00:00:00.000Z",
  subjects: [
    {
      name: "高等数学A（二）",
      note: "科目简介",
      assets: [
        {
          subject: "高等数学A（二）",
          role: "复习讲义",
          title: "高等数学A（二）_考前复习知识点讲义.pdf",
          publicPath: "高等数学A（二）/复习讲义/高等数学A（二）_考前复习知识点讲义.pdf",
          bytes: 237837,
          sha256: "8d78ba648b48cf8b36d306822a16f0bdbed08f74e3507bbaf65690f7a0eb93a6",
        },
        {
          subject: "高等数学A（二）",
          role: "课件PPT",
          title: "高等数学A（二）_课件_D10-1二重积分概念.ppt",
          publicPath: "高等数学A（二）/课件PPT/高等数学A（二）_课件_D10-1二重积分概念.ppt",
          bytes: 2318336,
          sha256: "66aa36fe4983fefddbd31eb9eb48a1730467fdf7af054b74b6f950e9ff7631f2",
          sourceNote: "老师课件,含引号 ' 与单引号''",
        },
        {
          subject: "高等数学A（二）",
          role: "待复核资料",
          title: "高等数学A（二）_待复核_第八章自测.docx",
          publicPath: "高等数学A（二）/待复核资料/高等数学A（二）_待复核_第八章自测.docx",
          reviewStatus: "needs_review",
          bytes: 33967,
          sha256: "47b88cdceab421b479ed2d2d28b6daa86e60c37fd9b5ae967576fc25094e8220",
        },
      ],
    },
  ],
};

function runImport({
  manifest = MANIFEST,
  slidesDir,
  slidesContent,
  expectedStatus = 0,
  school,
  college,
  major,
  grade,
} = {}) {
  const dir = mkdtempSync(join(tmpdir(), "henukit-import-"));
  const manifestPath = join(dir, "manifest.json");
  writeFileSync(manifestPath, JSON.stringify(manifest));
  const args = [
    "--manifest",
    manifestPath,
    "--sync-sha",
    "a".repeat(40),
    "--delivery",
    "delivery-import-test",
  ];
  for (const [flag, value] of [
    ["--school", school],
    ["--college", college],
    ["--major", major],
    ["--grade", grade],
  ]) {
    if (value !== undefined) args.push(flag, value);
  }
  if (slidesDir) {
    const slidesPath = join(
      slidesDir,
      "高等数学A（二）/课件PPT/高等数学A（二）_课件_D10-1二重积分概念.ppt.json",
    );
    mkdirSync(dirname(slidesPath), { recursive: true });
    writeFileSync(
      slidesPath,
      slidesContent ?? JSON.stringify({
        slides: [
          { title: "二重积分概念", blocks: ["定义", "几何意义"] },
        ],
      }),
    );
    args.push("--slides-dir", slidesDir);
  }
  const result = spawnSync(process.execPath, [script, ...args], {
    encoding: "utf8",
  });
  assert.equal(result.status, expectedStatus, result.stderr);
  rmSync(dir, { recursive: true, force: true });
  return { stdout: result.stdout, stderr: result.stderr, status: result.status };
}

test("import emits idempotent transaction SQL", () => {
  const { stdout } = runImport();
  const storageKey = MANIFEST.subjects[0].assets[0].publicPath;
  assert.match(stdout, /^BEGIN;/m);
  assert.match(stdout, /COMMIT;$/m);
  assert.doesNotMatch(stdout, /\b(?:ALTER|CREATE|DROP)\b/);
  assert.match(stdout, /ON CONFLICT \(storage_key\) WHERE deleted_at IS NULL DO UPDATE/);
  assert.ok(stdout.includes(`'${storageKey}'`), "importer must persist the relative storage identity");
  assert.ok(
    !stdout.includes(`'/materials/${storageKey}'`),
    "the public URL prefix belongs to the Portal API boundary, not storage_key",
  );
  assert.match(
    stdout,
    /INSERT INTO public\.henukit_materials_sync_state[^\n]*'a{40}', 'delivery-import-test'/,
  );
  assert.ok(
    stdout.indexOf("INSERT INTO public.henukit_materials_sync_state") < stdout.lastIndexOf("COMMIT;"),
    "durable sync marker must commit in the same transaction",
  );
});

test("every course lookup is scoped to school, college, major, grade, and course", () => {
  const manifest = {
    version: 1,
    subjects: [
      {
        name: "Shared Course",
        assets: [
          {
            role: "approved-reference",
            title: "Shared Course_target.pdf",
            publicPath: "course/target.pdf",
            bytes: 123,
            sha256: "b".repeat(64),
          },
        ],
      },
    ],
  };
  const { stdout } = runImport({
    manifest,
    school: "Target School",
    college: "Target College",
    major: "Target Major",
    grade: "2026",
  });

  assert.match(
    stdout,
    /NOT EXISTS \(SELECT 1 FROM public\.courses x WHERE x\.school_id = s\.id AND x\.college_id = c\.id AND x\.major_id = m\.id AND x\.grade = '2026' AND x\.name = 'Shared Course' AND x\.deleted_at IS NULL\)/,
  );
  assert.match(
    stdout,
    /UPDATE public\.courses x SET status = 'published'.*FROM public\.schools s JOIN public\.colleges c.*JOIN public\.majors m.*x\.school_id = s\.id AND x\.college_id = c\.id AND x\.major_id = m\.id AND x\.grade = '2026' AND x\.name = 'Shared Course'/,
  );
  assert.match(
    stdout,
    /FROM public\.schools s JOIN public\.colleges c.*JOIN public\.majors m.*JOIN public\.courses x ON x\.school_id = s\.id AND x\.college_id = c\.id AND x\.major_id = m\.id AND x\.grade = '2026' AND x\.name = 'Shared Course' AND x\.deleted_at IS NULL/,
  );
});

test(
  "real PostgreSQL rehomes an existing material to the one fully identified course without duplicate effects",
  { skip: !realPostgresURL },
  () => {
    const database = createRealImporterDatabase();
    try {
      runPsql(
        database.databaseURL,
        `
INSERT INTO schools VALUES
  ('00000000-0000-0000-0000-000000000001', 'Target School', 'subject-355129919b73', NULL, 'published', now(), now());
INSERT INTO colleges VALUES
  ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 'Target College', 'published', now(), now());
INSERT INTO majors VALUES
  ('00000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', 'Target Major', 'target-major', 'published', now(), now()),
  ('00000000-0000-0000-0000-000000000004', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', 'Other Major', 'other-major', 'published', now(), now());
INSERT INTO courses VALUES
  ('00000000-0000-0000-0000-000000000005', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000004', '2026', 'Shared Course', 'wrong-major-course', '', 'archived', now(), now(), NULL),
  ('00000000-0000-0000-0000-000000000006', '00000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000003', '2025', 'Shared Course', 'wrong-grade-course', '', 'archived', now(), now(), NULL);
INSERT INTO materials (
  id, course_id, title, type, description, storage_key, file_name, file_size,
  sha256, slides, access_level, status, reviewed_at, review_reason, created_at, updated_at, deleted_at
) VALUES (
  '00000000-0000-0000-0000-000000000007',
  '00000000-0000-0000-0000-000000000005',
  'stale target', 'note', '', 'course/target.pdf', 'target.pdf', 1,
  '${"c".repeat(64)}', NULL, 'free', 'published', now(), 'stale course link', now(), now(), NULL
);
`,
      );

      const manifest = {
        version: 1,
        subjects: [
          {
            name: "Shared Course",
            assets: [
              {
                role: "approved-reference",
                title: "Shared Course_target.pdf",
                publicPath: "course/target.pdf",
                bytes: 123,
                sha256: "b".repeat(64),
              },
            ],
          },
        ],
      };
      const { stdout } = runImport({
        manifest,
        school: "Target School",
        college: "Target College",
        major: "Target Major",
        grade: "2026",
      });
      runPsql(database.databaseURL, stdout);
      runPsql(database.databaseURL, stdout);

      assert.equal(
        queryPsql(
          database.databaseURL,
          "SELECT count(*) FROM courses WHERE school_id = '00000000-0000-0000-0000-000000000001' AND college_id = '00000000-0000-0000-0000-000000000002' AND major_id = '00000000-0000-0000-0000-000000000003' AND grade = '2026' AND name = 'Shared Course' AND deleted_at IS NULL;",
        ),
        "1",
      );
      assert.equal(
        queryPsql(
          database.databaseURL,
          "SELECT count(*) FROM courses WHERE name = 'Shared Course' AND deleted_at IS NULL;",
        ),
        "3",
      );
      assert.equal(
        queryPsql(
          database.databaseURL,
          "SELECT string_agg(status, ',' ORDER BY grade, major_id::text) FROM courses WHERE id IN ('00000000-0000-0000-0000-000000000005', '00000000-0000-0000-0000-000000000006');",
        ),
        "archived,archived",
      );
      assert.equal(
        queryPsql(
          database.databaseURL,
          "SELECT ma.name || '|' || c.grade FROM materials mt JOIN courses c ON c.id = mt.course_id JOIN majors ma ON ma.id = c.major_id WHERE mt.storage_key = 'course/target.pdf' AND mt.deleted_at IS NULL;",
        ),
        "Target Major|2026",
      );
      assert.equal(
        queryPsql(
          database.databaseURL,
          "SELECT count(*) FROM materials WHERE storage_key = 'course/target.pdf' AND deleted_at IS NULL;",
        ),
        "1",
      );
    } finally {
      database.cleanup();
    }
  },
);

test("normalizes titles: strips course prefix, role marker and extension", () => {
  const { stdout } = runImport();
  // 标题列已归一化(去掉科目前缀/类型标记/扩展名),type 紧随其后。
  assert.match(stdout, /'考前复习知识点讲义', 'path'/);
  assert.match(stdout, /'D10-1二重积分概念', 'slides'/);
  // 原始文件名不得作为标题出现(它仍会以 storage_key 形式出现)。
  assert.doesNotMatch(stdout, /'高等数学A（二）_考前复习知识点讲义\.pdf', 'path'/);
});

test("maps roles to portal types", () => {
  const { stdout } = runImport();
  // VALUES 列序:id, course_id, title, type, description, ...
  assert.match(stdout, /'考前复习知识点讲义', 'path', '科目简介'/);
  assert.match(stdout, /'D10-1二重积分概念', 'slides', '老师课件/);
});

test("skips assets whose role starts with 待复核", () => {
  const { stdout, stderr } = runImport();
  assert.doesNotMatch(stdout, /第八章自测/);
  assert.match(stderr, /"skipped_pending_review":1/);
});

test("quotes single quotes in text values", () => {
  const { stdout } = runImport();
  assert.match(stdout, /含引号 '' 与单引号''''/);
});

test("attaches converted slides jsonb for slides assets", () => {
  const dir = mkdtempSync(join(tmpdir(), "henukit-slides-"));
  const { stdout } = runImport({ slidesDir: dir });
  assert.match(stdout, /'\[\{"title":"二重积分概念".*\}\]'::jsonb/);
  assert.doesNotMatch(stdout, /'\{"slides":/);
  assert.match(stdout, /slides = EXCLUDED\.slides/);
  rmSync(dir, { recursive: true, force: true });
});

test("rejects malformed derived slides instead of silently publishing a partial catalogue", () => {
  const dir = mkdtempSync(join(tmpdir(), "henukit-slides-invalid-"));
  const { stderr } = runImport({
    slidesDir: dir,
    slidesContent: "{not-json",
    expectedStatus: 1,
  });
  assert.match(stderr, /Unexpected token|JSON/);
  rmSync(dir, { recursive: true, force: true });
});

test("rejects derived slides whose wrapper or Slide fields violate the Portal array contract", () => {
  const invalidPayloads = [
    { payload: { slides: "not-an-array" }, message: /slides must be an array/ },
    { payload: { slides: [{ title: 7, blocks: [] }] }, message: /title must be a string/ },
    {
      payload: { slides: [{ title: "valid", blocks: ["text", 7] }] },
      message: /blocks must contain only strings/,
    },
  ];
  for (const { payload, message } of invalidPayloads) {
    const dir = mkdtempSync(join(tmpdir(), "henukit-slides-schema-"));
    try {
      const { stderr } = runImport({
        slidesDir: dir,
        slidesContent: JSON.stringify(payload),
        expectedStatus: 1,
      });
      assert.match(stderr, message);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  }
});

test("rejects boolean and otherwise invalid manifest bytes instead of skipping the asset", () => {
  for (const bytes of [true, -1, 1.5, Number.MAX_SAFE_INTEGER + 1]) {
    const manifest = structuredClone(MANIFEST);
    manifest.subjects[0].assets[0].bytes = bytes;
    const { stderr } = runImport({ manifest, expectedStatus: 1 });
    assert.match(stderr, /asset\.bytes must be a non-negative safe integer/);
  }
});

test("the Python converter rejects bool bytes as distinct from int", () => {
  const root = mkdtempSync(join(tmpdir(), "henukit-converter-bytes-"));
  try {
    const mirror = join(root, "mirror");
    const out = join(root, "slides");
    const publicPath = "course/deck.pptx";
    mkdirSync(join(mirror, "course"), { recursive: true });
    writeFileSync(join(mirror, publicPath), "not reached\n");
    const manifestPath = join(root, "manifest.json");
    writeFileSync(
      manifestPath,
      JSON.stringify({
        subjects: [
          {
            name: "course",
            assets: [
              {
                role: "课件PPT",
                publicPath,
                bytes: true,
                sha256: "a".repeat(64),
              },
            ],
          },
        ],
      }),
    );

    const result = spawnSync(
      "python3",
      [converter, "--mirror", mirror, "--out", out, "--manifest", manifestPath],
      { encoding: "utf8" },
    );
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /asset\.bytes must be a non-negative integer/);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("deactivates mirror rows no longer present in the manifest", () => {
  const { stdout } = runImport();
  assert.match(stdout, /storage_key NOT IN \('高等数学A（二）\/复习讲义\//);
  assert.match(stdout, /AND sha256 IS NOT NULL/);
});

test("deactivates every mirrored row when no asset is publishable", () => {
  const manifest = structuredClone(MANIFEST);
  manifest.subjects[0].assets = manifest.subjects[0].assets.filter((asset) =>
    asset.role.startsWith("待复核"),
  );
  const { stdout } = runImport({ manifest });

  assert.match(
    stdout,
    /UPDATE public\.materials SET status = 'archived'.*sha256 IS NOT NULL;/,
  );
  assert.doesNotMatch(stdout, /storage_key NOT IN \(\)/);
});

test("rejects a manifest without subjects", () => {
  const dir = mkdtempSync(join(tmpdir(), "henukit-bad-"));
  const manifestPath = join(dir, "manifest.json");
  writeFileSync(manifestPath, JSON.stringify({ version: 1 }));
  const result = spawnSync(process.execPath, [
    script,
    "--manifest",
    manifestPath,
    "--sync-sha",
    "a".repeat(40),
    "--delivery",
    "delivery-bad-manifest",
  ]);
  assert.notEqual(result.status, 0);
  assert.match(String(result.stderr), /manifest\.subjects must be an array/);
  rmSync(dir, { recursive: true, force: true });
});
