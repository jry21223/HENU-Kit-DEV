import { MaterialStatus } from "@prisma/client";
import {
  findMaterial as findMockMaterial,
  getPublishedMaterialsByCourse as getMockMaterialsByCourse,
} from "@/constants/mock-data";
import { prisma, shouldUseMockData } from "@/lib/db";
import {
  formatFileSize,
  mapAccessLevel,
  mapMaterialStatus,
  mapMaterialType,
} from "@/services/mappers";
import type { Material } from "@/types";

type DbMaterial = Awaited<ReturnType<typeof prisma.material.findMany>>[number];

function mapMaterial(material: DbMaterial): Material {
  return {
    id: material.id,
    courseId: material.courseId,
    title: material.title,
    type: mapMaterialType(material.type),
    description: material.description,
    fileName: material.fileName ?? "未配置文件",
    fileSize: formatFileSize(material.fileSize),
    previewContent: material.previewContent,
    accessLevel: mapAccessLevel(material.accessLevel),
    status: mapMaterialStatus(material.status),
    updatedAt: material.updatedAt.toISOString().slice(0, 10),
  };
}

export async function listMaterialsByCourse(courseId: string): Promise<Material[]> {
  if (shouldUseMockData()) {
    return getMockMaterialsByCourse(courseId);
  }

  const materials = await prisma.material.findMany({
    where: {
      courseId,
      status: MaterialStatus.PUBLISHED,
    },
    orderBy: { updatedAt: "desc" },
  });

  return materials.map(mapMaterial);
}

export async function getMaterialById(materialId: string): Promise<Material | undefined> {
  if (shouldUseMockData()) {
    return findMockMaterial(materialId);
  }

  const material = await prisma.material.findFirst({
    where: {
      id: materialId,
      status: MaterialStatus.PUBLISHED,
    },
  });

  return material ? mapMaterial(material) : undefined;
}
