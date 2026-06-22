import Link from "next/link";
import { Course, Material, getApi } from "@/lib/api";

type PageProps = {
  params: Promise<{ id: string }>;
};

async function loadCourse(id: string) {
  try {
    const [courseResponse, materialsResponse] = await Promise.all([
      getApi<{ course: Course }>(`/courses/${id}`),
      getApi<{ materials: Material[] }>(`/courses/${id}/materials`),
    ]);
    return {
      course: courseResponse.data.course,
      materials: materialsResponse.data.materials,
      error: "",
    };
  } catch (error) {
    return {
      course: null as Course | null,
      materials: [] as Material[],
      error: error instanceof Error ? error.message : "API unavailable",
    };
  }
}

export default async function CourseDetailPage({ params }: PageProps) {
  const { id } = await params;
  const { course, materials, error } = await loadCourse(id);

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
