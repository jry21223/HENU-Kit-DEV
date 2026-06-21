import { RecordStatus } from "@prisma/client";
import { isDatabaseConfigured, prisma } from "@/lib/db";
import { mapAccessLevel, mapRecordStatus } from "@/services/mappers";
import type { CoursePackage, CoursePackageItem, PackageStatus } from "@/types";

type PackageBody = {
  id?: string;
  title: string;
  description: string;
  schoolId: string;
  majorId?: string | null;
  grade?: string | null;
  price: number;
  status?: PackageStatus;
  items?: Array<{
    resourceType: CoursePackageItem["resourceType"];
    resourceId: string;
  }>;
};

type EntitlementBody = {
  userId?: string;
  email?: string;
  resourceType: "package" | "material";
  resourceId: string;
  source?: string;
  expiresAt?: Date | null;
};

function toRecordStatus(status?: PackageStatus) {
  if (status === "published") return RecordStatus.PUBLISHED;
  if (status === "archived") return RecordStatus.ARCHIVED;
  return RecordStatus.DRAFT;
}

function formatPrice(value: unknown) {
  const amount = Number(value);
  if (!Number.isFinite(amount)) {
    return "0.00";
  }
  return amount.toFixed(2);
}

async function getUnlockedPackageIds(userId?: string) {
  if (!userId || !isDatabaseConfigured()) {
    return new Set<string>();
  }

  const entitlements = await prisma.entitlement.findMany({
    where: {
      userId,
      resourceType: "package",
      OR: [{ expiresAt: null }, { expiresAt: { gt: new Date() } }],
    },
    select: { resourceId: true },
  });

  return new Set(entitlements.map((item) => item.resourceId));
}

type DbPackage = Awaited<ReturnType<typeof prisma.coursePackage.findMany>>[number] & {
  school?: { name: string };
  major?: { name: string } | null;
  items?: Array<{
    id: string;
    resourceType: string;
    resourceId: string;
  }>;
};

function mapPackage(pkg: DbPackage, unlockedPackageIds: Set<string>): CoursePackage {
  return {
    id: pkg.id,
    title: pkg.title,
    description: pkg.description,
    schoolId: pkg.schoolId,
    schoolName: pkg.school?.name,
    majorId: pkg.majorId ?? undefined,
    majorName: pkg.major?.name,
    grade: pkg.grade ?? undefined,
    price: formatPrice(pkg.price),
    status: mapRecordStatus(pkg.status),
    itemCount: pkg.items?.length ?? 0,
    unlocked: unlockedPackageIds.has(pkg.id),
  };
}

export async function listPackages(userId?: string): Promise<CoursePackage[]> {
  if (!isDatabaseConfigured()) {
    return [];
  }

  const [packages, unlockedPackageIds] = await Promise.all([
    prisma.coursePackage.findMany({
      where: { status: RecordStatus.PUBLISHED },
      include: {
        school: { select: { name: true } },
        major: { select: { name: true } },
        items: { select: { id: true, resourceType: true, resourceId: true } },
      },
      orderBy: { createdAt: "desc" },
    }),
    getUnlockedPackageIds(userId),
  ]);

  return packages.map((pkg) => mapPackage(pkg, unlockedPackageIds));
}

export async function getPackageById(
  packageId: string,
  userId?: string,
): Promise<CoursePackage | null> {
  if (!isDatabaseConfigured()) {
    return null;
  }

  const [pkg, unlockedPackageIds] = await Promise.all([
    prisma.coursePackage.findFirst({
      where: { id: packageId, status: RecordStatus.PUBLISHED },
      include: {
        school: { select: { name: true } },
        major: { select: { name: true } },
        items: { select: { id: true, resourceType: true, resourceId: true } },
      },
    }),
    getUnlockedPackageIds(userId),
  ]);

  if (!pkg) {
    return null;
  }

  const materialIds = pkg.items
    .filter((item) => item.resourceType === "material")
    .map((item) => item.resourceId);
  const materials = materialIds.length
    ? await prisma.material.findMany({
        where: { id: { in: materialIds } },
        select: { id: true, title: true, accessLevel: true },
      })
    : [];
  const materialMap = new Map(materials.map((material) => [material.id, material]));

  return {
    ...mapPackage(pkg, unlockedPackageIds),
    items: pkg.items.map((item) => {
      const material = materialMap.get(item.resourceId);
      return {
        id: item.id,
        resourceType: item.resourceType as CoursePackageItem["resourceType"],
        resourceId: item.resourceId,
        title: material?.title ?? item.resourceId,
        accessLevel: material ? mapAccessLevel(material.accessLevel) : undefined,
      };
    }),
  };
}

