import Link from "next/link";
import { ArrowRight, BookMarked, CalendarDays, MessageCircle } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { BlogPost, getApi } from "@/lib/api";

const copy = {
  eyebrow: "Blog",
  title: "复习经验与课程笔记",
  intro: "只展示审核通过的公开博客。待审核、退回和草稿内容不会出现在学生端。",
  posts: "文章",
  empty: "暂无已发布博客。",
  updatedAt: "更新",
  comments: "评论",
};

async function loadBlog() {
  try {
    const response = await getApi<{ posts: BlogPost[] }>("/blog/posts");
    return {
      posts: response.data.posts,
      error: "",
    };
  } catch (error) {
    return {
      posts: [] as BlogPost[],
      error: error instanceof Error ? error.message : "API unavailable",
    };
  }
}

export default async function BlogPage() {
  const { posts, error } = await loadBlog();

  return (
    <SiteShell>
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0">
            <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
            <h1 className="mt-2 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{copy.title}</h1>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
          </div>
          <div className="flex shrink-0 items-center gap-2 rounded-2xl border border-border bg-background px-4 py-3 text-sm text-muted-foreground">
            <BookMarked className="size-4 text-primary" aria-hidden="true" />
            {posts.length} {copy.posts}
          </div>
        </div>
      </section>

      {error ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p> : null}

      <section className="grid gap-4">
        {posts.map((post) => (
          <Link
            className="group rounded-3xl border border-border bg-card p-5 shadow-sm transition hover:border-primary/60 hover:shadow-md"
            href={`/blog/${post.id}`}
            key={post.id}
          >
            <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone="success">published</Badge>
                  <Badge tone="muted">{post.slug}</Badge>
                </div>
                <h2 className="mt-3 break-words text-xl font-semibold tracking-tight">{post.title}</h2>
              </div>
              <span className="inline-flex shrink-0 items-center text-sm font-medium text-primary">
                查看
                <ArrowRight className="ml-1.5 size-4 transition group-hover:translate-x-0.5" aria-hidden="true" />
              </span>
            </div>
            <p className="mt-3 line-clamp-2 break-words text-sm leading-6 text-muted-foreground">{preview(post.content)}</p>
            <div className="mt-4 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span className="inline-flex items-center rounded-full bg-muted px-3 py-1">
                <CalendarDays className="mr-1.5 size-3.5" aria-hidden="true" />
                {copy.updatedAt} {formatDate(post.updatedAt)}
              </span>
              <span className="inline-flex items-center rounded-full bg-muted px-3 py-1">
                <MessageCircle className="mr-1.5 size-3.5" aria-hidden="true" />
                {post.commentCount} {copy.comments}
              </span>
            </div>
          </Link>
        ))}
      </section>

      {!error && posts.length === 0 ? (
        <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.empty}</p>
      ) : null}
    </SiteShell>
  );
}

function preview(value: string) {
  const normalized = value.replace(/\s+/g, " ").trim();
  return normalized || "这篇博客暂时没有正文摘要。";
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("zh-CN");
}
