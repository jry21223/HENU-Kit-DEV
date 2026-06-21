import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";
import { getAdminUser } from "@/lib/admin";
import { prisma } from "@/lib/db";
import { mapRecordStatus } from "@/services/mappers";

export default async function AdminPackagesPage() {
  const admin = await getAdminUser();

  if (!admin) {
    return (
      <PageShell
        eyebrow="403"
        title="无权访问课程包管理"
        description="课程包管理需要 admin 角色。"
      >
        <Link
          href="/login"
          className="inline-flex h-10 items-center rounded-md bg-brand px-4 text-sm font-semibold text-white"
        >
          去登录
        </Link>
      </PageShell>
    );
  }

  const packages = await prisma.coursePackage.findMany({
    include: {
      items: { select: { id: true } },
      school: { select: { name: true } },
      major: { select: { name: true } },
    },
    orderBy: { createdAt: "desc" },
  });

  return (
    <PageShell
      eyebrow="Admin"
      title="课程包管理"
      description="Phase 9 基础版通过 admin API 创建课程包、维护包内资源和手动授权。"
    >
      <section className="rounded-lg border border-line bg-white p-6 shadow-soft">
        <div className="rounded-lg border border-line bg-panel p-4 text-sm leading-6 text-muted">
          写操作接口：`POST /api/admin/packages`、`PATCH /api/admin/packages/:id`、
          `POST /api/admin/entitlements`。
        </div>
        <div className="mt-5 grid gap-4">
          {packages.map((pkg) => (
            <article key={pkg.id} className="rounded-lg border border-line bg-white p-5">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h2 className="text-base font-semibold text-ink">{pkg.title}</h2>
                  <p className="mt-2 text-sm leading-6 text-muted">{pkg.description}</p>
                  <p className="mt-2 text-xs text-muted">
                    {pkg.school.name} · {pkg.major?.name ?? "全部专业"} · {pkg.grade ?? "全部年级"}
                  </p>
                </div>
                <div className="text-sm font-semibold text-ink">
                  ￥{Number(pkg.price).toFixed(2)} · {pkg.items.length} 项 ·{" "}
                  {mapRecordStatus(pkg.status)}
                </div>
              </div>
            </article>
          ))}
          {packages.length === 0 ? (
            <div className="rounded-lg border border-line bg-white p-5 text-sm text-muted">
              暂无课程包。
            </div>
          ) : null}
        </div>
      </section>
    </PageShell>
  );
}
