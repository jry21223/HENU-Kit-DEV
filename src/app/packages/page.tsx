import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";
import { getCurrentUser } from "@/lib/auth";
import { listPackages } from "@/services/package-service";

export default async function PackagesPage() {
  const user = await getCurrentUser();
  const packages = await listPackages(user?.id);

  return (
    <PageShell
      eyebrow="课程复习包"
      title="按课程解锁复习资料"
      description="支持课程包浏览、手动授权和微信 Native 支付联调准备；真实微信商户参数配置后再进入扫码支付。"
    >
      {packages.length > 0 ? (
        <div className="grid gap-4 md:grid-cols-2">
          {packages.map((pkg) => (
            <article key={pkg.id} className="rounded-lg border border-line bg-white p-5 shadow-soft">
              <div className="flex min-h-[184px] flex-col justify-between gap-5">
                <div>
                  <div className="mb-3 flex flex-wrap gap-2 text-xs">
                    <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                      {pkg.schoolName ?? pkg.schoolId}
                    </span>
                    {pkg.grade ? (
                      <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                        {pkg.grade}
                      </span>
                    ) : null}
                    <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                      {pkg.unlocked ? "已解锁" : "未解锁"}
                    </span>
                  </div>
                  <h2 className="text-lg font-semibold text-ink">{pkg.title}</h2>
                  <p className="mt-2 text-sm leading-6 text-muted">{pkg.description}</p>
                </div>
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <p className="text-sm font-semibold text-ink">
                    ￥{pkg.price} · {pkg.itemCount} 项内容
                  </p>
                  <Link
                    href={`/packages/${pkg.id}`}
                    className="inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
                  >
                    查看详情
                  </Link>
                </div>
              </div>
            </article>
          ))}
        </div>
      ) : (
        <div className="rounded-lg border border-line bg-white p-6 text-sm text-muted shadow-soft">
          暂无已发布课程复习包。
        </div>
      )}
    </PageShell>
  );
}
