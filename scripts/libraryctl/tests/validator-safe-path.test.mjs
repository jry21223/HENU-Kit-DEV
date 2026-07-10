import assert from "node:assert/strict";
import {
  mkdirSync,
  mkdtempSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import test from "node:test";

import { rowsToCsv } from "../lib/materials.mjs";
import { createRootStructure } from "../lib/paths.mjs";
import { validateAll } from "../lib/validator.mjs";

const cliPath = fileURLToPath(new URL("../libraryctl.mjs", import.meta.url));

function createCourseFixture(t, materialPath) {
  const root = mkdtempSync(join(tmpdir(), "libraryctl-validator-"));
  createRootStructure(root);
  const courseRelPath = join(
    "02_学校库",
    "01_河南大学",
    "01_软件学院",
    "01_大一",
    "02_下学期",
    "01_离散数学",
  );
  const coursePath = join(root, courseRelPath);
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

test("validateAll rejects traversal even when the external target exists", (t) => {
  const originalPath = "../../outside.pdf";
  const { root, coursePath } = createCourseFixture(t, originalPath);
  const outsidePath = resolve(coursePath, originalPath);
  mkdirSync(dirname(outsidePath), { recursive: true });
  writeFileSync(outsidePath, "outside");

  const result = validateAll(root);
  const issue = result.errors.find(
    (candidate) => candidate.code === "PATH_TRAVERSAL_FORBIDDEN",
  );

  assert.equal(result.ok, false);
  assert.ok(issue);
  assert.equal(issue.level, "error");
  assert.equal(issue.course, "离散数学");
  assert.equal(issue.file, "00_课程档案/materials.csv 第 2 行");
  assert.equal(issue.path, originalPath);
  assert.match(issue.message, /PATH_TRAVERSAL_FORBIDDEN/);
});

test("validator reports a symlink escape as a structured error", (t) => {
  const originalPath = "04_原始资料/linked.pdf";
  const { root, coursePath } = createCourseFixture(t, originalPath);
  const outsidePath = join(root, "outside.pdf");
  const linkPath = join(coursePath, ...originalPath.split("/"));
  mkdirSync(dirname(linkPath), { recursive: true });
  writeFileSync(outsidePath, "outside");

  try {
    symlinkSync(outsidePath, linkPath, "file");
  } catch (error) {
    if (["EPERM", "EACCES", "ENOTSUP"].includes(error.code)) {
      t.skip(`当前平台不支持测试符号链接: ${error.code}`);
      return;
    }
    throw error;
  }

  const result = validateAll(root);
  const issue = result.errors.find(
    (candidate) => candidate.code === "SYMLINK_OUTSIDE_ROOT",
  );

  assert.equal(result.ok, false);
  assert.ok(issue);
  assert.equal(issue.path, originalPath);
});

test("validator treats an empty material path as INVALID_PATH error", (t) => {
  const { root } = createCourseFixture(t, "");

  const result = validateAll(root);
  const issue = result.errors.find(
    (candidate) => candidate.code === "INVALID_PATH",
  );

  assert.equal(result.ok, false);
  assert.ok(issue);
  assert.equal(issue.level, "error");
  assert.equal(issue.path, "");
});

test("validate CLI prints structured JSON and exits 1 for a path error", (t) => {
  const originalPath = "../outside.pdf";
  const { root, coursePath } = createCourseFixture(t, originalPath);
  writeFileSync(join(dirname(coursePath), "outside.pdf"), "outside");

  const result = spawnSync(
    process.execPath,
    [cliPath, "validate", "--root", root],
    { encoding: "utf-8" },
  );
  const output = JSON.parse(result.stdout);
  const issue = output.errors.find(
    (candidate) => candidate.code === "PATH_TRAVERSAL_FORBIDDEN",
  );

  assert.equal(result.status, 1);
  assert.ok(issue);
  assert.equal(issue.course, "离散数学");
  assert.equal(issue.path, originalPath);
});
