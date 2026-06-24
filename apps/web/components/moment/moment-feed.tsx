"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { Ban, Heart, Loader2, MessageCircle, Send, UserPlus } from "lucide-react";
import { apiBaseUrl, type Moment, type MomentComment, type User } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type MomentForm = {
  content: string;
  imageText: string;
  visibility: "public" | "mutual_friends";
};

const copy = {
  loading: "\u6b63\u5728\u8bfb\u53d6\u52a8\u6001...",
  loginRequired: "\u767b\u5f55\u540e\u53ef\u4ee5\u53d1\u5e03\u52a8\u6001\u3001\u70b9\u8d5e\u548c\u8bc4\u8bba\u3002",
  login: "\u53bb\u767b\u5f55",
  composerTitle: "\u53d1\u5e03\u5b66\u4e60\u52a8\u6001",
  composerIntro: "\u53ef\u4ee5\u8bb0\u5f55\u590d\u4e60\u8fdb\u5ea6\u3001\u8d44\u6599\u7ebf\u7d22\u548c\u8bfe\u7a0b\u7ecf\u9a8c\u3002\u516c\u5f00\u6216\u4ec5\u4e92\u5173\u53ef\u89c1\u7531\u670d\u52a1\u7aef\u63a7\u5236\u3002",
  content: "\u5185\u5bb9",
  contentPlaceholder: "\u4f8b\u5982\uff1a\u4eca\u5929\u628a\u79bb\u6563\u6570\u5b66\u56fe\u8bba\u90e8\u5206\u8fc7\u4e86\u4e00\u904d...",
  imageText: "\u56fe\u7247 URL\uff08\u53ef\u9009\uff09",
  imagePlaceholder: "\u6bcf\u884c\u4e00\u4e2a https://... \u6216 /uploads/...",
  public: "\u516c\u5f00",
  mutual: "\u4ec5\u4e92\u5173",
  submit: "\u53d1\u5e03\u52a8\u6001",
  submitting: "\u53d1\u5e03\u4e2d...",
  empty: "\u6682\u65e0\u53ef\u89c1\u52a8\u6001\u3002",
  commentPlaceholder: "\u5199\u4e00\u6761\u8bc4\u8bba...",
  comment: "\u8bc4\u8bba",
  like: "\u70b9\u8d5e",
  liked: "\u5df2\u70b9\u8d5e",
  follow: "\u5173\u6ce8",
  block: "\u5c4f\u853d",
  delete: "\u5220\u9664",
  posted: "\u52a8\u6001\u5df2\u53d1\u5e03\u3002",
  failed: "\u52a8\u6001\u529f\u80fd\u6682\u65f6\u4e0d\u53ef\u7528",
};

