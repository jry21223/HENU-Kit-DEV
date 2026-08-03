import Link from "next/link";
import { ArrowLeft, ArrowRight, FileDown, ShieldCheck, UsersRound } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button-link";
import { Course, CoursePackage, CoursePackageDetail, Material, getApi } from "@/lib/api";
import { labelAccessLevel, labelMaterialType, latestUpdatedAt, summarizeMaterialTypes } from "@/lib/presentation";

type PageProps = {
  params: Promise<{ id: string }>;
};

async function loadCourse(id: string) {
  try {
    const [courseResponse, materialsResponse, packagesResponse] = await Promise.all([
      getApi<{ course: Course }>(`/courses/${id}`),
      getApi<{ materials: Material[] }>(`/courses/${id}/materials`),
      getApi<{ packages: CoursePackage[] }>(`/courses/${id}/packages`),
    ]);
    const packageDetails = await Promise.all(
      packagesResponse.data.packages.map(async (coursePackage) => {
        const response = await getApi<CoursePackageDetail>(`/packages/${coursePackage.id}`);
        return response.data;
      }),
    );
    return {
      course: courseResponse.data.course,
      materials: materialsResponse.data.materials,
      packageDetails,
      error: "",
    };
  } catch (error) {
    return {
      course: null as Course | null,
      materials: [] as Material[],
      packageDetails: [] as CoursePackageDetail[],
      error: error instanceof Error ? error.message : "API unavailable",
    };
  }
}

