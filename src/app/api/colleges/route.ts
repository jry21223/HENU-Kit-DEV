import { NextResponse } from "next/server";
import { listColleges } from "@/services/catalog-service";

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url);
  const schoolId = searchParams.get("schoolId") ?? undefined;
  const colleges = await listColleges(schoolId);

  return NextResponse.json({
    colleges: colleges.map((college) => ({
      id: college.id,
      school_id: college.schoolId,
      name: college.name,
    })),
  });
}

