import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";

import { MAX_HASH_FILE_SIZE, hashAll, hashFile } from "../lib/hasher.mjs";
import { parseCsv, writeMaterialsCsv } from "../lib/materials.mjs";
import { createCourseStructure, createRootStructure } from "../lib/paths.mjs";

const SAMPLE_PATH = "04_原始资料/01_真题样卷/离散数学_真题_2023_期末试卷.pdf";

function sha256Of(content) {
  return createHash("sha256").update(content).digest("hex");
}

function createLibrary(t) {
  const sandbox = mkdtempSync(join(tmpdir(), "libraryctl-hasher-"));
  t.after(() => rmSync(sandbox, { recursive: true, force: true }));
  const root = join(sandbox, "资料库");
  createRootStructure(root);
  return root;
}

function addCourse(root, { course, courseIndex = 1, rows = [], files = {} }) {
  const { courseRelPath } = createCourseStructure(root, {
    school: "河南大学",
    college: "软件学院",
    stage: "大一",
    semester: "下学期",
    course,
    courseIndex,
  });
  const courseDir = join(root, courseRelPath);

  for (const [relPath, content] of Object.entries(files)) {
    const absPath = join(courseDir, ...relPath.split("/"));
    mkdirSync(dirname(absPath), { recursive: true });
    writeFileSync(absPath, content);
  }

  writeMaterialsCsv(courseDir, rows);
  return courseDir;
}

function materialRow(overrides) {
  return {
    local_id: "001",
    title: "离散数学2023期末真题",
    type: "真题",
    status: "reviewed",
    year: "2023",
    path: SAMPLE_PATH,
    source_note: "群内收集",
    sha256: "",
    web_id: "",
    notes: "",
    ...overrides,
  };
}

function readRows(courseDir) {
  return parseCsv(
    readFileSync(join(courseDir, "00_课程档案", "materials.csv"), "utf-8"),
  );
}

test("hashFile matches the digest of the file content", (t) => {
  const root = createLibrary(t);
  const courseDir = addCourse(root, {
    course: "离散数学",
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: "期末试卷" },
  });

  const digest = hashFile(join(courseDir, ...SAMPLE_PATH.split("/")));

  assert.equal(digest, sha256Of("期末试卷"));
});

test("hashFile hashes a file larger than one read chunk", (t) => {
  const root = createLibrary(t);
  const content = "x".repeat(1024 * 1024 + 4321);
  const courseDir = addCourse(root, {
    course: "离散数学",
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: content },
  });

  const digest = hashFile(join(courseDir, ...SAMPLE_PATH.split("/")));

  assert.equal(digest, sha256Of(content));
});

test("dry run reports digests without touching materials.csv", (t) => {
  const root = createLibrary(t);
  const courseDir = addCourse(root, {
    course: "离散数学",
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: "期末试卷" },
  });
  const before = readFileSync(
    join(courseDir, "00_课程档案", "materials.csv"),
    "utf-8",
  );

  const result = hashAll(root);

  assert.equal(result.ok, true);
  assert.equal(result.applied, false);
  assert.equal(result.summary.hashed, 1);
  assert.equal(result.summary.updatedCourses, 0);
  assert.equal(result.courses[0].updated, false);
  assert.equal(
    readFileSync(join(courseDir, "00_课程档案", "materials.csv"), "utf-8"),
    before,
  );
});

test("--apply writes missing digests back and keeps the other columns", (t) => {
  const root = createLibrary(t);
  const courseDir = addCourse(root, {
    course: "离散数学",
    rows: [materialRow({ notes: "备注,含逗号" })],
    files: { [SAMPLE_PATH]: "期末试卷" },
  });

  const result = hashAll(root, { apply: true });

  assert.equal(result.ok, true);
  assert.equal(result.applied, true);
  assert.equal(result.summary.hashed, 1);
  assert.equal(result.summary.updatedCourses, 1);

  const rows = readRows(courseDir);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].sha256, sha256Of("期末试卷"));
  assert.equal(rows[0].title, "离散数学2023期末真题");
  assert.equal(rows[0].path, SAMPLE_PATH);
  assert.equal(rows[0].notes, "备注,含逗号");
});

test("an already recorded sha256 is kept and still counted", (t) => {
  const root = createLibrary(t);
  const recorded = sha256Of("完全不同的内容");
  const courseDir = addCourse(root, {
    course: "离散数学",
    rows: [materialRow({ sha256: recorded })],
    files: { [SAMPLE_PATH]: "期末试卷" },
  });

  const result = hashAll(root, { apply: true });

  assert.equal(result.summary.hashed, 0);
  assert.equal(result.summary.alreadyHashed, 1);
  assert.equal(result.summary.updatedCourses, 0);
  assert.equal(readRows(courseDir)[0].sha256, recorded);
});

test("a malformed recorded sha256 warns and stays out of the duplicate groups", (t) => {
  const root = createLibrary(t);
  addCourse(root, {
    course: "离散数学",
    rows: [
      materialRow({ sha256: "not-a-digest" }),
      materialRow({ local_id: "002", sha256: "not-a-digest" }),
    ],
    files: { [SAMPLE_PATH]: "期末试卷" },
  });

  const result = hashAll(root);

  assert.equal(result.ok, true);
  assert.equal(result.summary.duplicateGroups, 0);
  assert.equal(result.warnings.length, 2);
  assert.match(result.warnings[0].message, /不是合法的 64 位十六进制值/);
});

