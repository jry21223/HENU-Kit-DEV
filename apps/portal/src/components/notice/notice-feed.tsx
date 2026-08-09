"use client";

import { useCallback, useEffect, useState } from "react";
import { redirectToLogin, fetchPortalNotices, PortalHttpError, PortalUnauthorizedError } from "@/lib/api/client";
import type { PortalNoticeFeed } from "@/lib/api/types";

type NoticeFeedState =
  | { status: "loading" }
  | { status: "ready"; data: PortalNoticeFeed }
  | { status: "signed-out" }
  | { status: "denied" }
  | { status: "error" };

function intakeTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}

export default function NoticeFeed() {
  const [state, setState] = useState<NoticeFeedState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);

  const load = useCallback(async () => {
    setState({ status: "loading" });
    try {
      const response = await fetchPortalNotices();
      setState({ status: "ready", data: response.data });
    } catch (error) {
      if (error instanceof PortalUnauthorizedError) {
        setState({ status: "signed-out" });
        return;
      }
      if (error instanceof PortalHttpError && error.status === 403) {
        setState({ status: "denied" });
        return;
      }
      setState({ status: "error" });
    }
  }, []);

  useEffect(() => {
    void load();
  }, [attempt, load]);

  const retry = () => setAttempt((value) => value + 1);

  return (
    <main className="mx-auto min-h-svh max-w-5xl px-5 pb-16 pt-28 md:px-8 md:pb-24 md:pt-32">
      <header className="border-b border-ink pb-7">
        <p className="font-mono text-xs tracking-[0.28em] text-ink/65">
          <span className="font-bold text-ink">NOTICE</span>
          <span className="mx-2">/</span>
          CAMPUS IN-APP
        </p>
        <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">通知</h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-ink/65">
          这里展示面向全校学生的站内通知。来源链接可供核对，正文在本页展开查看。
        </p>
      </header>

      {state.status === "loading" && (
        <section data-testid="notice-feed-loading" role="status" aria-live="polite" aria-atomic="true" className="mt-8 border border-dashed border-ink/30 px-5 py-16 text-center font-mono text-base tracking-[0.18em] text-ink/65">
          正在加载通知
        </section>
      )}

      {state.status === "signed-out" && (
        <section data-testid="notice-feed-signed-out" className="mt-8 border border-ink/25 p-7 md:p-9">
          <div role="status" aria-live="polite" aria-atomic="true">
            <h2 className="font-display text-2xl font-bold">登录后查看通知</h2>
            <p className="mt-3 max-w-xl text-base leading-7 text-ink/65">请先登录，再查看面向全校学生的站内通知。</p>
          </div>
          <button type="button" onClick={() => redirectToLogin("/notice")} className="mt-6 inline-flex min-h-11 items-center border border-ink px-4 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper">
            去登录
          </button>
        </section>
      )}

      {state.status === "denied" && (
        <section data-testid="notice-feed-denied" role="status" aria-live="polite" aria-atomic="true" className="mt-8 border border-ink/25 p-7 md:p-9">
          <h2 className="font-display text-2xl font-bold">暂时无法查看通知</h2>
          <p className="mt-3 text-base leading-7 text-ink/65">你暂时没有查看通知的权限，请联系管理员。</p>
        </section>
      )}

      {state.status === "error" && (
        <section data-testid="notice-feed-error" className="mt-8 border border-accent/60 bg-accent/5 p-7 md:p-9">
          <div role="alert" aria-live="assertive" aria-atomic="true">
            <h2 className="font-display text-2xl font-bold">通知暂时无法加载</h2>
            <p className="mt-3 text-base leading-7 text-ink/65">请稍后重试。</p>
          </div>
          <button type="button" onClick={retry} className="mt-6 inline-flex min-h-11 items-center border border-ink px-4 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper">
            重新加载
          </button>
        </section>
      )}

      {state.status === "ready" && (state.data.notices.length === 0 ? (
        <section data-testid="notice-feed-empty" role="status" aria-live="polite" aria-atomic="true" className="mt-8 border border-dashed border-ink/30 px-5 py-16 text-center font-mono text-base tracking-[0.14em] text-ink/65">
          暂时没有可查看的通知。
        </section>
      ) : (
        <section data-testid="notice-feed-ready" className="mt-8 divide-y divide-line border-y border-line">
          <p data-testid="notice-feed-ready-status" role="status" aria-live="polite" aria-atomic="true" className="sr-only">
            已加载 {state.data.notices.length} 条通知
          </p>
          {state.data.notices.map((notice, index) => (
            <article key={notice.id} className="grid min-w-0 gap-5 py-7 md:grid-cols-[4.5rem_minmax(0,1fr)] md:gap-8 md:py-9">
              <p className="font-mono text-xs tracking-widest text-ink/65">N-{String(index + 1).padStart(2, "0")}</p>
              <div className="min-w-0">
                <h2 className="min-w-0 break-words font-display text-2xl font-bold leading-tight md:text-3xl">{notice.title}</h2>
                <dl className="mt-4 flex min-w-0 flex-wrap gap-x-5 gap-y-2 font-mono text-sm tracking-wide text-ink/65">
                  <div className="flex min-w-0 max-w-full gap-2"><dt className="shrink-0">来源</dt><dd className="min-w-0 break-words">{notice.source.name}</dd></div>
                  <div className="flex min-w-0 max-w-full gap-2"><dt className="shrink-0">收录时间</dt><dd className="min-w-0 break-words">{intakeTime(notice.created_at)}</dd></div>
                </dl>
                <details className="mt-5 min-w-0 border-l-2 border-accent pl-4">
                  <summary className="min-h-11 min-w-0 cursor-pointer break-words py-3 font-mono text-xs leading-5 tracking-widest text-ink/75">查看详情</summary>
                  <p className="mt-4 min-w-0 break-words whitespace-pre-wrap text-base leading-7 text-ink/75">{notice.body}</p>
                </details>
                <a href={notice.source.url} target="_blank" rel="noreferrer" className="mt-5 inline-flex min-h-11 min-w-0 max-w-full items-center break-words font-mono text-xs tracking-widest text-ink/70 underline decoration-accent underline-offset-4 transition-colors hover:text-ink">
                  <span className="min-w-0 break-all">查看来源：{notice.source.name}（新标签页打开）</span>
                </a>
              </div>
            </article>
          ))}
        </section>
      ))}
    </main>
  );
}
