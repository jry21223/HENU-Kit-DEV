import { SubmissionStatus } from "@prisma/client";

export type ReviewAction = "approve" | "reject";

export function canReviewSubmissions(role?: string | null) {
  return role === "ADMIN" || role === "REVIEWER";
}

export function mapSubmissionStatus(status: SubmissionStatus) {
  const statusMap: Record<SubmissionStatus, "pending" | "approved" | "rejected" | "archived"> = {
    PENDING: "pending",
    APPROVED: "approved",
    REJECTED: "rejected",
    ARCHIVED: "archived",
  };
  return statusMap[status];
}

export function parseSubmissionStatus(status?: string | null): SubmissionStatus | undefined {
  const statusMap: Record<string, SubmissionStatus> = {
    pending: SubmissionStatus.PENDING,
    approved: SubmissionStatus.APPROVED,
    rejected: SubmissionStatus.REJECTED,
    archived: SubmissionStatus.ARCHIVED,
  };
  return status ? statusMap[status] : undefined;
}

export function parseReviewAction(action?: string | null): ReviewAction | null {
  if (action === "approve" || action === "reject") {
    return action;
  }
  return null;
}

export function canTransitionSubmission(
  currentStatus: SubmissionStatus,
  action: ReviewAction,
): boolean {
  return currentStatus === SubmissionStatus.PENDING && (action === "approve" || action === "reject");
}

export function validateReviewComment(action: ReviewAction, reviewComment?: string | null) {
  if (action === "reject" && !reviewComment?.trim()) {
    return { ok: false as const, message: "驳回投稿时必须填写原因。" };
  }
  if (reviewComment && reviewComment.length > 500) {
    return { ok: false as const, message: "审核说明不能超过 500 字。" };
  }
  return { ok: true as const };
}
