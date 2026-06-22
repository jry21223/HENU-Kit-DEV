import Link from "next/link";
import { Course, getApi } from "@/lib/api";

async function loadCourses() {
  try {
    const response = await getApi<{ courses: Course[] }>("/courses");
    return { courses: response.data.courses, error: "" };
  } catch (error) {
    return { courses: [] as Course[], error: error instanceof Error ? error.message : "API unavailable" };
  }
}

export default async function CoursesPage() {
  const { courses, error } = await loadCourses();

  return (
    <main className="min-h-screen px-5 py-6 sm:px-8">
      <section className="mx-auto max-w-5xl">
        <nav className="flex items-center justify-between text-sm">
          <Link className="font-semibold text-sage" href="/">
            Final Review V2
          </Link>
          <Link href="/login">登录</Link>
        </nav>

        <header className="mt-8">
          <p className="text-sm font-medium text-sage">课程库</p>
          <h1 className="mt-2 text-3xl font-semibold">按课程进入资料和刷题</h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-600">
            当前列表来自 Go API。筛选器、学校/专业联动和分页会在下一轮前端功能里接入。
          </p>
        </header>

        {error ? <p className="mt-6 rounded-md border border-line bg-white p-4 text-sm text-slate-600">{error}</p> : null}

        <div className="mt-6 grid gap-4 md:grid-cols-2">
          {courses.map((course) => (
            <Link key={course.id} className="rounded-lg border border-line bg-white p-5 shadow-sm transition hover:border-sage" href={`/courses/${course.id}`}>
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-lg font-semibold">{course.name}</h2>
                <span className="shrink-0 rounded-md bg-paper px-2 py-1 text-xs text-slate-600">{course.grade}</span>
              </div>
              <p className="mt-3 line-clamp-2 text-sm leading-6 text-slate-600">{course.description || "暂无课程简介"}</p>
              <p className="mt-4 text-xs text-slate-500">slug: {course.slug}</p>
            </Link>
          ))}
        </div>

        {!error && courses.length === 0 ? <p className="mt-6 rounded-md border border-line bg-white p-4 text-sm text-slate-600">暂无课程。</p> : null}
      </section>
    </main>
  );
}
