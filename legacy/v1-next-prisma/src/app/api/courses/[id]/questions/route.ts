import { NextResponse } from "next/server";
import { getCourseById } from "@/services/course-service";
import { listQuestionsByCourse } from "@/services/question-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  const course = await getCourseById(id);

  if (!course) {
    return NextResponse.json({ error: "课程不存在或未发布。" }, { status: 404 });
  }

  const questions = await listQuestionsByCourse(course.id);

  return NextResponse.json({
    questions: questions.map((question) => ({
      id: question.id,
      course_id: question.courseId,
      knowledge_point_id: question.knowledgePointId ?? null,
      knowledge_point_title: question.knowledgePointTitle ?? null,
      type: question.type,
      stem: question.stem,
      options: question.options,
      difficulty: question.difficulty,
    })),
  });
}
