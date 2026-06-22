import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";
import { getAdminUser } from "@/lib/admin";
import { prisma } from "@/lib/db";

export default async function AdminMaterialsPage() {
  const admin = await getAdminUser();
  if (!admin) {
    return (
      <PageShell title="无权访问资料管理" eyebrow="403">
        <Link href="/login" className="text-sm font-semibold text-brand">
          去登录
        </Link>
      </PageShell>
    );
  }

  const materials = await prisma.material.findMany({
    orderBy: { updatedAt: "desc" },
    take: 50,
    include: { course: { select: { name: true } } },
  });

  return (
    <PageShell title="资料管理" eyebrow="Admin" description="管理资料记录和状态。">
      <Link
        href="/admin/materials/new"
        className="mb-4 inline-flex h-10 items-center rounded-md bg-brand px-4 text-sm font-semibold text-white"
      >
        新增资料
      </Link>
      <div className="grid gap-3">
        {materials.map((material) => (
          <article key={material.id} className="rounded-lg border border-line bg-white p-4 shadow-soft">
            <h2 className="font-semibold text-ink">{material.title}</h2>
            <p className="mt-1 text-sm text-muted">
              {material.course.name} / {material.status.toLowerCase()} / {material.accessLevel.toLowerCase()}
            </p>
          </article>
        ))}
      </div>
    </PageShell>
  );
}

