// libraryctl — path utilities
// Generates canonical directory paths for the material library structure.

import { mkdirSync, existsSync, readdirSync, statSync } from "node:fs";
import { resolve, join } from "node:path";

/** Top-level root directories */
export const ROOT_DIRS = [
  "00_模板",
  "01_收件箱",
  "02_学校库",
  "90_公共资料",
  "99_全局归档",
];

/** Inbox subdirectories under 01_收件箱 */
export const INBOX_DIRS = ["00_待分类", "01_待去重", "02_待确认课程"];

/** Stage order for sorting */
export const STAGE_ORDER = ["大一", "大二", "大三", "大四"];

/** Semester order for sorting */
export const SEMESTER_ORDER = ["上学期", "下学期"];

/** Internal course directory structure */
export const COURSE_INTERNAL_DIRS = {
  "00_课程档案": [],
  "01_发布区": ["01_成品复习包", "02_单项可下载资料", "90_生成源文件"],
  "02_知识点库": [],
  "03_题库": [],
  "04_原始资料": [
    "01_真题样卷",
    "02_课件讲义",
    "03_题库练习",
    "04_教材题解",
    "05_笔记总结",
    "06_实验代码",
    "90_其他资料",
  ],
  "05_处理中": ["01_待OCR", "02_待清洗", "03_待补全信息", "04_待审核"],
  "99_课程归档": ["01_重复文件"],
};

/**
 * Build the numbered school directory name.
 * @param {number} schoolIndex - 1-based index
 * @param {string} schoolName
 * @returns {string} e.g. "01_河南大学"
 */
export function schoolDir(schoolIndex, schoolName) {
  return `${String(schoolIndex).padStart(2, "0")}_${schoolName}`;
}

/**
 * Build the numbered college directory name.
 * @param {number} collegeIndex - 1-based index
 * @param {string} collegeName
 * @returns {string} e.g. "01_软件学院"
 */
export function collegeDir(collegeIndex, collegeName) {
  return `${String(collegeIndex).padStart(2, "0")}_${collegeName}`;
}

/**
 * Build the numbered stage directory name.
 * @param {string} stage - 大一/大二/大三/大四
 * @returns {string} e.g. "01_大一"
 */
export function stageDir(stage) {
  const idx = STAGE_ORDER.indexOf(stage);
  const n = idx >= 0 ? idx + 1 : 0;
  return `${String(n).padStart(2, "0")}_${stage}`;
}

/**
 * Build the numbered semester directory name.
 * @param {string} semester - 上学期/下学期
 * @returns {string} e.g. "01_上学期"
 */
export function semesterDir(semester) {
  const idx = SEMESTER_ORDER.indexOf(semester);
  const n = idx >= 0 ? idx + 1 : 0;
  return `${String(n).padStart(2, "0")}_${semester}`;
}

/**
 * Build the numbered course directory name.
 * @param {number} courseIndex - 1-based index within the semester
 * @param {string} courseName
 * @returns {string} e.g. "02_离散数学"
 */
export function courseDir(courseIndex, courseName) {
  return `${String(courseIndex).padStart(2, "0")}_${courseName}`;
}

/**
 * Build the relative course path from the root.
 * School and college indices default to 1 for the first of each.
 *
 * @param {{school: string, college: string, stage: string, semester: string, course: string, schoolIndex?: number, collegeIndex?: number, courseIndex?: number}} opts
 * @returns {string} relative path like "02_学校库/01_河南大学/01_软件学院/01_大一/02_下学期/02_离散数学"
 */
export function buildCourseRelPath(opts) {
  const { school, college, stage, semester, course } = opts;
  const schoolI = opts.schoolIndex ?? 1;
  const collegeI = opts.collegeIndex ?? 1;
  const courseI = opts.courseIndex ?? 1;

  return join(
    "02_学校库",
    schoolDir(schoolI, school),
    collegeDir(collegeI, college),
    stageDir(stage),
    semesterDir(semester),
    courseDir(courseI, course),
  );
}

/**
 * Resolve the absolute course path.
 */
export function resolveCoursePath(root, opts) {
  return resolve(root, buildCourseRelPath(opts));
}

