import { ArrowLeft, BookMarked, CalendarDays, MessageCircle } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { ReportButton } from "@/components/report/report-button";
import { Badge } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button-link";
import { BlogPost, getApi } from "@/lib/api";

type PageProps = {
  params: Promise<{ id: string }>;
};

const copy = {
  back: "返回 Blog",
  eyebrow: "Blog",
  updatedAt: "最后更新",
  comments: "评论",
  reviewBoundary: "该页面只展示审核通过的公开博客；草稿、待审核和退回内容仍然只在作者与审核后台可见。",
};

async function loadPost(id: string) {
  try {
    const response = await getApi<{ post: BlogPost }>(`/blog/posts/${id}`);
    return {
      post: response.data.post,
      error: "",
    };
  } catch (error) {
    return {
      post: null as BlogPost | null,
      error: error instanceof Error ? error.message : "API unavailable",
    };
  }
}

export default async function BlogDetailPage({ params }: PageProps) {
  const { id } = await params;
  const { post, error } = await loadPost(id);

  return (
    <SiteShell>
      <nav className="flex items-center justify-between gap-3 text-sm">
        <ButtonLink href="/blog" variant="secondary">
          <ArrowLeft className="mr-2 size-4" aria-hidden="true" />
          {copy.back}
        </ButtonLink>
      </nav>

      {error ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p> : null}

      {post ? (
        <>
          <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
            <div className="flex min-w-0 flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
              <div className="min-w-0">
                <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <Badge tone="success">published</Badge>
                  <Badge tone="muted">{post.slug}</Badge>
                </div>
                <h1 className="mt-4 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{post.title}</h1>
                <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.reviewBoundary}</p>
              </div>

              <dl className="grid shrink-0 gap-3 text-sm sm:grid-cols-2 lg:w-72 lg:grid-cols-1">
                <div className="rounded-2xl border border-border bg-background p-4">
                  <dt className="flex items-center text-xs text-muted-foreground">
                    <CalendarDays className="mr-1.5 size-3.5" aria-hidden="true" />
                    {copy.updatedAt}
                  </dt>
                  <dd className="mt-1 font-medium">{formatDate(post.updatedAt)}</dd>
                </div>
                <div className="rounded-2xl border border-border bg-background p-4">
                  <dt className="flex items-center text-xs text-muted-foreground">
                    <MessageCircle className="mr-1.5 size-3.5" aria-hidden="true" />
                    {copy.comments}
                  </dt>
                  <dd className="mt-1 font-medium">{post.commentCount}</dd>
                </div>
                <div className="rounded-2xl border border-border bg-background p-4">
                  <dt className="flex items-center text-xs text-muted-foreground">
                    <BookMarked className="mr-1.5 size-3.5" aria-hidden="true" />
                    Author
                  </dt>
                  <dd className="mt-1 break-words font-medium">{post.authorId}</dd>
                </div>
                <ReportButton targetId={post.id} targetLabel={post.title} targetType="blog_post" />
              </dl>
            </div>
          </section>

          <article className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-7">
            <div className="whitespace-pre-wrap break-words text-sm leading-7 text-foreground sm:text-base sm:leading-8">
              {post.content}
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
