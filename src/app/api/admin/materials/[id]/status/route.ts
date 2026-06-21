import { NextResponse } from "next/server";
import { MaterialStatus } from "@prisma/client";
import { requireAdminResponse } from "@/lib/admin";
import { prisma } from "@/lib/db";

type RouteContext = {
  params: Promise<{ id: string }>;
};

const statusMap = {
  draft: MaterialStatus.DRAFT,
  pending_review: MaterialStatus.PENDING_REVIEW,
  published: MaterialStatus.PUBLISHED,
  archived: MaterialStatus.ARCHIVED,
} as const;

export async function PATCH(request: Request, context: RouteContext) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const { id } = await context.params;
  const body = (await request.json().catch(() => null)) as { status?: keyof typeof statusMap } | null;
  if (!body?.status || !statusMap[body.status]) {
    return NextResponse.json({ error: "状态无效。" }, { status: 400 });
  }

  const material = await prisma.material.update({
    where: { id },
    data: { status: statusMap[body.status] },
  });

  return NextResponse.json({
    material: { id: material.id, status: material.status.toLowerCase() },
  });
}

