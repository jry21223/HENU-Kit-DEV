"use client";

import Link from "next/link";
import { FormEvent, useState } from "react";
import { Flag, Loader2, X } from "lucide-react";
import { apiBaseUrl } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type ReportButtonProps = {
  targetType: "material" | "wiki_entry" | "blog_post" | "forum_post" | "forum_reply";
  targetId: string;
  targetLabel: string;
  compact?: boolean;
};

type ReportResponse = {
  report: {
    id: string;
    status: string;
  };
  created: boolean;
};

const copy = {
  trigger: "\u4e3e\u62a5",
  title: "\u63d0\u4ea4\u4e3e\u62a5",
  reason: "\u4e3e\u62a5\u7406\u7531",
  description: "\u8be6\u7ec6\u8bf4\u660e",
  descriptionPlaceholder: "\u53ef\u9009\uff1a\u8bf4\u660e\u95ee\u9898\u6240\u5728\uff0c\u4fbf\u4e8e\u5ba1\u6838\u5458\u5904\u7406\u3002",
  copyright: "\u7591\u4f3c\u4fb5\u6743",
  spam: "\u5e7f\u544a\u6216\u5783\u573e\u4fe1\u606f",
  harmful: "\u4e0d\u5f53\u6216\u6709\u5bb3\u5185\u5bb9",
  incorrect: "\u5185\u5bb9\u660e\u663e\u9519\u8bef",
  other: "\u5176\u4ed6\u95ee\u9898",
  cancel: "\u53d6\u6d88",
  submit: "\u63d0\u4ea4",
  submitting: "\u63d0\u4ea4\u4e2d...",
  reasonRequired: "\u8bf7\u9009\u62e9\u4e3e\u62a5\u7406\u7531\u3002",
  submitted: "\u4e3e\u62a5\u5df2\u63d0\u4ea4\uff0c\u5ba1\u6838\u5458\u5904\u7406\u540e\u4f1a\u901a\u77e5\u4f60\u3002",
  duplicate: "\u4f60\u5df2\u63d0\u4ea4\u8fc7\u8be5\u76ee\u6807\u7684\u5f85\u5904\u7406\u4e3e\u62a5\u3002",
  loginRequired: "\u767b\u5f55\u540e\u624d\u80fd\u63d0\u4ea4\u4e3e\u62a5\u3002",
  login: "\u53bb\u767b\u5f55",
  failed: "\u4e3e\u62a5\u63d0\u4ea4\u5931\u8d25",
};

const reasons = [
  copy.copyright,
  copy.spam,
  copy.harmful,
  copy.incorrect,
  copy.other,
];

export function ReportButton({ targetType, targetId, targetLabel, compact = false }: ReportButtonProps) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState(reasons[0]);
  const [description, setDescription] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [needsLogin, setNeedsLogin] = useState(false);

  async function submitReport(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextReason = reason.trim();
    if (!nextReason) {
      setError(copy.reasonRequired);
      return;
    }
    setSubmitting(true);
    setMessage("");
    setError("");
    setNeedsLogin(false);
    try {
      const response = await request<ReportResponse>("/reports", {
        method: "POST",
        body: JSON.stringify({
          targetType,
          targetId,
          reason: nextReason,
          description: description.trim(),
        }),
      });
      setMessage(response.data?.created === false ? copy.duplicate : copy.submitted);
      setDescription("");
    } catch (err) {
      if (err instanceof Error && err.message === "unauthorized") {
        setNeedsLogin(true);
        setError(copy.loginRequired);
      } else {
        setError(err instanceof Error ? err.message : copy.failed);
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <>
      <button
        className={
          compact
            ? "inline-flex items-center rounded-xl border border-border bg-card px-3 py-2 text-xs font-medium text-muted-foreground transition hover:border-primary hover:text-primary"
            : "inline-flex w-full items-center justify-center rounded-xl border border-border bg-card px-4 py-2.5 text-sm font-medium text-muted-foreground transition hover:border-primary hover:text-primary"
        }
        onClick={() => {
          setOpen(true);
          setMessage("");
          setError("");
          setNeedsLogin(false);
        }}
        type="button"
      >
        <Flag className="mr-2 size-4" aria-hidden="true" />
        {copy.trigger}
      </button>

      {open ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <section className="w-full max-w-md rounded-3xl border border-border bg-card p-5 shadow-xl">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h2 className="text-lg font-semibold tracking-tight">{copy.title}</h2>
                <p className="mt-1 break-words text-sm text-muted-foreground">{targetLabel}</p>
              </div>
              <button
                className="rounded-full p-2 text-muted-foreground transition hover:bg-muted hover:text-foreground"
                onClick={() => setOpen(false)}
                type="button"
              >
                <X className="size-4" aria-hidden="true" />
                <span className="sr-only">{copy.cancel}</span>
              </button>
            </div>

            <form className="mt-5 grid gap-4" onSubmit={submitReport}>
              <label className="grid gap-2 text-sm font-medium text-foreground">
                {copy.reason}
                <select
                  className="rounded-2xl border border-border bg-background px-3 py-2.5 text-sm shadow-sm"
                  onChange={(event) => setReason(event.target.value)}
                  value={reason}
                >
                  {reasons.map((item) => (
                    <option key={item} value={item}>
                      {item}
                    </option>
                  ))}
                </select>
              </label>
              <label className="grid gap-2 text-sm font-medium text-foreground">
                {copy.description}
                <textarea
                  className="min-h-28 rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
                  maxLength={2000}
                  onChange={(event) => setDescription(event.target.value)}
                  placeholder={copy.descriptionPlaceholder}
                  value={description}
                />
              </label>
              {message ? <p className="rounded-2xl border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700">{message}</p> : null}
              {error ? <p className="rounded-2xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</p> : null}
              {needsLogin ? (
                <Link className="inline-flex rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
                  {copy.login}
                </Link>
              ) : null}
              <div className="flex justify-end gap-2">
                <button className="rounded-xl border border-border px-4 py-2 text-sm font-medium" onClick={() => setOpen(false)} type="button">
                  {copy.cancel}
                </button>
                <button
                  className="inline-flex items-center rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={submitting}
                  type="submit"
                >
                  {submitting ? <Loader2 className="mr-2 size-4 animate-spin" aria-hidden="true" /> : null}
                  {submitting ? copy.submitting : copy.submit}
                </button>
              </div>
            </form>
          </section>
        </div>
      ) : null}
    </>
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
