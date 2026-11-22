import type { FormEvent } from "react";
import type { AccountTicketDetailResponse, AccountTicketStatus } from "@/lib/api/types";
import { Skeleton } from "@/components/ui/skeleton";
import { formatTimestamp } from "@/lib/format-date";

export type DetailState =
  | { kind: "idle" }
  | { kind: "loading"; ticketID: string }
  | { kind: "success"; response: AccountTicketDetailResponse }
  | { kind: "error"; ticketID: string; message: string };

export function statusLabel(status: AccountTicketStatus): string {
  switch (status) {
    case "open":
      return "待处理";
    case "in_progress":
      return "处理中";
    case "resolved":
      return "已解决";
  }
}

export default function TicketDetail({
  state,
  followUp,
  followUpError,
  followUpPending,
  onFollowUpChange,
  onSubmitFollowUp,
  onRetryDetail,
}: {
  state: DetailState;
  followUp: string;
  followUpError: string;
  followUpPending: boolean;
  onFollowUpChange: (value: string) => void;
  onSubmitFollowUp: (event: FormEvent<HTMLFormElement>) => void;
  onRetryDetail: () => void;
}) {
  if (state.kind === "idle") {
    return (
      <aside data-account-ticket-detail-state="idle" className="border border-line p-6 text-sm leading-6 text-ink/60">
        选择一条工单，即可查看完整记录并补充说明。
      </aside>
    );
  }
  if (state.kind === "loading") {
    return (
      <aside data-account-ticket-detail-state="loading" className="border border-line p-6 font-mono text-xs text-ink/60">
        工单详情加载中<span aria-hidden className="animate-pulse text-accent-text">…</span>
        <Skeleton className="mt-4 h-4 w-3/4" />
      </aside>
    );
  }
  if (state.kind === "error") {
    return (
      <aside data-account-ticket-detail-state="error" role="alert" className="border border-accent p-6">
        <p className="font-mono text-xs text-accent-text">工单详情加载失败</p>
        <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
        <button
          type="button"
          onClick={onRetryDetail}
          className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs transition-colors hover:bg-ink hover:text-paper"
        >
          重新加载
        </button>
      </aside>
    );
  }

  const { ticket, messages, events } = state.response.data;
  return (
    <aside data-account-ticket-detail-state="success" className="border border-ink p-5 md:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p title={ticket.reference} className="max-w-[13ch] truncate font-mono text-xs text-ink/60">{ticket.reference}</p>
          <h2 className="mt-2 font-display text-2xl font-bold">{ticket.title}</h2>
        </div>
        <span className="border border-accent px-2 py-1 font-mono text-xs text-accent-text">
          {statusLabel(ticket.status)}
        </span>
      </div>
      <p className="mt-3 font-mono text-xs text-ink/60">更新于 {formatTimestamp(ticket.updated_at)}</p>

      <div className="mt-6 space-y-3 border-y border-line py-5">
        {messages.length === 0 ? (
          <p className="text-sm text-ink/60">暂无消息记录。</p>
        ) : (
          messages.map((message) => (
            <article key={message.id} className="border-l-2 border-ink/20 pl-4">
              <p className="font-mono text-xs text-ink/60">
                {message.author_kind === "operator" ? "客服" : "我"} · {formatTimestamp(message.created_at)}
              </p>
              <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-ink/80">{message.body}</p>
            </article>
          ))
        )}
      </div>

      {events.length > 0 ? (
        <ul className="mt-5 space-y-2 border-b border-line pb-5 font-mono text-xs leading-5 text-ink/60">
          {events.map((event) => (
            <li key={event.id}>
              {event.kind === "reopened" ? "工单重新打开" : "工单状态更新"}：{statusLabel(event.from_status)} → {statusLabel(event.to_status)} · {formatTimestamp(event.created_at)}
            </li>
          ))}
        </ul>
      ) : null}

      <form onSubmit={onSubmitFollowUp} className="mt-6">
        <label className="block">
          <span className="font-mono text-xs text-ink/60">补充说明</span>
          <textarea
            required
            maxLength={5000}
            value={followUp}
            onChange={(event) => onFollowUpChange(event.target.value)}
            className="mt-2 min-h-28 w-full resize-y border border-ink/30 bg-transparent p-3 text-sm leading-6 outline-none transition-colors placeholder:text-ink/60 focus:border-ink"
            placeholder={ticket.status === "resolved" ? "若问题仍未解决，可补充后重新打开工单。" : "补充你的问题或最新情况。"}
          />
        </label>
        {followUpError ? (
          <p role="alert" className="mt-3 text-sm leading-6 text-accent-text">
            {followUpError}
          </p>
        ) : null}
        <button
          type="submit"
          disabled={followUpPending}
          className="mt-4 inline-flex min-h-11 items-center justify-center border border-ink bg-ink px-4 py-2 font-mono text-xs text-paper transition-colors hover:border-accent hover:bg-accent hover:text-ink disabled:cursor-wait disabled:opacity-50"
        >
          {followUpPending ? "正在提交…" : "提交补充"}
        </button>
      </form>
    </aside>
  );
}
