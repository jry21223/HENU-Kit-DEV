import { NextResponse } from "next/server";
import { getCourseById } from "@/services/course-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  const course = await getCourseById(id);

  if (!course) {
    return NextResponse.json({ error: "课程不存在或未发布。" }, { status: 404 });
  }

  return NextResponse.json({
    course: {
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
    },
  });
}

