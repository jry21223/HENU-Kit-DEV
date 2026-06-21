export type CourseDownloadInput = {
  courseId: string;
  courseName: string;
};

export type MaterialDownloadInput = {
  materialId: string;
  materialTitle: string;
  courseName: string;
};

export type WeakPointInput = {
  knowledgePointId: string | null;
  knowledgePointTitle: string | null;
  courseId: string;
  courseName: string;
};

export function topCourseDownloads(downloads: CourseDownloadInput[], limit = 5) {
  const counts = new Map<string, { courseId: string; courseName: string; count: number }>();

  for (const download of downloads) {
    const current = counts.get(download.courseId) ?? {
      courseId: download.courseId,
      courseName: download.courseName,
      count: 0,
    };
    current.count += 1;
    counts.set(download.courseId, current);
  }

  return [...counts.values()].sort((left, right) => right.count - left.count).slice(0, limit);
}

export function topMaterialDownloads(downloads: MaterialDownloadInput[], limit = 5) {
  const counts = new Map<
    string,
    { materialId: string; materialTitle: string; courseName: string; count: number }
  >();

  for (const download of downloads) {
    const current = counts.get(download.materialId) ?? {
      materialId: download.materialId,
      materialTitle: download.materialTitle,
      courseName: download.courseName,
      count: 0,
    };
    current.count += 1;
    counts.set(download.materialId, current);
  }

  return [...counts.values()].sort((left, right) => right.count - left.count).slice(0, limit);
}

export function topWeakPoints(wrongQuestions: WeakPointInput[], limit = 5) {
  const counts = new Map<
    string,
    {
      knowledgePointId: string;
      knowledgePointTitle: string;
      courseId: string;
      courseName: string;
      count: number;
    }
  >();

  for (const wrongQuestion of wrongQuestions) {
    const knowledgePointId = wrongQuestion.knowledgePointId ?? `course:${wrongQuestion.courseId}`;
    const knowledgePointTitle = wrongQuestion.knowledgePointTitle ?? "未关联知识点";
    const current = counts.get(knowledgePointId) ?? {
      knowledgePointId,
      knowledgePointTitle,
      courseId: wrongQuestion.courseId,
      courseName: wrongQuestion.courseName,
      count: 0,
    };
    current.count += 1;
    counts.set(knowledgePointId, current);
  }

  return [...counts.values()].sort((left, right) => right.count - left.count).slice(0, limit);
}
