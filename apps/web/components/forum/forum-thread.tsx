"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { CheckCircle2, Loader2, Send, Trophy } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { apiBaseUrl, type ForumPost, type ForumReply, type User } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type ThreadPayload = {
  post: ForumPost;
  replies: ForumReply[];
};

const copy = {
  replies: "\u56de\u590d",
  bestAnswer: "\u6700\u4f73\u7b54\u6848",
  noReplies: "\u6682\u65e0\u5df2\u53d1\u5e03\u56de\u590d\u3002",
  loginToReply: "\u767b\u5f55\u540e\u53ef\u4ee5\u63d0\u4ea4\u56de\u590d\u3002",
  login: "\u53bb\u767b\u5f55",
  replyPlaceholder: "\u5199\u4e0b\u4f60\u7684\u601d\u8def\u3001\u63d0\u793a\u6216\u8d44\u6599\u8865\u5145\u3002\u56de\u590d\u5c06\u8fdb\u5165\u5ba1\u6838\u3002",
  submitReply: "\u63d0\u4ea4\u56de\u590d",
  submitting: "\u63d0\u4ea4\u4e2d...",
  replySubmitted: "\u56de\u590d\u5df2\u63d0\u4ea4\u5ba1\u6838\uff0c\u901a\u8fc7\u540e\u4f1a\u516c\u5f00\u5c55\u793a\u3002",
  markBest: "\u8bbe\u4e3a\u6700\u4f73",
  markingBest: "\u5904\u7406\u4e2d...",
  bestSelected: "\u5df2\u9009\u62e9\u6700\u4f73\u7b54\u6848\u3002",
  alreadyHasBest: "\u672c\u5e16\u5df2\u6709\u6700\u4f73\u7b54\u6848\u3002",
  rewardSettled: "\u60ac\u8d4f\u5df2\u7ed3\u7b97",
  rewardEscrowed: "\u60ac\u8d4f\u5f85\u7ed3\u7b97",
  bestActionHint: "\u53ef\u5728\u975e\u53d1\u5e16\u4eba\u7684\u5df2\u5ba1\u6838\u56de\u590d\u4e0a\u9009\u62e9\u6700\u4f73\u7b54\u6848\u3002",
  loadFailed: "\u8ba8\u8bba\u72b6\u6001\u6682\u65f6\u4e0d\u53ef\u7528",
};

