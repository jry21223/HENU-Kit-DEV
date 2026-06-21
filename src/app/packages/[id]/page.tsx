import Link from "next/link";
import { notFound } from "next/navigation";
import { PageShell } from "@/components/layout/page-shell";
import { PurchasePackageButton } from "@/components/package/purchase-package-button";
import { getCurrentUser } from "@/lib/auth";
import { getPackageById } from "@/services/package-service";

type PackageDetailPageProps = {
  params: Promise<{ id: string }>;
};

export default async function PackageDetailPage({ params }: PackageDetailPageProps) {
  const { id } = await params;
  const user = await getCurrentUser();
  const pkg = await getPackageById(id, user?.id);

  if (!pkg) {
    notFound();
  }

  return (
    <PageShell
      eyebrow="课程复习包"
      title={pkg.title}
      description={pkg.description}
    >
      <section className="grid gap-5 lg:grid-cols-[1fr_0.7fr]">
        <article className="rounded-lg border border-line bg-white p-6 shadow-soft">
          <div className="mb-4 flex flex-wrap gap-2 text-xs">
            <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
              {pkg.schoolName ?? pkg.schoolId}
            </span>
            {pkg.majorName ? (
              <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                {pkg.majorName}
              </span>
            ) : null}
            {pkg.grade ? (
              <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                {pkg.grade}
              </span>
            ) : null}
            <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
              {pkg.unlocked ? "已解锁" : "未解锁"}
            </span>
          </div>
          <h2 className="text-lg font-semibold text-ink">包含内容</h2>
          <div className="mt-4 grid gap-3">
            {pkg.items?.map((item) => (
              <div key={item.id} className="rounded-lg border border-line bg-panel p-4">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <p className="text-sm font-semibold text-ink">{item.title}</p>
                    <p className="mt-1 text-xs text-muted">
                      {item.resourceType} · {item.accessLevel ?? "资源"}
                    </p>
                  </div>
                  {item.resourceType === "material" ? (
                    <Link
                      href={`/materials/${item.resourceId}`}
                      className="inline-flex h-10 items-center justify-center rounded-md border border-line bg-white px-4 text-sm font-semibold text-ink hover:bg-panel focus-ring"
                    >
                      查看资料
                    </Link>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        </article>

        <aside className="rounded-lg border border-line bg-white p-6 shadow-soft">
          <h2 className="text-lg font-semibold text-ink">解锁状态</h2>
          <p className="mt-3 text-3xl font-semibold text-ink">￥{pkg.price}</p>
          <p className="mt-3 text-sm leading-6 text-muted">
            目标方案为微信 Native 扫码支付。支付成功必须经过服务端回调确认后才会发放权限。
          </p>
          {pkg.unlocked ? (
            <div className="mt-5 rounded-lg border border-[#b8dccf] bg-[#f1faf6] p-4 text-sm font-semibold text-[#185c48]">
              当前账号已解锁该复习包。
            </div>
          ) : (
            <>
              <div className="mt-5 rounded-lg border border-line bg-panel p-4 text-sm text-muted">
                未解锁用户不能下载包内 paid 资料。
              </div>
              {user ? (
                <PurchasePackageButton packageId={pkg.id} />
              ) : (
                <Link
                  href="/login"
                  className="mt-5 inline-flex h-10 w-full items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
                >
                  登录后购买
                </Link>
              )}
            </>
          )}
          <Link
            href="/packages"
            className="mt-5 inline-flex h-10 w-full items-center justify-center rounded-md border border-line px-4 text-sm font-semibold text-ink hover:bg-panel focus-ring"
          >
            返回复习包列表
          </Link>
        </aside>
      </section>
    </PageShell>
  );
}
