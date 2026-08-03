"use client";

import Link from "next/link";
import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { Ban, BookOpenText, Heart, MessageCircle, Rss, ShieldCheck, UserMinus, UserPlus } from "lucide-react";
import { apiBaseUrl, type UserProfileResponse } from "@/lib/api";
import { Badge } from "@/components/ui/badge";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

const copy = {
  joined: "\u52a0\u5165",
  following: "\u5173\u6ce8",
  followers: "\u7c89\u4e1d",
  moments: "\u52a8\u6001",
  blog: "Blog",
  forum: "\u8ba8\u8bba",
  replies: "\u56de\u590d",
  follow: "\u5173\u6ce8",
  unfollow: "\u53d6\u6d88\u5173\u6ce8",
  block: "\u5c4f\u853d",
  unblock: "\u53d6\u6d88\u5c4f\u853d",
  mutual: "\u4e92\u5173\u597d\u53cb",
  followsMe: "\u5173\u6ce8\u4e86\u6211",
  publicOnly: "\u516c\u5f00\u4e3b\u9875\u53ea\u5c55\u793a\u5df2\u53d1\u5e03\u548c\u5f53\u524d\u8d26\u53f7\u53ef\u89c1\u5185\u5bb9\u3002",
  noMoments: "\u6682\u65e0\u53ef\u89c1\u52a8\u6001\u3002",
  noPosts: "\u6682\u65e0\u516c\u5f00\u5185\u5bb9\u3002",
  failed: "\u7528\u6237\u4e3b\u9875\u6682\u65f6\u4e0d\u53ef\u7528",
};

export function UserProfileView({ initialProfile, userId }: { initialProfile: UserProfileResponse; userId: string }) {
  const [profile, setProfile] = useState(initialProfile);
  const [error, setError] = useState("");

  useEffect(() => {
    void refreshProfile();
  }, [userId]);

  async function refreshProfile() {
    try {
      const response = await request<UserProfileResponse>(`/users/${userId}`, { method: "GET" });
      if (response.data) {
        setProfile(response.data);
      }
    } catch (err) {
      setError(formatError(err));
    }
  }

  async function relationAction(action: "follow" | "unfollow" | "block" | "unblock") {
    setError("");
    try {
      await request<Record<string, boolean>>(`/users/${userId}/${action}`, { method: "POST" });
      await refreshProfile();
    } catch (err) {
      setError(formatError(err));
    }
  }

  return (
    <div className="grid gap-5 lg:grid-cols-[0.85fr_1.4fr]">
      <aside className="space-y-4">
        <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <Badge tone="success">{roleLabel(profile.profile.role)}</Badge>
              <h1 className="mt-4 break-words text-3xl font-semibold tracking-tight">{profile.profile.name || "\u540c\u5b66"}</h1>
              <p className="mt-2 text-sm text-muted-foreground">
                {copy.joined} {formatDate(profile.profile.createdAt)}
              </p>
            </div>
            <ShieldCheck className="size-6 shrink-0 text-primary" aria-hidden="true" />
          </div>
          <p className="mt-4 text-sm leading-6 text-muted-foreground">{copy.publicOnly}</p>
          <div className="mt-4 grid grid-cols-2 gap-2 text-sm">
            <Metric label={copy.following} value={profile.profile.followingCount} />
            <Metric label={copy.followers} value={profile.profile.followersCount} />
            <Metric label={copy.moments} value={profile.profile.momentsCount} />
            <Metric label={copy.blog} value={profile.profile.blogPostsCount} />
          </div>
          <div className="mt-4 flex flex-wrap gap-2 text-xs">
            {profile.profile.mutualFriend ? <Badge tone="success">{copy.mutual}</Badge> : null}
            {profile.profile.followsMe && !profile.profile.mutualFriend ? <Badge tone="muted">{copy.followsMe}</Badge> : null}
            {profile.profile.blockedByMe ? <Badge tone="muted">{copy.block}</Badge> : null}
          </div>
        </section>

        <section className="grid gap-2 rounded-3xl border border-border bg-card p-5 shadow-sm">
          {profile.profile.blockedByMe ? (
            <button className="inline-flex min-h-10 items-center justify-center rounded-xl border border-border px-4 py-2 text-sm font-medium hover:bg-muted" onClick={() => void relationAction("unblock")} type="button">
              <Ban className="mr-2 size-4" aria-hidden="true" />
              {copy.unblock}
            </button>
          ) : (
            <>
              <button className="inline-flex min-h-10 items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-60" onClick={() => void relationAction(profile.profile.followingByMe ? "unfollow" : "follow")} type="button">
                {profile.profile.followingByMe ? <UserMinus className="mr-2 size-4" aria-hidden="true" /> : <UserPlus className="mr-2 size-4" aria-hidden="true" />}
                {profile.profile.followingByMe ? copy.unfollow : copy.follow}
              </button>
              <button className="inline-flex min-h-10 items-center justify-center rounded-xl border border-border px-4 py-2 text-sm font-medium hover:bg-muted" onClick={() => void relationAction("block")} type="button">
                <Ban className="mr-2 size-4" aria-hidden="true" />
                {copy.block}
              </button>
            </>
          )}
        </section>

        {error ? <p className="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700">{error}</p> : null}
      </aside>

      <section className="space-y-5">
        <ContentSection icon={<Rss className="size-4" aria-hidden="true" />} title={copy.moments}>
          {profile.moments.length === 0 ? <Empty text={copy.noMoments} /> : null}
          {profile.moments.map((moment) => (
            <article className="rounded-3xl border border-border bg-card p-5 shadow-sm" key={moment.id}>
              <p className="whitespace-pre-wrap break-words text-sm leading-7">{moment.content}</p>
              <div className="mt-4 flex flex-wrap gap-2 text-xs text-muted-foreground">
                <span className="rounded-full bg-muted px-3 py-1">{moment.visibility === "mutual_friends" ? "\u4e92\u5173\u53ef\u89c1" : "\u516c\u5f00"}</span>
                <span className="inline-flex items-center rounded-full bg-muted px-3 py-1">
                  <Heart className="mr-1 size-3.5" aria-hidden="true" />
                  {moment.likeCount}
                </span>
                <span className="inline-flex items-center rounded-full bg-muted px-3 py-1">
                  <MessageCircle className="mr-1 size-3.5" aria-hidden="true" />
                  {moment.commentCount}
                </span>
                <span className="rounded-full bg-muted px-3 py-1">{formatDate(moment.createdAt)}</span>
              </div>
            </article>
          ))}
        </ContentSection>

        <ContentSection icon={<BookOpenText className="size-4" aria-hidden="true" />} title={copy.blog}>
          {profile.blogPosts.length === 0 ? <Empty text={copy.noPosts} /> : null}
          {profile.blogPosts.map((post) => (
            <Link className="block rounded-3xl border border-border bg-card p-5 shadow-sm transition hover:border-primary/60 hover:shadow-md" href={`/blog/${post.id}`} key={post.id}>
              <h2 className="break-words text-lg font-semibold tracking-tight">{post.title}</h2>
              <p className="mt-2 line-clamp-2 break-words text-sm leading-6 text-muted-foreground">{post.content}</p>
            </Link>
          ))}
        </ContentSection>

        <ContentSection icon={<MessageCircle className="size-4" aria-hidden="true" />} title={copy.forum}>
          {profile.forumPosts.length === 0 && profile.forumReplies.length === 0 ? <Empty text={copy.noPosts} /> : null}
          {profile.forumPosts.map((post) => (
            <Link className="block rounded-3xl border border-border bg-card p-5 shadow-sm transition hover:border-primary/60 hover:shadow-md" href={`/forum/${post.id}`} key={post.id}>
              <div className="flex flex-wrap gap-2">
                <Badge tone={post.type === "reward" ? "success" : "muted"}>{forumPostTypeLabel(post.type)}</Badge>
                <Badge tone="muted">{post.commentCount} {copy.replies}</Badge>
              </div>
              <h2 className="mt-3 break-words text-lg font-semibold tracking-tight">{post.title}</h2>
              <p className="mt-2 line-clamp-2 break-words text-sm leading-6 text-muted-foreground">{post.content}</p>
            </Link>
          ))}
          {profile.forumReplies.map((reply) => (
            <Link className="block rounded-3xl border border-border bg-card p-5 shadow-sm transition hover:border-primary/60 hover:shadow-md" href={`/forum/${reply.postId}`} key={reply.id}>
              <Badge tone={reply.isBest ? "success" : "muted"}>{reply.isBest ? "\u6700\u4f73\u7b54\u6848" : copy.replies}</Badge>
              <h2 className="mt-3 break-words text-lg font-semibold tracking-tight">{reply.postTitle}</h2>
              <p className="mt-2 line-clamp-2 break-words text-sm leading-6 text-muted-foreground">{reply.content}</p>
            </Link>
          ))}
        </ContentSection>
      </section>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-2xl border border-border bg-background p-3">
      <strong className="block text-xl">{value}</strong>
      <span className="text-xs text-muted-foreground">{label}</span>
    </div>
  );
}

