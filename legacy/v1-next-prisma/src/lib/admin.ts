import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { canReviewSubmissions } from "@/lib/submissions";

export async function getAdminUser() {
  const user = await getCurrentUser();
  return user?.role === "ADMIN" ? user : null;
}

export async function requireAdminResponse() {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "请先登录管理员账号。" }, { status: 401 });
  }
  if (user.role !== "ADMIN") {
    return NextResponse.json({ error: "没有管理员权限。" }, { status: 403 });
  }
  return null;
}

export async function getReviewerUser() {
  const user = await getCurrentUser();
  return canReviewSubmissions(user?.role) ? user : null;
}

export async function requireReviewerResponse() {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "请先登录审核账号。" }, { status: 401 });
  }
  if (!canReviewSubmissions(user.role)) {
    return NextResponse.json({ error: "没有审核权限。" }, { status: 403 });
  }
  return null;
}
