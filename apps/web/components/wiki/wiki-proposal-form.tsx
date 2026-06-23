"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { Loader2, PenLine } from "lucide-react";
import { apiBaseUrl, type User, type WikiEditProposal, type WikiEntry } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type WikiProposalFormProps = {
  entry: WikiEntry;
};

const copy = {
  title: "\u63d0\u4ea4 Wiki \u4fee\u8ba2\u63d0\u6848",
  intro:
    "\u4fee\u8ba2\u4e0d\u4f1a\u7acb\u5373\u6539\u52a8\u516c\u5f00\u8bcd\u6761\u3002\u63d0\u4ea4\u540e\u4f1a\u8fdb\u5165 reviewer/admin \u5ba1\u6838\uff0c\u5ba1\u6838\u901a\u8fc7\u540e\u624d\u4f1a\u66f4\u65b0\u6b63\u5f0f\u5185\u5bb9\u548c\u7248\u672c\u5386\u53f2\u3002",
  currentVersion: "\u5f53\u524d\u7248\u672c",
  titleLabel: "\u6807\u9898",
  contentLabel: "\u5185\u5bb9",
  summaryLabel: "\u4fee\u8ba2\u6458\u8981",
  summaryPlaceholder: "\u4f8b\uff1a\u8865\u5145\u56fe\u8bba\u5e38\u89c1\u6613\u9519\u70b9",
  submit: "\u63d0\u4ea4\u5f85\u5ba1\u63d0\u6848",
  submitting: "\u63d0\u4ea4\u4e2d...",
  reset: "\u6062\u590d\u5f53\u524d\u7248\u672c",
  submitted: "\u4fee\u8ba2\u63d0\u6848\u5df2\u63d0\u4ea4\uff0c\u7b49\u5f85\u5ba1\u6838\u3002",
  loadingUser: "\u6b63\u5728\u8bfb\u53d6\u8d26\u53f7\u6743\u9650...",
  login: "\u53bb\u767b\u5f55",
  loginRequired: "\u9700\u8981\u5148\u767b\u5f55\u624d\u80fd\u63d0\u4ea4 Wiki \u4fee\u8ba2\u63d0\u6848\u3002",
  forbidden: "\u53ea\u6709 creator/admin \u53ef\u4ee5\u63d0\u4ea4 Wiki \u4fee\u8ba2\u63d0\u6848\u3002",
  permissionHint: "\u666e\u901a\u7528\u6237\u53ef\u4ee5\u9605\u8bfb\u516c\u5f00 Wiki\uff1b\u4fee\u8ba2\u63d0\u6848\u9700\u8981\u521b\u4f5c\u8005\u6216\u7ba1\u7406\u5458\u8eab\u4efd\uff0c\u5e76\u4e14\u5fc5\u987b\u7ecf\u8fc7\u5ba1\u6838\u624d\u4f1a\u53d1\u5e03\u3002",
  unchanged: "\u6807\u9898\u548c\u5185\u5bb9\u6ca1\u6709\u53d8\u5316\uff0c\u65e0\u9700\u63d0\u4ea4\u63d0\u6848\u3002",
  missingRequired: "\u8bf7\u586b\u5199\u6807\u9898\u548c\u5185\u5bb9\u3002",
  titleTooLong: "\u6807\u9898\u4e0d\u80fd\u8d85\u8fc7 200 \u4e2a\u5b57\u7b26\u3002",
  contentTooLong: "\u5185\u5bb9\u4e0d\u80fd\u8d85\u8fc7 80000 \u4e2a\u5b57\u7b26\u3002",
  summaryTooLong: "\u4fee\u8ba2\u6458\u8981\u4e0d\u80fd\u8d85\u8fc7 500 \u4e2a\u5b57\u7b26\u3002",
  failed: "\u4fee\u8ba2\u63d0\u6848\u63d0\u4ea4\u5931\u8d25",
};

