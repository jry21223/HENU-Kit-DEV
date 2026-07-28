"use client";

import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useReveal } from "@/components/account/use-reveal";
import { fetchAccountSummary } from "@/lib/api/client";
import type { AccountSummaryResponse } from "@/lib/api/types";
import { authStore, type AuthUser } from "@/lib/auth/store";

type SummaryState =
  | { kind: "loading" }
  | { kind: "success"; summary: AccountSummaryResponse }
  | { kind: "error" };

export default function AccountOverviewPage() {
  const { user } = useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);

  // A change of authenticated subject must not reuse the prior subject's
  // account result while the new read is in flight. uid is only a React key;
  // it is never rendered as a public identifier.
  if (!user) return null;
  return <AccountOverviewContent key={user.uid} user={user} />;
}

function AccountOverviewContent({ user }: { user: AuthUser }) {
  const [state, setState] = useState<SummaryState>({ kind: "loading" });
  const requestVersion = useRef(0);
  useReveal();

  const loadSummary = useCallback(() => {
    const version = ++requestVersion.current;
    void fetchAccountSummary().then(
      (summary) => {
        if (version === requestVersion.current) setState({ kind: "success", summary });
      },
      () => {
        if (version === requestVersion.current) setState({ kind: "error" });
      }
    );
  }, []);

  useEffect(() => {
    void loadSummary();
    return () => {
      requestVersion.current += 1;
    };
  }, [loadSummary]);

  const data = state.kind === "success" ? state.summary.data : null;
  const membershipLabel = data
    ? data.plan === "lifetime"
      ? "终身会员"
      : "免费会员"
    : state.kind === "error"
      ? "账户状态不可用"
      : "账户状态加载中";
  const cards = data
    ? [
        { label: "积分余额", value: String(data.points_balance), mono: "C-01" },
        { label: "会员", value: membershipLabel, mono: "C-02" },
        { label: "未读通知", value: String(data.unread_notification_count), mono: "C-03" },
        { label: "进行中工单", value: String(data.open_ticket_count), mono: "C-04" },
      ]
    : [];

  return (
    <div>
      <section data-enter className="flex items-center gap-5 border border-ink p-6" aria-label="账户身份">
        <span className="bg-blueprint flex h-16 w-16 shrink-0 items-center justify-center border border-ink font-display text-3xl font-bold" aria-hidden>
          {user.name.slice(0, 1)}
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl font-bold">{user.name}</h1>
          {user.email ? (
            <p className="mt-1 truncate font-mono text-[10px] tracking-[0.15em] text-ink/50">{user.email}</p>
          ) : (
            <p className="mt-1 font-mono text-[10px] tracking-[0.15em] text-ink/50">ACCOUNT PORTFOLIO</p>
          )}
        </div>
        <span className="ml-auto shrink-0 border border-accent px-2 py-1 font-mono text-[10px] tracking-widest text-accent">
          {membershipLabel}
        </span>
      </section>

      {state.kind === "loading" ? (
        <section
          data-account-summary-state="loading"
          aria-live="polite"
          className="mt-6 border border-line px-5 py-8 font-mono text-xs tracking-[0.2em] text-ink/50"
        >
          ACCOUNT PORTFOLIO LOADING<span className="animate-pulse text-accent">…</span>
        </section>
      ) : null}

      {state.kind === "error" ? (
        <section
          data-account-summary-state="error"
          role="alert"
          className="mt-6 border border-accent px-5 py-6"
        >
          <p className="font-mono text-xs tracking-[0.14em] text-accent">ACCOUNT PORTFOLIO UNAVAILABLE</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">账户概览暂不可用，未展示任何本地或会话内替代数据。</p>
          <button
            type="button"
            onClick={() => {
              setState({ kind: "loading" });
              loadSummary();
            }}
            className="mt-5 border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {data ? (
        <section data-account-summary-state="success" aria-live="polite">
          <div className="mt-6 grid grid-cols-2 gap-4 md:grid-cols-4">
            {cards.map((card) => (
              <div
                key={card.mono}
                data-enter
                className="border border-ink/25 p-5"
              >
                <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
                  {card.mono} / {card.label}
                </p>
                <p className="mt-3 font-display text-3xl font-bold">{card.value}</p>
                <p className="mt-2 font-mono text-[10px] text-ink/40">详情功能分批接入中</p>
              </div>
            ))}
          </div>

          {data.unread_notification_count === 0 && data.open_ticket_count === 0 ? (
            <p data-enter className="mt-6 border-y border-line py-4 font-mono text-[11px] tracking-[0.12em] text-ink/50">
              暂无通知和进行中工单
            </p>
          ) : null}

          <p data-enter className="mt-8 font-mono text-[10px] tracking-[0.22em] text-ink/40">
            数据来自持久化 ACCOUNT PORTFOLIO · 新用户从 0 积分和免费会员开始
          </p>
        </section>
      ) : null}
    </div>
  );
}
