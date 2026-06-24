import { ArrowLeft } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { ButtonLink } from "@/components/ui/button-link";
import { WikiEntrySubmissionForm } from "@/components/wiki/wiki-entry-submission-form";
import { Course, getApi } from "@/lib/api";

const copy = {
  back: "\u8fd4\u56de Wiki",
  eyebrow: "Wiki Contribution",
  title: "\u521b\u4f5c\u8005\u8bcd\u6761\u6295\u7a3f",
  intro:
    "\u8fd9\u91cc\u53ea\u63d0\u4ea4\u5f85\u5ba1\u8349\u7a3f\uff1b\u6b63\u5f0f\u516c\u5f00\u53d1\u5e03\u4ecd\u7531 reviewer/admin \u5ba1\u6838\u3002\u8bf7\u4fdd\u6301\u6765\u6e90\u6e05\u6670\u3001\u7ed3\u6784\u660e\u786e\uff0c\u4e0d\u8981\u586b\u5199\u672a\u7ecf\u786e\u8ba4\u7684\u8003\u8bd5\u4fe1\u606f\u3002",
  courseLoadError: "\u8bfe\u7a0b\u5217\u8868\u6682\u65f6\u4e0d\u53ef\u7528\uff0c\u4ecd\u53ef\u63d0\u4ea4\u901a\u7528 Wiki \u8bcd\u6761\u3002",
};

async function loadCourses() {
  try {
    const response = await getApi<{ courses: Course[] }>("/courses");
    return { courses: response.data.courses, error: "" };
  } catch (error) {
    return {
      courses: [] as Course[],
      error: error instanceof Error ? error.message : copy.courseLoadError,
    };
  }
}

export default async function NewWikiEntryPage() {
  const { courses, error } = await loadCourses();

  return (
    <SiteShell>
      <nav className="flex items-center justify-between gap-3 text-sm">
        <ButtonLink href="/wiki" variant="secondary">
          <ArrowLeft className="mr-2 size-4" aria-hidden="true" />
          {copy.back}
        </ButtonLink>
      </nav>

      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
        <h1 className="mt-2 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{copy.title}</h1>
        <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
      </section>

      {error ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p> : null}

      <WikiEntrySubmissionForm courses={courses} />
    </SiteShell>
  );
}
