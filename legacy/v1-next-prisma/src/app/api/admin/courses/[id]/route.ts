import { NextResponse } from "next/server";
import { requireAdminResponse } from "@/lib/admin";
import { prisma } from "@/lib/db";

type RouteContext = {
  params: Promise<{ id: string }>;
};

type PatchCourseBody = {
  name?: string;
  description?: string;
  exam_scope?: string;
  teacher?: string | null;
  grades?: string[];
  major_ids?: string[];
};

export async function PATCH(request: Request, context: RouteContext) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const { id } = await context.params;
  const body = (await request.json().catch(() => null)) as PatchCourseBody | null;
  if (!body) {
    return NextResponse.json({ error: "请求体无效。" }, { status: 400 });
  }

  const course = await prisma.$transaction(async (tx) => {
    const updated = await tx.course.update({
      where: { id },
      data: {
        name: body.name?.trim(),
        description: body.description?.trim(),
        examScope: body.exam_scope?.trim(),
        teacher: body.teacher ?? undefined,
        grades: body.grades,
      },
    });

    if (body.major_ids) {
      await tx.courseMajor.deleteMany({ where: { courseId: id } });
      await tx.courseMajor.createMany({
        data: body.major_ids.map((majorId) => ({ courseId: id, majorId })),
        skipDuplicates: true,
      });
    }

    return updated;
  });

  return NextResponse.json({ course: { id: course.id, name: course.name } });
}

