"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { FileText, Loader2, PenLine, ShieldCheck } from "lucide-react";
import { apiBaseUrl, type User, type WikiCreatorApplication } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

const copy = {
  title: "\u7533\u8bf7\u6210\u4e3a Wiki \u521b\u4f5c\u8005",
  intro:
    "\u666e\u901a\u7528\u6237\u53ef\u4ee5\u63d0\u4ea4\u521b\u4f5c\u8005\u7533\u8bf7\u548c\u8bd5\u7a3f\u3002\u901a\u8fc7\u5ba1\u6838\u540e\uff0c\u624d\u80fd\u53d1\u8d77 Wiki \u8bcd\u6761\u6295\u7a3f\u548c\u4fee\u8ba2\u63d0\u6848\u3002",
  loading: "\u6b63\u5728\u8bfb\u53d6\u521b\u4f5c\u8005\u7533\u8bf7\u72b6\u6001...",
  loginRequired: "\u767b\u5f55\u540e\u53ef\u4ee5\u67e5\u770b\u7533\u8bf7\u72b6\u6001\u5e76\u63d0\u4ea4\u8bd5\u7a3f\u3002",
  login: "\u53bb\u767b\u5f55",
  alreadyCreator: "\u5f53\u524d\u8d26\u53f7\u5df2\u5177\u5907 Wiki \u521b\u4f5c\u6743\u9650\uff0c\u53ef\u4ee5\u5728\u516c\u5f00\u8bcd\u6761\u8be6\u60c5\u9875\u63d0\u4ea4\u4fee\u8ba2\u63d0\u6848\u3002",
  elevatedRole:
    "\u5f53\u524d\u8d26\u53f7\u662f\u5ba1\u6838\u6216\u8fd0\u8425\u89d2\u8272\uff0c\u6682\u4e0d\u901a\u8fc7\u5b66\u751f\u7aef\u521b\u4f5c\u8005\u7533\u8bf7\u8f6c\u6362\u8eab\u4efd\u3002",
  latestStatus: "\u6700\u8fd1\u7533\u8bf7",
  reasonLabel: "\u7533\u8bf7\u7406\u7531",
  sampleTitleLabel: "\u8bd5\u7a3f\u6807\u9898",
  sampleBodyLabel: "\u8bd5\u7a3f\u5185\u5bb9",
  reasonPlaceholder: "\u8bf4\u660e\u4f60\u60f3\u5171\u5efa\u7684\u8bfe\u7a0b\u6216\u77e5\u8bc6\u70b9\uff0c\u4ee5\u53ca\u53ef\u4ee5\u6301\u7eed\u7ef4\u62a4\u7684\u5185\u5bb9\u65b9\u5411\u3002",
  sampleTitlePlaceholder: "\u4f8b\uff1a\u79bb\u6563\u6570\u5b66\u56fe\u8bba\u6613\u9519\u70b9\u6574\u7406",
  sampleBodyPlaceholder: "\u7c98\u8d34\u4e00\u6bb5\u4f60\u81ea\u5df1\u6574\u7406\u7684\u8bd5\u7a3f\uff0c\u5ba1\u6838\u5458\u4f1a\u6839\u636e\u7ed3\u6784\u3001\u51c6\u786e\u6027\u548c\u53ef\u8bfb\u6027\u5224\u65ad\u3002",
  submit: "\u63d0\u4ea4\u7533\u8bf7",
  submitting: "\u63d0\u4ea4\u4e2d...",
  submitted: "\u7533\u8bf7\u5df2\u63d0\u4ea4\uff0c\u8bf7\u7b49\u5f85\u5ba1\u6838\u3002",
  pendingHint: "\u7533\u8bf7\u6b63\u5728\u5ba1\u6838\u4e2d\uff0c\u6682\u65f6\u4e0d\u80fd\u91cd\u590d\u63d0\u4ea4\u3002",
  approvedHint: "\u7533\u8bf7\u5df2\u901a\u8fc7\u3002\u5237\u65b0\u767b\u5f55\u72b6\u6001\u540e\uff0c\u8d26\u53f7\u5c06\u5177\u5907\u521b\u4f5c\u8005\u6743\u9650\u3002",
  rejectedHint: "\u7533\u8bf7\u5df2\u88ab\u9a73\u56de\uff0c\u53ef\u4ee5\u6839\u636e\u5ba1\u6838\u610f\u89c1\u4fee\u6539\u540e\u91cd\u65b0\u63d0\u4ea4\u3002",
  missingRequired: "\u8bf7\u586b\u5199\u7533\u8bf7\u7406\u7531\u3001\u8bd5\u7a3f\u6807\u9898\u548c\u8bd5\u7a3f\u5185\u5bb9\u3002",
  reasonTooLong: "\u7533\u8bf7\u7406\u7531\u4e0d\u80fd\u8d85\u8fc7 1000 \u4e2a\u5b57\u7b26\u3002",
  sampleTitleTooLong: "\u8bd5\u7a3f\u6807\u9898\u4e0d\u80fd\u8d85\u8fc7 200 \u4e2a\u5b57\u7b26\u3002",
  sampleBodyTooLong: "\u8bd5\u7a3f\u5185\u5bb9\u4e0d\u80fd\u8d85\u8fc7 20000 \u4e2a\u5b57\u7b26\u3002",
  pendingDuplicate: "\u4f60\u5df2\u6709\u4e00\u4e2a\u5f85\u5ba1\u6838\u7684\u521b\u4f5c\u8005\u7533\u8bf7\u3002",
  alreadyCreatorError: "\u5f53\u524d\u8d26\u53f7\u5df2\u662f\u521b\u4f5c\u8005\u6216\u7ba1\u7406\u5458\u3002",
  failed: "\u521b\u4f5c\u8005\u7533\u8bf7\u63d0\u4ea4\u5931\u8d25",
  reviewedAt: "\u5ba1\u6838\u65f6\u95f4",
  reviewReason: "\u5ba1\u6838\u610f\u89c1",
  createdAt: "\u63d0\u4ea4\u65f6\u95f4",
  noReviewReason: "\u6682\u65e0\u5ba1\u6838\u610f\u89c1",
};