export function WikiProposalForm({ entry }: WikiProposalFormProps) {
  const [user, setUser] = useState<User | null>(null);
  const [authLoading, setAuthLoading] = useState(true);
  const [title, setTitle] = useState(entry.title);
  const [content, setContent] = useState(entry.content);
  const [summary, setSummary] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [needsLogin, setNeedsLogin] = useState(false);

  useEffect(() => {
    async function loadUser() {
      setAuthLoading(true);
      try {
        const response = await request<User>("/auth/me", { method: "GET" });
        setUser(response.data ?? null);
      } catch {
        setUser(null);
      } finally {
        setAuthLoading(false);
      }
    }

    void loadUser();
  }, []);

  async function submitProposal(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setMessage("");
    setError("");
    setNeedsLogin(false);
    try {
      const response = await request<{ proposal: WikiEditProposal }>(`/wiki/entries/${entry.id}/proposals`, {
        method: "POST",
        body: JSON.stringify({
          title: title.trim(),
          content: content.trim(),
          summary: summary.trim(),
        }),
      });
      if (response.data?.proposal) {
        setMessage(copy.submitted);
        setSummary("");
      }
    } catch (err) {
      const nextError = err instanceof Error ? formatError(err.message) : copy.failed;
      setError(nextError);
      setNeedsLogin(err instanceof Error && err.message === "unauthorized");
    } finally {
      setSubmitting(false);
    }
  }

  function resetToCurrent() {
    setTitle(entry.title);
    setContent(entry.content);
    setSummary("");
    setMessage("");
    setError("");
    setNeedsLogin(false);
  }

  if (authLoading) {
    return <BoundaryPanel entry={entry} message={copy.loadingUser} />;
  }

  if (!user) {
    return <BoundaryPanel entry={entry} message={copy.loginRequired} showLogin />;
  }

  if (!canPropose(user.role)) {
    return <BoundaryPanel entry={entry} message={copy.permissionHint} />;
  }

  return (
    <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <PenLine className="size-5 text-primary" aria-hidden="true" />
            <h2 className="text-xl font-semibold tracking-tight">{copy.title}</h2>
          </div>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
        </div>
        <span className="inline-flex shrink-0 rounded-full border border-border bg-background px-3 py-1 text-xs text-muted-foreground">
          {copy.currentVersion}: v{entry.version}
        </span>
      </div>

      <form className="mt-5 grid gap-4" onSubmit={submitProposal}>
        <label className="grid gap-2 text-sm font-medium text-foreground">
          {copy.titleLabel}
          <input
            className="rounded-2xl border border-border bg-background px-3 py-2.5 text-sm shadow-sm"
            maxLength={200}
            onChange={(event) => setTitle(event.target.value)}
            required
            value={title}
          />
        </label>

        <label className="grid gap-2 text-sm font-medium text-foreground">
          {copy.contentLabel}
          <textarea
            className="min-h-72 rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
            maxLength={80000}
            onChange={(event) => setContent(event.target.value)}
            required
            value={content}
          />
        </label>

        <label className="grid gap-2 text-sm font-medium text-foreground">
          {copy.summaryLabel}
          <textarea
            className="min-h-24 rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
            maxLength={500}
            onChange={(event) => setSummary(event.target.value)}
            placeholder={copy.summaryPlaceholder}
            value={summary}
          />
        </label>

        {message ? <p className="rounded-2xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700">{message}</p> : null}
        {error ? <p className="rounded-2xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</p> : null}
        {needsLogin ? (
          <Link className="inline-flex w-fit rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
            {copy.login}
          </Link>
        ) : null}

        <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
          <button
            className="rounded-xl border border-border px-4 py-2 text-sm font-medium hover:bg-muted"
            disabled={submitting}
            onClick={resetToCurrent}
            type="button"
          >
            {copy.reset}
          </button>
          <button
            className="inline-flex items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] disabled:cursor-not-allowed disabled:opacity-60"
            disabled={submitting}
            type="submit"
          >
            {submitting ? <Loader2 className="mr-2 size-4 animate-spin" aria-hidden="true" /> : null}
            {submitting ? copy.submitting : copy.submit}
          </button>
        </div>
      </form>
    </section>
  );
}

function BoundaryPanel({ entry, message, showLogin = false }: { entry: WikiEntry; message: string; showLogin?: boolean }) {
  return (
    <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <PenLine className="size-5 text-primary" aria-hidden="true" />
            <h2 className="text-xl font-semibold tracking-tight">{copy.title}</h2>
          </div>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">{message}</p>
        </div>
        <span className="inline-flex shrink-0 rounded-full border border-border bg-background px-3 py-1 text-xs text-muted-foreground">
          {copy.currentVersion}: v{entry.version}
        </span>
      </div>
      {showLogin ? (
        <Link className="mt-4 inline-flex w-fit rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
          {copy.login}
        </Link>
      ) : null}
    </section>
  );
}

function canPropose(role: string) {
  return role === "creator" || role === "admin" || role === "super_admin";
}

function formatError(message: string) {
  const labels: Record<string, string> = {
    unauthorized: copy.loginRequired,
    forbidden: copy.forbidden,
    user_frozen: copy.forbidden,
    proposal_unchanged: copy.unchanged,
    missing_required_fields: copy.missingRequired,
    title_too_long: copy.titleTooLong,
    content_too_long: copy.contentTooLong,
    summary_too_long: copy.summaryTooLong,
  };
  return labels[message] ?? message ?? copy.failed;
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
