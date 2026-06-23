import Link from "next/link";
import { ArrowLeft, MessageSquareText, Trophy } from "lucide-react";
import { ForumThread } from "@/components/forum/forum-thread";
import { SiteShell } from "@/components/layout/site-shell";
import { ReportButton } from "@/components/report/report-button";
import { Badge } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button-link";
import { ForumPost, ForumReply, getApi } from "@/lib/api";

type PageProps = {
  params: Promise<{ id: string }>;
};

const copy = {
  back: "\u8fd4\u56de\u8ba8\u8bba",
  eyebrow: "\u8ba8\u8bba\u8be6\u60c5",
  reward: "\u60ac\u8d4f",
  question: "\u95ee\u7b54",
  normal: "\u8ba8\u8bba",
  replies: "\u56de\u590d",
  updatedAt: "\u6700\u540e\u66f4\u65b0",
};

async function loadThread(id: string) {
  try {
    const response = await getApi<{ post: ForumPost; replies: ForumReply[] }>(`/forum/posts/${id}`);
    return {
      post: response.data.post,
      replies: response.data.replies,
      error: "",
    };
  } catch (error) {
    return {
      post: null as ForumPost | null,
      replies: [] as ForumReply[],
      error: error instanceof Error ? error.message : "API unavailable",
    };
  }
}

export default async function ForumDetailPage({ params }: PageProps) {
  const { id } = await params;
  const { post, replies, error } = await loadThread(id);

  return (
    <SiteShell>
      <nav className="flex items-center justify-between gap-3 text-sm">
        <ButtonLink href="/forum" variant="secondary">
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
                  <Badge tone={post.type === "reward" ? "success" : "muted"}>{labelPostType(post)}</Badge>
                  {post.rewardStatus ? <Badge tone="muted">{post.rewardStatus}</Badge> : null}
                </div>
                <h1 className="mt-4 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{post.title}</h1>
                <p className="mt-4 whitespace-pre-wrap break-words text-sm leading-7 text-muted-foreground">{post.content}</p>
              </div>

              <div className="grid shrink-0 gap-3 text-sm sm:grid-cols-2 lg:w-72 lg:grid-cols-1">
                <dl className="contents">
                  <div className="rounded-2xl border border-border bg-background p-4">
                    <dt className="text-xs text-muted-foreground">{copy.replies}</dt>
                    <dd className="mt-1 flex items-center font-medium">
                      <MessageSquareText className="mr-1.5 size-4 text-primary" aria-hidden="true" />
                      {post.commentCount}
                    </dd>
                  </div>
                  <div className="rounded-2xl border border-border bg-background p-4">
                    <dt className="text-xs text-muted-foreground">{copy.updatedAt}</dt>
                    <dd className="mt-1 font-medium">{formatDate(post.updatedAt)}</dd>
                  </div>
                </dl>
                <ReportButton targetId={post.id} targetLabel={post.title} targetType="forum_post" />
              </div>
            </div>
          </section>

          <ForumThread initialPost={post} initialReplies={replies} />
        </>
      ) : null}
    </SiteShell>
  );
}

function labelPostType(post: ForumPost) {
  if (post.type === "reward") {
    return (
      <span className="inline-flex items-center gap-1">
        <Trophy className="size-3" aria-hidden="true" />
        {copy.reward} {post.rewardPoints}
      </span>
    );
  }
  if (post.type === "question") return copy.question;
  return copy.normal;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("zh-CN");
}