export function WikiCreatorApplicationPanel() {
  const [user, setUser] = useState<User | null>(null);
  const [applications, setApplications] = useState<WikiCreatorApplication[]>([]);
  const [loading, setLoading] = useState(true);
  const [reason, setReason] = useState("");
  const [sampleTitle, setSampleTitle] = useState("");
  const [sampleBody, setSampleBody] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const latest = applications[0];
  const activeApplication = useMemo(
    () => applications.find((item) => item.status === "pending" || item.status === "draft" || item.status === "needs_changes"),
    [applications],
  );

  useEffect(() => {
    async function load() {
      setLoading(true);
      try {
        const userResponse = await request<User>("/auth/me", { method: "GET" });
        const currentUser = userResponse.data ?? null;
        setUser(currentUser);
        if (currentUser) {
          const applicationsResponse = await request<{ applications: WikiCreatorApplication[] }>("/wiki/creator-applications/me", {
            method: "GET",
          });
          setApplications(applicationsResponse.data?.applications ?? []);
        }
      } catch {
        setUser(null);
        setApplications([]);
      } finally {
        setLoading(false);
      }
    }

    void load();
  }, []);

  async function submitApplication(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setMessage("");
    setError("");
    try {
      const response = await request<{ application: WikiCreatorApplication }>("/wiki/creator-applications", {
        method: "POST",
        body: JSON.stringify({
          reason: reason.trim(),
          sampleTitle: sampleTitle.trim(),
          sampleBody: sampleBody.trim(),
        }),
      });
      if (response.data?.application) {
        setApplications([response.data.application, ...applications]);
        setReason("");
        setSampleTitle("");
        setSampleBody("");
        setMessage(copy.submitted);
      }
    } catch (err) {
      setError(err instanceof Error ? formatError(err.message) : copy.failed);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6" data-testid="wiki-creator-application-panel">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <PenLine className="size-5 text-primary" aria-hidden="true" />
            <h2 className="text-xl font-semibold tracking-tight">{copy.title}</h2>
          </div>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
        </div>
        {latest ? (
          <span className="inline-flex w-fit shrink-0 rounded-full border border-border bg-background px-3 py-1 text-xs text-muted-foreground">
            {copy.latestStatus}: {statusLabel(latest.status)}
          </span>
        ) : null}
      </div>

      {loading ? <p className="mt-4 rounded-2xl border border-border bg-background p-3 text-sm text-muted-foreground">{copy.loading}</p> : null}

      {!loading && !user ? (
        <div className="mt-4 rounded-2xl border border-border bg-background p-4">
          <p className="text-sm leading-6 text-muted-foreground">{copy.loginRequired}</p>
          <Link className="mt-3 inline-flex rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground" href="/login">
            {copy.login}
          </Link>
        </div>
      ) : null}

      {!loading && user && isCreatorCapable(user.role) ? (
        <StatusNotice icon="shield" message={copy.alreadyCreator} tone="success" />
      ) : null}

      {!loading && user && !isCreatorCapable(user.role) && user.role !== "user" ? (
        <StatusNotice icon="file" message={copy.elevatedRole} tone="muted" />
      ) : null}

      {!loading && user && user.role === "user" && latest ? <ApplicationStatus application={latest} /> : null}

      {!loading && user && user.role === "user" && !activeApplication ? (
        <form className="mt-5 grid gap-4" onSubmit={submitApplication}>
          <label className="grid gap-2 text-sm font-medium text-foreground">
            {copy.reasonLabel}
            <textarea
              className="min-h-24 rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
              data-testid="wiki-creator-application-reason"
              maxLength={1000}
              onChange={(event) => setReason(event.target.value)}
              placeholder={copy.reasonPlaceholder}
              required
              value={reason}
            />
          </label>
          <label className="grid gap-2 text-sm font-medium text-foreground">
            {copy.sampleTitleLabel}
            <input
              className="rounded-2xl border border-border bg-background px-3 py-2.5 text-sm shadow-sm"
              data-testid="wiki-creator-application-sample-title"
              maxLength={200}
              onChange={(event) => setSampleTitle(event.target.value)}
              placeholder={copy.sampleTitlePlaceholder}
              required
              value={sampleTitle}
            />
          </label>
          <label className="grid gap-2 text-sm font-medium text-foreground">
            {copy.sampleBodyLabel}
            <textarea
              className="min-h-40 rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
              data-testid="wiki-creator-application-sample-body"
              maxLength={20000}
              onChange={(event) => setSampleBody(event.target.value)}
              placeholder={copy.sampleBodyPlaceholder}
              required
              value={sampleBody}
            />
          </label>

          {message ? <p className="rounded-2xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700">{message}</p> : null}
          {error ? <p className="rounded-2xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</p> : null}

          <div className="flex justify-end">
            <button
              className="inline-flex items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] disabled:cursor-not-allowed disabled:opacity-60"
              data-testid="wiki-creator-application-submit"
              disabled={submitting}
              type="submit"
            >
              {submitting ? <Loader2 className="mr-2 size-4 animate-spin" aria-hidden="true" /> : null}
              {submitting ? copy.submitting : copy.submit}
            </button>
          </div>
        </form>
      ) : null}

      {!loading && user && user.role === "user" && activeApplication ? (
        <p className="mt-4 rounded-2xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">{copy.pendingHint}</p>
      ) : null}
    </section>
  );
}

function ApplicationStatus({ application }: { application: WikiCreatorApplication }) {
  return (
    <div className="mt-4 rounded-2xl border border-border bg-background p-4">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <p className="text-sm font-semibold text-foreground">{application.sampleTitle}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {copy.createdAt}: {formatDate(application.createdAt)}
          </p>
        </div>
        <span className={`w-fit rounded-full px-3 py-1 text-xs ${statusClass(application.status)}`}>{statusLabel(application.status)}</span>
      </div>
      <p className="mt-3 text-sm leading-6 text-muted-foreground">{statusHint(application.status)}</p>
      {application.reviewedAt ? (
        <p className="mt-2 text-xs text-muted-foreground">
          {copy.reviewedAt}: {formatDate(application.reviewedAt)}
        </p>
      ) : null}
      {application.reviewReason ? (
        <p className="mt-2 rounded-xl border border-border bg-card p-3 text-sm text-muted-foreground">
          {copy.reviewReason}: {application.reviewReason}
        </p>
      ) : application.status === "rejected" ? (
        <p className="mt-2 rounded-xl border border-border bg-card p-3 text-sm text-muted-foreground">{copy.noReviewReason}</p>
      ) : null}
    </div>
  );
}

function StatusNotice({ icon, message, tone }: { icon: "shield" | "file"; message: string; tone: "success" | "muted" }) {
  const Icon = icon === "shield" ? ShieldCheck : FileText;
  const toneClass = tone === "success" ? "border-emerald-200 bg-emerald-50 text-emerald-800" : "border-border bg-background text-muted-foreground";
  return (
    <div className={`mt-4 flex gap-3 rounded-2xl border p-4 text-sm leading-6 ${toneClass}`}>
      <Icon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <p>{message}</p>
    </div>
  );
}

function isCreatorCapable(role: string) {
  return role === "creator" || role === "admin" || role === "super_admin";
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    draft: "\u8349\u7a3f",
    pending: "\u5f85\u5ba1\u6838",
    needs_changes: "\u9700\u4fee\u6539",
    approved: "\u5df2\u901a\u8fc7",
    rejected: "\u5df2\u9a73\u56de",
  };
  return labels[status] ?? status;
}

function statusHint(status: string) {
  if (status === "approved") return copy.approvedHint;
  if (status === "rejected") return copy.rejectedHint;
  return copy.pendingHint;
}

function statusClass(status: string) {
  if (status === "approved") return "bg-emerald-100 text-emerald-800";
  if (status === "rejected") return "bg-red-100 text-red-700";
  if (status === "pending" || status === "needs_changes") return "bg-amber-100 text-amber-800";
  return "bg-muted text-muted-foreground";
}

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatError(message: string) {
  const labels: Record<string, string> = {
    unauthorized: copy.loginRequired,
    user_frozen: copy.elevatedRole,
    creator_application_pending: copy.pendingDuplicate,
    already_creator: copy.alreadyCreatorError,
    creator_application_role_not_supported: copy.elevatedRole,
    missing_required_fields: copy.missingRequired,
    reason_too_long: copy.reasonTooLong,
    sample_title_too_long: copy.sampleTitleTooLong,
    sample_body_too_long: copy.sampleBodyTooLong,
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
    throw new Error(payload.message || "网络不太顺畅，请检查网络后重试");
  }
  return payload;
}
