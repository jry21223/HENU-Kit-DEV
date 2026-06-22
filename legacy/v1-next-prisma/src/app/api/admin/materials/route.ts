import { NextResponse } from "next/server";
import { MaterialAccessLevel, MaterialStatus, MaterialType } from "@prisma/client";
import { requireAdminResponse } from "@/lib/admin";
import { getCurrentUser } from "@/lib/auth";
import { prisma } from "@/lib/db";

type MaterialBody = {
  id?: string;
  course_id?: string;
  title?: string;
  type?: "knowledge_note" | "mock_paper" | "answer" | "quick_review" | "past_exam" | "other";
  description?: string;
  file_url?: string;
  file_name?: string;
  file_size?: number;
  preview_content?: string;
  access_level?: "free" | "login_required" | "paid";
  status?: "draft" | "pending_review" | "published" | "archived";
};

const typeMap: Record<NonNullable<MaterialBody["type"]>, MaterialType> = {
  knowledge_note: MaterialType.KNOWLEDGE_NOTE,
  mock_paper: MaterialType.MOCK_PAPER,
  answer: MaterialType.ANSWER,
  quick_review: MaterialType.QUICK_REVIEW,
  past_exam: MaterialType.PAST_EXAM,
  other: MaterialType.OTHER,
};

const accessMap: Record<NonNullable<MaterialBody["access_level"]>, MaterialAccessLevel> = {
  free: MaterialAccessLevel.FREE,
  login_required: MaterialAccessLevel.LOGIN_REQUIRED,
  paid: MaterialAccessLevel.PAID,
};

const statusMap: Record<NonNullable<MaterialBody["status"]>, MaterialStatus> = {
  draft: MaterialStatus.DRAFT,
  pending_review: MaterialStatus.PENDING_REVIEW,
  published: MaterialStatus.PUBLISHED,
  archived: MaterialStatus.ARCHIVED,
};

function slugify(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}

export async function POST(request: Request) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;
  const admin = await getCurrentUser();

  const body = (await request.json().catch(() => null)) as MaterialBody | null;
  if (!body?.course_id || !body.title || !body.type || !body.description || !body.preview_content) {
    return NextResponse.json({ error: "缺少资料必填字段。" }, { status: 400 });
  }
  if (!typeMap[body.type]) {
    return NextResponse.json({ error: "资料类型无效。" }, { status: 400 });
  }

  const materialId = body.id?.trim() || `${body.course_id}-${slugify(body.title)}`;
  const material = await prisma.material.create({
    data: {
      id: materialId,
      courseId: body.course_id,
      title: body.title.trim(),
      type: typeMap[body.type],
      description: body.description.trim(),
      fileUrl: body.file_url,
      fileName: body.file_name,
      fileSize: body.file_size,
      previewContent: body.preview_content.trim(),
      accessLevel: accessMap[body.access_level ?? "login_required"],
      status: statusMap[body.status ?? "draft"],
      createdById: admin?.id,
    },
  });

  return NextResponse.json({ material: { id: material.id, title: material.title } }, { status: 201 });
}

