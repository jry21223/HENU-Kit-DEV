import Link from "next/link";
import { ArrowRight, BookOpen, FileText, PackageOpen, Search, Tags } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { getApi, type SearchResponse, type SearchResult } from "@/lib/api";

type SearchPageProps = {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
};

const groupLabels: Array<{
  key: keyof SearchResponse["results"];
  title: string;
  empty: string;
}> = [
  { key: "courses", title: "课程", empty: "没有匹配课程" },
  { key: "materials", title: "资料", empty: "没有匹配资料" },
  { key: "packages", title: "课程包", empty: "没有匹配课程包" },
  { key: "wiki", title: "Wiki", empty: "没有匹配 Wiki" },
  { key: "blog", title: "Blog", empty: "没有匹配 Blog" },
  { key: "forum", title: "讨论", empty: "没有匹配讨论" },
];

async function loadSearch(query: string) {
  if (!query.trim()) {
    return { data: null as SearchResponse | null, error: "" };
  }
  try {
    const response = await getApi<SearchResponse>(`/search?q=${encodeURIComponent(query)}&limit=8`);
    return { data: response.data, error: "" };
  } catch (error) {
    return {
      data: null,
      error: error instanceof Error ? error.message : "搜索服务暂时不可用",
    };
  }
}

export default async function SearchPage({ searchParams }: SearchPageProps) {
  const resolvedSearchParams = (await searchParams) ?? {};
  const rawQuery = resolvedSearchParams.q;
  const query = Array.isArray(rawQuery) ? rawQuery[0] ?? "" : rawQuery ?? "";
  const trimmedQuery = query.trim();
  const { data, error } = await loadSearch(trimmedQuery);

  return (
    <SiteShell>
      <section className="rounded-2xl border border-border bg-card p-4 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <Badge tone="success">全站搜索</Badge>
            <h1 className="mt-3 text-2xl font-semibold tracking-tight sm:text-3xl">搜索课程、资料和讨论内容</h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
              目前搜索公开发布的课程、资料、课程包、Wiki、Blog 和论坛帖子；待审核、草稿、私密内容不会出现在结果里。
            </p>
          </div>
          <form className="flex w-full min-w-0 rounded-xl border border-border bg-background p-1.5 lg:max-w-md" action="/search">
            <label className="sr-only" htmlFor="q">
              搜索关键词
            </label>
            <div className="flex min-w-0 flex-1 items-center gap-2 px-2">
              <Search className="size-4 flex-none text-muted-foreground" aria-hidden="true" />
              <input
                id="q"
                name="q"
                className="min-w-0 flex-1 bg-transparent py-2 text-sm outline-none placeholder:text-muted-foreground"
                placeholder="离散数学、模拟卷、图论..."
                defaultValue={trimmedQuery}
              />
            </div>
            <button className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground" type="submit">
              搜索
            </button>
          </form>
        </div>
      </section>

      {!trimmedQuery ? (
        <section className="rounded-2xl border border-border bg-card p-5 text-sm leading-6 text-muted-foreground">
          输入关键词后开始搜索。建议使用课程名、资料类型或知识点，例如“离散数学”“模拟卷”“图论”。
        </section>
      ) : null}

      {error ? (
        <section className="rounded-2xl border border-border bg-card p-5 text-sm text-muted-foreground">{error}</section>
      ) : null}

      {data ? (
        <section className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">
              「<span className="font-medium text-foreground">{data.query}</span>」共找到 {data.total} 条结果
            </p>
            <Link className="inline-flex items-center text-sm font-medium text-primary" href="/courses">
              浏览课程库
              <ArrowRight className="ml-1.5 size-4" aria-hidden="true" />
            </Link>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            {groupLabels.map((group) => (
              <ResultGroup
                key={group.key}
                title={group.title}
                empty={group.empty}
                results={data.results[group.key] ?? []}
              />
            ))}
          </div>
        </section>
      ) : null}
    </SiteShell>
  );
}

function ResultGroup({ title, empty, results }: { title: string; empty: string; results: SearchResult[] }) {
  return (
    <section className="rounded-2xl border border-border bg-card p-4 shadow-sm">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-base font-semibold tracking-tight">{title}</h2>
        <span className="rounded-full bg-muted px-2.5 py-1 text-xs text-muted-foreground">{results.length}</span>
      </div>
      <div className="mt-4 space-y-3">
        {results.length === 0 ? (
          <p className="rounded-xl bg-muted/60 p-3 text-sm text-muted-foreground">{empty}</p>
        ) : (
          results.map((result) => <ResultCard key={`${result.type}-${result.id}`} result={result} />)
        )}
      </div>
    </section>
  );
}

function ResultCard({ result }: { result: SearchResult }) {
  const Icon = resultIcon(result.type);
  return (
    <Link
      className="group block rounded-xl border border-border bg-background p-3 transition hover:border-primary/60 hover:shadow-sm"
      href={result.url}
    >
      <div className="flex min-w-0 items-start gap-3">
        <span className="grid size-9 flex-none place-items-center rounded-lg bg-muted text-muted-foreground">
          <Icon className="size-4" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-start justify-between gap-3">
            <h3 className="min-w-0 break-words text-sm font-medium text-foreground">{result.title}</h3>
            <ArrowRight className="mt-0.5 size-4 flex-none text-muted-foreground transition group-hover:translate-x-0.5 group-hover:text-primary" />
          </div>
          {result.description ? (
            <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">{result.description}</p>
          ) : null}
          {result.meta ? <p className="mt-2 text-xs text-muted-foreground">{result.meta}</p> : null}
        </div>
      </div>
    </Link>
  );
}

function resultIcon(type: SearchResult["type"]) {
  switch (type) {
    case "course":
      return BookOpen;
    case "package":
      return PackageOpen;
    case "material":
      return FileText;
    default:
      return Tags;
  }
}
