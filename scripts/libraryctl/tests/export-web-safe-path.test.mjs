import assert from "node:assert/strict";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import test from "node:test";

import {
  ExportPathError,
  generateWebManifest,
} from "../lib/export-web.mjs";
import { rowsToCsv } from "../lib/materials.mjs";

const cliPath = fileURLToPath(new URL("../libraryctl.mjs", import.meta.url));

function createCourseFixture(t, materialPath) {
  const root = mkdtempSync(join(tmpdir(), "libraryctl-export-"));
  const coursePath = join(
    root,
    "02_学校库",
    "01_河南大学",
    "01_软件学院",
    "01_大一",
    "02_下学期",
    "01_离散数学",
  );
  const archivePath = join(coursePath, "00_课程档案");
  mkdirSync(archivePath, { recursive: true });
  writeFileSync(
    join(archivePath, "course.yaml"),
    [
      "school: 河南大学",
      "college: 软件学院",
      "stage: 大一",
      "semester: 下学期",
      "course_name: 离散数学",
      "",
    ].join("\n"),
  );
  writeFileSync(
    join(archivePath, "materials.csv"),
    rowsToCsv([
      {
        local_id: "MAT-001",
        title: "路径边界测试",
        type: "真题",
        status: "raw",
        path: materialPath,
      },
    ]),
  );
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return { root, coursePath };
}

function runExport(root, outPath) {
  return spawnSync(
    process.execPath,
    [cliPath, "export-web", "--root", root, "--out", outPath],
    { encoding: "utf-8" },
  );
}

test("direct export rejects an unsafe row without relying on validate", (t) => {
  const originalPath = "../outside.pdf";
  const { root, coursePath } = createCourseFixture(t, originalPath);
  writeFileSync(join(dirname(coursePath), "outside.pdf"), "outside");

  assert.throws(
    () => generateWebManifest(root),
    (error) =>
      error instanceof ExportPathError &&
      error.code === "PATH_TRAVERSAL_FORBIDDEN" &&
      error.course === "离散数学" &&
      error.localId === "MAT-001" &&
      error.path === originalPath,
  );
});

test("failed CLI export creates no manifest and returns structured JSON", (t) => {
  const originalPath = "..\\outside.pdf";
  const { root } = createCourseFixture(t, originalPath);
  const outPath = join(root, "output", "manifest.json");

  const result = runExport(root, outPath);
  const error = JSON.parse(result.stderr);

  assert.equal(result.status, 1);
  assert.equal(existsSync(outPath), false);
  assert.equal(error.ok, false);
  assert.equal(error.code, "PATH_TRAVERSAL_FORBIDDEN");
  assert.equal(error.course, "离散数学");
  assert.equal(error.localId, "MAT-001");
  assert.equal(error.path, originalPath);
});

test("failed CLI export preserves an existing output file", (t) => {
  const { root } = createCourseFixture(t, "C:\\Windows\\system.ini");
  const outPath = join(root, "manifest.json");
  writeFileSync(outPath, "existing-manifest");

  const result = runExport(root, outPath);

  assert.equal(result.status, 1);
  assert.equal(readFileSync(outPath, "utf-8"), "existing-manifest");
});

test("normal export succeeds and normalizes separators to forward slashes", (t) => {
  const inputPath =
    "04_原始资料\\01_真题样卷\\离散数学_真题_2023_期末试卷.pdf";
  const expectedPath =
    "04_原始资料/01_真题样卷/离散数学_真题_2023_期末试卷.pdf";
  const { root, coursePath } = createCourseFixture(t, inputPath);
  const materialPath = join(coursePath, ...expectedPath.split("/"));
  mkdirSync(dirname(materialPath), { recursive: true });
  writeFileSync(materialPath, "fixture");

  const manifest = generateWebManifest(root);
  assert.equal(manifest.courses[0].materials[0].path, expectedPath);

  const outPath = join(root, "output", "manifest.json");
  const result = runExport(root, outPath);
  const written = JSON.parse(readFileSync(outPath, "utf-8"));

  assert.equal(result.status, 0);
  assert.equal(written.courses[0].materials[0].path, expectedPath);
});
