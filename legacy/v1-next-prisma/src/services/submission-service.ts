import {
  MaterialAccessLevel,
  MaterialStatus,
  MaterialType,
  RecordStatus,
  SubmissionStatus,
  type Prisma,
} from "@prisma/client";
import { prisma } from "@/lib/db";
import {
  canTransitionSubmission,
  mapSubmissionStatus,
  type ReviewAction,
} from "@/lib/submissions";

type SubmissionRecord = Prisma.SubmissionGetPayload<{
  include: {
    course: { select: { id: true; name: true } };
    user: { select: { id: true; email: true } };
  };
}>;

export type SubmissionView = {
  id: string;
  title: string;
  description: string;
  fileUrl: string;
  status: "pending" | "approved" | "rejected" | "archived";
  reviewComment: string | null;
  createdAt: string;
  reviewedAt: string | null;
  course: {
    id: string;
    name: string;
  };
  user: {
    id: string;
    email: string;
  };
};

export type ReviewSubmissionResult =
  | { ok: true; submission: SubmissionView; materialId?: string }
  | { ok: false; status: number; message: string };

function mapSubmission(submission: SubmissionRecord): SubmissionView {
  return {
    id: submission.id,
    title: submission.title,
    description: submission.description,
    fileUrl: submission.fileUrl,
    status: mapSubmissionStatus(submission.status),
    reviewComment: submission.reviewComment,
    createdAt: submission.createdAt.toISOString(),
    reviewedAt: submission.reviewedAt?.toISOString() ?? null,
    course: submission.course,
    user: submission.user,
  };
}

export async function getSubmittableCourse(courseId: string) {
  return prisma.course.findFirst({
    where: {
      id: courseId,
      status: RecordStatus.PUBLISHED,
    },
    select: {
      id: true,
      name: true,
    },
  });
}

export async function createSubmission(input: {
  userId: string;
  courseId: string;
  title: string;
  description: string;
  fileUrl: string;
}) {
  const submission = await prisma.submission.create({
    data: {
      userId: input.userId,
      courseId: input.courseId,
      title: input.title.trim(),
      description: input.description.trim(),
      fileUrl: input.fileUrl,
      status: SubmissionStatus.PENDING,
    },
    include: {
      course: { select: { id: true, name: true } },
      user: { select: { id: true, email: true } },
    },
  });

  return mapSubmission(submission);
}

export async function listUserSubmissions(userId: string) {
  const submissions = await prisma.submission.findMany({
    where: { userId },
    include: {
      course: { select: { id: true, name: true } },
      user: { select: { id: true, email: true } },
    },
    orderBy: { createdAt: "desc" },
  });

  return submissions.map(mapSubmission);
}

export async function listReviewSubmissions(status?: SubmissionStatus) {
  const submissions = await prisma.submission.findMany({
    where: status ? { status } : undefined,
    include: {
      course: { select: { id: true, name: true } },
      user: { select: { id: true, email: true } },
    },
    orderBy: { createdAt: "desc" },
    take: 100,
  });

  return submissions.map(mapSubmission);
}

export async function reviewSubmission(input: {
  submissionId: string;
  action: ReviewAction;
  reviewComment?: string;
}): Promise<ReviewSubmissionResult> {
  const existing = await prisma.submission.findUnique({
    where: { id: input.submissionId },
    include: {
      course: { select: { id: true, name: true } },
      user: { select: { id: true, email: true } },
    },
  });

  if (!existing) {
    return { ok: false, status: 404, message: "投稿不存在。" };
  }

  if (!canTransitionSubmission(existing.status, input.action)) {
    return { ok: false, status: 409, message: "当前投稿状态不允许审核操作。" };
  }

  const now = new Date();
  const reviewComment = input.reviewComment?.trim() || null;

  if (input.action === "reject") {
    const submission = await prisma.submission.update({
      where: { id: existing.id },
      data: {
        status: SubmissionStatus.REJECTED,
        reviewComment,
        reviewedAt: now,
      },
      include: {
        course: { select: { id: true, name: true } },
        user: { select: { id: true, email: true } },
      },
    });
    return { ok: true, submission: mapSubmission(submission) };
  }

  const materialId = `submission-${existing.id}`;
  const result = await prisma.$transaction(async (tx) => {
    const material = await tx.material.create({
      data: {
        id: materialId,
        courseId: existing.courseId,
        title: existing.title,
        type: MaterialType.OTHER,
        description: existing.description,
        fileUrl: existing.fileUrl,
        fileName: existing.fileUrl.split("/").pop() ?? existing.title,
        previewContent: `学生投稿资料：${existing.description}`,
        accessLevel: MaterialAccessLevel.LOGIN_REQUIRED,
        status: MaterialStatus.PUBLISHED,
        createdById: existing.userId,
      },
      select: { id: true },
    });

    const submission = await tx.submission.update({
      where: { id: existing.id },
      data: {
        status: SubmissionStatus.APPROVED,
        reviewComment: reviewComment ?? "审核通过，已发布为课程资料。",
        reviewedAt: now,
      },
      include: {
        course: { select: { id: true, name: true } },
        user: { select: { id: true, email: true } },
      },
    });

    return { submission, material };
  });

  return {
    ok: true,
    submission: mapSubmission(result.submission),
    materialId: result.material.id,
  };
}
