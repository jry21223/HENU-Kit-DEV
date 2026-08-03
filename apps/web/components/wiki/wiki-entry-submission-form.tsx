"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { FileText, Loader2, PenLine } from "lucide-react";
import { apiBaseUrl, type Course, type User, type WikiEntry } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type WikiEntrySubmissionFormProps = {
  courses: Course[];
};

const copy = {
  title: "\u65b0\u5efa Wiki \u8bcd\u6761",
  intro:
    "\u65b0\u8bcd\u6761\u4e0d\u4f1a\u76f4\u63a5\u516c\u5f00\u3002\u63d0\u4ea4\u540e\u8fdb\u5165\u5ba1\u6838\uff0c\u901a\u8fc7\u540e\u624d\u4f1a\u51fa\u73b0\u5728\u516c\u5f00 Wiki \u5217\u8868\u3002",
  loadingUser: "\u6b63\u5728\u8bfb\u53d6\u8d26\u53f7\u6743\u9650...",
  loginRequired: "\u9700\u8981\u5148\u767b\u5f55\uff0c\u5e76\u83b7\u5f97\u521b\u4f5c\u8005\u6216\u7ba1\u7406\u5458\u6743\u9650\u540e\u624d\u80fd\u6295\u7a3f Wiki \u8bcd\u6761\u3002",
  forbidden:
    "\u5f53\u524d\u8d26\u53f7\u8fd8\u4e0d\u662f Wiki \u521b\u4f5c\u8005\u3002\u53ef\u4ee5\u5148\u8fd4\u56de Wiki \u9875\u9762\u63d0\u4ea4\u521b\u4f5c\u8005\u7533\u8bf7\u3002",
  login: "\u53bb\u767b\u5f55",
  apply: "\u7533\u8bf7\u521b\u4f5c\u8005",
  courseLabel: "\u5173\u8054\u8bfe\u7a0b",
  noCourse: "\u901a\u7528\u8bcd\u6761\uff08\u4e0d\u5173\u8054\u5177\u4f53\u8bfe\u7a0b\uff09",
  titleLabel: "\u8bcd\u6761\u6807\u9898",
  titlePlaceholder: "\u4f8b\uff1a\u79bb\u6563\u6570\u5b66\u56fe\u8bba\u9ad8\u9891\u8003\u70b9",
  slugLabel: "\u94fe\u63a5\u522b\u540d",
  slugPlaceholder: "\u4ec5\u652f\u6301\u5c0f\u5199\u5b57\u6bcd\u3001\u6570\u5b57\u548c\u8fde\u5b57\u7b26\uff0c\u4f8b\uff1adiscrete-graph-review",
  contentLabel: "\u6b63\u6587",
  contentPlaceholder: "\u5199\u5165\u7ed3\u6784\u5316\u5185\u5bb9\uff0c\u53ef\u5305\u542b\u77e5\u8bc6\u70b9\u3001\u6613\u9519\u70b9\u3001\u4f8b\u9898\u548c\u590d\u4e60\u5efa\u8bae\u3002",
  summaryLabel: "\u7248\u672c\u6458\u8981",
  summaryPlaceholder: "\u4f8b\uff1a\u521d\u59cb\u6295\u7a3f\uff0c\u8986\u76d6\u56fe\u8bba\u57fa\u7840\u4e0e\u5178\u578b\u9898\u578b",
  submit: "\u63d0\u4ea4\u5f85\u5ba1\u8bcd\u6761",
  submitting: "\u63d0\u4ea4\u4e2d...",
  submitted: "\u8bcd\u6761\u5df2\u63d0\u4ea4\uff0c\u5f53\u524d\u72b6\u6001\u4e3a\u5f85\u5ba1\u6838\u3002",
  reset: "\u6e05\u7a7a",
  missingRequired: "\u8bf7\u586b\u5199\u6807\u9898\u3001\u94fe\u63a5\u522b\u540d\u548c\u6b63\u6587\u3002",
  titleTooLong: "\u6807\u9898\u4e0d\u80fd\u8d85\u8fc7 200 \u4e2a\u5b57\u7b26\u3002",
  invalidSlug: "\u94fe\u63a5\u522b\u540d\u53ea\u80fd\u5305\u542b\u5c0f\u5199\u5b57\u6bcd\u3001\u6570\u5b57\u548c\u8fde\u5b57\u7b26\u3002",
  contentTooLong: "\u6b63\u6587\u4e0d\u80fd\u8d85\u8fc7 80000 \u4e2a\u5b57\u7b26\u3002",
  summaryTooLong: "\u7248\u672c\u6458\u8981\u4e0d\u80fd\u8d85\u8fc7 500 \u4e2a\u5b57\u7b26\u3002",
  courseNotFound: "\u9009\u62e9\u7684\u8bfe\u7a0b\u4e0d\u53ef\u7528\u6216\u672a\u53d1\u5e03\u3002",
  createFailed: "\u8bcd\u6761\u63d0\u4ea4\u5931\u8d25",
};

