import { NextResponse } from "next/server";
import { RecordStatus } from "@prisma/client";
import { requireAdminResponse } from "@/lib/admin";
import { prisma } from "@/lib/db";

type CourseBody = {
  id?: string;
  school_id?: string;
  college_id?: string;
  major_ids?: string[];
  grades?: string[];
  name?: string;
  slug?: string;
  description?: string;
  exam_scope?: string;
  status?: "draft" | "published" | "archived";
  teacher?: string;
};

function toRecordStatus(status?: CourseBody["status"]) {
  if (status === "published") return RecordStatus.PUBLISHED;
  if (status === "archived") return RecordStatus.ARCHIVED;
  return RecordStatus.DRAFT;
}

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

  const body = (await request.json().catch(() => null)) as CourseBody | null;
  if (!body?.school_id || !body.college_id || !body.name || !body.description || !body.exam_scope) {
    return NextResponse.json({ error: "缺少课程必填字段。" }, { status: 400 });
  }
  if (!body.major_ids?.length || !body.grades?.length) {
    return NextResponse.json({ error: "至少选择一个专业和年级。" }, { status: 400 });
  }

  const slug = body.slug?.trim() || slugify(body.name);
  if (!slug) {
    return NextResponse.json({ error: "课程 slug 无效。" }, { status: 400 });
  }

  const courseId = body.id?.trim() || slug;
  const course = await prisma.$transaction(async (tx) => {
    const created = await tx.course.create({
      data: {
        id: courseId,
        schoolId: body.school_id!,
        collegeId: body.college_id!,
        name: body.name!.trim(),
        slug,
        description: body.description!.trim(),
        examScope: body.exam_scope!.trim(),
        grades: body.grades!,
        status: toRecordStatus(body.status),
        teacher: body.teacher?.trim() || null,
      },
    });
    await tx.courseMajor.createMany({
      data: body.major_ids!.map((majorId) => ({ courseId: created.id, majorId })),
      skipDuplicates: true,
    });
    return created;
  });

  return NextResponse.json({ course: { id: course.id, name: course.name } }, { status: 201 });
}

