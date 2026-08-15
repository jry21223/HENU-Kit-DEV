"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import WorkRadar, { type WorkRadarStatus } from "@/components/career/work-radar";

const SOURCE_TOTAL = 16;

const DEMO_JOBS = [
  { score: 92, company: "字节跳动", title: "AI Agent 开发实习生", meta: "北京 · Python / LLM / Agent" },
  { score: 88, company: "美团", title: "后端开发实习生", meta: "北京 · Go / Redis / MySQL" },
  { score: 84, company: "百度", title: "大模型应用研发实习生", meta: "北京 · Python / RAG / NLP" },
];

export default function CareerPrototypePage() {
  const [status, setStatus] = useState<WorkRadarStatus>("idle");
  const [tick, setTick] = useState(0);

  useEffect(() => {
    if (status !== "queued") return;
    const timer = window.setTimeout(() => setStatus("running"), 650);
    return () => window.clearTimeout(timer);
  }, [status]);

  useEffect(() => {
    if (status !== "running") return;
    const timer = window.setInterval(() => {
      setTick((current) => Math.min(SOURCE_TOTAL, current + 1));
    }, 620);
    return () => window.clearInterval(timer);
  }, [status]);

  useEffect(() => {
    if (status === "running" && tick >= SOURCE_TOTAL) {
      setStatus("completed");
    }
  }, [status, tick]);

  const metrics = useMemo(() => {
    const jobsFound = Math.min(73, Math.round(tick * 4.55));
    const matched = Math.min(11, Math.floor(tick * 0.7));
    return { jobsFound, matched };
  }, [tick]);

  const startScan = () => {
    setTick(0);
    setStatus("queued");
  };

  const reset = () => {
    setTick(0);
    setStatus("idle");
  };

  return (
    <main className="min-h-svh bg-paper text-ink">
      <header className="border-b border-ink">
        <div className="mx-auto flex max-w-[1440px] items-center justify-between px-5 py-4 md:px-8">
          <Link href="/" className="font-display text-sm font-bold tracking-tight transition-colors hover:text-accent">
            ← HENU KIT<span className="text-accent">®</span>
          </Link>
          <p className="font-mono text-[10px] tracking-[0.22em] text-ink/45">
            05 / WORK RADAR / UI PROTOTYPE
          </p>
        </div>
      </header>

      <section
        className="border-b border-ink"
        style={{
          backgroundImage:
            "linear-gradient(rgba(22,21,19,.045) 1px, transparent 1px), linear-gradient(90deg, rgba(22,21,19,.045) 1px, transparent 1px)",
          backgroundSize: "28px 28px",
        }}
      >
        <div className="mx-auto max-w-[1440px] px-5 py-10 md:px-8 md:py-14">
          <div className="flex items-center justify-between font-mono text-[10px] tracking-[0.2em] text-ink/55">
            <span>HENU KIT // COMMUNITY UPDATE — 05</span>
            <span>N34.79 E114.30</span>
          </div>
          <h1 className="mt-4 font-display text-[clamp(4rem,13vw,11rem)] font-bold leading-[0.82] tracking-[-0.075em]">
            WORK RADAR
          </h1>
          <div className="mt-5 flex flex-col gap-6 border-t border-ink pt-5 md:flex-row md:items-end md:justify-between">
            <p className="max-w-3xl font-display text-2xl font-bold leading-tight md:text-4xl">
              上传简历，让求职雷达替你扫描大厂招聘官网。
            </p>
            <p className="max-w-sm text-sm leading-6 text-ink/60">
              当前 PR 只演示网页视觉与异步任务状态。真实会员鉴权、GetWork 与邮件发送后续接入。
            </p>
          </div>
        </div>
      </section>

      <section className="mx-auto grid max-w-[1440px] gap-8 px-5 py-8 md:px-8 lg:grid-cols-[minmax(0,1fr)_22rem] lg:py-12">
        <WorkRadar
          status={status}
          sourcesCompleted={tick}
          sourcesTotal={SOURCE_TOTAL}
          jobsFound={metrics.jobsFound}
          matched={metrics.matched}
        />

        <aside className="flex flex-col border border-ink bg-paper p-5 md:p-6">
          <p className="font-mono text-[10px] tracking-[0.22em] text-ink/45">MISSION CONTROL</p>
          <h2 className="mt-3 font-display text-3xl font-bold">开始一次岗位扫描</h2>
          <p className="mt-3 text-sm leading-6 text-ink/60">
            正式版点击后会立即创建后台任务。你可以离开页面，完成后再回来查看，同时收到邮件提醒。
          </p>

          <dl className="mt-8 space-y-3 border-y border-ink py-5 font-mono text-[10px] tracking-[0.12em]">
            <div className="flex justify-between gap-4"><dt className="text-ink/40">TARGET</dt><dd>AI / 后端 / 全栈</dd></div>
            <div className="flex justify-between gap-4"><dt className="text-ink/40">LOCATION</dt><dd>北京 / 上海 / 杭州</dd></div>
            <div className="flex justify-between gap-4"><dt className="text-ink/40">MEMBERSHIP</dt><dd>LIFETIME VIP</dd></div>
            <div className="flex justify-between gap-4"><dt className="text-ink/40">MAIL</dt><dd>ON COMPLETE</dd></div>
          </dl>

          {status === "idle" || status === "failed" ? (
            <button
              type="button"
              onClick={startScan}
              className="mt-6 inline-flex min-h-12 items-center justify-center border border-ink bg-ink px-5 font-mono text-xs tracking-[0.18em] text-paper transition-colors hover:border-accent hover:bg-accent"
            >
              START SCAN →
            </button>
          ) : null}

          {status === "queued" || status === "running" ? (
            <div className="mt-6 border border-accent p-4">
              <p className="font-mono text-xs tracking-[0.18em] text-accent">
                {status === "queued" ? "QUEUED" : "SCANNING"}
              </p>
              <p className="mt-2 text-sm leading-6 text-ink/65">
                雷达正在扫描。真实版本不要求你停留在页面，任务会在后台继续运行。
              </p>
            </div>
          ) : null}

          {status === "completed" ? (
            <div className="mt-6">
              <div className="border border-ink bg-ink p-4 text-paper">
                <p className="font-mono text-xs tracking-[0.18em] text-accent">SCAN COMPLETE</p>
                <p className="mt-2 font-display text-3xl font-bold">发现 {metrics.matched} 个匹配岗位</p>
              </div>
              <button
                type="button"
                onClick={reset}
                className="mt-3 inline-flex min-h-11 w-full items-center justify-center border border-ink px-4 font-mono text-xs tracking-[0.16em] transition-colors hover:bg-ink hover:text-paper"
              >
                RESET DEMO
              </button>
            </div>
          ) : null}

          <button
            type="button"
            onClick={() => setStatus("failed")}
            className="mt-auto pt-8 text-left font-mono text-[9px] tracking-[0.16em] text-ink/35 transition-colors hover:text-accent"
          >
            PREVIEW FAULT STATE
          </button>
        </aside>
      </section>

      {status === "completed" ? (
        <section className="mx-auto max-w-[1440px] px-5 pb-14 md:px-8">
          <div className="border-t border-ink pt-8">
            <div className="flex items-end justify-between gap-6">
              <div>
                <p className="font-mono text-[10px] tracking-[0.22em] text-ink/45">DEMO RESULTS</p>
                <h2 className="mt-2 font-display text-4xl font-bold">高匹配岗位</h2>
              </div>
              <p className="font-mono text-[10px] tracking-[0.16em] text-ink/40">STATIC PROTOTYPE DATA</p>
            </div>

            <div className="mt-6 border-t border-ink">
              {DEMO_JOBS.map((job, index) => (
                <article key={`${job.company}-${job.title}`} className="grid gap-3 border-b border-line py-5 md:grid-cols-[4rem_7rem_minmax(0,1fr)_auto] md:items-center md:gap-6">
                  <span className="font-mono text-sm text-ink/30">{String(index + 1).padStart(2, "0")}</span>
                  <span className="font-display text-3xl font-bold text-accent">{job.score}%</span>
                  <span>
                    <strong className="block font-display text-xl">{job.company} · {job.title}</strong>
                    <span className="mt-1 block text-sm text-ink/55">{job.meta}</span>
                  </span>
                  <span className="font-mono text-xs tracking-[0.14em] text-ink/45">VIEW →</span>
                </article>
              ))}
            </div>
          </div>
        </section>
      ) : null}

      <footer className="border-t border-ink bg-ink px-5 py-5 text-paper md:px-8">
        <div className="mx-auto flex max-w-[1440px] flex-col gap-2 font-mono text-[10px] tracking-[0.2em] sm:flex-row sm:items-center sm:justify-between">
          <span>UI PROTOTYPE · ISSUE #395</span>
          <span className="text-accent">LIFETIME VIP · WORK RADAR</span>
        </div>
      </footer>
    </main>
  );
}