export function MomentFeed({ initialMoments }: { initialMoments: Moment[] }) {
  const [moments, setMoments] = useState(initialMoments);
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [form, setForm] = useState<MomentForm>({ content: "", imageText: "", visibility: "public" });
  const [commentTextByMoment, setCommentTextByMoment] = useState<Record<string, string>>({});

  useEffect(() => {
    async function load() {
      try {
        const meResponse = await request<User>("/auth/me", { method: "GET" });
        setUser(meResponse.data ?? null);
      } catch {
        setUser(null);
      }
      try {
        const momentsResponse = await request<{ moments: Moment[] }>("/moments", { method: "GET" });
        setMoments(momentsResponse.data?.moments ?? []);
      } catch {
        setMoments(initialMoments);
      } finally {
        setLoading(false);
      }
    }

    void load();
  }, [initialMoments]);

  const contentLength = useMemo(() => Array.from(form.content).length, [form.content]);

  async function submitMoment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!form.content.trim()) return;
    setSubmitting(true);
    setError("");
    setMessage("");
    try {
      const response = await request<{ moment: Moment }>("/moments", {
        method: "POST",
        body: JSON.stringify({
          content: form.content.trim(),
          images: parseImageText(form.imageText),
          visibility: form.visibility,
        }),
      });
      if (response.data?.moment) {
        setMoments((current) => [response.data!.moment, ...current]);
      }
      setForm({ content: "", imageText: "", visibility: "public" });
      setMessage(copy.posted);
    } catch (err) {
      setError(formatError(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function likeMoment(momentID: string) {
    try {
      const response = await request<{ moment: Moment }>(`/moments/${momentID}/like`, { method: "POST" });
      if (response.data?.moment) replaceMoment(response.data.moment);
    } catch (err) {
      setError(formatError(err));
    }
  }

  async function commentMoment(momentID: string) {
    const content = commentTextByMoment[momentID]?.trim();
    if (!content) return;
    try {
      const response = await request<{ comment: MomentComment }>(`/moments/${momentID}/comments`, {
        method: "POST",
        body: JSON.stringify({ content }),
      });
      if (response.data?.comment) {
        setMoments((current) =>
          current.map((item) =>
            item.id === momentID
              ? {
                  ...item,
                  commentCount: item.commentCount + 1,
                  recentComments: [...(item.recentComments ?? []), response.data!.comment].slice(-5),
                }
              : item,
          ),
        );
      }
      setCommentTextByMoment((current) => ({ ...current, [momentID]: "" }));
    } catch (err) {
      setError(formatError(err));
    }
  }

  async function followUser(userID: string) {
    try {
      await request<{ following: boolean }>(`/users/${userID}/follow`, { method: "POST" });
      setMessage("\u5df2\u5173\u6ce8\u8be5\u7528\u6237\u3002");
    } catch (err) {
      setError(formatError(err));
    }
  }

  async function blockUser(userID: string) {
    try {
      await request<{ blocked: boolean }>(`/users/${userID}/block`, { method: "POST" });
      setMoments((current) => current.filter((item) => item.author.id !== userID));
      setMessage("\u5df2\u5c4f\u853d\u8be5\u7528\u6237\uff0c\u76f8\u5173\u52a8\u6001\u5df2\u9690\u85cf\u3002");
    } catch (err) {
      setError(formatError(err));
    }
  }

  async function deleteMoment(momentID: string) {
    try {
      await request<{ deleted: boolean }>(`/moments/${momentID}`, { method: "DELETE" });
      setMoments((current) => current.filter((item) => item.id !== momentID));
    } catch (err) {
      setError(formatError(err));
    }
  }

  function replaceMoment(nextMoment: Moment) {
    setMoments((current) => current.map((item) => (item.id === nextMoment.id ? nextMoment : item)));
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[0.9fr_1.5fr]">
      <aside className="space-y-4">
        <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
          {loading ? <p className="text-sm text-muted-foreground">{copy.loading}</p> : null}
          {!loading && !user ? (
            <>
              <p className="text-sm leading-6 text-muted-foreground">{copy.loginRequired}</p>
              <Link className="mt-4 inline-flex rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
                {copy.login}
              </Link>
            </>
          ) : null}
          {user ? (
            <>
              <h2 className="text-lg font-semibold tracking-tight">{copy.composerTitle}</h2>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{copy.composerIntro}</p>
              <form className="mt-4 grid gap-3" onSubmit={submitMoment}>
                <label className="block text-sm font-medium">
                  {copy.content}
                  <textarea
                    className="mt-2 min-h-32 w-full rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
                    maxLength={500}
                    onChange={(event) => setForm((current) => ({ ...current, content: event.target.value }))}
                    placeholder={copy.contentPlaceholder}
                    value={form.content}
                  />
                </label>
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span>{contentLength}/500</span>
                  <div className="flex rounded-full border border-border bg-background p-1">
                    {(["public", "mutual_friends"] as const).map((visibility) => (
                      <button
                        className={`rounded-full px-3 py-1.5 ${form.visibility === visibility ? "bg-primary text-primary-foreground" : "text-muted-foreground"}`}
                        key={visibility}
                        onClick={() => setForm((current) => ({ ...current, visibility }))}
                        type="button"
                      >
                        {visibility === "public" ? copy.public : copy.mutual}
                      </button>
                    ))}
                  </div>
                </div>
                <label className="block text-sm font-medium">
                  {copy.imageText}
                  <textarea
                    className="mt-2 min-h-20 w-full rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
                    onChange={(event) => setForm((current) => ({ ...current, imageText: event.target.value }))}
                    placeholder={copy.imagePlaceholder}
                    value={form.imageText}
                  />
                </label>
                <button
                  className="inline-flex w-full items-center justify-center rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground shadow-sm disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={submitting || !form.content.trim()}
                  type="submit"
                >
                  {submitting ? <Loader2 className="mr-2 size-4 animate-spin" aria-hidden="true" /> : <Send className="mr-2 size-4" aria-hidden="true" />}
                  {submitting ? copy.submitting : copy.submit}
                </button>
              </form>
            </>
          ) : null}
        </section>
        {message ? <p className="rounded-2xl border border-border bg-muted p-4 text-sm text-foreground">{message}</p> : null}
        {error ? <p className="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700">{error}</p> : null}
      </aside>

      <section className="space-y-4">
        {moments.map((moment) => {
          const canActOnAuthor = Boolean(user && user.id !== moment.author.id);
          return (
            <article className="rounded-3xl border border-border bg-card p-5 shadow-sm" key={moment.id}>
              <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <Link className="break-words font-semibold hover:text-primary" href={`/users/${moment.author.id}`}>
                    {moment.author.name || "\u533f\u540d\u540c\u5b66"}
                  </Link>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {visibilityLabel(moment.visibility)} / {formatDate(moment.createdAt)}
                  </p>
                </div>
                {canActOnAuthor ? (
                  <div className="flex shrink-0 gap-2">
                    <button className="rounded-lg border border-border px-3 py-2 text-xs hover:bg-muted" onClick={() => void followUser(moment.author.id)} type="button">
                      <UserPlus className="mr-1 inline size-3.5" aria-hidden="true" />
                      {copy.follow}
                    </button>
                    <button className="rounded-lg border border-border px-3 py-2 text-xs hover:bg-muted" onClick={() => void blockUser(moment.author.id)} type="button">
                      <Ban className="mr-1 inline size-3.5" aria-hidden="true" />
                      {copy.block}
                    </button>
                  </div>
                ) : null}
              </div>
              <p className="mt-4 whitespace-pre-wrap break-words text-sm leading-7">{moment.content}</p>
              {moment.images.length > 0 ? (
                <div className="mt-4 grid gap-2 sm:grid-cols-2">
                  {moment.images.slice(0, 4).map((image) => (
                    <a className="truncate rounded-2xl border border-border bg-background p-3 text-xs text-muted-foreground hover:text-foreground" href={image} key={image}>
                      {image}
                    </a>
                  ))}
                </div>
              ) : null}
              <div className="mt-4 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                <button
                  className="inline-flex min-h-9 items-center rounded-lg border border-border px-3 py-1.5 hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={!user}
                  onClick={() => void likeMoment(moment.id)}
                  type="button"
                >
                  <Heart className={`mr-1.5 size-4 ${moment.likedByMe ? "fill-current text-red-600" : ""}`} aria-hidden="true" />
                  {moment.likedByMe ? copy.liked : copy.like} {moment.likeCount}
                </button>
                <span className="inline-flex min-h-9 items-center rounded-lg border border-border px-3 py-1.5">
                  <MessageCircle className="mr-1.5 size-4" aria-hidden="true" />
                  {moment.commentCount}
                </span>
                {user?.id === moment.author.id ? (
                  <button className="inline-flex min-h-9 items-center rounded-lg border border-border px-3 py-1.5 hover:bg-muted" onClick={() => void deleteMoment(moment.id)} type="button">
                    {copy.delete}
                  </button>
                ) : null}
              </div>

              {(moment.recentComments ?? []).length > 0 ? (
                <div className="mt-4 space-y-2 rounded-2xl border border-border bg-background p-3">
                  {(moment.recentComments ?? []).map((comment) => (
                    <div className="text-sm" key={comment.id}>
                      <span className="font-medium">{comment.author.name || "\u540c\u5b66"}</span>
                      <span className="ml-2 text-muted-foreground">{comment.content}</span>
                    </div>
                  ))}
                </div>
              ) : null}

              {user ? (
                <form
                  className="mt-3 flex min-w-0 gap-2"
                  onSubmit={(event) => {
                    event.preventDefault();
                    void commentMoment(moment.id);
                  }}
                >
                  <input
                    className="min-w-0 flex-1 rounded-xl border border-border bg-background px-3 py-2.5 text-sm"
                    maxLength={500}
                    onChange={(event) => setCommentTextByMoment((current) => ({ ...current, [moment.id]: event.target.value }))}
                    placeholder={copy.commentPlaceholder}
                    value={commentTextByMoment[moment.id] ?? ""}
                  />
                  <button className="rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground" type="submit">
                    {copy.comment}
                  </button>
                </form>
              ) : null}
            </article>
          );
        })}
        {moments.length === 0 ? <p className="rounded-3xl border border-border bg-card p-5 text-sm text-muted-foreground shadow-sm">{copy.empty}</p> : null}
      </section>
    </div>
  );
}

async function request<T>(path: string, init: RequestInit): Promise<Envelope<T>> {
  const headers = new Headers(init.headers);
  if (init.body && !(init.body instanceof FormData)) {
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

function parseImageText(value: string) {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean)
    .slice(0, 9);
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatError(error: unknown) {
  const message = error instanceof Error ? error.message : copy.failed;
  const labels: Record<string, string> = {
    content_required: "\u8bf7\u5148\u586b\u5199\u52a8\u6001\u5185\u5bb9\u3002",
    content_too_long: "\u52a8\u6001\u6700\u591a 500 \u5b57\u3002",
    too_many_images: "\u56fe\u7247\u6700\u591a 9 \u5f20\u3002",
    invalid_image_url: "\u56fe\u7247 URL \u53ea\u652f\u6301 http(s) \u6216 /uploads/ \u8def\u5f84\u3002",
    blocked_relation: "\u5df2\u5b58\u5728\u5c4f\u853d\u5173\u7cfb\uff0c\u4e0d\u80fd\u5173\u6ce8\u3002",
    user_frozen: "\u8d26\u53f7\u5df2\u88ab\u51bb\u7ed3\uff0c\u6682\u65f6\u4e0d\u80fd\u64cd\u4f5c\u3002",
  };
  return labels[message] ?? message;
}

function visibilityLabel(value: string) {
  return value === "mutual_friends" ? copy.mutual : copy.public;
}
