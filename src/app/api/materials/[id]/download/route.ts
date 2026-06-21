import { NextResponse } from "next/server";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { prisma } from "@/lib/db";
import { getCurrentUser } from "@/lib/auth";
import { addPdfWatermark, isPdfFileName } from "@/lib/pdf-watermark";
import { canDownloadMaterial } from "@/lib/permissions";
import { userCanAccessMaterial } from "@/services/package-service";

export const runtime = "nodejs";

type RouteContext = {
  params: Promise<{ id: string }>;
};

function getContentType(fileName?: string | null) {
  if (fileName?.toLowerCase().endsWith(".pdf")) {
    return "application/pdf";
  }
  if (fileName?.toLowerCase().endsWith(".txt")) {
    return "text/plain; charset=utf-8";
  }
  return "application/octet-stream";
}

function resolveLocalUploadPath(fileUrl: string) {
  const normalized = fileUrl.replaceAll("\\", "/");
  if (!normalized.startsWith("/uploads/")) {
    return null;
  }
  const relativePath = normalized.replace(/^\/uploads\//, "");
  const uploadRoot = path.resolve(/*turbopackIgnore: true*/ process.cwd(), "uploads");
  const resolvedPath = path.resolve(uploadRoot, relativePath);
  return resolvedPath.startsWith(uploadRoot) ? resolvedPath : null;
}

export async function GET(request: Request, context: RouteContext) {
  const { id } = await context.params;
  const material = await prisma.material.findUnique({ where: { id } });

  if (!material || material.status !== "PUBLISHED") {
    return NextResponse.json({ error: "资料不存在或未发布。" }, { status: 404 });
  }

  const user = await getCurrentUser();
  const hasPaidAccess =
    material.accessLevel === "PAID" && user
      ? await userCanAccessMaterial(user.id, material.id)
      : false;
  const permission = canDownloadMaterial(material, user, hasPaidAccess);

  if (!permission.allowed) {
    return NextResponse.json({ error: permission.message }, { status: permission.status });
  }

  if (!material.fileUrl || !material.fileName) {
    return NextResponse.json({ error: "资料文件尚未配置。" }, { status: 404 });
  }

  const filePath = resolveLocalUploadPath(material.fileUrl);
  if (!filePath) {
    return NextResponse.json({ error: "资料文件路径无效。" }, { status: 404 });
  }

  let file: Buffer;
  try {
    file = await readFile(filePath);
  } catch {
    return NextResponse.json({ error: "资料文件不存在。" }, { status: 404 });
  }

  const downloadedAt = new Date();

  await prisma.download.create({
    data: {
      userId: user?.id,
      materialId: material.id,
      ip:
        request.headers.get("x-forwarded-for")?.split(",")[0]?.trim() ??
        request.headers.get("x-real-ip"),
      userAgent: request.headers.get("user-agent"),
    },
  });

  const shouldWatermark = isPdfFileName(material.fileName);
  const responseFile = shouldWatermark
    ? await addPdfWatermark(file, {
        userEmail: user?.email,
        downloadedAt,
      })
    : file;

  return new NextResponse(new Uint8Array(responseFile), {
    headers: {
      "Content-Type": getContentType(material.fileName),
      "Content-Disposition": `attachment; filename=\"${encodeURIComponent(material.fileName)}\"`,
      "Content-Length": responseFile.byteLength.toString(),
      "Cache-Control": "private, no-store",
      "X-Watermark-Applied": shouldWatermark ? "true" : "false",
    },
  });
}
