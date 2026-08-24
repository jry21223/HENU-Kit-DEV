"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import CareerFreeView from "@/components/career/career-free-view";
import CareerGuestView from "@/components/career/career-guest-view";
import CareerNoProfileView from "@/components/career/career-no-profile-view";
import CareerReadyView from "@/components/career/career-ready-view";
import { resolveCareerView } from "@/lib/career/page-state";
import type { CareerViewState } from "@/lib/career/page-state";

type PageState = { kind: "loading" } | CareerViewState;

function LoadingBlock() {
  return (
    <div data-career-state="loading" className="flex min-h-[40vh] items-center justify-center">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/40">
        WORK RADAR LOADING<span className="animate-pulse text-accent">…</span>
      </p>
    </div>
  );
}

export default function CareerPage() {
  const searchParams = useSearchParams();
  const requestedSearchID = searchParams.get("search")?.trim() || null;
  const [state, setState] = useState<PageState>({ kind: "loading" });
  const requestVersion = useRef(0);

  const requestState = useCallback((version: number) => {
    void resolveCareerView().then((next) => {
      if (version === requestVersion.current) setState(next);
    });
  }, []);

  const load = useCallback(() => {
    const version = ++requestVersion.current;
    setState({ kind: "loading" });
    requestState(version);
  }, [requestState]);

  useEffect(() => {
    const version = ++requestVersion.current;
    requestState(version);
    return () => {
      requestVersion.current += 1;
    };
  }, [requestState]);

  useEffect(() => {
    let refreshTimer: number | null = null;
    const scheduleRefresh = () => {
      if (document.visibilityState !== "visible" || refreshTimer !== null) return;
      refreshTimer = window.setTimeout(() => {
        refreshTimer = null;
        const version = ++requestVersion.current;
        requestState(version);
      }, 50);
    };
    window.addEventListener("focus", scheduleRefresh);
    document.addEventListener("visibilitychange", scheduleRefresh);
    return () => {
      window.removeEventListener("focus", scheduleRefresh);
      document.removeEventListener("visibilitychange", scheduleRefresh);
      if (refreshTimer !== null) window.clearTimeout(refreshTimer);
    };
  }, [requestState]);

  return (
    // 与 library / campus / food 等子站二级页保持同一容器：1440 栅格 + md:px-8，
    // 这样正文与顶部 CareerNav（同样是 max-w-[1440px]）左右对齐。
    <main className="mx-auto max-w-[1440px] px-5 py-10 md:px-8">
      {state.kind === "loading" ? <LoadingBlock /> : null}

      {state.kind === "anonymous" ? <CareerGuestView /> : null}

      {state.kind === "free" ? <CareerFreeView /> : null}

      {state.kind === "lifetime-no-profile" ? <CareerNoProfileView /> : null}

      {state.kind === "lifetime-ready" ? (
        <CareerReadyView
          profile={state.profile}
          searches={state.searches}
          requestedSearchID={requestedSearchID}
        />
      ) : null}

      {state.kind === "error" ? (
        <section
          data-career-state="error"
          role="alert"
          className="mt-10 max-w-2xl border border-accent px-5 py-6"
        >
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">R-01</span>
            <span className="mx-2">/</span>
            RADAR UNAVAILABLE
          </p>
          <h1 className="mt-3 font-display text-2xl font-bold tracking-tight md:text-3xl">
            求职雷达暂时不可用
          </h1>
          <p className="mt-4 text-sm leading-6 text-ink/65">{state.message}</p>
          <p className="mt-3 text-sm leading-6 text-ink/60">
            求职雷达数据加载不出来时，不会以本地或会话数据替代真实数据。
          </p>
          <button
            type="button"
            onClick={load}
            className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}
    </main>
  );
}
