import { NextResponse } from "next/server";
import { getCourseById } from "@/services/course-service";
import { listMaterialsByCourse } from "@/services/material-service";

type RouteContext = {
  params: Promise<{ id: string }>;
};

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  const course = await getCourseById(id);

  if (!course) {
    return NextResponse.json({ error: "课程不存在或未发布。" }, { status: 404 });
  }

  const materials = await listMaterialsByCourse(id);

  return NextResponse.json({
    materials: materials.map((material) => ({
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
    })),
  });
}

