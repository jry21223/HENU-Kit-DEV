// libraryctl — course.yaml read/write

import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { resolve, join } from "node:path";

/**
 * Generate a default course.yaml string.
 * @param {{school: string, college: string, stage: string, semester: string, courseName: string, applicableMajors?: string[]}} opts
 * @returns {string}
 */
export function generateDefaultCourseYaml(opts) {
  const majors = (opts.applicableMajors ?? ["软件工程", "网络工程"])
    .map((m) => `  - ${m}`)
    .join("\n");

  return `# ${opts.courseName} 课程档案
# 由 libraryctl init-course 自动生成
#
# 字段说明：
#   stage:     培养方案阶段（大一/大二/大三/大四），非入学级
#   semester:  学期（上学期/下学期）
#   status:    课程资料整理状态 collecting | active | complete | archived
#   exam_type: 常见考试题型

school: ${opts.school}
college: ${opts.college}
stage: ${opts.stage}
semester: ${opts.semester}
course_name: ${opts.courseName}
course_aliases: []
applicable_majors:
${majors}
teacher: ""
exam_type: []
status: collecting
maintainer: ""
notes: ""
`;
}

/**
 * Parse a course.yaml string into an object.
 * Handles simple flat YAML (key: value, arrays with - items).
 *
 * @param {string} content
 * @returns {object}
 */
export function parseCourseYaml(content) {
  const result = {};
  let currentKey = null;

  for (const rawLine of content.split("\n")) {
    const line = rawLine.trim();

    // Skip comments and empty lines
    if (!line || line.startsWith("#")) continue;

    // Array item
    if (line.startsWith("- ")) {
      const value = line.slice(2).trim();
      // Remove quotes if present
      const clean = value.replace(/^['"]|['"]$/g, "");
      if (currentKey && Array.isArray(result[currentKey])) {
        result[currentKey].push(clean);
      } else if (currentKey) {
        result[currentKey] = [clean];
      }
      continue;
    }

    // Key: value
    const colonIdx = line.indexOf(":");
    if (colonIdx === -1) {
      // Might be a continuation of previous scalar
      if (currentKey) {
        result[currentKey] += " " + line;
      }
      continue;
    }

    const key = line.slice(0, colonIdx).trim();
    let value = line.slice(colonIdx + 1).trim();

    currentKey = key;

    if (value === "" || value === "[]") {
      result[key] = value === "[]" ? [] : "";
    } else {
      // Remove surrounding quotes
      result[key] = value.replace(/^['"]|['"]$/g, "");
    }
  }

  return result;
}

/**
 * Read and parse course.yaml from a course directory.
 * @param {string} courseDir - absolute path to the course directory
 * @returns {object|null} parsed YAML object, or null if not found
 */
export function readCourseYaml(courseDir) {
  const yamlPath = resolve(courseDir, "00_课程档案", "course.yaml");
  if (!existsSync(yamlPath)) return null;
  const content = readFileSync(yamlPath, "utf-8");
  return parseCourseYaml(content);
}

/**
 * Write course.yaml to a course directory.
 * @param {string} courseDir
 * @param {string} content
 */
export function writeCourseYaml(courseDir, content) {
  const archiveDir = resolve(courseDir, "00_课程档案");
  const yamlPath = join(archiveDir, "course.yaml");
  writeFileSync(yamlPath, content, "utf-8");
}
