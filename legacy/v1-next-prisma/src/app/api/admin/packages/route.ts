import { NextResponse } from "next/server";
import { requireAdminResponse } from "@/lib/admin";
import { createCoursePackage } from "@/services/package-service";
import type { CoursePackageItem, PackageStatus } from "@/types";

type PackageBody = {
  id?: string;
  title?: string;
  description?: string;
  school_id?: string;
  major_id?: string | null;
  grade?: string | null;
  price?: number;
  status?: PackageStatus;
  items?: Array<{
    resource_type?: CoursePackageItem["resourceType"];
    resource_id?: string;
  }>;
};

function normalizeItems(items: PackageBody["items"]) {
  if (!items) return [];
  const allowedTypes = new Set(["material", "course", "question_set"]);

  return items
    .filter((item) => item.resource_type && allowedTypes.has(item.resource_type) && item.resource_id)
    .map((item) => ({
      resourceType: item.resource_type!,
      resourceId: item.resource_id!,
    }));
}

export async function POST(request: Request) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const body = (await request.json().catch(() => null)) as PackageBody | null;
  if (!body?.title || !body.description || !body.school_id || body.price === undefined) {
    return NextResponse.json({ error: "缺少课程包必填字段。" }, { status: 400 });
  }
  if (!Number.isFinite(Number(body.price)) || Number(body.price) < 0) {
    return NextResponse.json({ error: "价格无效。" }, { status: 400 });
  }

  const pkg = await createCoursePackage({
    id: body.id,
    title: body.title,
    description: body.description,
    schoolId: body.school_id,
    majorId: body.major_id,
    grade: body.grade,
    price: Number(body.price),
    status: body.status,
    items: normalizeItems(body.items),
  });

  return NextResponse.json({ package: pkg }, { status: 201 });
}
