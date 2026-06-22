import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { deleteWrongQuestionForUser } from "@/services/question-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function DELETE(_request: Request, context: RouteContext) {
  const user = await getCurrentUser();

  if (!user) {
    return NextResponse.json({ error: "请先登录。" }, { status: 401 });
  }

  const { id } = await context.params;
  const deleted = await deleteWrongQuestionForUser(user.id, id);

  if (!deleted) {
    return NextResponse.json({ error: "错题不存在或不属于当前用户。" }, { status: 404 });
  }

  return NextResponse.json({ ok: true });
}
