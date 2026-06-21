import { NextResponse } from "next/server";
import { getCurrentUser } from "@/lib/auth";
import { saveUploadFile, validateUploadFile } from "@/lib/uploads";
import {
  createSubmission,
  getSubmittableCourse,
} from "@/services/submission-service";

export const runtime = "nodejs";

function getRequiredText(formData: FormData, key: string) {
  const value = formData.get(key);
  return typeof value === "string" ? value.trim() : "";
}

export async function POST(request: Request) {
  const user = await getCurrentUser();
  if (!user) {
    return NextResponse.json({ error: "请先使用学生邮箱登录。" }, { status: 401 });
  }
  if (!user.emailVerified) {
    return NextResponse.json({ error: "请先完成学生邮箱验证。" }, { status: 403 });
  }

  const formData = await request.formData();
  const courseId = getRequiredText(formData, "course_id");
  const title = getRequiredText(formData, "title");
  const description = getRequiredText(formData, "description");
  const file = formData.get("file");

  if (!courseId || !title || !description) {
    return NextResponse.json({ error: "缺少投稿必填字段。" }, { status: 400 });
  }
  if (title.length < 2 || title.length > 100) {
    return NextResponse.json({ error: "标题长度需在 2 到 100 字之间。" }, { status: 400 });
  }
  if (description.length > 1000) {
    return NextResponse.json({ error: "说明不能超过 1000 字。" }, { status: 400 });
  }
  if (!(file instanceof File)) {
    return NextResponse.json({ error: "请上传资料文件。" }, { status: 400 });
  }

  const validation = validateUploadFile(file);
  if (!validation.ok) {
    return NextResponse.json({ error: validation.message }, { status: 400 });
  }

  const course = await getSubmittableCourse(courseId);
  if (!course) {
    return NextResponse.json({ error: "课程不存在或未发布。" }, { status: 404 });
  }

  const savedFile = await saveUploadFile(file, "submissions");
  const submission = await createSubmission({
    userId: user.id,
    courseId,
    title,
    description,
    fileUrl: savedFile.fileUrl,
  });

  return NextResponse.json({ submission }, { status: 201 });
}
