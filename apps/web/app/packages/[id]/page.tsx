import Link from "next/link";
import { ArrowLeft, FileText, ShieldCheck, WalletCards } from "lucide-react";
import { PackageEntitlementPanel } from "@/components/course-package/package-entitlement-panel";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button-link";
import { CoursePackageDetail, getApi } from "@/lib/api";
import { labelAccessLevel, labelMaterialType } from "@/lib/presentation";

type PageProps = {
  params: Promise<{ id: string }>;
};

const copy = {
  backToCourses: "返回课程库",
  packageBadge: "课程复习包",
  intro:
    "课程包用于把讲义、模拟卷、解析和速背资料组合交付。当前微信支付仍在联调准备中，已授权账号可直接访问包内付费资料。",
  price: "价格",
  materialList: "包内资料",
  emptyMaterials: "该课程包正在整理中，请稍后查看。",
  serviceGuard: "服务端权限校验",
  serviceGuardBody: "课程包本身不直接暴露 PDF 地址。每个资料下载都会校验账号权限并附加水印标识。",
  delivery: "交付状态",
  deliveryBody: "内测阶段支持后台手动授权；后续接入微信支付回调后，会由服务端自动发放课程包权限。",
  viewMaterial: "查看资料",
  unavailable: "课程包暂时不可访问",
};

async function loadPackage(id: string) {
  try {
    const response = await getApi<CoursePackageDetail>(`/packages/${id}`);
    return { detail: response.data, error: "" };
  } catch (error) {
    return {
      detail: null as CoursePackageDetail | null,
      error: error instanceof Error ? error.message : copy.unavailable,
    };
  }
}

export default async function PackageDetailPage({ params }: PageProps) {
  const { id } = await params;
  const { detail, error } = await loadPackage(id);

  return (
    <SiteShell>
      <nav className="flex items-center justify-between gap-3 text-sm">
        <ButtonLink href={detail?.package.courseId ? `/courses/${detail.package.courseId}` : "/courses"} variant="secondary">
          <ArrowLeft className="mr-2 size-4" aria-hidden="true" />
          {detail?.package.courseId ? "返回课程" : copy.backToCourses}
        </ButtonLink>
        <Link className="rounded-lg px-3 py-2 text-muted-foreground hover:bg-muted hover:text-foreground" href="/me">
          我的权益
        </Link>
      </nav>

      {error ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p> : null}

      {detail ? (
        <article className="grid gap-5 lg:grid-cols-[1fr_340px]">
          <section className="min-w-0 rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
            <Badge tone="success">{copy.packageBadge}</Badge>
            <h1 className="mt-4 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{detail.package.title}</h1>
            <p className="mt-4 max-w-3xl text-sm leading-6 text-muted-foreground">{detail.package.description || copy.intro}</p>

            <dl className="mt-6 grid gap-3 text-sm sm:grid-cols-3">
              <div className="rounded-2xl border border-border bg-background p-4">
                <dt className="text-xs text-muted-foreground">{copy.price}</dt>
                <dd className="mt-1 font-semibold">{formatPrice(detail.package.priceFen, detail.package.currency)}</dd>
              </div>
              <div className="rounded-2xl border border-border bg-background p-4">
                <dt className="text-xs text-muted-foreground">年级</dt>
                <dd className="mt-1 font-semibold">{detail.package.grade || "未设置"}</dd>
              </div>
              <div className="rounded-2xl border border-border bg-background p-4">
                <dt className="text-xs text-muted-foreground">已发布资料</dt>
                <dd className="mt-1 font-semibold">{detail.materials.length} 份</dd>
              </div>
            </dl>

            <section className="mt-6">
              <div className="flex items-center gap-2">
                <FileText className="size-5 text-primary" aria-hidden="true" />
                <h2 className="text-xl font-semibold tracking-tight">{copy.materialList}</h2>
              </div>
              <div className="mt-4 grid gap-3">
                {detail.materials.map((material) => (
                  <Link
                    className="group rounded-2xl border border-border bg-background p-4 transition hover:border-primary/60 hover:bg-card"
                    href={`/materials/${material.id}`}
                    key={material.id}
                  >
                    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                      <div className="min-w-0">
                        <div className="flex flex-wrap gap-2">
                          <Badge tone="muted">{labelMaterialType(material.type)}</Badge>
                          <Badge tone="muted">{labelAccessLevel(material.accessLevel)}</Badge>
                        </div>
                        <h3 className="mt-3 break-words text-base font-semibold tracking-tight">{material.title}</h3>
                        <p className="mt-2 line-clamp-2 break-words text-sm leading-6 text-muted-foreground">
                          {material.description || material.previewContent || "暂无资料说明。"}
                        </p>
                      </div>
                      <span className="shrink-0 text-sm font-medium text-primary">{copy.viewMaterial}</span>
                    </div>
                  </Link>
                ))}
              </div>
              {detail.materials.length === 0 ? (
                <p className="mt-4 rounded-2xl border border-border bg-background p-4 text-sm leading-6 text-muted-foreground">
                  {copy.emptyMaterials}
                </p>
              ) : null}
            </section>
          </section>

          <aside className="grid gap-5 self-start">
            <PackageEntitlementPanel coursePackage={detail.package} materials={detail.materials} />

            <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
              <ShieldCheck className="size-5 text-primary" aria-hidden="true" />
              <h2 className="mt-3 text-lg font-semibold tracking-tight">{copy.serviceGuard}</h2>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{copy.serviceGuardBody}</p>
            </section>

            <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
              <WalletCards className="size-5 text-primary" aria-hidden="true" />
              <h2 className="mt-3 text-lg font-semibold tracking-tight">{copy.delivery}</h2>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{copy.deliveryBody}</p>
            </section>
          </aside>
        </article>
      ) : null}
    </SiteShell>
  );
}

function formatPrice(priceFen: number, currency: string) {
  const normalizedCurrency = currency?.toUpperCase() || "CNY";
  const amount = (priceFen / 100).toFixed(priceFen % 100 === 0 ? 0 : 2);
  return normalizedCurrency === "CNY" ? `CNY ${amount}` : `${amount} ${normalizedCurrency}`;
}
