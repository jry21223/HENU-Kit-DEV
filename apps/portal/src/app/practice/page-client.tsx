"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  fetchQuizCraftCatalog,
} from "@/lib/api/client";
import { quizCraftCatalogEnabled } from "@/lib/api/env";
import type { QuizCraftCatalogBank } from "@/lib/api/types";
import { usePersonalPracticeStats } from "@/lib/practice/personal-stats";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import BankHero from "@/components/practice/bank-hero";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";

const quizCraftCatalogIsEnabled = quizCraftCatalogEnabled();

function QuizCraftCatalogCard({
  bank,
  index,
}: {
  bank: QuizCraftCatalogBank;
  index: number;
}) {
  const href = `/practice/quiz?bank_id=${encodeURIComponent(bank.bank_id)}&bank_version_id=${encodeURIComponent(bank.bank_version_id)}`;
  return (
    <article
      data-testid="quizcraft-catalog"
      className="group border border-ink/25 bg-paper p-5"
    >
      <div className="flex items-start justify-between">
        <span className="font-mono text-xs text-accent-text">
          题库 {String(index + 1).padStart(2, "0")}
        </span>
        <span className="font-mono text-xs text-ink/60">
          {bank.available ? "可练习" : "暂不可用"}
        </span>
      </div>
      <h3 className="mt-3 font-display text-xl font-bold leading-snug">{bank.name}</h3>
      <p className="mt-2 font-mono text-xs text-ink/60">
        {bank.question_count} 题
      </p>
      <div className="mt-5 border-t border-line pt-3">
        {bank.available ? (
          <Link
            data-testid="quizcraft-catalog-start"
            href={href}
            className="inline-flex min-h-11 items-center border border-ink px-3 font-mono text-xs transition-colors hover:bg-ink hover:text-paper"
          >
            开始刷题 →
          </Link>
        ) : (
          <span className="font-mono text-xs text-ink/60">
            当前版本暂不可练习
          </span>
        )}
      </div>
    </article>
  );
}

type LoadState = "loading" | "ready" | "error";

export default function PracticeBankPage() {
  usePageEnter(null);
  const { state: masteryState } = usePersonalPracticeStats();

  const [quizCraftBanks, setQuizCraftBanks] = useState<QuizCraftCatalogBank[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);

  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLDivElement>(null);

  const load = useCallback(async () => {
    setLoadState("loading");
    setError(null);
    if (!quizCraftCatalogIsEnabled) {
      // Browser flag off: render an empty, non-probing state. ADR-0036 removed
      // the legacy schools/banks → gateway cache → mock fallback chain, so
      // there is no other data source to try and nothing to probe.
      setQuizCraftBanks([]);
      setLoadState("ready");
      return;
    }
    try {
      const response = await fetchQuizCraftCatalog();
      setQuizCraftBanks(response.banks);
      setLoadState("ready");
    } catch {
      // The flag is a real-data cutover seam. Never replace a failed Core
      // read with legacy Portal API, cached, or local mock catalog data.
      setQuizCraftBanks([]);
      setError("题库暂时加载不出来，请检查网络后重试。");
      setLoadState("error");
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  const searching = query.trim().length > 0;
  const filteredQuizCraftBanks = useMemo(() => {
    if (!searching) return quizCraftBanks;
    const q = query.trim();
    return quizCraftBanks.filter((bank) => bank.name.includes(q));
  }, [query, quizCraftBanks, searching]);
  const clearSearch = () => {
    setQuery("");
    // 清除搜索按钮会随空状态一起消失；焦点交给搜索区，键盘和读屏用户不丢位置。
    // 不直接聚焦搜索框：手机上会弹出输入法，挡住刚恢复的题库。
    searchRef.current?.focus();
  };

  return (
    <main>
      <BankHero
        query={query}
        onQueryChange={setQuery}
        catalogMode={quizCraftCatalogIsEnabled}
        masteryState={masteryState}
        searchRef={searchRef}
      />

      <div data-block className="border-t border-line">
        <div className="mx-auto flex max-w-site items-center justify-between px-5 py-3 md:px-8">
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent-text">02</span>
            <span className="mx-2">/</span>
            BROWSE
          </p>
        </div>
      </div>

      <div className="mx-auto max-w-site px-5 pt-6 md:px-8">
        {loadState === "error" && error && (
          <ErrorBanner message={error} onRetry={() => void load()} className="mb-6" />
        )}
      </div>

      {loadState === "loading" ? (
        <div className="mx-auto max-w-site px-5 py-6 md:px-8 lg:py-10">
          <LoadingBlock label="加载题库" />
        </div>
      ) : loadState === "error" ? null : (
        <div className="mx-auto max-w-site lg:flex">
          <div data-block className="flex-1 px-5 py-6 md:px-8 lg:py-10">
            {quizCraftCatalogIsEnabled ? (
              filteredQuizCraftBanks.length === 0 ? (
                <EmptyBlock
                  label={searching ? "无匹配题库" : "暂无题库"}
                  action={searching ? { label: "清除搜索", onClick: clearSearch } : undefined}
                />
              ) : (
                <div data-enter className="grid gap-5 md:grid-cols-2">
                  {filteredQuizCraftBanks.map((bank, index) => (
                    <QuizCraftCatalogCard
                      key={`${bank.bank_id}:${bank.bank_version_id}`}
                      bank={bank}
                      index={index}
                    />
                  ))}
                </div>
              )
            ) : (
              <EmptyBlock label="暂无题库" />
            )}
          </div>
        </div>
      )}
    </main>
  );
}
