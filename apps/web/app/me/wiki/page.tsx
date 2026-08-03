"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { ArrowRight, FileText, GitPullRequestDraft } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { apiBaseUrl, type MyWikiEntry, type MyWikiProposal } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type EditingTarget =
  | {
      kind: "entry";
      id: string;
      courseId?: string;
      title: string;
      slug: string;
      content: string;
    }
  | {
      kind: "proposal";
      id: string;
      title: string;
      content: string;
      summary: string;
    };

const copy = {
  back: "\u8fd4\u56de\u4e2a\u4eba\u4e2d\u5fc3",
  wiki: "Wiki",
  eyebrow: "\u6211\u7684 Wiki",
  title: "\u8bcd\u6761\u6295\u7a3f\u4e0e\u4fee\u8ba2\u63d0\u6848",
  intro:
    "\u8fd9\u91cc\u53ea\u5c55\u793a\u5f53\u524d\u8d26\u53f7\u63d0\u4ea4\u7684 Wiki \u8bcd\u6761\u548c\u4fee\u8ba2\u63d0\u6848\u3002\u5f85\u5ba1\u548c\u9a73\u56de\u5185\u5bb9\u53ea\u5bf9\u672c\u4eba\u53ef\u89c1\uff0c\u516c\u5f00 Wiki \u4ecd\u53ea\u5c55\u793a\u5df2\u53d1\u5e03\u5185\u5bb9\u3002",
  entries: "\u6211\u7684\u8bcd\u6761",
  proposals: "\u6211\u7684\u4fee\u8ba2\u63d0\u6848",
  loading: "\u6b63\u5728\u52a0\u8f7d Wiki \u6295\u7a3f\u8bb0\u5f55...",
  login: "\u53bb\u767b\u5f55",
  emptyEntries: "\u6682\u65e0\u81ea\u5df1\u63d0\u4ea4\u7684 Wiki \u8bcd\u6761\u3002",
  emptyProposals: "\u6682\u65e0\u81ea\u5df1\u63d0\u4ea4\u7684 Wiki \u4fee\u8ba2\u63d0\u6848\u3002",
  reviewReason: "\u5ba1\u6838\u8bf4\u660e",
  publishedLink: "\u67e5\u770b\u516c\u5f00\u9875",
  notPublic: "\u672a\u516c\u5f00",
  version: "\u7248\u672c",
  baseVersion: "\u57fa\u51c6\u7248\u672c",
  updatedAt: "\u66f4\u65b0",
  fallbackError: "Wiki \u6295\u7a3f\u8bb0\u5f55\u6682\u65f6\u4e0d\u53ef\u7528",
  edit: "\u4fee\u6539\u5e76\u91cd\u65b0\u63d0\u4ea4",
  cancel: "\u53d6\u6d88",
  save: "\u4fdd\u5b58\u91cd\u63d0",
  saving: "\u63d0\u4ea4\u4e2d...",
  resubmitted: "\u5df2\u91cd\u65b0\u63d0\u4ea4\u5ba1\u6838\u3002",
  titleLabel: "\u6807\u9898",
  slugLabel: "URL Slug",
  contentLabel: "\u5185\u5bb9",
  summaryLabel: "\u4fee\u8ba2\u6458\u8981",
  editHint: "\u4fee\u6539\u540e\u4f1a\u56de\u5230\u5f85\u5ba1\u72b6\u6001\uff0c\u65e7\u5ba1\u6838\u8bf4\u660e\u4f1a\u88ab\u6e05\u7a7a\u3002",
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

export default function MyWikiPage() {
  const [entries, setEntries] = useState<MyWikiEntry[]>([]);
  const [proposals, setProposals] = useState<MyWikiProposal[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [editing, setEditing] = useState<EditingTarget | null>(null);
  const [saving, setSaving] = useState(false);

  async function loadWikiSubmissions() {
    setLoading(true);
    setError("");
    try {
      const [entriesResponse, proposalsResponse] = await Promise.all([
        request<{ entries: MyWikiEntry[] }>("/me/wiki-entries"),
        request<{ proposals: MyWikiProposal[] }>("/me/wiki-proposals"),
      ]);
      setEntries(entriesResponse.data?.entries ?? []);
      setProposals(proposalsResponse.data?.proposals ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.fallbackError);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadWikiSubmissions();
  }, []);

  async function resubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editing) return;
    setSaving(true);
    setMessage("");
    setError("");
    try {
      if (editing.kind === "entry") {
        await request<{ entry: MyWikiEntry }>(`/me/wiki-entries/${editing.id}`, {
          method: "PATCH",
          body: JSON.stringify({
            courseId: editing.courseId ?? "",
            title: editing.title.trim(),
            slug: editing.slug.trim(),
            content: editing.content.trim(),
            summary: "resubmitted from my wiki page",
          }),
        });
      } else {
        await request<{ proposal: MyWikiProposal }>(`/me/wiki-proposals/${editing.id}`, {
          method: "PATCH",
          body: JSON.stringify({
            title: editing.title.trim(),
            content: editing.content.trim(),
            summary: editing.summary.trim(),
          }),
        });
      }
      setEditing(null);
      setMessage(copy.resubmitted);
      await loadWikiSubmissions();
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.fallbackError);
    } finally {
      setSaving(false);
    }
  }

  return (
    <SiteShell>
      <nav className="flex flex-wrap items-center justify-between gap-3 text-sm">
        <Link className="font-semibold text-primary" href="/me">
          {copy.back}
        </Link>
        <Link className="text-muted-foreground hover:text-foreground" href="/wiki">
          {copy.wiki}
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
      {message ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-foreground">{message}</p> : null}

      {!loading && !error ? (
        <div className="grid gap-5 lg:grid-cols-2">
          <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
            <SectionHeader count={entries.length} icon="entry" title={copy.entries} />
            <div className="mt-5 grid gap-3">
              {entries.map((entry) => (
                <article className="rounded-2xl border border-border bg-background p-4" key={entry.id}>
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge status={entry.status} />
                    <Badge tone="muted">
                      {copy.version} v{entry.version}
                    </Badge>
                  </div>
                  <h2 className="mt-3 break-words font-semibold">{entry.title}</h2>
                  <p className="mt-2 line-clamp-3 break-words text-sm leading-6 text-muted-foreground">{entry.content}</p>
                  <ReviewReason value={entry.reviewReason} />
                  {editing?.kind === "entry" && editing.id === entry.id ? (
                    <EntryEditForm editing={editing} onCancel={() => setEditing(null)} onChange={setEditing} onSubmit={resubmit} saving={saving} />
                  ) : (
                    <ResubmitAction
                      canEdit={canEditStatus(entry.status)}
                      onClick={() =>
                        setEditing({
                          kind: "entry",
                          id: entry.id,
                          courseId: entry.courseId,
                          title: entry.title,
                          slug: entry.slug,
                          content: entry.content,
                        })
                      }
                    />
                  )}
                  <SubmissionFooter href={entry.status === "published" ? `/wiki/${entry.id}` : ""} updatedAt={entry.updatedAt} />
                </article>
              ))}
              {entries.length === 0 ? <EmptyState label={copy.emptyEntries} /> : null}
            </div>
          </section>

          <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
            <SectionHeader count={proposals.length} icon="proposal" title={copy.proposals} />
            <div className="mt-5 grid gap-3">
              {proposals.map((proposal) => (
                <article className="rounded-2xl border border-border bg-background p-4" key={proposal.id}>
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge status={proposal.status} />
                    <Badge tone="muted">
                      {copy.baseVersion} v{proposal.baseVersion}
                    </Badge>
                    {proposal.entryStatus ? <Badge tone="muted">{statusLabel(proposal.entryStatus)}</Badge> : null}
                  </div>
                  <h2 className="mt-3 break-words font-semibold">{proposal.proposedTitle}</h2>
                  <p className="mt-1 text-xs text-muted-foreground">{proposal.entryTitle || proposal.entryId}</p>
                  <p className="mt-2 line-clamp-3 break-words text-sm leading-6 text-muted-foreground">{proposal.proposedContent}</p>
                  {proposal.summary ? <p className="mt-2 text-xs text-muted-foreground">{proposal.summary}</p> : null}
                  <ReviewReason value={proposal.reviewReason} />
                  {editing?.kind === "proposal" && editing.id === proposal.id ? (
                    <ProposalEditForm editing={editing} onCancel={() => setEditing(null)} onChange={setEditing} onSubmit={resubmit} saving={saving} />
                  ) : (
                    <ResubmitAction
                      canEdit={canEditStatus(proposal.status)}
                      onClick={() =>
                        setEditing({
                          kind: "proposal",
                          id: proposal.id,
                          title: proposal.proposedTitle,
                          content: proposal.proposedContent,
                          summary: proposal.summary,
                        })
                      }
                    />
                  )}
                  <SubmissionFooter href={proposal.entryStatus === "published" ? `/wiki/${proposal.entryId}` : ""} updatedAt={proposal.updatedAt} />
                </article>
              ))}
              {proposals.length === 0 ? <EmptyState label={copy.emptyProposals} /> : null}
            </div>
          </section>
        </div>
      ) : null}
    </SiteShell>
  );
}