export default async function CourseDetailPage({ params }: PageProps) {
  const { id } = await params;
  const { course, materials, packageDetails, error } = await loadCourse(id);
  const materialTypes = summarizeMaterialTypes(materials);

  return (
    <SiteShell>
      <nav className="flex items-center justify-between gap-3 text-sm">
        <ButtonLink href="/courses" variant="secondary">
          <ArrowLeft className="mr-2 size-4" aria-hidden="true" />
          返回课程库
        </ButtonLink>
        {course ? (
          <Link className="rounded-lg px-3 py-2 text-muted-foreground hover:bg-muted hover:text-foreground" href={`/courses/${course.id}/quiz`}>
            题目练习
          </Link>
        ) : null}
      </nav>

      {error ? (
        <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p>
      ) : null}

      {course ? (
        <>
          <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
            <div className="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
              <div className="min-w-0">
                <Badge tone="success">资料保障</Badge>
                <h1 className="mt-4 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{course.name}</h1>
                <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">
                  {course.description || "这门课程的 PDF 资料会持续整理，下载时附带不影响阅读的轻水印。"}
                </p>
              </div>
              <dl className="grid shrink-0 gap-3 text-sm sm:grid-cols-3 lg:w-80 lg:grid-cols-1">
                <div className="rounded-2xl border border-border bg-background p-4">
                  <dt className="text-xs text-muted-foreground">适用年级</dt>
                  <dd className="mt-1 font-medium">{course.grade || "年级未设置"}</dd>
                </div>
                <div className="rounded-2xl border border-border bg-background p-4">
                  <dt className="text-xs text-muted-foreground">资料数量</dt>
                  <dd className="mt-1 font-medium">{materials.length} 份</dd>
                </div>
                <div className="rounded-2xl border border-border bg-background p-4">
                  <dt className="text-xs text-muted-foreground">最近维护</dt>
                  <dd className="mt-1 font-medium">{latestUpdatedAt(materials)}</dd>
                </div>
              </dl>
            </div>
          </section>

          <section className="grid gap-4 lg:grid-cols-[1fr_280px]">
            <div className="grid gap-4">
              <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
                <div className="flex flex-wrap items-end justify-between gap-3">
                  <div>
                    <p className="text-sm font-medium text-primary">课程复习包</p>
                    <h2 className="mt-2 text-2xl font-semibold tracking-tight">资料打包解锁</h2>
                  </div>
                  <p className="max-w-xl text-sm leading-6 text-muted-foreground">
                    paid 资料会经过服务端课程包授权校验；当前先展示包内容和价格，不接入真实支付。
                  </p>
                </div>

                <div className="mt-4 grid gap-3">
                  {packageDetails.map((detail) => (
                    <article key={detail.package.id} className="rounded-2xl border border-border bg-background p-4">
                      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                        <div className="min-w-0">
                          <h3 className="break-words text-lg font-semibold tracking-tight">{detail.package.title}</h3>
                          <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
                            {detail.package.description || "暂无课程包说明。"}
                          </p>
                        </div>
                        <div className="shrink-0 rounded-2xl border border-border bg-card px-4 py-3 text-left sm:text-right">
                          <p className="text-xs text-muted-foreground">价格</p>
                          <p className="mt-1 text-lg font-semibold">{formatPrice(detail.package.priceFen, detail.package.currency)}</p>
                          <Link
                            className="mt-2 inline-flex rounded-lg border border-border px-3 py-1.5 text-xs font-medium text-primary transition hover:bg-muted"
                            href={`/packages/${detail.package.id}`}
                          >
                            查看课程包
                          </Link>
                        </div>
                      </div>

                      <div className="mt-4 grid gap-3 md:grid-cols-[1fr_220px]">
                        <div>
                          <p className="text-sm font-medium">包含资料</p>
                          <div className="mt-2 flex flex-wrap gap-2">
                            {detail.materials.map((material) => (
                              <Link
                                key={material.id}
                                className="rounded-full border border-border bg-card px-3 py-1 text-xs text-muted-foreground transition hover:border-primary hover:text-primary"
                                href={`/materials/${material.id}`}
                              >
                                {material.title}
                              </Link>
                            ))}
                            {detail.materials.length === 0 ? <span className="text-sm text-muted-foreground">暂无已发布资料</span> : null}
                          </div>
                        </div>
                        <div className="rounded-2xl border border-border bg-card p-3 text-sm text-muted-foreground">
                          <p className="font-medium text-foreground">支付联调中</p>
                          <p className="mt-1 text-xs leading-5">后续支付成功后写入 package grant，由服务端解锁包内 paid 资料。</p>
                        </div>
                      </div>
                    </article>
                  ))}
                </div>

                {packageDetails.length === 0 ? (
                  <p className="mt-4 rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">
                    暂无已发布课程包，当前仍可查看已开放的课程资料。
                  </p>
                ) : null}
              </section>

              <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <p className="text-sm font-medium text-primary">课程材料</p>
                    <h2 className="mt-2 text-2xl font-semibold tracking-tight">PDF 资料</h2>
                  </div>
                  {materials.length > 0 ? (
                    <span className="inline-flex items-center self-start rounded-full bg-muted px-3 py-1 text-xs font-medium text-muted-foreground sm:self-auto">
                      <FileDown className="mr-1.5 size-3.5" aria-hidden="true" />
                      进入详情后下载
                    </span>
                  ) : null}
                </div>

                <div className="mt-4 grid gap-4">
                  {materials.map((material) => (
                    <Link
                      key={material.id}
                      className="group rounded-3xl border border-border bg-background p-5 shadow-sm transition hover:border-primary/60 hover:shadow-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
                      href={`/materials/${material.id}`}
                    >
                      <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge tone="muted">{labelMaterialType(material.type)}</Badge>
                            <Badge tone="muted">{labelAccessLevel(material.accessLevel)}</Badge>
                          </div>
                          <h3 className="mt-3 break-words text-lg font-semibold tracking-tight">{material.title}</h3>
                        </div>
                        <span className="inline-flex shrink-0 items-center text-sm font-medium text-primary">
                          查看资料
                          <ArrowRight className="ml-1.5 size-4 transition group-hover:translate-x-0.5" aria-hidden="true" />
                        </span>
                      </div>
                      <p className="mt-3 line-clamp-2 break-words text-sm leading-6 text-muted-foreground">
                        {material.description || material.previewContent || "这份资料正在补充说明，下载前可先查看详情页。"}
                      </p>
                    </Link>
                  ))}
                </div>

                {materials.length === 0 ? (
                  <p className="mt-4 rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">
                    这门课程暂时没有已发布资料，资料库会继续按课程补齐讲义、真题和解析。
                  </p>
                ) : null}
              </section>
            </div>

            <aside className="grid gap-4 self-start">
              <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
                <ShieldCheck className="size-5 text-primary" aria-hidden="true" />
                <h2 className="mt-3 text-lg font-semibold tracking-tight">资料保障</h2>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  PDF 下载由服务端校验，资料会持续整理维护，轻水印用于来源标识，不影响阅读。
                </p>
                {materialTypes.length > 0 ? (
                  <div className="mt-4 flex flex-wrap gap-2">
                    {materialTypes.map((item) => (
                      <span key={item.label} className="rounded-full bg-muted px-3 py-1 text-xs text-muted-foreground">
                        {item.label} {item.count}
                      </span>
                    ))}
                  </div>
                ) : null}
              </section>

              <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
                <UsersRound className="size-5 text-primary" aria-hidden="true" />
                <h2 className="mt-3 text-lg font-semibold tracking-tight">课程社区预留</h2>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">
                  后续课程社区会围绕资料补充建议、勘误和讨论展开；当前优先保证 PDF 资料入口清晰可用。
                </p>
              </section>

              {course.examScope ? (
                <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
                  <h2 className="text-lg font-semibold tracking-tight">考试范围</h2>
                  <p className="mt-2 break-words text-sm leading-6 text-muted-foreground">{course.examScope}</p>
                </section>
              ) : null}
            </aside>
          </section>
        </>
      ) : null}
    </SiteShell>
  );
}

function formatPrice(priceFen: number, currency: string) {
  const amount = (priceFen / 100).toFixed(priceFen % 100 === 0 ? 0 : 2);
  return `${currency?.toUpperCase() === "CNY" || !currency ? "¥" : ""}${amount}${currency?.toUpperCase() === "CNY" || !currency ? "" : ` ${currency}`}`;
}
