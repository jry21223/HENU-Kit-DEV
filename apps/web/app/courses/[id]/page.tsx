import Link from "next/link";
import { Course, CoursePackage, CoursePackageDetail, Material, getApi } from "@/lib/api";

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

  return (
    <main className="min-h-screen px-5 py-6 sm:px-8">
      <section className="mx-auto max-w-5xl">
        <nav className="flex items-center justify-between text-sm">
          <Link className="font-semibold text-sage" href="/courses">
            返回课程库
          </Link>
          <Link href={`/courses/${id}/quiz`}>进入刷题</Link>
        </nav>

        {error ? <p className="mt-6 rounded-md border border-line bg-white p-4 text-sm text-slate-600">{error}</p> : null}

        {course ? (
          <>
            <header className="mt-8">
              <p className="text-sm font-medium text-sage">课程详情</p>
              <h1 className="mt-2 text-3xl font-semibold">{course.name}</h1>
              <p className="mt-3 max-w-3xl text-sm leading-6 text-slate-600">{course.description || "暂无课程简介"}</p>
              <dl className="mt-5 grid gap-3 text-sm sm:grid-cols-3">
                <div className="rounded-md border border-line bg-white p-3">
                  <dt className="text-slate-500">适用年级</dt>
                  <dd className="mt-1 font-medium">{course.grade}</dd>
                </div>
                <div className="rounded-md border border-line bg-white p-3">
                  <dt className="text-slate-500">状态</dt>
                  <dd className="mt-1 font-medium">{course.status}</dd>
                </div>
                <div className="rounded-md border border-line bg-white p-3">
                  <dt className="text-slate-500">资料数量</dt>
                  <dd className="mt-1 font-medium">{materials.length}</dd>
                </div>
              </dl>
              {course.examScope ? (
                <div className="mt-4 rounded-md border border-line bg-white p-4">
                  <h2 className="text-sm font-semibold">考试范围</h2>
                  <p className="mt-2 text-sm leading-6 text-slate-600">{course.examScope}</p>
                </div>
              ) : null}
            </header>

            <section className="mt-8">
              <div className="flex flex-wrap items-end justify-between gap-3">
                <div>
                  <p className="text-sm font-medium text-sage">课程复习包</p>
                  <h2 className="mt-1 text-xl font-semibold">资料打包解锁</h2>
                </div>
                <p className="max-w-xl text-sm leading-6 text-slate-600">
                  paid 资料必须经过 Go API 的课程包授权校验。当前支付处于微信 Native 联调准备阶段，前台先展示包内容和价格。
                </p>
              </div>
              <div className="mt-4 grid gap-4">
                {packageDetails.map((detail) => (
                  <article key={detail.package.id} className="rounded-lg border border-line bg-white p-5 shadow-sm">
                    <div className="flex flex-wrap items-start justify-between gap-4">
                      <div>
                        <h3 className="text-lg font-semibold">{detail.package.title}</h3>
                        <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600">{detail.package.description || "暂无课程包说明"}</p>
                      </div>
                      <div className="shrink-0 rounded-md bg-paper px-3 py-2 text-right">
                        <p className="text-xs text-slate-500">价格</p>
                        <p className="text-lg font-semibold">{formatPrice(detail.package.priceFen, detail.package.currency)}</p>
                      </div>
                    </div>
                    <div className="mt-4 grid gap-3 md:grid-cols-[1fr_auto]">
                      <div>
                        <p className="text-sm font-medium">包含资料</p>
                        <div className="mt-2 flex flex-wrap gap-2">
                          {detail.materials.map((material) => (
                            <Link
                              key={material.id}
                              className="rounded-md border border-line px-2 py-1 text-xs text-slate-600 transition hover:border-sage"
                              href={`/materials/${material.id}`}
                            >
                              {material.title}
                            </Link>
                          ))}
                          {detail.materials.length === 0 ? <span className="text-sm text-slate-500">暂无已发布资料</span> : null}
                        </div>
                      </div>
                      <div className="rounded-md border border-line px-3 py-2 text-sm text-slate-600">
                        <p className="font-medium text-slate-800">支付联调中</p>
                        <p className="mt-1 text-xs leading-5">支付成功后将写入 package grant，服务端自动解锁包内 paid 资料。</p>
                      </div>
                    </div>
                  </article>
                ))}
              </div>
              {packageDetails.length === 0 ? <p className="mt-4 rounded-md border border-line bg-white p-4 text-sm text-slate-600">暂无已发布课程包。</p> : null}
            </section>

            <section className="mt-8">
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-xl font-semibold">课程资料</h2>
                <Link className="rounded-md bg-sage px-3 py-2 text-sm font-medium text-white" href={`/courses/${course.id}/quiz`}>
                  开始刷题
                </Link>
              </div>
              <div className="mt-4 grid gap-4 md:grid-cols-2">
                {materials.map((material) => (
                  <Link key={material.id} className="rounded-lg border border-line bg-white p-5 shadow-sm transition hover:border-sage" href={`/materials/${material.id}`}>
                    <div className="flex items-center justify-between gap-3">
                      <h3 className="font-semibold">{material.title}</h3>
                      <span className="shrink-0 rounded-md bg-paper px-2 py-1 text-xs text-slate-600">{material.accessLevel}</span>
                    </div>
                    <p className="mt-3 line-clamp-2 text-sm leading-6 text-slate-600">{material.description || material.previewContent || "暂无简介"}</p>
                    <p className="mt-4 text-xs text-slate-500">{material.type}</p>
                  </Link>
                ))}
              </div>
              {materials.length === 0 ? <p className="mt-4 rounded-md border border-line bg-white p-4 text-sm text-slate-600">暂无已发布资料。</p> : null}
            </section>
          </>
        ) : null}
      </section>
    </main>
  );
}

function formatPrice(priceFen: number, currency: string) {
  const amount = (priceFen / 100).toFixed(priceFen % 100 === 0 ? 0 : 2);
  return `${amount} ${currency || "CNY"}`;
}
