import Link from "next/link";
import { ArrowRight, BookOpenText, History, Layers3, PenLine } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { WikiCreatorApplicationPanel } from "@/components/wiki/wiki-creator-application-panel";
import { Course, WikiEntry, getApi } from "@/lib/api";

type PageProps = {
  searchParams: Promise<{ courseId?: string }>;
};

const copy = {
  eyebrow: "Wiki",
  title: "课程知识共创 Wiki",
  intro: "只展示已经审核通过的公开 Wiki 词条。草稿、待审核和退回内容不会出现在学生端。",
  allCourses: "全部课程",
  entries: "词条",
  version: "版本",
  updatedAt: "更新",
  createEntry: "\u65b0\u5efa\u8bcd\u6761",
  empty: "暂无已发布 Wiki 词条。",
};

async function loadWiki(courseId?: string) {
  try {
    const entriesPath = courseId ? `/wiki/entries?courseId=${encodeURIComponent(courseId)}` : "/wiki/entries";
    const [entriesResponse, coursesResponse] = await Promise.all([
      getApi<{ entries: WikiEntry[] }>(entriesPath),
      getApi<{ courses: Course[] }>("/courses"),
    ]);
    return {
      entries: entriesResponse.data.entries,
      courses: coursesResponse.data.courses,
      error: "",
    };
  } catch (error) {
    return {
      entries: [] as WikiEntry[],
      courses: [] as Course[],
      error: error instanceof Error ? error.message : "服务暂时不可用",
    };
  }
}

export default async function WikiPage({ searchParams }: PageProps) {
  const { courseId } = await searchParams;
  const { entries, courses, error } = await loadWiki(courseId);
  const courseMap = new Map(courses.map((course) => [course.id, course]));

  return (
    <SiteShell>
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0">
            <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
            <h1 className="mt-2 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{copy.title}</h1>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <div className="flex items-center gap-2 rounded-2xl border border-border bg-background px-4 py-3 text-sm text-muted-foreground">
              <BookOpenText className="size-4 text-primary" aria-hidden="true" />
              {entries.length} {copy.entries}
            </div>
            <Link
              className="inline-flex items-center rounded-2xl bg-primary px-4 py-3 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42]"
              href="/wiki/new"
            >
              <PenLine className="mr-2 size-4" aria-hidden="true" />
              {copy.createEntry}
            </Link>
          </div>
        </div>
      </section>

      {error ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p> : null}

      <WikiCreatorApplicationPanel />

      <nav className="flex gap-2 overflow-x-auto pb-1">
        <Link
          className={`shrink-0 rounded-full border px-4 py-2 text-sm ${
            !courseId ? "border-primary bg-primary text-primary-foreground" : "border-border bg-card text-muted-foreground hover:text-foreground"
          }`}
          href="/wiki"
        >
          {copy.allCourses}
        </Link>
        {courses.map((course) => (
          <Link
            className={`shrink-0 rounded-full border px-4 py-2 text-sm ${
              course.id === courseId ? "border-primary bg-primary text-primary-foreground" : "border-border bg-card text-muted-foreground hover:text-foreground"
            }`}
            href={`/wiki?courseId=${course.id}`}
            key={course.id}
          >
            {course.name}
          </Link>
        ))}
      </nav>

      <section className="grid gap-4">
        {entries.map((entry) => {
          const course = entry.courseId ? courseMap.get(entry.courseId) : undefined;
          return (
            <Link
              className="group rounded-3xl border border-border bg-card p-5 shadow-sm transition hover:border-primary/60 hover:shadow-md"
              href={`/wiki/${entry.id}`}
              key={entry.id}
            >
              <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge tone="success">已发布</Badge>
                    {course ? <Badge tone="muted">{course.name}</Badge> : null}
                  </div>
                  <h2 className="mt-3 break-words text-xl font-semibold tracking-tight">{entry.title}</h2>
                </div>
                <span className="inline-flex shrink-0 items-center text-sm font-medium text-primary">
                  查看
                  <ArrowRight className="ml-1.5 size-4 transition group-hover:translate-x-0.5" aria-hidden="true" />
                </span>
              </div>
              <p className="mt-3 line-clamp-2 break-words text-sm leading-6 text-muted-foreground">{preview(entry.content)}</p>
              <div className="mt-4 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                <span className="inline-flex items-center rounded-full bg-muted px-3 py-1">
                  <Layers3 className="mr-1.5 size-3.5" aria-hidden="true" />
                  {copy.version} {entry.version}
                </span>
                <span className="inline-flex items-center rounded-full bg-muted px-3 py-1">
                  <History className="mr-1.5 size-3.5" aria-hidden="true" />
                  {copy.updatedAt} {formatDate(entry.updatedAt)}
                </span>
              </div>
            </Link>
          );
        })}
      </section>

      {!error && entries.length === 0 ? (
        <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.empty}</p>
      ) : null}
    </SiteShell>
  );
}

function preview(value: string) {
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized || "这个词条暂时没有正文摘要。";
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("zh-CN");
}
