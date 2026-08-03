"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { Ban, RefreshCw, UserMinus, UserPlus, UsersRound } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { apiBaseUrl, type UserSummary } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type RelationSet = {
  followers: UserSummary[];
  following: UserSummary[];
  friends: UserSummary[];
};

const copy = {
  back: "\u8fd4\u56de\u4e2a\u4eba\u4e2d\u5fc3",
  eyebrow: "\u5173\u7cfb\u7ba1\u7406",
  title: "\u5173\u6ce8\u3001\u7c89\u4e1d\u4e0e\u4e92\u5173\u597d\u53cb",
  intro:
    "\u8fd9\u91cc\u53ea\u663e\u793a\u5f53\u524d\u8d26\u53f7\u81ea\u5df1\u7684\u5173\u7cfb\u5217\u8868\u3002\u5df2\u5c4f\u853d\u7684\u5173\u7cfb\u4e0d\u4f1a\u51fa\u73b0\uff0c\u5217\u8868\u4e5f\u4e0d\u8fd4\u56de\u7528\u6237\u90ae\u7bb1\u3002",
  refresh: "\u5237\u65b0",
  loading: "\u6b63\u5728\u8bfb\u53d6\u5173\u7cfb...",
  login: "\u53bb\u767b\u5f55",
  following: "\u6211\u5173\u6ce8\u7684",
  followers: "\u5173\u6ce8\u6211\u7684",
  friends: "\u4e92\u5173\u597d\u53cb",
  empty: "\u6682\u65e0\u8bb0\u5f55\u3002",
  viewProfile: "\u4e3b\u9875",
  follow: "\u5173\u6ce8",
  unfollow: "\u53d6\u6d88\u5173\u6ce8",
  block: "\u5c4f\u853d",
  mutual: "\u4e92\u5173",
  fallbackError: "\u5173\u7cfb\u5217\u8868\u6682\u65f6\u4e0d\u53ef\u7528",
};

export default function RelationsPage() {
  const [relations, setRelations] = useState<RelationSet>({ followers: [], following: [], friends: [] });
  const [loading, setLoading] = useState(true);
  const [savingID, setSavingID] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    void loadRelations();
  }, []);

  const followingIDs = useMemo(() => new Set(relations.following.map((user) => user.id)), [relations.following]);
  const friendIDs = useMemo(() => new Set(relations.friends.map((user) => user.id)), [relations.friends]);

  async function loadRelations() {
    setLoading(true);
    setError("");
    try {
      const [followingResponse, followersResponse, friendsResponse] = await Promise.all([
        request<{ users: UserSummary[] }>("/me/following"),
        request<{ users: UserSummary[] }>("/me/followers"),
        request<{ users: UserSummary[] }>("/me/friends"),
      ]);
      setRelations({
        followers: followersResponse.data?.users ?? [],
        following: followingResponse.data?.users ?? [],
        friends: friendsResponse.data?.users ?? [],
      });
    } catch (err) {
      setError(formatError(err));
    } finally {
      setLoading(false);
    }
  }

  async function relationAction(userID: string, action: "follow" | "unfollow" | "block") {
    setSavingID(`${action}:${userID}`);
    setError("");
    try {
      await request<Record<string, boolean>>(`/users/${userID}/${action}`, { method: "POST" });
      await loadRelations();
    } catch (err) {
      setError(formatError(err));
    } finally {
      setSavingID("");
    }
  }

  return (
    <SiteShell>
      <nav className="flex flex-wrap items-center justify-between gap-3 text-sm">
        <Link className="font-semibold text-primary" href="/me">
          {copy.back}
        </Link>
        <button
          className="inline-flex min-h-10 items-center rounded-xl border border-border px-3 py-2 font-medium text-foreground hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
          disabled={loading}
          onClick={() => void loadRelations()}
          type="button"
        >
          <RefreshCw className={`mr-2 size-4 ${loading ? "animate-spin" : ""}`} aria-hidden="true" />
          {copy.refresh}
        </button>
      </nav>

      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
          <div className="min-w-0">
            <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
            <h1 className="mt-2 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{copy.title}</h1>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
          </div>
          <div className="grid shrink-0 grid-cols-3 gap-2 text-center text-sm">
            <Metric label={copy.following} value={relations.following.length} />
            <Metric label={copy.followers} value={relations.followers.length} />
            <Metric label={copy.friends} value={relations.friends.length} />
          </div>
        </div>
      </section>

      {loading ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.loading}</p> : null}
      {error ? (
        <div className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">
          <p>{error}</p>
          {error === "unauthorized" ? (
            <Link className="mt-3 inline-flex rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
              {copy.login}
            </Link>
          ) : null}
        </div>
      ) : null}

      <section className="grid gap-4 lg:grid-cols-3">
        <RelationColumn
          actionFor={(user) => (followingIDs.has(user.id) ? "unfollow" : "follow")}
          friendIDs={friendIDs}
          onAction={relationAction}
          savingID={savingID}
          title={copy.followers}
          users={relations.followers}
        />
        <RelationColumn
          actionFor={() => "unfollow"}
          friendIDs={friendIDs}
          onAction={relationAction}
          savingID={savingID}
          title={copy.following}
          users={relations.following}
        />
        <RelationColumn
          actionFor={() => "unfollow"}
          friendIDs={friendIDs}
          onAction={relationAction}
          savingID={savingID}
          title={copy.friends}
          users={relations.friends}
        />
      </section>
    </SiteShell>
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

