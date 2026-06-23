"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ArrowRight, MessageSquareText, PenLine } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { apiBaseUrl, type MyForumPost, type MyForumReply } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

const copy = {
  back: "\u8fd4\u56de\u4e2a\u4eba\u4e2d\u5fc3",
  forum: "\u8bfe\u7a0b\u8ba8\u8bba",
  eyebrow: "\u6211\u7684\u8ba8\u8bba",
  title: "\u5e16\u5b50\u4e0e\u56de\u590d\u5ba1\u6838\u8ffd\u8e2a",
  intro:
    "\u8fd9\u91cc\u53ea\u5c55\u793a\u5f53\u524d\u767b\u5f55\u8d26\u53f7\u63d0\u4ea4\u7684\u8ba8\u8bba\u5185\u5bb9\u3002\u5f85\u5ba1\u548c\u9a73\u56de\u5185\u5bb9\u4ec5\u672c\u4eba\u53ef\u89c1\uff0c\u516c\u5f00\u8ba8\u8bba\u9875\u4ecd\u53ea\u5c55\u793a\u5df2\u53d1\u5e03\u5185\u5bb9\u3002",
  posts: "\u6211\u7684\u5e16\u5b50",
  replies: "\u6211\u7684\u56de\u590d",
  loading: "\u6b63\u5728\u52a0\u8f7d\u8ba8\u8bba\u8bb0\u5f55...",
  login: "\u53bb\u767b\u5f55",
  emptyPosts: "\u6682\u65e0\u81ea\u5df1\u53d1\u8d77\u7684\u5e16\u5b50\u3002",
  emptyReplies: "\u6682\u65e0\u81ea\u5df1\u63d0\u4ea4\u7684\u56de\u590d\u3002",
  reviewReason: "\u5ba1\u6838\u8bf4\u660e",
  publishedLink: "\u67e5\u770b\u516c\u5f00\u9875",
  notPublic: "\u672a\u516c\u5f00",
  fallbackError: "\u8ba8\u8bba\u8bb0\u5f55\u6682\u65f6\u4e0d\u53ef\u7528",
};

const statusLabels: Record<string, string> = {
  draft: "\u8349\u7a3f",
  pending: "\u5f85\u5ba1",
  approved: "\u5df2\u901a\u8fc7",
  rejected: "\u5df2\u9a73\u56de",
  needs_changes: "\u9700\u4fee\u6539",
  published: "\u5df2\u53d1\u5e03",
  archived: "\u5df2\u5f52\u6863",
};

