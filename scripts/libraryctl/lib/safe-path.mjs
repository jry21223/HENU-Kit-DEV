// libraryctl — shared material path boundary

import { existsSync, realpathSync } from "node:fs";
import { isAbsolute, relative, resolve, sep, win32 } from "node:path";

const ERROR_MESSAGES = {
  INVALID_PATH: "路径必须是非空字符串，且不能包含 NUL 字节",
  ABSOLUTE_PATH_FORBIDDEN: "禁止使用绝对路径或 Windows 盘符路径",
  PATH_TRAVERSAL_FORBIDDEN: "禁止使用 .. 路径段",
  PATH_OUTSIDE_ROOT: "路径越出课程目录",
  SYMLINK_OUTSIDE_ROOT: "符号链接最终指向课程目录外",
  PATH_NOT_FOUND: "路径指向的文件不存在",
};

export class SafePathError extends Error {
  constructor(code, { rootPath, inputPath, cause } = {}) {
    super(ERROR_MESSAGES[code] ?? "路径校验失败", { cause });
    this.name = "SafePathError";
    this.code = code;
    this.rootPath = rootPath;
    this.inputPath = inputPath;
  }
}

function fail(code, details) {
  throw new SafePathError(code, details);
}

function isOutside(relativePath) {
  return (
    relativePath === ".." ||
    relativePath.startsWith(`..${sep}`) ||
    isAbsolute(relativePath)
  );
}

function toManifestPath(relativePath) {
  return relativePath.split(sep).join("/");
}

/**
 * Resolve a material path and prove that both its lexical and real paths stay
 * within the supplied course root.
 *
 * @param {string} rootPath
 * @param {string} inputPath
 * @param {{mustExist?: boolean}} options
 * @returns {{absolutePath: string, relativePath: string, realPath: string | null}}
 */
export function resolveWithinRoot(rootPath, inputPath, options = {}) {
  if (
    typeof rootPath !== "string" ||
    rootPath.trim() === "" ||
    rootPath.includes("\0") ||
    typeof inputPath !== "string" ||
    inputPath.trim() === "" ||
    inputPath.includes("\0")
  ) {
    fail("INVALID_PATH", { rootPath, inputPath });
  }

  const mustExist = options.mustExist ?? true;
  const normalizedInput = inputPath.replaceAll("\\", "/");

  if (
    normalizedInput.startsWith("/") ||
    win32.isAbsolute(inputPath) ||
    /^[A-Za-z]:/.test(inputPath)
  ) {
    fail("ABSOLUTE_PATH_FORBIDDEN", { rootPath, inputPath });
  }

  if (normalizedInput.split("/").includes("..")) {
    fail("PATH_TRAVERSAL_FORBIDDEN", { rootPath, inputPath });
  }

  const absoluteRoot = resolve(rootPath);
  const absolutePath = resolve(absoluteRoot, normalizedInput);
  const lexicalRelativePath = relative(absoluteRoot, absolutePath);

  if (lexicalRelativePath === "") {
    fail("INVALID_PATH", { rootPath, inputPath });
  }
  if (isOutside(lexicalRelativePath)) {
    fail("PATH_OUTSIDE_ROOT", { rootPath, inputPath });
  }

  if (!existsSync(absoluteRoot)) {
    fail("PATH_NOT_FOUND", { rootPath, inputPath });
  }
  if (!existsSync(absolutePath)) {
    if (mustExist) {
      fail("PATH_NOT_FOUND", { rootPath, inputPath });
    }
    return {
      absolutePath,
      relativePath: toManifestPath(lexicalRelativePath),
      realPath: null,
    };
  }

  let realRoot;
  let realPath;
  try {
    realRoot = realpathSync(absoluteRoot);
    realPath = realpathSync(absolutePath);
  } catch (cause) {
    const code = cause?.code === "ENOENT" || cause?.code === "ENOTDIR"
      ? "PATH_NOT_FOUND"
      : "INVALID_PATH";
    fail(code, { rootPath, inputPath, cause });
  }

  const realRelativePath = relative(realRoot, realPath);
  if (isOutside(realRelativePath)) {
    fail("SYMLINK_OUTSIDE_ROOT", { rootPath, inputPath });
  }

  return {
    absolutePath,
    relativePath: toManifestPath(lexicalRelativePath),
    realPath,
  };
}
