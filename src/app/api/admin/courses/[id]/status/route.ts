import { NextResponse } from "next/server";
import { RecordStatus } from "@prisma/client";
import { requireAdminResponse } from "@/lib/admin";
import { prisma } from "@/lib/db";

type RouteContext = {
  params: Promise<{ id: string }>;
};

const statusMap = {
  draft: RecordStatus.DRAFT,
  published: RecordStatus.PUBLISHED,
  archived: RecordStatus.ARCHIVED,
} as const;

export async function PATCH(request: Request, context: RouteContext) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const { id } = await context.params;
  const body = (await request.json().catch(() => null)) as { status?: keyof typeof statusMap } | null;
  if (!body?.status || !statusMap[body.status]) {
    return NextResponse.json({ error: "状态无效。" }, { status: 400 });
  }

  const course = await prisma.course.update({
    where: { id },
    data: { status: statusMap[body.status] },
  });

  return NextResponse.json({ course: { id: course.id, status: course.status.toLowerCase() } });
}