export default function MyForumPage() {
  const [posts, setPosts] = useState<MyForumPost[]>([]);
  const [replies, setReplies] = useState<MyForumReply[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    async function loadForumSubmissions() {
      setLoading(true);
      setError("");
      try {
        const [postsResponse, repliesResponse] = await Promise.all([
          request<{ posts: MyForumPost[] }>("/me/forum-posts"),
          request<{ replies: MyForumReply[] }>("/me/forum-replies"),
        ]);
        setPosts(postsResponse.data?.posts ?? []);
        setReplies(repliesResponse.data?.replies ?? []);
      } catch (err) {
        setError(err instanceof Error ? err.message : copy.fallbackError);
      } finally {
        setLoading(false);
      }
    }

    void loadForumSubmissions();
  }, []);

  return (
    <SiteShell>
      <nav className="flex flex-wrap items-center justify-between gap-3 text-sm">
        <Link className="font-semibold text-primary" href="/me">
          {copy.back}
        </Link>
        <Link className="text-muted-foreground hover:text-foreground" href="/forum">
          {copy.forum}
        </Link>
      </nav>

      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
        <h1 className="mt-2 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{copy.title}</h1>
        <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
      </section>

      {loading ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.loading}</p> : null}
      {error ? (
        <div className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">
          <p>{error}</p>
          <Link className="mt-3 inline-flex rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
            {copy.login}
          </Link>
        </div>
      ) : null}

      {!loading && !error ? (
        <div className="grid gap-5 lg:grid-cols-2">
          <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
            <div className="flex items-center gap-3">
              <span className="grid size-10 place-items-center rounded-2xl bg-primary text-primary-foreground">
                <PenLine className="size-5" aria-hidden="true" />
              </span>
              <div>
                <h2 className="text-xl font-semibold">{copy.posts}</h2>
                <p className="text-sm text-muted-foreground">{posts.length}</p>
              </div>
            </div>
            <div className="mt-5 grid gap-3">
              {posts.map((post) => (
                <article className="rounded-2xl border border-border bg-background p-4" key={post.id}>
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge status={post.status} />
                    <Badge tone="muted">{labelPostType(post.type)}</Badge>
                    {post.rewardStatus ? <Badge tone="muted">{post.rewardStatus}</Badge> : null}
                  </div>
                  <h3 className="mt-3 break-words font-semibold">{post.title}</h3>
                  <p className="mt-2 line-clamp-3 break-words text-sm leading-6 text-muted-foreground">{post.content}</p>
                  <ReviewReason value={post.reviewReason} />
                  <SubmissionFooter href={post.status === "published" ? `/forum/${post.id}` : ""} updatedAt={post.updatedAt} />
                </article>
              ))}
              {posts.length === 0 ? <p className="rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">{copy.emptyPosts}</p> : null}
            </div>
          </section>

          <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
            <div className="flex items-center gap-3">
              <span className="grid size-10 place-items-center rounded-2xl bg-primary text-primary-foreground">
                <MessageSquareText className="size-5" aria-hidden="true" />
              </span>
              <div>
                <h2 className="text-xl font-semibold">{copy.replies}</h2>
                <p className="text-sm text-muted-foreground">{replies.length}</p>
              </div>
            </div>
            <div className="mt-5 grid gap-3">
              {replies.map((reply) => (
                <article className="rounded-2xl border border-border bg-background p-4" key={reply.id}>
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge status={reply.status} />
                    {reply.isBest ? <Badge tone="success">\u6700\u4f73\u7b54\u6848</Badge> : null}
                  </div>
                  <h3 className="mt-3 break-words text-sm font-semibold text-muted-foreground">{reply.postTitle || reply.postId}</h3>
                  <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-foreground">{reply.content}</p>
                  <ReviewReason value={reply.reviewReason} />
                  <SubmissionFooter href={reply.postStatus === "published" ? `/forum/${reply.postId}` : ""} updatedAt={reply.updatedAt} />
                </article>
              ))}
              {replies.length === 0 ? <p className="rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">{copy.emptyReplies}</p> : null}
            </div>
          </section>
        </div>
      ) : null}
    </SiteShell>
  );
}

function StatusBadge({ status }: { status: string }) {
  const tone = status === "published" ? "success" : status === "rejected" ? "default" : "muted";
  return <Badge tone={tone}>{statusLabels[status] ?? status}</Badge>;
}

function ReviewReason({ value }: { value?: string }) {
  if (!value) return null;
  return (
    <p className="mt-3 rounded-2xl border border-border bg-card p-3 text-sm leading-6 text-muted-foreground">
      <span className="font-medium text-foreground">{copy.reviewReason}: </span>
      {value}
    </p>
  );
}

function SubmissionFooter({ href, updatedAt }: { href: string; updatedAt: string }) {
  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
      <span>{formatDate(updatedAt)}</span>
      {href ? (
        <Link className="inline-flex items-center font-medium text-primary" href={href}>
          {copy.publishedLink}
          <ArrowRight className="ml-1 size-3.5" aria-hidden="true" />
        </Link>
      ) : (
        <span>{copy.notPublic}</span>
      )}
    </div>
  );
}

function labelPostType(type: MyForumPost["type"]) {
  if (type === "reward") return "\u60ac\u8d4f";
  if (type === "question") return "\u95ee\u7b54";
  return "\u8ba8\u8bba";
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

async function request<T>(path: string): Promise<Envelope<T>> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `API request failed with ${response.status}`);
  }
  return payload;
}
