// libraryctl — export web import manifest

import { discoverCourses } from './paths.mjs';
import { readCourseYaml } from './course.mjs';
import { readMaterialsCsv } from './materials.mjs';

/**
 * Generate the web import manifest JSON.
 *
 * @param {string} root - absolute root path
 * @returns {object} manifest object
 */
export function generateWebManifest(root) {
  const courses = discoverCourses(root);

  const manifest = {
    version: 1,
    generatedAt: new Date().toISOString(),
    courses: [],
  };

  for (const c of courses) {
    const yaml = readCourseYaml(c.absPath);

    const courseEntry = {
      localCourseId: [c.school, c.college, c.stage, c.semester, c.course].join('/'),
      school: yaml?.school ?? c.school,
      college: yaml?.college ?? c.college,
      stage: yaml?.stage ?? c.stage,
      semester: yaml?.semester ?? c.semester,
      courseName: yaml?.course_name ?? c.course,
      courseAliases: yaml?.course_aliases ?? [],
      applicableMajors: yaml?.applicable_majors ?? [],
      teacher: yaml?.teacher ?? '',
      examType: yaml?.exam_type ?? [],
      status: yaml?.status ?? 'collecting',
      maintainer: yaml?.maintainer ?? '',
      materials: [],
    };

    // Read materials
    const materials = readMaterialsCsv(c.absPath);
    if (materials && materials.rows.length > 0) {
      for (const row of materials.rows) {
        courseEntry.materials.push({
          localId: row.local_id ?? '',
          title: row.title ?? '',
          type: row.type ?? '',
          status: row.status ?? 'raw',
          year: row.year ?? '',
          path: row.path ?? '',
          sourceNote: row.source_note ?? '',
          sha256: row.sha256 ?? '',
          webId: row.web_id ?? '',
          notes: row.notes ?? '',
        });
      }
    }

    manifest.courses.push(courseEntry);
  }

  return manifest;
}
