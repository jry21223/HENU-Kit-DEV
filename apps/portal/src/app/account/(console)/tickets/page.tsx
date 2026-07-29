"use client";

import { type FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { useAccountConsoleUnauthorizedHandler } from "@/components/account/account-console-session";
import { useReveal } from "@/components/account/use-reveal";
import {
  createAccountTicket,
  createAccountTicketFollowUp,
  fetchAccountTicket,
  fetchAccountTickets,
  formatPortalError,
  PortalHttpError,
} from "@/lib/api/client";
import type {
  AccountTicket,
  AccountTicketDetailResponse,
  AccountTicketStatus,
  AccountTicketsResponse,
} from "@/lib/api/types";

type ListState =
  | { kind: "loading" }
  | { kind: "success"; response: AccountTicketsResponse }
  | { kind: "error"; message: string };

type DetailState =
  | { kind: "idle" }
  | { kind: "loading"; ticketID: string }
  | { kind: "success"; response: AccountTicketDetailResponse }
  | { kind: "error"; ticketID: string; message: string };

const CATEGORY_OPTIONS = [
  { value: "practice", label: "刷题与学习记录" },
  { value: "account", label: "账户与会员" },
  { value: "feedback", label: "内容纠错" },
  { value: "other", label: "其他问题" },
] as const;

function commandKey(prefix: string): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `${prefix}:${crypto.randomUUID()}`;
  }
  return `${prefix}:${Date.now().toString(36)}:${Math.random().toString(36).slice(2)}`;
}

function statusLabel(status: AccountTicketStatus): string {
  switch (status) {
    case "open":
      return "待处理";
    case "in_progress":
      return "处理中";
    case "resolved":
      return "已解决";
  }
}

function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function updateTicketList(
  state: ListState,
  ticket: AccountTicket,
  requestID: string,
  prepend = false
): ListState {
  const existingTickets = state.kind === "success" ? state.response.data.tickets : [];
  const remaining = existingTickets.filter((item) => item.id !== ticket.id);
  return {
    kind: "success",
    response: {
      data: { tickets: prepend ? [ticket, ...remaining] : [...remaining, ticket] },
      request_id: requestID,
    },
  };
}

