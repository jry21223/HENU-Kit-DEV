import assert from "node:assert/strict";
import {
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";

import {
  SafePathError,
  resolveWithinRoot,
} from "../lib/safe-path.mjs";

function createSandbox(t) {
  const sandbox = mkdtempSync(join(tmpdir(), "libraryctl-safe-path-"));
  const root = join(sandbox, "离散数学");
  mkdirSync(root, { recursive: true });
  t.after(() => rmSync(sandbox, { recursive: true, force: true }));
  return { sandbox, root };
}

test("resolves a normal nested material path and returns manifest separators", (t) => {
  const { root } = createSandbox(t);
  const relativePath =
    "04_原始资料/01_真题样卷/离散数学_真题_2023_期末试卷.pdf";
  const absolutePath = join(root, ...relativePath.split("/"));
  mkdirSync(dirname(absolutePath), { recursive: true });
  writeFileSync(absolutePath, "fixture");

  const result = resolveWithinRoot(root, relativePath);

  assert.equal(result.absolutePath, absolutePath);
  assert.equal(result.relativePath, relativePath);
  assert.equal(result.realPath, realpathSync(absolutePath));
});

test("treats backslashes as input separators for a safe nested path", (t) => {
  const { root } = createSandbox(t);
  const absolutePath = join(root, "04_原始资料", "01_真题样卷", "sample.pdf");
  mkdirSync(dirname(absolutePath), { recursive: true });
  writeFileSync(absolutePath, "fixture");

  const result = resolveWithinRoot(
    root,
    "04_原始资料\\01_真题样卷\\sample.pdf",
  );

  assert.equal(result.relativePath, "04_原始资料/01_真题样卷/sample.pdf");
});

test("rejects traversal, absolute, drive, UNC, NUL, and invalid paths", (t) => {
  const { root } = createSandbox(t);
  const cases = [
    ["", "INVALID_PATH"],
    ["../outside.pdf", "PATH_TRAVERSAL_FORBIDDEN"],
    ["../../outside.pdf", "PATH_TRAVERSAL_FORBIDDEN"],
    ["a/../../outside.pdf", "PATH_TRAVERSAL_FORBIDDEN"],
    ["..\\outside.pdf", "PATH_TRAVERSAL_FORBIDDEN"],
    ["/tmp/outside.pdf", "ABSOLUTE_PATH_FORBIDDEN"],
    ["C:\\Windows\\system.ini", "ABSOLUTE_PATH_FORBIDDEN"],
    ["C:/Windows/system.ini", "ABSOLUTE_PATH_FORBIDDEN"],
    ["\\\\server\\share\\file.pdf", "ABSOLUTE_PATH_FORBIDDEN"],
    ["folder\0file.pdf", "INVALID_PATH"],
  ];

  for (const [inputPath, code] of cases) {
    assert.throws(
      () => resolveWithinRoot(root, inputPath),
      (error) => error instanceof SafePathError && error.code === code,
      inputPath || "empty path",
    );
  }
});

test("returns PATH_NOT_FOUND for a safe but missing relative path", (t) => {
  const { root } = createSandbox(t);

  assert.throws(
    () => resolveWithinRoot(root, "04_原始资料/missing.pdf"),
    (error) => error instanceof SafePathError && error.code === "PATH_NOT_FOUND",
  );
});

test("rejects a symlink whose real target is outside the course root", (t) => {
  const { sandbox, root } = createSandbox(t);
  const outsidePath = join(sandbox, "outside.pdf");
  const linkPath = join(root, "04_原始资料", "linked.pdf");
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

  assert.throws(
    () => resolveWithinRoot(root, "04_原始资料/linked.pdf"),
    (error) =>
      error instanceof SafePathError && error.code === "SYMLINK_OUTSIDE_ROOT",
  );
});