/**
 * Create the root library skeleton directories.
 * @param {string} root - absolute root path
 * @returns {{ created: string[], skipped: string[] }}
 */
export function createRootStructure(root) {
  const created = [];
  const skipped = [];

  const allDirs = [
    ...ROOT_DIRS,
    ...INBOX_DIRS.map((d) => join("01_收件箱", d)),
  ];

  for (const d of allDirs) {
    const full = resolve(root, d);
    if (!existsSync(full)) {
      mkdirSync(full, { recursive: true });
      created.push(d);
    } else {
      skipped.push(d);
    }
  }

  return { created, skipped };
}

/**
 * Create the course directory with all internal subdirectories.
 * @param {string} root - absolute root path
 * @param {{school: string, college: string, stage: string, semester: string, course: string}} opts
 * @returns {{ created: string[], skipped: string[], courseRelPath: string }}
 */
export function createCourseStructure(root, opts) {
  const relPath = buildCourseRelPath(opts);
  const courseRoot = resolve(root, relPath);
  const created = [];
  const skipped = [];

  for (const [main, subs] of Object.entries(COURSE_INTERNAL_DIRS)) {
    const mainFull = resolve(courseRoot, main);
    if (!existsSync(mainFull)) {
      mkdirSync(mainFull, { recursive: true });
      created.push(join(relPath, main));
    } else {
      skipped.push(join(relPath, main));
    }
    for (const sub of subs) {
      const subFull = resolve(mainFull, sub);
      if (!existsSync(subFull)) {
        mkdirSync(subFull, { recursive: true });
        created.push(join(relPath, main, sub));
      } else {
        skipped.push(join(relPath, main, sub));
      }
    }
  }

  return { created, skipped, courseRelPath: relPath };
}

/**
 * Walk the library and discover all courses.
 * A directory is considered a course if it contains `00_课程档案/` or `course.yaml`.
 *
 * @param {string} root
 * @returns {Array<{relPath: string, absPath: string, school: string, college: string, stage: string, semester: string, course: string}>}
 */
export function discoverCourses(root) {
  const schoolLib = resolve(root, "02_学校库");
  if (!existsSync(schoolLib)) return [];

  const courses = [];

  const schools = readdirSync(schoolLib);
  for (const schoolDirName of schools) {
    const schoolPath = join(schoolLib, schoolDirName);
    if (!statSync(schoolPath).isDirectory()) continue;
    const schoolName = schoolDirName.replace(/^\d+_/, "");

    const colleges = readdirSync(schoolPath);
    for (const collegeDirName of colleges) {
      const collegePath = join(schoolPath, collegeDirName);
      if (!statSync(collegePath).isDirectory()) continue;
      const collegeName = collegeDirName.replace(/^\d+_/, "");

      const stages = readdirSync(collegePath);
      for (const stageDirName of stages) {
        const stagePath = join(collegePath, stageDirName);
        if (!statSync(stagePath).isDirectory()) continue;
        const stage = stageDirName.replace(/^\d+_/, "");

        const semesters = readdirSync(stagePath);
        for (const semDirName of semesters) {
          const semPath = join(stagePath, semDirName);
          if (!statSync(semPath).isDirectory()) continue;
          const semester = semDirName.replace(/^\d+_/, "");

          const courseList = readdirSync(semPath);
          for (const courseDirName of courseList) {
            const coursePath = join(semPath, courseDirName);
            if (!statSync(coursePath).isDirectory()) continue;

            // Check if it looks like a course directory
            const hasArchive = existsSync(join(coursePath, "00_课程档案"));
            const hasYaml = existsSync(
              join(coursePath, "00_课程档案", "course.yaml"),
            );

            if (hasArchive || hasYaml) {
              const courseName = courseDirName.replace(/^\d+_/, "");
              courses.push({
                relPath: join(
                  "02_学校库",
                  schoolDirName,
                  collegeDirName,
                  stageDirName,
                  semDirName,
                  courseDirName,
                ),
                absPath: coursePath,
                school: schoolName,
                college: collegeName,
                stage,
                semester,
                course: courseName,
              });
            }
          }
        }
      }
    }
  }

  return courses;
}