export async function userCanAccessMaterial(userId: string, materialId: string) {
  if (!isDatabaseConfigured()) {
    return false;
  }

  const activeEntitlementWhere = {
    userId,
    OR: [{ expiresAt: null }, { expiresAt: { gt: new Date() } }],
  };

  const directEntitlement = await prisma.entitlement.findFirst({
    where: {
      ...activeEntitlementWhere,
      resourceType: "material",
      resourceId: materialId,
    },
    select: { id: true },
  });

  if (directEntitlement) {
    return true;
  }

  const packageEntitlements = await prisma.entitlement.findMany({
    where: {
      ...activeEntitlementWhere,
      resourceType: "package",
    },
    select: { resourceId: true },
  });
  const packageIds = packageEntitlements.map((item) => item.resourceId);

  if (packageIds.length === 0) {
    return false;
  }

  const packageItem = await prisma.packageItem.findFirst({
    where: {
      packageId: { in: packageIds },
      resourceType: "material",
      resourceId: materialId,
      package: { status: RecordStatus.PUBLISHED },
    },
    select: { id: true },
  });

  return Boolean(packageItem);
}

export async function createCoursePackage(body: PackageBody): Promise<CoursePackage> {
  const created = await prisma.$transaction(async (tx) => {
    const pkg = await tx.coursePackage.create({
      data: {
        id: body.id,
        title: body.title.trim(),
        description: body.description.trim(),
        schoolId: body.schoolId,
        majorId: body.majorId || null,
        grade: body.grade || null,
        price: body.price,
        status: toRecordStatus(body.status),
      },
    });

    if (body.items?.length) {
      await tx.packageItem.createMany({
        data: body.items.map((item) => ({
          packageId: pkg.id,
          resourceType: item.resourceType,
          resourceId: item.resourceId,
        })),
        skipDuplicates: true,
      });
    }

    return pkg;
  });

  return {
    id: created.id,
    title: created.title,
    description: created.description,
    schoolId: created.schoolId,
    majorId: created.majorId ?? undefined,
    grade: created.grade ?? undefined,
    price: formatPrice(created.price),
    status: mapRecordStatus(created.status),
    itemCount: body.items?.length ?? 0,
    unlocked: false,
  };
}

export async function updateCoursePackage(packageId: string, body: Partial<PackageBody>) {
  const updated = await prisma.$transaction(async (tx) => {
    const pkg = await tx.coursePackage.update({
      where: { id: packageId },
      data: {
        title: body.title?.trim(),
        description: body.description?.trim(),
        schoolId: body.schoolId,
        majorId: body.majorId === undefined ? undefined : body.majorId || null,
        grade: body.grade === undefined ? undefined : body.grade || null,
        price: body.price,
        status: body.status ? toRecordStatus(body.status) : undefined,
      },
    });

    if (body.items) {
      await tx.packageItem.deleteMany({ where: { packageId } });
      if (body.items.length) {
        await tx.packageItem.createMany({
          data: body.items.map((item) => ({
            packageId,
            resourceType: item.resourceType,
            resourceId: item.resourceId,
          })),
          skipDuplicates: true,
        });
      }
    }

    return pkg;
  });

  return {
    id: updated.id,
    title: updated.title,
    description: updated.description,
    schoolId: updated.schoolId,
    majorId: updated.majorId ?? undefined,
    grade: updated.grade ?? undefined,
    price: formatPrice(updated.price),
    status: mapRecordStatus(updated.status),
    itemCount: body.items?.length ?? 0,
    unlocked: false,
  };
}

export async function grantEntitlement(body: EntitlementBody) {
  if (!body.userId && !body.email) {
    return null;
  }

  const user = body.userId
    ? await prisma.user.findUnique({ where: { id: body.userId } })
    : await prisma.user.findUnique({ where: { email: body.email! } });

  if (!user) {
    return null;
  }

  return prisma.entitlement.upsert({
    where: {
      userId_resourceType_resourceId: {
        userId: user.id,
        resourceType: body.resourceType,
        resourceId: body.resourceId,
      },
    },
    update: {
      source: body.source ?? "manual",
      expiresAt: body.expiresAt,
    },
    create: {
      userId: user.id,
      resourceType: body.resourceType,
      resourceId: body.resourceId,
      source: body.source ?? "manual",
      expiresAt: body.expiresAt,
    },
  });
}
