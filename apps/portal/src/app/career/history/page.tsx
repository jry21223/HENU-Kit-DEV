"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import CareerHistoryView from "@/components/career/career-history-view";
import SectionHeading from "@/components/ui/section-heading";
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

export default function CareerHistoryPage() {
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
    <main className="mx-auto max-w-7xl px-5 py-16 md:px-10">
      <SectionHeading index="R-02" en="SCAN HISTORY" title="扫描历史" />

      {state.kind === "loading" ? <LoadingBlock /> : null}

      {state.kind === "loading" ? null : <CareerHistoryView state={state} />}
    </main>
  );
}