export default function TicketsPage() {
  const [listState, setListState] = useState<ListState>({ kind: "loading" });
  const [detailState, setDetailState] = useState<DetailState>({ kind: "idle" });
  const [createOpen, setCreateOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [category, setCategory] = useState<(typeof CATEGORY_OPTIONS)[number]["value"]>("practice");
  const [body, setBody] = useState("");
  const [createPending, setCreatePending] = useState(false);
  const [createError, setCreateError] = useState("");
  const [followUp, setFollowUp] = useState("");
  const [followUpPending, setFollowUpPending] = useState(false);
  const [followUpError, setFollowUpError] = useState("");
  const listRequestVersion = useRef(0);
  const detailRequestVersion = useRef(0);
  const createKeyRef = useRef<string | null>(null);
  const followUpKeyRef = useRef<string | null>(null);
  const handleUnauthorized = useAccountConsoleUnauthorizedHandler();
  useReveal();

  const loadTickets = useCallback(() => {
    const requestVersion = ++listRequestVersion.current;
    void fetchAccountTickets().then(
      (response) => {
        if (requestVersion === listRequestVersion.current) {
          setListState({ kind: "success", response });
        }
      },
      (error: unknown) => {
        if (requestVersion === listRequestVersion.current && !handleUnauthorized(error)) {
          setListState({ kind: "error", message: formatPortalError(error) });
        }
      }
    );
  }, [handleUnauthorized]);

  const loadTicket = useCallback((ticketID: string) => {
    const requestVersion = ++detailRequestVersion.current;
    setDetailState({ kind: "loading", ticketID });
    void fetchAccountTicket(ticketID).then(
      (response) => {
        if (requestVersion === detailRequestVersion.current) {
          setDetailState({ kind: "success", response });
        }
      },
      (error: unknown) => {
        if (requestVersion === detailRequestVersion.current && !handleUnauthorized(error)) {
          setDetailState({ kind: "error", ticketID, message: formatPortalError(error) });
        }
      }
    );
  }, [handleUnauthorized]);

  const applyTicketCommand = useCallback((ticket: AccountTicket, requestID: string, prepend = false) => {
    // A delayed list read started before this acknowledged write must not erase
    // the durable ticket returned by the command response.
    listRequestVersion.current += 1;
    setListState((current) => updateTicketList(current, ticket, requestID, prepend));
  }, []);

  useEffect(() => {
    loadTickets();
    return () => {
      listRequestVersion.current += 1;
      detailRequestVersion.current += 1;
    };
  }, [loadTickets]);

  const resetCreateKeyOnEdit = () => {
    if (!createPending) createKeyRef.current = null;
    setCreateError("");
  };

  const submitCreate = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const cleanedTitle = title.trim();
    const cleanedBody = body.trim();
    if (!cleanedTitle || !cleanedBody) {
      setCreateError("请填写工单标题和问题说明。");
      return;
    }

    const idempotencyKey = createKeyRef.current ?? commandKey("portal-ticket");
    createKeyRef.current = idempotencyKey;
    setCreatePending(true);
    setCreateError("");
    try {
      const response = await createAccountTicket(
        { title: cleanedTitle, category, body: cleanedBody },
        idempotencyKey
      );
      createKeyRef.current = null;
      applyTicketCommand(response.data.ticket, response.request_id, true);
      setTitle("");
      setBody("");
      setCreateOpen(false);
      loadTicket(response.data.ticket.id);
    } catch (error) {
      if (!handleUnauthorized(error)) setCreateError(formatPortalError(error));
    } finally {
      setCreatePending(false);
    }
  };

  const resetFollowUpKeyOnEdit = () => {
    if (!followUpPending) followUpKeyRef.current = null;
    setFollowUpError("");
  };

  const submitFollowUp = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (detailState.kind !== "success") return;
    const message = followUp.trim();
    if (!message) {
      setFollowUpError("请填写补充说明。");
      return;
    }

    const ticket = detailState.response.data.ticket;
    const idempotencyKey = followUpKeyRef.current ?? commandKey("portal-followup");
    followUpKeyRef.current = idempotencyKey;
    setFollowUpPending(true);
    setFollowUpError("");
    try {
      const response = await createAccountTicketFollowUp(
        ticket.id,
        { body: message, expected_version: ticket.version },
        idempotencyKey
      );
      followUpKeyRef.current = null;
      applyTicketCommand(response.data.ticket, response.request_id);
      setFollowUp("");
      loadTicket(ticket.id);
    } catch (error) {
      if (handleUnauthorized(error)) {
        return;
      }
      if (error instanceof PortalHttpError && error.status === 409) {
        followUpKeyRef.current = null;
        setFollowUpError("工单刚刚发生更新，已刷新最新记录后再试。");
        loadTicket(ticket.id);
      } else {
        setFollowUpError(formatPortalError(error));
      }
    } finally {
      setFollowUpPending(false);
    }
  };

  const tickets = listState.kind === "success" ? listState.response.data.tickets : [];

  return (
    <div>
      <section data-enter className="flex flex-wrap items-end justify-between gap-4 border-b border-ink pb-5">
        <div>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/55">
            <span className="text-accent">A-05</span>
            <span className="mx-2">/</span>
            SUPPORT TICKETS
          </p>
          <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">工单</h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-ink/60">
            这里的每条记录都来自持久化客服工单；提交或追问失败时不会显示本地成功结果。
          </p>
        </div>
        <button
          type="button"
          onClick={() => {
            setCreateOpen((open) => !open);
            setCreateError("");
          }}
          className="inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          aria-expanded={createOpen}
        >
          {createOpen ? "收起表单" : "新建工单"}
        </button>
      </section>

      {createOpen ? (
        <form
          data-account-ticket-create-state={createPending ? "pending" : "ready"}
          onSubmit={submitCreate}
          className="mt-6 border border-ink p-5 md:p-6"
        >
          <h2 className="font-display text-xl font-bold">提交问题</h2>
          <div className="mt-5 grid gap-4 md:grid-cols-[1fr_12rem]">
            <label className="block">
              <span className="font-mono text-[10px] tracking-[0.2em] text-ink/50">标题</span>
              <input
                required
                maxLength={160}
                value={title}
                onChange={(event) => {
                  resetCreateKeyOnEdit();
                  setTitle(event.target.value);
                }}
                className="mt-2 w-full border-b border-ink/30 bg-transparent px-0 py-2 text-sm outline-none transition-colors focus:border-ink"
                placeholder="简要说明你遇到的问题"
              />
            </label>
            <label className="block">
              <span className="font-mono text-[10px] tracking-[0.2em] text-ink/50">类别</span>
              <select
                value={category}
                onChange={(event) => {
                  resetCreateKeyOnEdit();
                  setCategory(event.target.value as (typeof CATEGORY_OPTIONS)[number]["value"]);
                }}
                className="mt-2 w-full border-b border-ink/30 bg-paper px-0 py-2 text-sm outline-none transition-colors focus:border-ink"
              >
                {CATEGORY_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <label className="mt-5 block">
            <span className="font-mono text-[10px] tracking-[0.2em] text-ink/50">问题说明</span>
            <textarea
              required
              maxLength={5000}
              value={body}
              onChange={(event) => {
                resetCreateKeyOnEdit();
                setBody(event.target.value);
              }}
              className="mt-2 min-h-32 w-full resize-y border border-ink/30 bg-transparent p-3 text-sm leading-6 outline-none transition-colors focus:border-ink"
              placeholder="请写明发生时间、页面和复现步骤；不要在工单中提交密码或验证码。"
            />
          </label>
          {createError ? (
            <p role="alert" className="mt-4 text-sm leading-6 text-accent">
              {createError}
            </p>
          ) : null}
          <div className="mt-5 flex flex-wrap gap-3">
            <button
              type="submit"
              disabled={createPending}
              className="inline-flex min-h-11 items-center justify-center border border-ink bg-ink px-4 py-2 font-mono text-xs tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent disabled:cursor-wait disabled:opacity-50"
            >
              {createPending ? "正在提交…" : "提交工单"}
            </button>
            <button
              type="button"
              onClick={() => setCreateOpen(false)}
              disabled={createPending}
              className="inline-flex min-h-11 items-center justify-center border border-ink/35 px-4 py-2 font-mono text-xs tracking-widest text-ink/65 transition-colors hover:border-ink hover:text-ink disabled:opacity-50"
            >
              取消
            </button>
          </div>
        </form>
      ) : null}

      {listState.kind === "loading" ? (
        <section
          data-account-tickets-state="loading"
          aria-live="polite"
          className="mt-6 border border-line px-5 py-8 font-mono text-xs tracking-[0.2em] text-ink/50"
        >
          TICKETS LOADING<span className="animate-pulse text-accent">…</span>
        </section>
      ) : null}

      {listState.kind === "error" ? (
        <section data-account-tickets-state="error" role="alert" className="mt-6 border border-accent px-5 py-6">
          <p className="font-mono text-xs tracking-[0.14em] text-accent">SUPPORT TICKETS UNAVAILABLE</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{listState.message}</p>
          <button
            type="button"
            onClick={() => {
              setListState({ kind: "loading" });
              loadTickets();
            }}
            className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {listState.kind === "success" ? (
        <section data-account-tickets-state="success" className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,0.9fr)_minmax(22rem,1.1fr)]">
          <div className="border-t border-ink">
            {tickets.length === 0 ? (
              <div data-account-tickets-empty className="border-b border-line py-8">
                <p className="font-display text-xl font-bold">暂无工单</p>
                <p className="mt-2 text-sm leading-6 text-ink/60">遇到需要人工处理的问题时，可以新建一条持久化工单。</p>
              </div>
            ) : (
              tickets.map((ticket) => {
                const selected = detailState.kind === "success" && detailState.response.data.ticket.id === ticket.id;
                return (
                  <button
                    key={ticket.id}
                    type="button"
                    onClick={() => loadTicket(ticket.id)}
                    className={`block w-full border-b border-line px-1 py-5 text-left transition-colors hover:bg-ink/5 ${
                      selected ? "bg-ink/5" : ""
                    }`}
                    aria-current={selected ? "page" : undefined}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <p className="min-w-0 truncate font-display text-lg font-bold">{ticket.title}</p>
                      <span className="shrink-0 border border-ink/30 px-2 py-1 font-mono text-[10px] tracking-wider text-ink/65">
                        {statusLabel(ticket.status)}
                      </span>
                    </div>
                    <p className="mt-2 font-mono text-[10px] tracking-[0.12em] text-ink/45">{ticket.reference}</p>
                    <p className="mt-2 font-mono text-[10px] text-ink/45">最后更新 {formatTimestamp(ticket.updated_at)}</p>
                  </button>
                );
              })
            )}
          </div>

          <TicketDetail
            state={detailState}
            followUp={followUp}
            followUpError={followUpError}
            followUpPending={followUpPending}
            onFollowUpChange={(value) => {
              resetFollowUpKeyOnEdit();
              setFollowUp(value);
            }}
            onSubmitFollowUp={submitFollowUp}
            onRetryDetail={() => {
              if (detailState.kind === "error") loadTicket(detailState.ticketID);
            }}
          />
        </section>
      ) : null}
    </div>
  );
}