function ResubmitAction({ canEdit, onClick }: { canEdit: boolean; onClick: () => void }) {
  if (!canEdit) return null;
  return (
    <button className="mt-3 rounded-xl border border-border px-3 py-2 text-sm font-medium text-foreground hover:bg-muted" onClick={onClick} type="button">
      {copy.edit}
    </button>
  );
}

function EntryEditForm({
  editing,
  onCancel,
  onChange,
  onSubmit,
  saving,
}: {
  editing: Extract<EditingTarget, { kind: "entry" }>;
  onCancel: () => void;
  onChange: (next: EditingTarget) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  saving: boolean;
}) {
  return (
    <form className="mt-4 grid gap-3 rounded-2xl border border-border bg-card p-3" onSubmit={onSubmit}>
      <p className="text-xs text-muted-foreground">{copy.editHint}</p>
      <label className="grid gap-1 text-sm font-medium">
        {copy.titleLabel}
        <input className="rounded-xl border border-border bg-background px-3 py-2 text-sm" maxLength={200} onChange={(event) => onChange({ ...editing, title: event.target.value })} required value={editing.title} />
      </label>
      <label className="grid gap-1 text-sm font-medium">
        {copy.slugLabel}
        <input className="rounded-xl border border-border bg-background px-3 py-2 text-sm" maxLength={220} onChange={(event) => onChange({ ...editing, slug: event.target.value })} required value={editing.slug} />
      </label>
      <label className="grid gap-1 text-sm font-medium">
        {copy.contentLabel}
        <textarea className="min-h-32 rounded-xl border border-border bg-background px-3 py-2 text-sm leading-6" maxLength={80000} onChange={(event) => onChange({ ...editing, content: event.target.value })} required value={editing.content} />
      </label>
      <FormActions onCancel={onCancel} saving={saving} />
    </form>
  );
}

