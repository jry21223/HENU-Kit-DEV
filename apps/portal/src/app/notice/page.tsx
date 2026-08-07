"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import { useReveal } from "@/components/account/use-reveal";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import NoticeFeed from "@/components/notice/notice-feed";
import { useScrollRestoration } from "@/components/use-scroll-restoration";
import {
  fetchNoticeFeed,
  formatPortalError,
  PortalUnauthorizedError,
} from "@/lib/api/client";
import type { NoticeFeedItem } from "@/lib/api/types";

type LoadState = "loading" | "ready" | "error" | "signed_out";

export default function NoticeBoardPage() {
  const [items, setItems] = useState<NoticeFeedItem[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    try {
      const envelope = await fetchNoticeFeed();
      // Gateway 已按生命周期过滤快照（只有 distributed 的公告才会离开
      // 网关），前端直接渲染即可，无需重复过滤。
      setItems(envelope.data.items);
      setLoadState("ready");
    } catch (cause) {
      if (cause instanceof PortalUnauthorizedError) {
        setLoadState("signed_out");
        return;
      }
      setItems([]);
      setError(formatPortalError(cause));
      setLoadState("error");
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  useReveal([loadState]);
  useScrollRestoration(loadState === "ready");

  return (
    <main>
      <section className="border-b border-ink bg-ink text-paper">
        <div className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
          <div data-enter>
            <p className="font-mono text-xs tracking-[0.3em] text-paper/55">
              <span className="text-accent">N-01</span>
              <span className="mx-2">/</span>
              NOTICE BOARD
            </p>
            <h1 className="mt-4 font-display text-5xl font-bold tracking-tight sm:text-6xl md:text-7xl">
              通知公告
            </h1>
            <p className="mt-5 max-w-2xl text-sm leading-7 text-paper/65 md:text-base">
              来自通知服务的真实公告：学校办公室、学院与社团发布的通知按公告创建时间在这里展示。
            </p>
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-[1440px] px-5 py-10 md:px-8 md:py-12">
        <div data-enter className="flex items-end justify-between border-b border-ink pb-6">
          <div>
            <p className="font-mono text-[10px] tracking-[0.28em] text-ink/45">
              PUBLISHED NOTICES
            </p>
            <p className="mt-2 font-display text-2xl font-bold">已发布公告</p>
          </div>
          <p className="font-mono text-[10px] tracking-[0.2em] text-ink/45">
            {loadState === "ready" ? `${items.length} ENTRIES` : "SYNCING"}
          </p>
        </div>

        {loadState === "signed_out" && (
          <section data-enter className="mt-8 border border-ink p-8 text-center">
            <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
              SIGNED OUT
            </p>
            <p className="mt-3 font-display text-2xl font-bold">
              需要登录后才能查看通知
            </p>
            <p className="mt-3 text-sm leading-6 text-ink/65">
              通知面向你的校园账号开放；完成统一认证后即可查看。
            </p>
            <Link
              href="/account/login?next=/notice"
              className="mt-6 inline-flex min-h-11 items-center border border-ink px-5 font-mono text-xs tracking-[0.18em] transition-colors hover:bg-ink hover:text-paper"
            >
              登录 / 注册
            </Link>
          </section>
        )}

        {loadState === "error" && error && (
          <ErrorBanner message={error} onRetry={() => void load()} className="mt-8" />
        )}

        {loadState === "loading" ? (
          <div className="mt-8">
            <LoadingBlock label="加载通知" />
          </div>
        ) : loadState === "ready" && items.length === 0 ? (
          <div className="mt-8">
            <EmptyBlock label="暂无已发布公告" />
            <p className="mt-4 text-center font-mono text-[10px] leading-5 text-ink/40">
              通知服务当前没有已发布的公告；待审核与未分发的内容不会对外展示。
            </p>
          </div>
        ) : loadState === "ready" ? (
          <div className="mt-8">
            <NoticeFeed items={items} />
            <p className="mt-5 border-t border-line pt-5 font-mono text-[10px] leading-5 text-ink/45">
              内容来自通知服务发布的真实公告，按公告创建时间排序；仅展示已发布的公告。
            </p>
          </div>
        ) : null}
      </div>
    </main>
  );
}