function RelationColumn({
  actionFor,
  friendIDs,
  onAction,
  savingID,
  title,
  users,
}: {
  actionFor: (user: UserSummary) => "follow" | "unfollow";
  friendIDs: Set<string>;
  onAction: (userID: string, action: "follow" | "unfollow" | "block") => Promise<void>;
  savingID: string;
  title: string;
  users: UserSummary[];
}) {
  return (
    <div className="rounded-3xl border border-border bg-card p-4 shadow-sm">
      <h2 className="flex items-center gap-2 font-semibold tracking-tight">
        <UsersRound className="size-4 text-primary" aria-hidden="true" />
        {title}
      </h2>
      <div className="mt-4 grid gap-3">
        {users.map((user) => {
          const action = actionFor(user);
          return (
            <article className="rounded-2xl border border-border bg-background p-4" key={user.id}>
              <div className="flex min-w-0 items-start justify-between gap-3">
                <div className="min-w-0">
                  <Link className="break-words font-semibold hover:text-primary" href={`/users/${user.id}`}>
                    {user.name || "\u540c\u5b66"}
                  </Link>
                  <div className="mt-2 flex flex-wrap gap-2">
                    <Badge tone="muted">{roleLabel(user.role)}</Badge>
                    {friendIDs.has(user.id) ? <Badge tone="success">{copy.mutual}</Badge> : null}
                  </div>
                </div>
                <Link className="shrink-0 rounded-lg border border-border px-2.5 py-1.5 text-xs hover:bg-muted" href={`/users/${user.id}`}>
                  {copy.viewProfile}
                </Link>
              </div>
              <div className="mt-4 flex flex-wrap gap-2">
                <button
                  className="inline-flex min-h-9 items-center rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground disabled:opacity-60"
                  disabled={savingID === `${action}:${user.id}`}
                  onClick={() => void onAction(user.id, action)}
                  type="button"
                >
                  {action === "follow" ? <UserPlus className="mr-1.5 size-3.5" aria-hidden="true" /> : <UserMinus className="mr-1.5 size-3.5" aria-hidden="true" />}
                  {action === "follow" ? copy.follow : copy.unfollow}
                </button>
                <button
                  className="inline-flex min-h-9 items-center rounded-lg border border-border px-3 py-1.5 text-xs font-medium hover:bg-muted disabled:opacity-60"
                  disabled={savingID === `block:${user.id}`}
                  onClick={() => void onAction(user.id, "block")}
                  type="button"
                >
                  <Ban className="mr-1.5 size-3.5" aria-hidden="true" />
                  {copy.block}
                </button>
              </div>
            </article>
          );
        })}
        {users.length === 0 ? <p className="rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">{copy.empty}</p> : null}
      </div>
    </div>
  );
}

async function request<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
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

function formatError(error: unknown) {
  const message = error instanceof Error ? error.message : copy.fallbackError;
  const labels: Record<string, string> = {
    blocked_relation: "\u5df2\u5b58\u5728\u5c4f\u853d\u5173\u7cfb\uff0c\u4e0d\u80fd\u5173\u6ce8\u3002",
    invalid_target: "\u4e0d\u80fd\u5bf9\u81ea\u5df1\u64cd\u4f5c\u3002",
    target_not_found: "\u7528\u6237\u4e0d\u5b58\u5728\u6216\u4e0d\u53ef\u89c1\u3002",
  };
  return labels[message] ?? message;
}
