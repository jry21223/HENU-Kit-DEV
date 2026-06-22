import { NextResponse } from "next/server";
import { MaterialAccessLevel, MaterialType } from "@prisma/client";
import { requireAdminResponse } from "@/lib/admin";
import { prisma } from "@/lib/db";

type RouteContext = {
  params: Promise<{ id: string }>;
};

type PatchMaterialBody = {
  title?: string;
  type?: "knowledge_note" | "mock_paper" | "answer" | "quick_review" | "past_exam" | "other";
  description?: string;
  file_url?: string;
  file_name?: string;
  file_size?: number;
  preview_content?: string;
  access_level?: "free" | "login_required" | "paid";
};

const typeMap = {
  knowledge_note: MaterialType.KNOWLEDGE_NOTE,
  mock_paper: MaterialType.MOCK_PAPER,
  answer: MaterialType.ANSWER,
  quick_review: MaterialType.QUICK_REVIEW,
  past_exam: MaterialType.PAST_EXAM,
  other: MaterialType.OTHER,
} as const;

const accessMap = {
  free: MaterialAccessLevel.FREE,
  login_required: MaterialAccessLevel.LOGIN_REQUIRED,
  paid: MaterialAccessLevel.PAID,
} as const;

export async function PATCH(request: Request, context: RouteContext) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const { id } = await context.params;
  const body = (await request.json().catch(() => null)) as PatchMaterialBody | null;
  if (!body) {
    return NextResponse.json({ error: "请求体无效。" }, { status: 400 });
  }

  const material = await prisma.material.update({
    where: { id },
    data: {
      title: body.title?.trim(),
      type: body.type ? typeMap[body.type] : undefined,
      description: body.description?.trim(),
      fileUrl: body.file_url,
      fileName: body.file_name,
      fileSize: body.file_size,
      previewContent: body.preview_content?.trim(),
      accessLevel: body.access_level ? accessMap[body.access_level] : undefined,
    },
  });

  return NextResponse.json({ material: { id: material.id, title: material.title } });
}
