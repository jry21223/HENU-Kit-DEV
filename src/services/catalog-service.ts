import { RecordStatus } from "@prisma/client";
import {
  colleges as mockColleges,
  grades as mockGrades,
  majors as mockMajors,
  schools as mockSchools,
} from "@/constants/mock-data";
import { isDatabaseConfigured, prisma } from "@/lib/db";
import type { College, Major, School } from "@/types";

export async function listSchools(): Promise<School[]> {
  if (!isDatabaseConfigured()) {
    return mockSchools;
  }

  const schools = await prisma.school.findMany({
    where: { status: RecordStatus.PUBLISHED },
    orderBy: { name: "asc" },
  });

  return schools.map((school) => ({
    id: school.id,
    name: school.name,
    slug: school.slug,
    emailDomains: school.emailDomains,
    status: school.status === RecordStatus.PUBLISHED ? "published" : "archived",
  }));
}

export async function listMajors(): Promise<Major[]> {
  if (!isDatabaseConfigured()) {
    return mockMajors;
  }

  const majors = await prisma.major.findMany({
    orderBy: { name: "asc" },
  });

  return majors.map((major) => ({
    id: major.id,
    schoolId: major.schoolId,
    collegeId: major.collegeId,
    name: major.name,
    slug: major.slug,
  }));
}

export async function listColleges(schoolId?: string): Promise<College[]> {
  if (!isDatabaseConfigured()) {
    return mockColleges.filter((college) => !schoolId || college.schoolId === schoolId);
  }

  const colleges = await prisma.college.findMany({
    where: { schoolId },
    orderBy: { name: "asc" },
  });

  return colleges.map((college) => ({
    id: college.id,
    schoolId: college.schoolId,
    name: college.name,
  }));
}

export async function listGrades(): Promise<string[]> {
  if (!isDatabaseConfigured()) {
    return mockGrades;
  }

  const courses = await prisma.course.findMany({
    where: { status: RecordStatus.PUBLISHED },
    select: { grades: true },
  });

  return Array.from(new Set(courses.flatMap((course) => course.grades))).sort();
}
