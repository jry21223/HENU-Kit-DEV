import { ArrowLeft, BookOpenText, History, Layers3 } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button-link";
import { ReportButton } from "@/components/report/report-button";
import { Course, WikiEntry, getApi } from "@/lib/api";

type PageProps = {
  params: Promise<{ id: string }>;
};

const copy = {
  back: "返回 Wiki",
  eyebrow: "Wiki 词条",
  version: "版本",
  updatedAt: "最后更新",
  course: "关联课程",
  noCourse: "通用词条",
  reviewBoundary: "该页面只展示已审核通过的公开内容；编辑提案和草稿仍由后台审核流处理。",
};

async function loadEntry(id: string) {
  try {
    const response = await getApi<{ entry: WikiEntry }>(`/wiki/entries/${id}`);
    const entry = response.data.entry;
    let course: Course | null = null;
    if (entry.courseId) {
      try {
        const courseResponse = await getApi<{ course: Course }>(`/courses/${entry.courseId}`);
        course = courseResponse.data.course;
      } catch {
        course = null;
      }
    }
    return {
      entry,
      course,
      error: "",
    };
  } catch (error) {
    return {
      entry: null as WikiEntry | null,
      course: null as Course | null,
      error: error instanceof Error ? error.message : "API unavailable",
    };
  }
}

export default async function WikiDetailPage({ params }: PageProps) {
  const { id } = await params;
  const { entry, course, error } = await loadEntry(id);

  return (
    <SiteShell>
      <nav className="flex items-center justify-between gap-3 text-sm">
        <ButtonLink href="/wiki" variant="secondary">
          <ArrowLeft className="mr-2 size-4" aria-hidden="true" />
          {copy.back}
        </ButtonLink>
      </nav>

      {error ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p> : null}

      {entry ? (
        <>
          <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
            <div className="flex min-w-0 flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
              <div className="min-w-0">
                <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <Badge tone="success">published</Badge>
                  <Badge tone="muted">{course?.name ?? copy.noCourse}</Badge>
                </div>
                <h1 className="mt-4 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{entry.title}</h1>
                <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.reviewBoundary}</p>
              </div>

              <dl className="grid shrink-0 gap-3 text-sm sm:grid-cols-3 lg:w-80 lg:grid-cols-1">
                <div className="rounded-2xl border border-border bg-background p-4">
                  <dt className="flex items-center text-xs text-muted-foreground">
                    <Layers3 className="mr-1.5 size-3.5" aria-hidden="true" />
                    {copy.version}
                  </dt>
                  <dd className="mt-1 font-medium">v{entry.version}</dd>
                </div>
                <div className="rounded-2xl border border-border bg-background p-4">
                  <dt className="flex items-center text-xs text-muted-foreground">
                    <History className="mr-1.5 size-3.5" aria-hidden="true" />
                    {copy.updatedAt}
                  </dt>
                  <dd className="mt-1 font-medium">{formatDate(entry.updatedAt)}</dd>
                </div>
                <div className="rounded-2xl border border-border bg-background p-4">
                  <dt className="flex items-center text-xs text-muted-foreground">
                    <BookOpenText className="mr-1.5 size-3.5" aria-hidden="true" />
                    {copy.course}
                  </dt>
                  <dd className="mt-1 break-words font-medium">{course?.name ?? copy.noCourse}</dd>
                </div>
                <div className="sm:col-span-3 lg:col-span-1">
                  <ReportButton targetId={entry.id} targetLabel={entry.title} targetType="wiki_entry" />
                </div>
              </dl>
            </div>
          </section>

          <article className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-7">
            <div className="whitespace-pre-wrap break-words text-sm leading-7 text-foreground sm:text-base sm:leading-8">
              {entry.content}
            </div>
          </article>
        </>
      ) : null}
    </SiteShell>
  );
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
