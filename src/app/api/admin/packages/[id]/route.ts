import { NextResponse } from "next/server";
import { requireAdminResponse } from "@/lib/admin";
import { updateCoursePackage } from "@/services/package-service";
import type { CoursePackageItem, PackageStatus } from "@/types";

type RouteContext = {
  params: Promise<{ id: string }>;
};

type PackageBody = {
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
  if (!items) return undefined;
  const allowedTypes = new Set(["material", "course", "question_set"]);

  return items
    .filter((item) => item.resource_type && allowedTypes.has(item.resource_type) && item.resource_id)
    .map((item) => ({
      resourceType: item.resource_type!,
      resourceId: item.resource_id!,
    }));
}

export async function PATCH(request: Request, context: RouteContext) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const { id } = await context.params;
  const body = (await request.json().catch(() => null)) as PackageBody | null;
  if (!body) {
    return NextResponse.json({ error: "请求体无效。" }, { status: 400 });
  }
  if (body.price !== undefined && (!Number.isFinite(Number(body.price)) || Number(body.price) < 0)) {
    return NextResponse.json({ error: "价格无效。" }, { status: 400 });
  }

  try {
    const pkg = await updateCoursePackage(id, {
      title: body.title,
      description: body.description,
      schoolId: body.school_id,
      majorId: body.major_id,
      grade: body.grade,
      price: body.price === undefined ? undefined : Number(body.price),
      status: body.status,
      items: normalizeItems(body.items),
    });

    return NextResponse.json({ package: pkg });
  } catch {
    return NextResponse.json({ error: "课程包不存在或更新失败。" }, { status: 404 });
  }
}
