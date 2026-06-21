import { NextResponse } from "next/server";
import { requireAdminResponse } from "@/lib/admin";
import { getAiJob } from "@/services/ai-job-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(_request: Request, context: RouteContext) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const { id } = await context.params;
  const job = await getAiJob(id);
  if (!job) {
    return NextResponse.json({ error: "AI 任务不存在。" }, { status: 404 });
  }

  return NextResponse.json({ job });
}
