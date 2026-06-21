import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { listWeakPointsForUser } from "@/services/question-service";

export async function GET() {
  const user = await getCurrentUser();

  if (!user) {
    return NextResponse.json({ error: "请先登录。" }, { status: 401 });
  }

  const weakPoints = await listWeakPointsForUser(user.id);

  return NextResponse.json({
    weak_points: weakPoints.map((item) => ({
      course: item.course,
      knowledge_point_id: item.knowledgePointId ?? null,
      knowledge_point_title: item.knowledgePointTitle,
      wrong_count: item.wrongCount,
    })),
  });
}