function ContentSection({ children, icon, title }: { children: ReactNode; icon: ReactNode; title: string }) {
  return (
    <section className="space-y-3">
      <h2 className="flex items-center gap-2 text-lg font-semibold tracking-tight">
        {icon}
        {title}
      </h2>
      {children}
    </section>
  );
}

function Empty({ text }: { text: string }) {
  return <p className="rounded-3xl border border-border bg-card p-5 text-sm text-muted-foreground shadow-sm">{text}</p>;
}

async function request<T>(path: string, init: RequestInit): Promise<Envelope<T>> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || "网络不太顺畅，请检查网络后重试");
  }
  return payload;
}

function roleLabel(role: string) {
  const labels: Record<string, string> = {
    admin: "管理员",
    creator: "创作者",
    reviewer: "审核员",
    student: "学生",
  };
  return labels[role] ?? role;
}

function forumPostTypeLabel(type: string) {
  const labels: Record<string, string> = {
    normal: "讨论",
    question: "问答",
    reward: "悬赏",
  };
  return labels[type] ?? type;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString("zh-CN");
}

function formatError(error: unknown) {
  const message = error instanceof Error ? error.message : copy.failed;
  const labels: Record<string, string> = {
    unauthorized: "\u8bf7\u5148\u767b\u5f55\u540e\u64cd\u4f5c\u3002",
    blocked_relation: "\u5df2\u5b58\u5728\u5c4f\u853d\u5173\u7cfb\uff0c\u4e0d\u80fd\u5173\u6ce8\u3002",
    invalid_target: "\u4e0d\u80fd\u5bf9\u81ea\u5df1\u64cd\u4f5c\u3002",
    user_not_found: "\u7528\u6237\u4e0d\u5b58\u5728\u6216\u4e0d\u53ef\u89c1\u3002",
  };
  return labels[message] ?? message;
}