export function WikiEntrySubmissionForm({ courses }: WikiEntrySubmissionFormProps) {
  const [user, setUser] = useState<User | null>(null);
  const [authLoading, setAuthLoading] = useState(true);
  const [courseId, setCourseId] = useState("");
  const [title, setTitle] = useState("");
  const [slug, setSlug] = useState("");
  const [content, setContent] = useState("");
  const [summary, setSummary] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const canCreate = useMemo(() => Boolean(user && canCreateWiki(user.role)), [user]);

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

  function updateTitle(value: string) {
    setTitle(value);
    if (!slug.trim()) {
      setSlug(slugify(value));
    }
  }

  async function submitEntry(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setMessage("");
    setError("");
    try {
      const response = await request<{ entry: WikiEntry & { status?: string } }>("/wiki/entries", {
        method: "POST",
        body: JSON.stringify({
          courseId,
          title: title.trim(),
          slug: slug.trim(),
          content: content.trim(),
          summary: summary.trim(),
        }),
      });
      if (response.data?.entry) {
        setMessage(copy.submitted);
        resetForm(false);
      }
    } catch (err) {
      setError(err instanceof Error ? formatError(err.message) : copy.createFailed);
    } finally {
      setSubmitting(false);
    }
  }

  function resetForm(clearMessages = true) {
    setCourseId("");
    setTitle("");
    setSlug("");
    setContent("");
    setSummary("");
    if (clearMessages) {
      setMessage("");
      setError("");
    }
  }

  if (authLoading) {
    return <BoundaryPanel message={copy.loadingUser} />;
  }

  if (!user) {
    return <BoundaryPanel actionHref="/login" actionLabel={copy.login} message={copy.loginRequired} />;
  }

  if (!canCreate) {
    return <BoundaryPanel actionHref="/wiki" actionLabel={copy.apply} message={copy.forbidden} />;
  }

  return (
    <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6" data-testid="wiki-entry-submission-form">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <PenLine className="size-5 text-primary" aria-hidden="true" />
            <h2 className="text-xl font-semibold tracking-tight">{copy.title}</h2>
          </div>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
        </div>
        <span className="inline-flex w-fit shrink-0 rounded-full border border-border bg-background px-3 py-1 text-xs text-muted-foreground">
          {roleLabel(user.role)}
        </span>
      </div>

      <form className="mt-5 grid gap-4" onSubmit={submitEntry}>
        <label className="grid gap-2 text-sm font-medium text-foreground">
          {copy.courseLabel}
          <select
            className="rounded-2xl border border-border bg-background px-3 py-2.5 text-sm shadow-sm"
            data-testid="wiki-entry-course"
            onChange={(event) => setCourseId(event.target.value)}
            value={courseId}
          >
            <option value="">{copy.noCourse}</option>
            {courses.map((course) => (
              <option key={course.id} value={course.id}>
                {course.name} / {course.grade}
              </option>
            ))}
          </select>
        </label>

        <label className="grid gap-2 text-sm font-medium text-foreground">
          {copy.titleLabel}
          <input
            className="rounded-2xl border border-border bg-background px-3 py-2.5 text-sm shadow-sm"
            data-testid="wiki-entry-title"
            maxLength={200}
            onChange={(event) => updateTitle(event.target.value)}
            placeholder={copy.titlePlaceholder}
            required
            value={title}
          />
        </label>

        <label className="grid gap-2 text-sm font-medium text-foreground">
          {copy.slugLabel}
          <input
            className="rounded-2xl border border-border bg-background px-3 py-2.5 text-sm shadow-sm"
            data-testid="wiki-entry-slug"
            maxLength={220}
            onChange={(event) => setSlug(slugify(event.target.value))}
            placeholder={copy.slugPlaceholder}
            required
            value={slug}
          />
        </label>

        <label className="grid gap-2 text-sm font-medium text-foreground">
          {copy.contentLabel}
          <textarea
            className="min-h-72 rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
            data-testid="wiki-entry-content"
            maxLength={80000}
            onChange={(event) => setContent(event.target.value)}
            placeholder={copy.contentPlaceholder}
            required
            value={content}
          />
        </label>

        <label className="grid gap-2 text-sm font-medium text-foreground">
          {copy.summaryLabel}
          <textarea
            className="min-h-24 rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
            data-testid="wiki-entry-summary"
            maxLength={500}
            onChange={(event) => setSummary(event.target.value)}
            placeholder={copy.summaryPlaceholder}
            value={summary}
          />
        </label>

        {message ? <p className="rounded-2xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700">{message}</p> : null}
        {error ? <p className="rounded-2xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</p> : null}

        <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
          <button
            className="rounded-xl border border-border px-4 py-2 text-sm font-medium hover:bg-muted"
            disabled={submitting}
            onClick={() => resetForm()}
            type="button"
          >
            {copy.reset}
          </button>
          <button
            className="inline-flex items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] disabled:cursor-not-allowed disabled:opacity-60"
            data-testid="wiki-entry-submit"
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

function BoundaryPanel({ message, actionHref, actionLabel }: { message: string; actionHref?: string; actionLabel?: string }) {
  return (
    <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
      <div className="flex items-start gap-3">
        <FileText className="mt-0.5 size-5 shrink-0 text-primary" aria-hidden="true" />
        <div>
          <h2 className="text-xl font-semibold tracking-tight">{copy.title}</h2>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">{message}</p>
          {actionHref && actionLabel ? (
            <Link className="mt-4 inline-flex rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground" href={actionHref}>
              {actionLabel}
            </Link>
          ) : null}
        </div>
      </div>
    </section>
  );
}

function canCreateWiki(role: string) {
  return role === "creator" || role === "admin" || role === "super_admin";
}

function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 220);
}

function formatError(message: string) {
  const labels: Record<string, string> = {
    unauthorized: copy.loginRequired,
    forbidden: copy.forbidden,
    user_frozen: copy.forbidden,
    missing_required_fields: copy.missingRequired,
    title_too_long: copy.titleTooLong,
    invalid_slug: copy.invalidSlug,
    content_too_long: copy.contentTooLong,
    summary_too_long: copy.summaryTooLong,
    course_not_found: copy.courseNotFound,
    create_failed: copy.createFailed,
  };
  return labels[message] ?? message ?? copy.createFailed;
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