function TicketDetail({
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
      <aside data-account-ticket-detail-state="idle" className="border border-line p-6 text-sm leading-6 text-ink/55">
        从左侧选择一条工单，即可查看完整记录并补充说明。
      </aside>
    );
  }
  if (state.kind === "loading") {
    return (
      <aside data-account-ticket-detail-state="loading" className="border border-line p-6 font-mono text-xs tracking-[0.16em] text-ink/50">
        TICKET DETAIL LOADING<span className="animate-pulse text-accent">…</span>
      </aside>
    );
  }
  if (state.kind === "error") {
    return (
      <aside data-account-ticket-detail-state="error" role="alert" className="border border-accent p-6">
        <p className="font-mono text-xs tracking-[0.14em] text-accent">TICKET DETAIL UNAVAILABLE</p>
        <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
        <button
          type="button"
          onClick={onRetryDetail}
          className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
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
          <p className="font-mono text-[10px] tracking-[0.15em] text-ink/45">{ticket.reference}</p>
          <h2 className="mt-2 font-display text-2xl font-bold">{ticket.title}</h2>
        </div>
        <span className="border border-accent px-2 py-1 font-mono text-[10px] tracking-wider text-accent">
          {statusLabel(ticket.status)}
        </span>
      </div>
      <p className="mt-3 font-mono text-[10px] tracking-[0.1em] text-ink/45">版本 {ticket.version} · 更新于 {formatTimestamp(ticket.updated_at)}</p>

      <div className="mt-6 space-y-3 border-y border-line py-5">
        {messages.length === 0 ? (
          <p className="text-sm text-ink/55">暂无消息记录。</p>
        ) : (
          messages.map((message) => (
            <article key={message.id} className="border-l-2 border-ink/20 pl-4">
              <p className="font-mono text-[10px] tracking-[0.15em] text-ink/50">
                {message.author_kind === "operator" ? "客服" : "我"} · {formatTimestamp(message.created_at)}
              </p>
              <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-ink/80">{message.body}</p>
            </article>
          ))
        )}
      </div>

      {events.length > 0 ? (
        <ul className="mt-5 space-y-2 border-b border-line pb-5 font-mono text-[10px] leading-5 text-ink/50">
          {events.map((event) => (
            <li key={event.id}>
              {event.kind === "reopened" ? "用户重新打开工单" : "工单状态更新"}：{statusLabel(event.from_status)} → {statusLabel(event.to_status)} · {formatTimestamp(event.created_at)}
            </li>
          ))}
        </ul>
      ) : null}

      <form onSubmit={onSubmitFollowUp} className="mt-6">
        <label className="block">
          <span className="font-mono text-[10px] tracking-[0.2em] text-ink/50">补充说明</span>
          <textarea
            required
            maxLength={5000}
            value={followUp}
            onChange={(event) => onFollowUpChange(event.target.value)}
            className="mt-2 min-h-28 w-full resize-y border border-ink/30 bg-transparent p-3 text-sm leading-6 outline-none transition-colors focus:border-ink"
            placeholder={ticket.status === "resolved" ? "若问题仍未解决，可补充后重新打开工单。" : "补充你的问题或最新情况。"}
          />
        </label>
        {followUpError ? (
          <p role="alert" className="mt-3 text-sm leading-6 text-accent">
            {followUpError}
          </p>
        ) : null}
        <button
          type="submit"
          disabled={followUpPending}
          className="mt-4 inline-flex min-h-11 items-center justify-center border border-ink bg-ink px-4 py-2 font-mono text-xs tracking-widest text-paper transition-colors hover:border-accent hover:bg-accent disabled:cursor-wait disabled:opacity-50"
        >
          {followUpPending ? "正在提交…" : "提交补充"}
        </button>
      </form>
    </aside>
  );
}
