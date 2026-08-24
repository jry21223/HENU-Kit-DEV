#!/usr/bin/env node
// libraryctl — 本地资料库规范化工具 CLI
//
// Usage:
//   node scripts/libraryctl/libraryctl.mjs <command> [options]
//
// Commands (V1):
//   init-root      初始化资料库根目录结构
//   init-course    初始化单个课程目录
//   validate       校验资料库结构和数据
//   export-web     生成网页后台导入清单
//   hash           计算 SHA256 并检测可能重复
//
// Commands (V2, not yet implemented):
//   scan           扫描资料库统计
//   normalize      规范文件命名
//   dedupe         去重

import { randomUUID } from "node:crypto";
import { resolve, join, dirname, basename } from "node:path";
import {
  existsSync,
  mkdirSync,
  renameSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { createRootStructure, createCourseStructure } from "./lib/paths.mjs";
import { generateDefaultCourseYaml, writeCourseYaml } from "./lib/course.mjs";
import { MATERIALS_HEADER } from "./lib/materials.mjs";
import { validateAll } from "./lib/validator.mjs";
import { ExportPathError, generateWebManifest } from "./lib/export-web.mjs";
import { hashAll } from "./lib/hasher.mjs";
import { SafePathError } from "./lib/safe-path.mjs";

function writeJsonAtomically(outPath, value) {
  const outputDir = dirname(outPath);
  mkdirSync(outputDir, { recursive: true });
  const tempPath = join(
    outputDir,
    `.${basename(outPath)}.${process.pid}.${randomUUID()}.tmp`,
  );

  try {
    writeFileSync(tempPath, JSON.stringify(value, null, 2), {
      encoding: "utf-8",
      flag: "wx",
    });
    renameSync(tempPath, outPath);
  } finally {
    if (existsSync(tempPath)) unlinkSync(tempPath);
  }
}

// ----- CLI argument parsing -----

function parseArgs(argv) {
  const args = { command: "", options: {}, flags: [] };

  for (let i = 2; i < argv.length; i++) {
    const arg = argv[i];

    if (arg.startsWith("--")) {
      const eqIdx = arg.indexOf("=");
      if (eqIdx !== -1) {
        const key = arg.slice(2, eqIdx);
        const val = arg.slice(eqIdx + 1);
        args.options[key] = val;
      } else {
        // Check next arg for value
        if (i + 1 < argv.length && !argv[i + 1].startsWith("--")) {
          args.options[arg.slice(2)] = argv[i + 1];
          i++;
        } else {
          args.flags.push(arg.slice(2));
        }
      }
    } else if (arg.startsWith("-")) {
      args.flags.push(arg.slice(1));
    } else if (!args.command) {
      args.command = arg;
    }
  }

  return args;
}

function printHelp() {
  console.log(`libraryctl — 本地资料库规范化工具

用法:
  node scripts/libraryctl/libraryctl.mjs <command> [options]

命令 (V1):
  init-root    --root <path>                  初始化资料库根目录结构
  init-course  --root <path> --school <name>  初始化单个课程目录
               --college <name> --stage <阶段>
               --semester <学期> --course <课程名>
  validate     --root <path>                  校验资料库结构和数据
  export-web   --root <path> [--out <file>]   生成网页后台导入清单
  hash         --root <path> [--course <名称>] 补全 materials.csv 缺失的 sha256
               [--apply]                       并列出可能重复的资料

命令 (V2, 尚未实现):
  scan         --root <path>                  扫描资料库统计
  normalize    --root <path> [--dry-run]      规范文件命名
  dedupe       --root <path> [--dry-run]      去重

选项:
  --root       资料库根目录路径（必填）
  --out        输出文件路径（export-web 使用）
  --course     只处理指定课程（hash 使用，课程名或课程目录名）
  --dry-run    仅预览，不实际修改（默认行为）
  --apply      实际执行修改
  --help       显示此帮助

示例:
  node scripts/libraryctl/libraryctl.mjs init-root --root ./资料库
  node scripts/libraryctl/libraryctl.mjs init-course --root ./资料库 \\
    --school 河南大学 --college 软件学院 --stage 大一 \\
    --semester 下学期 --course 离散数学
  node scripts/libraryctl/libraryctl.mjs validate --root ./资料库
  node scripts/libraryctl/libraryctl.mjs export-web --root ./资料库 \\
    --out ./dist/material-import-manifest.json
  node scripts/libraryctl/libraryctl.mjs hash --root ./资料库
  node scripts/libraryctl/libraryctl.mjs hash --root ./资料库 --course 离散数学 --apply
`);
}

// ----- Main -----

function main() {
  const argv = process.argv;
  const args = parseArgs(argv);

  if (args.flags.includes("help") || !args.command) {
    printHelp();
    process.exit(args.command ? 0 : 1);
  }

  const { command, options, flags } = args;

  try {
    switch (command) {
      case "init-root": {
        const root = resolve(options.root || ".");
        const result = createRootStructure(root);
        console.log(
          JSON.stringify(
            {
              ok: true,
              root,
              created: result.created,
              skipped: result.skipped,
              message: `根目录结构已初始化：创建 ${result.created.length} 个，跳过 ${result.skipped.length} 个（已存在）`,
            },
            null,
            2,
          ),
        );
        break;
      }

      case "init-course": {
        const root = resolve(options.root || ".");
        const school = options.school;
        const college = options.college;
        const stage = options.stage;
        const semester = options.semester;
        const course = options.course;
        const courseIndex = parseInt(options["course-index"] || "1", 10);

        if (!school || !college || !stage || !semester || !course) {
          console.error(
            JSON.stringify(
              {
                ok: false,
                error:
                  "缺少必填参数: --school, --college, --stage, --semester, --course",
              },
              null,
              2,
            ),
          );
          process.exit(1);
        }

        const validStages = ["大一", "大二", "大三", "大四"];
        const validSemesters = ["上学期", "下学期"];
        if (!validStages.includes(stage)) {
          console.error(
            JSON.stringify(
              {
                ok: false,
                error: `--stage 值不合法: "${stage}"，应为 ${validStages.join("/")}`,
              },
              null,
              2,
            ),
          );
          process.exit(1);
        }
        if (!validSemesters.includes(semester)) {
          console.error(
            JSON.stringify(
              {
                ok: false,
                error: `--semester 值不合法: "${semester}"，应为 ${validSemesters.join("/")}`,
              },
              null,
              2,
            ),
          );
          process.exit(1);
        }

        const opts = { school, college, stage, semester, course, courseIndex };
        const result = createCourseStructure(root, opts);

        // Write course.yaml
        const yamlContent = generateDefaultCourseYaml({
          school,
          college,
          stage,
          semester,
          courseName: course,
        });
        const courseRoot = resolve(root, result.courseRelPath);
        writeCourseYaml(courseRoot, yamlContent);

        // Write empty materials.csv
        const archiveDir = resolve(courseRoot, "00_课程档案");
        const csvPath = join(archiveDir, "materials.csv");
        writeFileSync(csvPath, MATERIALS_HEADER.join(",") + "\n", "utf-8");

        console.log(
          JSON.stringify(
            {
              ok: true,
              root,
              courseRelPath: result.courseRelPath,
              created: result.created.length,
              skipped: result.skipped.length,
              filesCreated: [
                join(result.courseRelPath, "00_课程档案", "course.yaml"),
                join(result.courseRelPath, "00_课程档案", "materials.csv"),
              ],
              message: `课程 "${course}" 已初始化于 ${result.courseRelPath}`,
            },
            null,
            2,
          ),
        );
        break;
      }

      case "validate": {
        const root = resolve(options.root || ".");
        const result = validateAll(root);
        console.log(JSON.stringify(result, null, 2));
        process.exit(result.ok ? 0 : 1);
        break;
      }

      case "export-web": {
        const root = resolve(options.root || ".");
        const manifest = generateWebManifest(root);

        const outPath = options.out ? resolve(options.out) : null;

        if (outPath) {
          writeJsonAtomically(outPath, manifest);
          console.log(
            JSON.stringify(
              {
                ok: true,
                outputFile: outPath,
                courses: manifest.courses.length,
                totalMaterials: manifest.courses.reduce(
                  (sum, c) => sum + c.materials.length,
                  0,
                ),
              },
              null,
              2,
            ),
          );
        } else {
          console.log(JSON.stringify(manifest, null, 2));
        }
        break;
      }

      case "hash": {
        const root = resolve(options.root || ".");
        const apply = flags.includes("apply");
        const dryRun = flags.includes("dry-run");

        if (apply && dryRun) {
          console.error(
            JSON.stringify(
              { ok: false, error: "--apply 与 --dry-run 不能同时使用" },
              null,
              2,
            ),
          );
          process.exit(1);
        }

        const result = hashAll(root, {
          apply,
          course: options.course ?? null,
        });
        console.log(JSON.stringify(result, null, 2));
        // exitCode rather than exit(): the duplicate list can outgrow the pipe
        // buffer, and exiting mid-write would truncate it.
        process.exitCode = result.ok ? 0 : 1;
        break;
      }

      // V2 stubs
      case "scan":
      case "normalize":
      case "dedupe": {
        console.error(
          JSON.stringify(
            {
              ok: false,
              error: `[libraryctl] "${command}" 命令尚未实现（V2 规划中）`,
            },
            null,
            2,
          ),
        );
        process.exit(1);
        break;
      }

      default: {
        console.error(
          JSON.stringify(
            {
              ok: false,
              error: `未知命令: "${command}"。使用 --help 查看可用命令。`,
            },
            null,
            2,
          ),
        );
        process.exit(1);
      }
    }
  } catch (err) {
    const isSafePathError =
      err instanceof SafePathError || err instanceof ExportPathError;
    console.error(
      JSON.stringify(
        {
          ok: false,
          error: err.message,
          ...(err.code ? { code: err.code } : {}),
          ...(err.course ? { course: err.course } : {}),
          ...(err.localId !== undefined ? { localId: err.localId } : {}),
          ...(err.path !== undefined ? { path: err.path } : {}),
          ...(!isSafePathError && err.stack ? { stack: err.stack } : {}),
        },
        null,
        2,
      ),
    );
    process.exit(1);
  }
}

main();
