import { NextResponse } from "next/server";
import { listMajors } from "@/services/catalog-service";

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url);
  const schoolId = searchParams.get("schoolId");
  const collegeId = searchParams.get("collegeId");
  const majors = await listMajors();

  return NextResponse.json({
    majors: majors
      .filter((major) => !schoolId || major.schoolId === schoolId)
      .filter((major) => !collegeId || major.collegeId === collegeId)
      .map((major) => ({
        id: major.id,
        school_id: major.schoolId,
        college_id: major.collegeId,
        name: major.name,
        slug: major.slug,
      })),
  });
}

