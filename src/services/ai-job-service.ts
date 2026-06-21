import { randomUUID } from "node:crypto";
import {
  AiJobStatus,
  MaterialAccessLevel,
  MaterialStatus,
  RecordStatus,
  type MaterialType,
  type Prisma,
} from "@prisma/client";
import {
  buildAiDraftContent,
  mapAiJobStatus,
  mapAiOutputType,
  parseAiOutputType,
} from "@/lib/ai-jobs";
import { prisma } from "@/lib/db";

type AiJobRecord = Prisma.AiJobGetPayload<{
  include: {
    course: { select: { id: true; name: true } };
  };
}>;

export type AiJobView = {
  id: string;
  courseId: string;
  courseName: string;
  inputMaterialIds: string[];
  outputType: string;
  status: "queued" | "running" | "succeeded" | "failed" | "cancelled";
  result: Prisma.JsonValue | null;
  error: string | null;
  createdAt: string;
  updatedAt: string;
};

export type CreateAiJobResult =
  | { ok: true; job: AiJobView }
  | { ok: false; status: number; message: string };

function mapAiJob(job: AiJobRecord): AiJobView {
  return {
    id: job.id,
    courseId: job.courseId,
    courseName: job.course.name,
    inputMaterialIds: job.inputMaterialIds,
    outputType: mapAiOutputType(job.outputType),
    status: mapAiJobStatus(job.status),
    result: job.result,
    error: job.error,
    createdAt: job.createdAt.toISOString(),
    updatedAt: job.updatedAt.toISOString(),
  };
}

export async function listAiJobs(status?: AiJobStatus) {
  const jobs = await prisma.aiJob.findMany({
    where: status ? { status } : undefined,
    include: {
      course: { select: { id: true, name: true } },
    },
    orderBy: { createdAt: "desc" },
    take: 100,
  });

  return jobs.map(mapAiJob);
}

export async function getAiJob(jobId: string) {
  const job = await prisma.aiJob.findUnique({
    where: { id: jobId },
    include: {
      course: { select: { id: true, name: true } },
    },
  });

  return job ? mapAiJob(job) : null;
}

export async function createAiJob(input: {
  adminId: string;
  courseId: string;
  inputMaterialIds: string[];
  outputType: string;
  simulateFailure?: boolean;
}): Promise<CreateAiJobResult> {
  const outputType = parseAiOutputType(input.outputType);
  if (!outputType) {
    return { ok: false, status: 400, message: "AI 输出类型无效。" };
  }

  const course = await prisma.course.findFirst({
    where: {
      id: input.courseId,
      status: RecordStatus.PUBLISHED,
    },
    select: {
      id: true,
      name: true,
    },
  });

  if (!course) {
    return { ok: false, status: 404, message: "课程不存在或未发布。" };
  }

  const inputMaterialIds = Array.from(new Set(input.inputMaterialIds.filter(Boolean)));
  const sourceMaterials = inputMaterialIds.length
    ? await prisma.material.findMany({
        where: {
          id: { in: inputMaterialIds },
          courseId: course.id,
          status: MaterialStatus.PUBLISHED,
        },
        select: {
          id: true,
          title: true,
        },
      })
    : [];

  if (sourceMaterials.length !== inputMaterialIds.length) {
    return { ok: false, status: 400, message: "来源资料不存在、未发布或不属于该课程。" };
  }

  const jobId = `ai-job-${randomUUID()}`;

  if (input.simulateFailure) {
    const failedJob = await prisma.aiJob.create({
      data: {
        id: jobId,
        courseId: course.id,
        inputMaterialIds,
        outputType,
        status: AiJobStatus.FAILED,
        error: "本地模拟生成失败，未创建资料草稿。",
      },
      include: {
        course: { select: { id: true, name: true } },
      },
    });

    return { ok: true, job: mapAiJob(failedJob) };
  }

  const draft = buildAiDraftContent({
    courseName: course.name,
    outputType,
    sourceTitles: sourceMaterials.map((material) => material.title),
  });
  const materialId = `ai-material-${randomUUID()}`;
  const result = {
    materialId,
    materialStatus: "draft",
    title: draft.title,
    sourceMaterialIds: inputMaterialIds,
    notice: "AI 生成内容已进入草稿，必须人工审核后才能发布。",
  };

  const job = await prisma.$transaction(async (tx) => {
    await tx.material.create({
      data: {
        id: materialId,
        courseId: course.id,
        title: draft.title,
        type: outputType as MaterialType,
        description: draft.description,
        previewContent: draft.previewContent,
        accessLevel: MaterialAccessLevel.LOGIN_REQUIRED,
        status: MaterialStatus.DRAFT,
        createdById: input.adminId,
      },
    });

    return tx.aiJob.create({
      data: {
        id: jobId,
        courseId: course.id,
        inputMaterialIds,
        outputType,
        status: AiJobStatus.SUCCEEDED,
        result,
      },
      include: {
        course: { select: { id: true, name: true } },
      },
    });
  });

  return { ok: true, job: mapAiJob(job) };
}
