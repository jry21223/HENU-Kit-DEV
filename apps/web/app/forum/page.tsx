import Link from "next/link";
import { ArrowRight, MessageSquareText, Trophy } from "lucide-react";
import { ForumPostComposer } from "@/components/forum/forum-post-composer";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { ForumBoard, ForumPost, getApi } from "@/lib/api";

type PageProps = {
  searchParams: Promise<{ boardId?: string }>;
};

const copy = {
  eyebrow: "\u8bfe\u7a0b\u8ba8\u8bba",
  title: "\u8bfe\u7a0b\u4e92\u52a9\u4e0e\u7b54\u7591",
  intro:
    "\u53ea\u5c55\u793a\u5df2\u5ba1\u6838\u901a\u8fc7\u7684\u516c\u5f00\u5e16\u5b50\u3002\u56de\u590d\u9700\u767b\u5f55\u540e\u63d0\u4ea4\uff0c\u901a\u8fc7\u5ba1\u6838\u540e\u624d\u4f1a\u516c\u5f00\u5c55\u793a\u3002",
  allBoards: "\u5168\u90e8\u7248\u5757",
  posts: "\u5e16\u5b50",
  replies: "\u56de\u590d",
  reward: "\u60ac\u8d4f",
  question: "\u95ee\u7b54",
  normal: "\u8ba8\u8bba",
  empty: "\u6682\u65e0\u5df2\u53d1\u5e03\u8ba8\u8bba\u3002",
};

async function loadForum(boardId?: string) {
  try {
    const postsPath = boardId ? `/forum/posts?boardId=${encodeURIComponent(boardId)}` : "/forum/posts";
    const [boardsResponse, postsResponse] = await Promise.all([
      getApi<{ boards: ForumBoard[] }>("/forum/boards"),
      getApi<{ posts: ForumPost[] }>(postsPath),
    ]);
    return {
      boards: boardsResponse.data.boards,
      posts: postsResponse.data.posts,
      error: "",
    };
  } catch (error) {
    return {
      boards: [] as ForumBoard[],
      posts: [] as ForumPost[],
      error: error instanceof Error ? error.message : "服务暂时不可用",
    };
  }
}

export default async function ForumPage({ searchParams }: PageProps) {
  const { boardId } = await searchParams;
  const { boards, posts, error } = await loadForum(boardId);

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
            <MessageSquareText className="size-4 text-primary" aria-hidden="true" />
            {posts.length} {copy.posts}
          </div>
        </div>
      </section>

      {error ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p> : null}

      <ForumPostComposer boards={boards} selectedBoardId={boardId} />

      <nav className="flex gap-2 overflow-x-auto pb-1">
        <Link
          className={`shrink-0 rounded-full border px-4 py-2 text-sm ${
            !boardId ? "border-primary bg-primary text-primary-foreground" : "border-border bg-card text-muted-foreground hover:text-foreground"
          }`}
          href="/forum"
        >
          {copy.allBoards}
        </Link>
        {boards.map((board) => (
          <Link
            className={`shrink-0 rounded-full border px-4 py-2 text-sm ${
              board.id === boardId ? "border-primary bg-primary text-primary-foreground" : "border-border bg-card text-muted-foreground hover:text-foreground"
            }`}
            href={`/forum?boardId=${board.id}`}
            key={board.id}
          >
            {board.name}
          </Link>
        ))}
      </nav>

      <section className="grid gap-4">
        {posts.map((post) => (
          <Link
            className="group rounded-3xl border border-border bg-card p-5 shadow-sm transition hover:border-primary/60 hover:shadow-md"
            href={`/forum/${post.id}`}
            key={post.id}
          >
            <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone={post.type === "reward" ? "success" : "muted"}>{labelPostType(post)}</Badge>
                  {post.rewardStatus ? <Badge tone="muted">{rewardStatusLabel(post.rewardStatus)}</Badge> : null}
                </div>
                <h2 className="mt-3 break-words text-xl font-semibold tracking-tight">{post.title}</h2>
              </div>
              <span className="inline-flex shrink-0 items-center text-sm font-medium text-primary">
                \u67e5\u770b
                <ArrowRight className="ml-1.5 size-4 transition group-hover:translate-x-0.5" aria-hidden="true" />
              </span>
            </div>
            <p className="mt-3 line-clamp-2 break-words text-sm leading-6 text-muted-foreground">{post.content}</p>
            <div className="mt-4 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span className="rounded-full bg-muted px-3 py-1">
                {post.commentCount} {copy.replies}
              </span>
              <span className="rounded-full bg-muted px-3 py-1">{formatDate(post.updatedAt)}</span>
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

function rewardStatusLabel(status?: string) {
  const labels: Record<string, string> = {
    open: "悬赏中",
    completed: "已解决",
    expired: "已过期",
  };
  return labels[status ?? ""] ?? status ?? "";
}
