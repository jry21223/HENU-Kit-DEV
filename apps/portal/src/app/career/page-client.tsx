"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import SectionHeading from "@/components/ui/section-heading";
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

  return (
    <main className="mx-auto max-w-7xl px-5 py-16 md:px-10">
      <SectionHeading index="05" en="WORK RADAR" title="求职雷达" />

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
          <p className="font-mono text-xs tracking-[0.14em] text-accent">
            WORK RADAR UNAVAILABLE
          </p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
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
