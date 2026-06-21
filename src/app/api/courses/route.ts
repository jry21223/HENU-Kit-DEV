import { NextResponse } from "next/server";
import { listCourses } from "@/services/course-service";

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url);
  const courses = await listCourses({
    schoolId: searchParams.get("schoolId") ?? undefined,
    majorId: searchParams.get("majorId") ?? undefined,
    grade: searchParams.get("grade") ?? undefined,
  });

  return NextResponse.json({
    courses: courses.map((course) => ({
      id: course.id,
      school_id: course.schoolId,
      college_id: course.collegeId,
      major_ids: course.majorIds,
      grades: course.grades,
      name: course.name,
      slug: course.slug,
      description: course.description,
      exam_scope: course.examScope,
      teacher: course.teacher ?? null,
      material_count: course.materialCount ?? 0,
      has_mock_paper: Boolean(course.hasMockPaper),
      has_answer: Boolean(course.hasAnswer),
      supports_practice: course.supportsPractice,
    })),
  });
}

