import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { listUserSubmissions } from "@/services/submission-service";

export async function GET() {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "请先登录。" }, { status: 401 });
  }

  const submissions = await listUserSubmissions(user.id);
  return NextResponse.json({ submissions });
}