export function ForumThread({ initialPost, initialReplies }: { initialPost: ForumPost; initialReplies: ForumReply[] }) {
  const [post, setPost] = useState(initialPost);
  const [replies, setReplies] = useState(initialReplies);
  const [user, setUser] = useState<User | null>(null);
  const [content, setContent] = useState("");
  const [replying, setReplying] = useState(false);
  const [markingReplyId, setMarkingReplyId] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    async function loadMe() {
      try {
        const response = await request<User>("/auth/me", { method: "GET" });
        setUser(response.data ?? null);
      } catch {
        setUser(null);
      }
    }

    void loadMe();
  }, []);

  const hasBestAnswer = useMemo(() => replies.some((reply) => reply.isBest), [replies]);
  const canSelectBest = user ? user.id === post.authorId || user.role === "admin" || user.role === "super_admin" : false;

  async function submitReply(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextContent = content.trim();
    if (!nextContent) return;
    setReplying(true);
    setError("");
    setMessage("");
    try {
      await request<{ reply: ForumReply }>(`/forum/posts/${post.id}/replies`, {
        method: "POST",
        body: JSON.stringify({ content: nextContent }),
      });
      setContent("");
      setMessage(copy.replySubmitted);
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.loadFailed);
    } finally {
      setReplying(false);
    }
  }

  async function markBest(reply: ForumReply) {
    setMarkingReplyId(reply.id);
    setError("");
    setMessage("");
    try {
      await request<{ marked: boolean; replyId: string; postId: string }>(`/forum/replies/${reply.id}/mark-best`, {
        method: "POST",
        body: JSON.stringify({}),
      });
      await refreshThread();
      setMessage(copy.bestSelected);
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.loadFailed);
    } finally {
      setMarkingReplyId("");
    }
  }

  async function refreshThread() {
    const response = await request<ThreadPayload>(`/forum/posts/${post.id}`, { method: "GET" });
    if (response.data) {
      setPost(response.data.post);
      setReplies(response.data.replies);
    }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[1fr_280px]">
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-sm font-medium text-primary">{copy.replies}</p>
            <h2 className="mt-2 text-2xl font-semibold tracking-tight">
              {replies.length} {copy.replies}
            </h2>
          </div>
          {post.type === "reward" ? <RewardBadge post={post} /> : null}
        </div>

        <div className="mt-5 grid gap-3">
          {replies.map((reply) => {
            const isSelfAnswer = reply.authorId === post.authorId;
            const showMarkButton = canSelectBest && !hasBestAnswer && !isSelfAnswer;
            return (
              <article
                className={`rounded-2xl border p-4 ${
                  reply.isBest ? "border-emerald-200 bg-emerald-50/70" : "border-border bg-background"
                }`}
                key={reply.id}
              >
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      {reply.isBest ? (
                        <Badge tone="success">
                          <CheckCircle2 className="mr-1 size-3" aria-hidden="true" />
                          {copy.bestAnswer}
                        </Badge>
                      ) : null}
                      <span className="text-xs text-muted-foreground">{formatDate(reply.createdAt)}</span>
                    </div>
                    <p className="mt-3 whitespace-pre-wrap break-words text-sm leading-6 text-foreground">{reply.content}</p>
                  </div>
                  {showMarkButton ? (
                    <button
                      className="inline-flex shrink-0 items-center justify-center rounded-xl border border-border bg-card px-3 py-2 text-sm font-medium text-primary transition hover:border-primary disabled:cursor-not-allowed disabled:opacity-60"
                      disabled={Boolean(markingReplyId)}
                      onClick={() => void markBest(reply)}
                      type="button"
                    >
                      {markingReplyId === reply.id ? <Loader2 className="mr-2 size-4 animate-spin" aria-hidden="true" /> : <Trophy className="mr-2 size-4" aria-hidden="true" />}
                      {markingReplyId === reply.id ? copy.markingBest : copy.markBest}
                    </button>
                  ) : null}
                </div>
              </article>
            );
          })}
        </div>

        {replies.length === 0 ? (
          <p className="mt-5 rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">{copy.noReplies}</p>
        ) : null}

        {message ? <p className="mt-5 rounded-2xl border border-border bg-muted p-4 text-sm text-foreground">{message}</p> : null}
        {error ? <p className="mt-5 rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700">{error}</p> : null}
      </section>

      <aside className="grid gap-4 self-start">
        <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
          {user ? (
            <form onSubmit={submitReply}>
              <label className="text-sm font-medium text-foreground" htmlFor="forum-reply">
                {copy.submitReply}
              </label>
              <textarea
                className="mt-3 min-h-36 w-full rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
                id="forum-reply"
                onChange={(event) => setContent(event.target.value)}
                placeholder={copy.replyPlaceholder}
                value={content}
              />
              <button
                className="mt-3 inline-flex w-full items-center justify-center rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] disabled:cursor-not-allowed disabled:opacity-60"
                disabled={replying || !content.trim()}
                type="submit"
              >
                {replying ? <Loader2 className="mr-2 size-4 animate-spin" aria-hidden="true" /> : <Send className="mr-2 size-4" aria-hidden="true" />}
                {replying ? copy.submitting : copy.submitReply}
              </button>
            </form>
          ) : (
            <div>
              <p className="text-sm leading-6 text-muted-foreground">{copy.loginToReply}</p>
              <Link className="mt-4 inline-flex rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
                {copy.login}
              </Link>
            </div>
          )}
        </section>

        {canSelectBest ? (
          <section className="rounded-3xl border border-border bg-card p-5 text-sm leading-6 text-muted-foreground shadow-sm">
            {hasBestAnswer ? copy.alreadyHasBest : copy.bestActionHint}
          </section>
        ) : null}
      </aside>
    </div>
  );
}

function RewardBadge({ post }: { post: ForumPost }) {
  const isSettled = post.rewardStatus === "settled";
  return (
    <span
      className={`inline-flex items-center rounded-full px-3 py-1 text-xs font-medium ${
        isSettled ? "bg-emerald-50 text-emerald-700" : "bg-amber-50 text-amber-700"
      }`}
    >
      <Trophy className="mr-1.5 size-3.5" aria-hidden="true" />
      {isSettled ? copy.rewardSettled : copy.rewardEscrowed} / {post.rewardPoints}
    </span>
  );
}

async function request<T>(path: string, init: RequestInit): Promise<Envelope<T>> {
  const headers = new Headers(init.headers);
  if (!(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `API request failed with ${response.status}`);
  }
  return payload;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
