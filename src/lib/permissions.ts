import type { Material, User } from "@prisma/client";

type DownloadUser = Pick<User, "id" | "emailVerified"> | null;
type DownloadMaterial = Pick<Material, "accessLevel" | "status">;

export type DownloadPermissionResult =
  | { allowed: true }
  | { allowed: false; status: number; message: string };

export function canDownloadMaterial(
  material: DownloadMaterial,
  user: DownloadUser,
  hasPaidAccess = false,
): DownloadPermissionResult {
  if (material.status !== "PUBLISHED") {
    return { allowed: false, status: 404, message: "资料不存在或未发布。" };
  }

  if (material.accessLevel === "FREE") {
    return { allowed: true };
  }

  if (material.accessLevel === "LOGIN_REQUIRED") {
    if (!user || !user.emailVerified) {
      return { allowed: false, status: 401, message: "请先使用学生邮箱登录。" };
    }
    return { allowed: true };
  }

  if (!user || !user.emailVerified) {
    return { allowed: false, status: 401, message: "请先使用学生邮箱登录并解锁课程复习包。" };
  }

  if (hasPaidAccess) {
    return { allowed: true };
  }

  return { allowed: false, status: 402, message: "该资料需要解锁课程复习包。" };
}
