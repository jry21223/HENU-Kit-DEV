import { NextResponse } from "next/server";
import { listSchools } from "@/services/catalog-service";

export async function GET() {
  const schools = await listSchools();

  return NextResponse.json({
    schools: schools.map((school) => ({
      id: school.id,
      name: school.name,
      slug: school.slug,
      email_domains: school.emailDomains,
      status: school.status,
    })),
  });
}

