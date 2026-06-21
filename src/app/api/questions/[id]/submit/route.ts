import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { isSupportedAnswerInput } from "@/lib/questions";
import { submitQuestionAnswer } from "@/services/question-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

type SubmitBody = {
  answer?: unknown;
};

export async function POST(request: Request, context: RouteContext) {
  const { id } = await context.params;
  const body = (await request.json().catch(() => null)) as SubmitBody | null;

  if (!body || !isSupportedAnswerInput(body.answer)) {
    return NextResponse.json({ error: "请选择或填写答案。" }, { status: 400 });
  }

  const user = await getCurrentUser();
  const result = await submitQuestionAnswer(id, body.answer, user);

  if (!result) {
    return NextResponse.json({ error: "题目不存在或未发布。" }, { status: 404 });
  }

  return NextResponse.json({
    result: {
      question_id: result.questionId,
      is_correct: result.isCorrect,
      submitted_answer: result.submittedAnswer,
      correct_answer: result.correctAnswer,
      correct_answer_label: result.correctAnswerLabel,
      explanation: result.explanation,
      wrong_question_saved: result.wrongQuestionSaved,
    },
  });
}
