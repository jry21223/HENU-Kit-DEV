import { NextResponse } from "next/server";
import { requireAdminResponse } from "@/lib/admin";
import { saveUploadFile, validateUploadFile } from "@/lib/uploads";

export const runtime = "nodejs";

export async function POST(request: Request) {
  const forbidden = await requireAdminResponse();
  if (forbidden) return forbidden;

  const formData = await request.formData();
  const file = formData.get("file");

  if (!(file instanceof File)) {
    return NextResponse.json({ error: "请上传文件。" }, { status: 400 });
  }
  const validation = validateUploadFile(file);
  if (!validation.ok) {
    return NextResponse.json({ error: validation.message }, { status: 400 });
  }

  const savedFile = await saveUploadFile(file, "admin");

  return NextResponse.json({
    file: {
      file_url: savedFile.fileUrl,
      file_name: savedFile.fileName,
      file_size: savedFile.fileSize,
      content_type: savedFile.contentType,
    },
  });
}
