import { NextResponse } from "next/server";
import { getQuestionById } from "@/services/question-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  const question = await getQuestionById(id);

  if (!question) {
    return NextResponse.json({ error: "题目不存在或未发布。" }, { status: 404 });
  }

  return NextResponse.json({
    question: {
      id: question.id,
      course_id: question.courseId,
      knowledge_point_id: question.knowledgePointId ?? null,
      knowledge_point_title: question.knowledgePointTitle ?? null,
      type: question.type,
      stem: question.stem,
      options: question.options,
      difficulty: question.difficulty,
    },
  });
}
