"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ArrowRight, FileText, GitPullRequestDraft } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { apiBaseUrl, type MyWikiEntry, type MyWikiProposal } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
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

  useEffect(() => {
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

    void loadWikiSubmissions();
  }, []);

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
