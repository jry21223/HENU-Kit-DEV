import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { listPackages } from "@/services/package-service";

export async function GET() {
  const user = await getCurrentUser();
  const packages = await listPackages(user?.id);

  return NextResponse.json({
    packages: packages.map((pkg) => ({
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
    })),
  });
}
