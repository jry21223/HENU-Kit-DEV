import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { getPackageById } from "@/services/package-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  const user = await getCurrentUser();
  const pkg = await getPackageById(id, user?.id);

  if (!pkg) {
    return NextResponse.json({ error: "课程包不存在或未发布。" }, { status: 404 });
  }

  return NextResponse.json({
    package: {
      id: pkg.id,
      title: pkg.title,
      description: pkg.description,
      school_id: pkg.schoolId,
      school_name: pkg.schoolName ?? null,
      major_id: pkg.majorId ?? null,
      major_name: pkg.majorName ?? null,
      grade: pkg.grade ?? null,
      price: pkg.price,
      status: pkg.status,
      item_count: pkg.itemCount,
      unlocked: pkg.unlocked,
      items: pkg.items?.map((item) => ({
        id: item.id,
        resource_type: item.resourceType,
        resource_id: item.resourceId,
        title: item.title,
        access_level: item.accessLevel ?? null,
      })),
    },
  });
}
