// libraryctl — structure and data validator

import { existsSync } from 'node:fs';
import { resolve, join, basename, posix } from 'node:path';
import { ROOT_DIRS, INBOX_DIRS, COURSE_INTERNAL_DIRS, discoverCourses } from './paths.mjs';
import { readCourseYaml } from './course.mjs';
import { readMaterialsCsv, VALID_TYPES, VALID_STATUSES } from './materials.mjs';
import { checkFilename } from './normalizer.mjs';
import { SafePathError, resolveWithinRoot } from './safe-path.mjs';

/**
 * Validate the root library structure.
 * @param {string} root - absolute root path
 * @returns {Array<{level: string, path: string, message: string}>}
 */
export function validateRoot(root) {
  const issues = [];

  for (const dir of ROOT_DIRS) {
    const full = resolve(root, dir);
    if (!existsSync(full)) {
      issues.push({
        level: 'error',
        path: dir,
        message: `缺少根目录: ${dir}`,
      });
    }
  }

  for (const sub of INBOX_DIRS) {
    const rel = join('01_收件箱', sub);
    const full = resolve(root, rel);
    if (!existsSync(full)) {
      issues.push({
        level: 'warning',
        path: rel,
        message: `缺少收件箱子目录: ${rel}`,
      });
    }
  }

  return issues;
}

/**
 * Flatten the COURSE_INTERNAL_DIRS into a list of relative paths.
 */
function flattenCourseDirs() {
  const dirs = [];
  for (const [main, subs] of Object.entries(COURSE_INTERNAL_DIRS)) {
    dirs.push(main);
    for (const sub of subs) {
      dirs.push(join(main, sub));
    }
  }
  return dirs;
}

/**
 * Validate a single course directory.
 * @param {string} coursePath - absolute path to course directory
 * @param {string} courseRelPath - relative path (for messages)
 * @returns {Array<{level: string, course?: string, file?: string, message: string}>}
 */
export function validateCourse(coursePath, courseRelPath) {
  const issues = [];
  const courseName = basename(courseRelPath).replace(/^\d+_/, '');

  // 1. Check internal directory structure
  const courseDirs = flattenCourseDirs();
  for (const dir of courseDirs) {
    const full = resolve(coursePath, dir);
    if (!existsSync(full)) {
      issues.push({
        level: 'warning',
        course: courseName,
        message: `缺少课程子目录: ${dir}`,
      });
    }
  }

  // 2. Check course.yaml
  const yamlPath = resolve(coursePath, '00_课程档案', 'course.yaml');
  if (!existsSync(yamlPath)) {
    issues.push({
      level: 'error',
      course: courseName,
      file: '00_课程档案/course.yaml',
      message: 'course.yaml 不存在',
    });
  } else {
    const yamlData = readCourseYaml(coursePath);
    if (!yamlData) {
      issues.push({
        level: 'error',
        course: courseName,
        file: '00_课程档案/course.yaml',
        message: 'course.yaml 解析失败',
      });
    } else {
      // Check required fields
      const requiredFields = ['school', 'college', 'stage', 'semester', 'course_name'];
      for (const field of requiredFields) {
        if (!yamlData[field]) {
          issues.push({
            level: 'error',
            course: courseName,
            file: '00_课程档案/course.yaml',
            message: `缺少必填字段: ${field}`,
          });
        }
      }
      // Check stage
      const validStages = ['大一', '大二', '大三', '大四'];
      if (yamlData.stage && !validStages.includes(yamlData.stage)) {
        issues.push({
          level: 'error',
          course: courseName,
          file: '00_课程档案/course.yaml',
          message: `stage 值不合法: "${yamlData.stage}"，应为 ${validStages.join('/')}`,
        });
      }
      // Check semester
      const validSemesters = ['上学期', '下学期'];
      if (yamlData.semester && !validSemesters.includes(yamlData.semester)) {
        issues.push({
          level: 'error',
          course: courseName,
          file: '00_课程档案/course.yaml',
          message: `semester 值不合法: "${yamlData.semester}"，应为 ${validSemesters.join('/')}`,
        });
      }
    }
  }

  // 3. Check materials.csv
  const materials = readMaterialsCsv(coursePath);
  if (!materials) {
    issues.push({
      level: 'warning',
      course: courseName,
      file: '00_课程档案/materials.csv',
      message: 'materials.csv 不存在（课程无已登记资料）',
    });
  } else {
    // Validate each row
    for (let i = 0; i < materials.rows.length; i++) {
      const row = materials.rows[i];
      const rowLabel = `00_课程档案/materials.csv 第 ${i + 2} 行`; // +2 for header and 0-index

      // Check required fields
      if (!row.title) {
        issues.push({
          level: 'warning',
          course: courseName,
          file: rowLabel,
          message: 'title 为空',
        });
      }

      // Check type
      if (row.type && !VALID_TYPES.has(row.type)) {
        issues.push({
          level: 'error',
          course: courseName,
          file: rowLabel,
          message: `type "${row.type}" 不合法，合法值: ${[...VALID_TYPES].join(', ')}`,
        });
      }

      // Check status
      if (row.status && !VALID_STATUSES.has(row.status)) {
        issues.push({
          level: 'error',
          course: courseName,
          file: rowLabel,
          message: `status "${row.status}" 不合法，合法值: ${[...VALID_STATUSES].join(', ')}`,
        });
      }

      // Check path boundary and existence
      try {
        const safePath = resolveWithinRoot(coursePath, row.path);

        // Check filename for illegal characters
        const fileName = posix.basename(safePath.relativePath);
        const nameCheck = checkFilename(fileName);
        if (!nameCheck.ok) {
          issues.push({
            level: 'warning',
            course: courseName,
            file: rowLabel,
            message: `文件名包含非法字符: ${nameCheck.illegalChars.join(' ')} — ${fileName}`,
          });
        }
      } catch (error) {
        if (!(error instanceof SafePathError)) throw error;
        issues.push({
          level: 'error',
          course: courseName,
          file: rowLabel,
          code: error.code,
          path: row.path ?? '',
          message: `资料路径校验失败 [${error.code}]: ${error.message}`,
        });
      }
    }
  }

  return issues;
}

/**
 * Validate the entire library: root + all courses.
 * @param {string} root - absolute root path
 * @returns {{ok: boolean, errors: Array, warnings: Array, summary: {courses: number, errors: number, warnings: number}}}
 */
export function validateAll(root) {
  const allIssues = [];

  // Validate root
  allIssues.push(...validateRoot(root));

  // Discover and validate courses
  const courses = discoverCourses(root);
  for (const c of courses) {
    allIssues.push(...validateCourse(c.absPath, c.relPath));
  }

  // If no courses found, add info
  if (courses.length === 0) {
    allIssues.push({
      level: 'info',
      message: '未发现任何课程目录（02_学校库 下无 course.yaml）',
    });
  }

  const errors = allIssues.filter(i => i.level === 'error');
  const warnings = allIssues.filter(i => i.level === 'warning');

  return {
    ok: errors.length === 0,
    errors: allIssues.filter(i => i.level === 'error'),
    warnings: allIssues.filter(i => i.level === 'warning'),
    summary: {
      courses: courses.length,
      errors: errors.length,
      warnings: warnings.length,
    },
  };
}