test("identical files across courses surface as one duplicate group", (t) => {
  const root = createLibrary(t);
  addCourse(root, {
    course: "离散数学",
    courseIndex: 1,
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: "同一份真题" },
  });
  addCourse(root, {
    course: "线性代数",
    courseIndex: 2,
    rows: [materialRow({ local_id: "007" })],
    files: { [SAMPLE_PATH]: "同一份真题" },
  });

  const result = hashAll(root);

  assert.equal(result.summary.courses, 2);
  assert.equal(result.summary.duplicateGroups, 1);
  assert.equal(result.summary.duplicateFiles, 2);

  const group = result.possibleDuplicates[0];
  assert.equal(group.sha256, sha256Of("同一份真题"));
  assert.deepEqual(
    group.entries.map((e) => `${e.course}/${e.localId}`).sort(),
    ["离散数学/001", "线性代数/007"],
  );
});

test("a file over the size limit is skipped with a warning", (t) => {
  const root = createLibrary(t);
  const courseDir = addCourse(root, {
    course: "离散数学",
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: "x".repeat(2048) },
  });

  const result = hashAll(root, { apply: true, maxFileSize: 1024 });

  assert.equal(result.ok, true);
  assert.equal(result.summary.hashed, 0);
  assert.equal(result.summary.skipped, 1);
  assert.equal(result.summary.updatedCourses, 0);
  assert.match(result.warnings[0].message, /超过 0\.0MB 上限/);
  assert.equal(readRows(courseDir)[0].sha256, "");
});

test("the default size limit is 500MB", () => {
  assert.equal(MAX_HASH_FILE_SIZE, 500 * 1024 * 1024);
});

test("an unsafe or missing path is reported without stopping the other rows", (t) => {
  const root = createLibrary(t);
  const courseDir = addCourse(root, {
    course: "离散数学",
    rows: [
      materialRow({ local_id: "001", path: "../../越界.pdf" }),
      materialRow({ local_id: "002", path: "04_原始资料/01_真题样卷/缺失.pdf" }),
      materialRow({ local_id: "003" }),
    ],
    files: { [SAMPLE_PATH]: "期末试卷" },
  });

  const result = hashAll(root, { apply: true });

  assert.equal(result.ok, false);
  assert.equal(result.summary.errors, 2);
  assert.equal(result.summary.hashed, 1);
  assert.deepEqual(
    result.errors.map((e) => e.code),
    ["PATH_TRAVERSAL_FORBIDDEN", "PATH_NOT_FOUND"],
  );

  const rows = readRows(courseDir);
  assert.equal(rows[0].sha256, "");
  assert.equal(rows[1].sha256, "");
  assert.equal(rows[2].sha256, sha256Of("期末试卷"));
});

test("an unknown column blocks the write back instead of dropping it", (t) => {
  const root = createLibrary(t);
  const courseDir = addCourse(root, {
    course: "离散数学",
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: "期末试卷" },
  });
  const csvPath = join(courseDir, "00_课程档案", "materials.csv");
  const [header, row] = readFileSync(csvPath, "utf-8").trim().split("\n");
  writeFileSync(csvPath, `${header},owner\n${row},jerry\n`, "utf-8");

  const result = hashAll(root, { apply: true });

  assert.equal(result.ok, false);
  assert.equal(result.summary.updatedCourses, 0);
  assert.match(result.errors[0].message, /写回会丢失数据/);
  assert.equal(readFileSync(csvPath, "utf-8"), `${header},owner\n${row},jerry\n`);
});

test("a course without materials.csv is reported as a warning", (t) => {
  const root = createLibrary(t);
  const courseDir = addCourse(root, { course: "离散数学", rows: [] });
  rmSync(join(courseDir, "00_课程档案", "materials.csv"));

  const result = hashAll(root);

  assert.equal(result.ok, true);
  assert.equal(result.summary.materials, 0);
  assert.match(result.warnings[0].message, /materials\.csv 不存在/);
});

test("--course limits the run to one course", (t) => {
  const root = createLibrary(t);
  const target = addCourse(root, {
    course: "离散数学",
    courseIndex: 1,
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: "离散数学真题" },
  });
  const other = addCourse(root, {
    course: "线性代数",
    courseIndex: 2,
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: "线性代数真题" },
  });

  const result = hashAll(root, { apply: true, course: "离散数学" });

  assert.equal(result.summary.courses, 1);
  assert.equal(result.summary.hashed, 1);
  assert.equal(readRows(target)[0].sha256, sha256Of("离散数学真题"));
  assert.equal(readRows(other)[0].sha256, "");
});

test("--course also accepts the numbered course directory name", (t) => {
  const root = createLibrary(t);
  addCourse(root, {
    course: "离散数学",
    courseIndex: 2,
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: "期末试卷" },
  });

  const result = hashAll(root, { course: "02_离散数学" });

  assert.equal(result.summary.courses, 1);
  assert.equal(result.summary.hashed, 1);
});

test("--course that matches nothing is an error, not a silent no-op", (t) => {
  const root = createLibrary(t);
  addCourse(root, {
    course: "离散数学",
    rows: [materialRow({})],
    files: { [SAMPLE_PATH]: "期末试卷" },
  });

  const result = hashAll(root, { course: "不存在的课程" });

  assert.equal(result.ok, false);
  assert.equal(result.summary.courses, 0);
  assert.match(result.errors[0].message, /未找到匹配 --course/);
});
