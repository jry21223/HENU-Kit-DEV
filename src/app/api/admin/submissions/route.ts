import { NextResponse } from "next/server";
import { requireReviewerResponse } from "@/lib/admin";
import { parseSubmissionStatus } from "@/lib/submissions";
import { listReviewSubmissions } from "@/services/submission-service";

export async function GET(request: Request) {
  const forbidden = await requireReviewerResponse();
  if (forbidden) return forbidden;

  const { searchParams } = new URL(request.url);
  const status = parseSubmissionStatus(searchParams.get("status"));
  if (searchParams.get("status") && !status) {
    return NextResponse.json({ error: "投稿状态无效。" }, { status: 400 });
  }

  const submissions = await listReviewSubmissions(status);
  return NextResponse.json({ submissions });
}
