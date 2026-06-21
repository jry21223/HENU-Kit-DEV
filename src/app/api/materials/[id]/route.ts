import { NextResponse } from "next/server";
import { getMaterialById } from "@/services/material-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  const material = await getMaterialById(id);

  if (!material) {
    return NextResponse.json({ error: "资料不存在或未发布。" }, { status: 404 });
  }

  return NextResponse.json({
    material: {
      id: material.id,
      course_id: material.courseId,
      title: material.title,
      type: material.type,
      description: material.description,
      file_name: material.fileName,
      file_size: material.fileSize,
      preview_content: material.previewContent,
      access_level: material.accessLevel,
      updated_at: material.updatedAt,
    },
  });
}

