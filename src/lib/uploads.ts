import { randomUUID } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

export const MAX_UPLOAD_FILE_SIZE = 10 * 1024 * 1024;
export const ALLOWED_UPLOAD_TYPES = new Set(["application/pdf", "text/plain"]);

export type UploadLike = {
  name: string;
  type: string;
  size: number;
};

export type UploadValidationResult =
  | { ok: true }
  | { ok: false; message: string };

export type SavedUploadFile = {
  fileUrl: string;
  fileName: string;
  fileSize: number;
  contentType: string;
};

export function sanitizeFileName(fileName: string) {
  const safeName = fileName.replace(/[^a-zA-Z0-9._-]+/g, "-").replace(/^[.-]+|[.-]+$/g, "");
  return safeName || "upload";
}

export function validateUploadFile(file: UploadLike): UploadValidationResult {
  if (!ALLOWED_UPLOAD_TYPES.has(file.type)) {
    return { ok: false, message: "仅支持 PDF 或 TXT 文件。" };
  }
  if (file.size <= 0) {
    return { ok: false, message: "文件不能为空。" };
  }
  if (file.size > MAX_UPLOAD_FILE_SIZE) {
    return { ok: false, message: "文件不能超过 10MB。" };
  }
  return { ok: true };
}

export async function saveUploadFile(file: File, bucket: "admin" | "submissions"): Promise<SavedUploadFile> {
  const safeName = sanitizeFileName(file.name);
  const storedName = `${Date.now()}-${randomUUID()}-${safeName}`;
  const uploadDir = path.resolve(/*turbopackIgnore: true*/ process.cwd(), "uploads", bucket);
  await mkdir(uploadDir, { recursive: true });
  await writeFile(path.join(uploadDir, storedName), Buffer.from(await file.arrayBuffer()));

  return {
    fileUrl: `/uploads/${bucket}/${storedName}`,
    fileName: file.name,
    fileSize: file.size,
    contentType: file.type,
  };
}
