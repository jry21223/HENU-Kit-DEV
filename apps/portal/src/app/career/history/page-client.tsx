"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import CareerHistoryView from "@/components/career/career-history-view";
import { resolveCareerHistoryView } from "@/lib/career/page-state";
import type { CareerHistoryViewState } from "@/lib/career/page-state";

type PageState = { kind: "loading" } | CareerHistoryViewState;

function LoadingBlock() {
  return (
    <div
      data-career-history-state="loading"
      className="flex min-h-[40vh] items-center justify-center"
    >
      <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
        SCAN HISTORY LOADING<span className="animate-pulse text-accent">…</span>
      </p>
    </div>
  );
}

export default function CareerHistoryPageClient() {
  const [state, setState] = useState<PageState>({ kind: "loading" });
  const requestVersion = useRef(0);

  const requestState = useCallback((version: number) => {
    void resolveCareerHistoryView().then((next) => {
      if (version === requestVersion.current) setState(next);
    });
  }, []);

  useEffect(() => {
    const version = ++requestVersion.current;
    requestState(version);
    return () => {
      requestVersion.current += 1;
    };
  }, [requestState]);

  return (
    // 容器与 /career 及其他子站二级页一致，正文与 CareerNav 左右对齐。
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      {state.kind === "loading" ? <LoadingBlock /> : <CareerHistoryView state={state} />}
    </main>
  );
}
