import Link from "next/link";
import { ArrowRight, FileDown, ShieldCheck } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { Course, getApi } from "@/lib/api";

async function loadCourses() {
  try {
    const response = await getApi<{ courses: Course[] }>("/courses");
    return { courses: response.data.courses, error: "" };
  } catch (error) {
    return { courses: [] as Course[], error: error instanceof Error ? error.message : "服务暂时不可用" };
  }
}

export default async function CoursesPage() {
  const { courses, error } = await loadCourses();

  return (
    <SiteShell>
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <p className="text-sm font-medium text-primary">课程资料库</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">按课程查找软件学院 PDF 资料</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
          当前资料按课程组织，重点展示讲义、真题、解析和考前速背。后续课程社区会围绕资料补充建议和讨论。
        </p>
      </section>

      {error ? (
        <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p>
      ) : null}

      <section className="grid gap-4 md:grid-cols-2">
        {courses.map((course) => (
          <Link
            key={course.id}
            className="group rounded-3xl border border-border bg-card p-5 shadow-sm transition hover:border-primary/60 hover:shadow-md"
            href={`/courses/${course.id}`}
          >
            <div className="flex items-start justify-between gap-3">
              <div>
                <Badge tone="success">资料保障</Badge>
                <h2 className="mt-4 text-lg font-semibold tracking-tight">{course.name}</h2>
              </div>
              <span className="shrink-0 rounded-full border border-border bg-muted px-3 py-1 text-xs text-muted-foreground">
                {course.grade || "年级未设置"}
              </span>
            </div>
            <p className="mt-3 line-clamp-2 text-sm leading-6 text-muted-foreground">
              {course.description || "这门课程的 PDF 资料会按讲义、真题、解析等类型持续整理。"}
            </p>
            <div className="mt-5 flex flex-wrap items-center gap-2 text-xs font-medium text-muted-foreground">
              <span className="inline-flex items-center rounded-full bg-muted px-3 py-1">
                <FileDown className="mr-1.5 size-3.5" aria-hidden="true" />
                PDF 资料入口
              </span>
              <span className="inline-flex items-center rounded-full bg-muted px-3 py-1">
                <ShieldCheck className="mr-1.5 size-3.5" aria-hidden="true" />
                轻水印下载
              </span>
            </div>
            <span className="mt-5 inline-flex items-center text-sm font-medium text-primary">
              查看资料
              <ArrowRight className="ml-1.5 size-4 transition group-hover:translate-x-0.5" aria-hidden="true" />
            </span>
          </Link>
        ))}
      </section>

      {!error && courses.length === 0 ? (
        <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">暂无课程。</p>
      ) : null}
    </SiteShell>
  );
}
