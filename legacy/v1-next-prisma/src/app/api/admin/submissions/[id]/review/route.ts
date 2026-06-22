import { NextResponse } from "next/server";
import { requireReviewerResponse } from "@/lib/admin";
import { parseReviewAction, validateReviewComment } from "@/lib/submissions";
import { reviewSubmission } from "@/services/submission-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

type ReviewBody = {
  action?: string;
  review_comment?: string;
};

export async function PATCH(request: Request, context: RouteContext) {
  const forbidden = await requireReviewerResponse();
  if (forbidden) return forbidden;

  const { id } = await context.params;
  const body = (await request.json().catch(() => null)) as ReviewBody | null;
  const action = parseReviewAction(body?.action);
  if (!action) {
    return NextResponse.json({ error: "审核动作无效。" }, { status: 400 });
  }

  const commentValidation = validateReviewComment(action, body?.review_comment);
  if (!commentValidation.ok) {
    return NextResponse.json({ error: commentValidation.message }, { status: 400 });
  }

  const result = await reviewSubmission({
    submissionId: id,
    action,
    reviewComment: body?.review_comment,
  });

  if (!result.ok) {
    return NextResponse.json({ error: result.message }, { status: result.status });
  }

  return NextResponse.json({
    submission: result.submission,
    material_id: result.materialId,
  });
}
