import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { listWrongQuestionsForUser } from "@/services/question-service";

export async function GET(request: Request) {
  const user = await getCurrentUser();

  if (!user) {
    return NextResponse.json({ error: "请先登录。" }, { status: 401 });
  }

  const { searchParams } = new URL(request.url);
  const wrongQuestions = await listWrongQuestionsForUser(user.id, {
    courseId: searchParams.get("courseId") ?? undefined,
    knowledgePointId: searchParams.get("knowledgePointId") ?? undefined,
  });

  return NextResponse.json({
    wrong_questions: wrongQuestions.map((item) => ({
      id: item.id,
      created_at: item.createdAt,
      course: item.course,
      knowledge_point_title: item.knowledgePointTitle ?? null,
      question: {
        id: item.question.id,
        course_id: item.question.courseId,
        knowledge_point_id: item.question.knowledgePointId ?? null,
        knowledge_point_title: item.question.knowledgePointTitle ?? null,
        type: item.question.type,
        stem: item.question.stem,
        options: item.question.options,
        difficulty: item.question.difficulty,
      },
    })),
  });
}