function ProposalEditForm({
  editing,
  onCancel,
  onChange,
  onSubmit,
  saving,
}: {
  editing: Extract<EditingTarget, { kind: "proposal" }>;
  onCancel: () => void;
  onChange: (next: EditingTarget) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  saving: boolean;
}) {
  return (
    <form className="mt-4 grid gap-3 rounded-2xl border border-border bg-card p-3" onSubmit={onSubmit}>
      <p className="text-xs text-muted-foreground">{copy.editHint}</p>
      <label className="grid gap-1 text-sm font-medium">
        {copy.titleLabel}
        <input className="rounded-xl border border-border bg-background px-3 py-2 text-sm" maxLength={200} onChange={(event) => onChange({ ...editing, title: event.target.value })} required value={editing.title} />
      </label>
      <label className="grid gap-1 text-sm font-medium">
        {copy.contentLabel}
        <textarea className="min-h-32 rounded-xl border border-border bg-background px-3 py-2 text-sm leading-6" maxLength={80000} onChange={(event) => onChange({ ...editing, content: event.target.value })} required value={editing.content} />
      </label>
      <label className="grid gap-1 text-sm font-medium">
        {copy.summaryLabel}
        <textarea className="min-h-20 rounded-xl border border-border bg-background px-3 py-2 text-sm leading-6" maxLength={500} onChange={(event) => onChange({ ...editing, summary: event.target.value })} value={editing.summary} />
      </label>
      <FormActions onCancel={onCancel} saving={saving} />
    </form>
  );
}

function FormActions({ onCancel, saving }: { onCancel: () => void; saving: boolean }) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
      <button className="rounded-xl border border-border px-3 py-2 text-sm font-medium hover:bg-muted" disabled={saving} onClick={onCancel} type="button">
        {copy.cancel}
      </button>
      <button className="rounded-xl bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:opacity-60" disabled={saving} type="submit">
        {saving ? copy.saving : copy.save}
      </button>
    </div>
  );
}

function SectionHeader({ count, icon, title }: { count: number; icon: "entry" | "proposal"; title: string }) {
  const Icon = icon === "entry" ? FileText : GitPullRequestDraft;
  return (
    <div className="flex items-center gap-3">
      <span className="grid size-10 place-items-center rounded-2xl bg-primary text-primary-foreground">
        <Icon className="size-5" aria-hidden="true" />
      </span>
      <div>
        <h2 className="text-xl font-semibold">{title}</h2>
        <p className="text-sm text-muted-foreground">{count}</p>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const tone = status === "published" || status === "approved" ? "success" : "muted";
  return <Badge tone={tone}>{statusLabel(status)}</Badge>;
}

function ReviewReason({ value }: { value?: string }) {
  if (!value) return null;
  return (
    <p className="mt-3 rounded-xl border border-border bg-card p-3 text-sm text-muted-foreground">
      <span className="font-medium text-foreground">{copy.reviewReason}: </span>
      {value}
    </p>
  );
}

function SubmissionFooter({ href, updatedAt }: { href: string; updatedAt: string }) {
  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
      <span>
        {copy.updatedAt} {formatDate(updatedAt)}
      </span>
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

function EmptyState({ label }: { label: string }) {
  return <p className="rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">{label}</p>;
}

function statusLabel(status: string) {
  return statusLabels[status] ?? status;
}

function canEditStatus(status: string) {
  return status === "draft" || status === "pending" || status === "needs_changes" || status === "rejected";
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

async function request<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
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
    throw new Error(payload.message || "网络不太顺畅，请检查网络后重试");
  }
  return payload;
}
