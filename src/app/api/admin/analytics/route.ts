import { NextResponse } from "next/server";
import { requireAdminResponse } from "@/lib/admin";
import { getAdminAnalytics } from "@/services/analytics-service";

export async function GET() {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const analytics = await getAdminAnalytics();
  return NextResponse.json({ analytics });
}
