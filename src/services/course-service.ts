import { MaterialStatus, MaterialType, RecordStatus } from "@prisma/client";
import {
  findCourse as findMockCourse,
  getPublishedCourses as getMockCourses,
} from "@/constants/mock-data";
import { isDatabaseConfigured, prisma } from "@/lib/db";
import { mapRecordStatus } from "@/services/mappers";
import type { Course } from "@/types";

type CourseFilters = {
  schoolId?: string;
  majorId?: string;
  grade?: string;
};

function filterMockCourses(filters: CourseFilters) {
  return getMockCourses().filter((course) => {
    if (filters.schoolId && course.schoolId !== filters.schoolId) {
      return false;
    }
    if (filters.majorId && !course.majorIds.includes(filters.majorId)) {
      return false;
    }
    if (filters.grade && !course.grades.includes(filters.grade)) {
      return false;
    }
    return true;
  });
}

type DbCourse = Awaited<ReturnType<typeof prisma.course.findMany>>[number] & {
  majorLinks?: Array<{ majorId: string }>;
  materials?: Array<{ type: MaterialType }>;
  questions?: Array<{ id: string }>;
};

function mapCourse(course: DbCourse): Course {
  const materialTypes = course.materials ?? [];

  return {
    id: course.id,
    schoolId: course.schoolId,
    collegeId: course.collegeId,
    majorIds: course.majorLinks?.map((link) => link.majorId) ?? [],
    grades: course.grades,
    name: course.name,
    slug: course.slug,
    description: course.description,
    examScope: course.examScope,
    status: mapRecordStatus(course.status),
    teacher: course.teacher ?? undefined,
    supportsPractice: Boolean(course.questions?.length),
    materialCount: materialTypes.length,
    hasMockPaper: materialTypes.some((material) => material.type === MaterialType.MOCK_PAPER),
    hasAnswer: materialTypes.some((material) => material.type === MaterialType.ANSWER),
  };
}

export async function listCourses(filters: CourseFilters = {}): Promise<Course[]> {
  if (!isDatabaseConfigured()) {
    return filterMockCourses(filters);
  }

  const courses = await prisma.course.findMany({
    where: {
      status: RecordStatus.PUBLISHED,
      schoolId: filters.schoolId,
      grades: filters.grade ? { has: filters.grade } : undefined,
      majorLinks: filters.majorId ? { some: { majorId: filters.majorId } } : undefined,
    },
    include: {
      majorLinks: { select: { majorId: true } },
      materials: {
        where: { status: MaterialStatus.PUBLISHED },
        select: { type: true },
      },
      questions: {
        where: { status: RecordStatus.PUBLISHED },
        select: { id: true },
        take: 1,
      },
    },
    orderBy: { name: "asc" },
  });

  return courses.map(mapCourse);
}

export async function getCourseById(courseId: string): Promise<Course | undefined> {
  if (!isDatabaseConfigured()) {
    return findMockCourse(courseId);
  }

  const course = await prisma.course.findFirst({
    where: {
      id: courseId,
      status: RecordStatus.PUBLISHED,
    },
    include: {
      majorLinks: { select: { majorId: true } },
      materials: {
        where: { status: MaterialStatus.PUBLISHED },
        select: { type: true },
      },
      questions: {
        where: { status: RecordStatus.PUBLISHED },
        select: { id: true },
        take: 1,
      },
    },
  });

  return course ? mapCourse(course) : undefined;
}
