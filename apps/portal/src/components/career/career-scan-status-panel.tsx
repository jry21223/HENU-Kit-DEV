"use client";

import {
  careerDigestStatusLabel,
  careerScanFailedMessage,
  careerScanStageLabel,
  careerSearchStatusLabel,
  formatCareerSearchTime,
} from "@/lib/career/career-scan-state";
import type { CareerScanPollState } from "@/lib/career/career-scan-state";
import type { CareerSearch } from "@/lib/api/types";
import { visibleCareerMatches } from "@/lib/career/result";

/**
 * 异步扫描任务状态区（#402）：active/completed/failed 三态展示。
 *
 * - active：queued/running + stage 中文阶段；提示可关闭页面。
 * - completed：只展示持久化结果（status=completed + 创建时间），绝不本地伪造计数。
 * - failed：稳定可理解文案 + 重新扫描入口，不展示内部细节。
 */
export default function CareerScanStatusPanel({
  state,
  emailEnabled,
  onRetry,
}: {
  state: CareerScanPollState;
  /** 画像里的邮件简报开关；决定 active 态提示是否承诺邮件通知。 */
  emailEnabled: boolean;
  onRetry: () => void;
}) {
  if (state.kind === "active") {
    return <ActivePanel search={state.search} pollError={state.pollError} emailEnabled={emailEnabled} />;
  }
  if (state.kind === "completed") {
    return <CompletedPanel search={state.search} />;
  }
  return <FailedPanel search={state.search} onRetry={onRetry} />;
}

function ActivePanel({
  search,
  pollError,
  emailEnabled,
}: {
  search: CareerSearch;
  pollError: string | null;
  emailEnabled: boolean;
}) {
  const stageLabel = careerScanStageLabel(search.stage);
  return (
    <div
      data-career-scan-status="running"
      aria-live="polite"
      className="mt-6 max-w-xl border border-ink px-4 py-4"
    >
      <div className="flex items-center justify-between gap-3">
        <p className="font-mono text-[10px] tracking-[0.2em] text-ink/45">TASK RUNNING</p>
        <span className="border border-ink/30 px-2 py-0.5 font-mono text-[10px] tracking-widest text-ink/60">
          {careerSearchStatusLabel(search.status)}
        </span>
      </div>
      <p className="mt-2 text-sm leading-6 text-ink/75">
        {search.status === "queued"
          ? "任务已进入队列，正在等待后台执行。"
          : stageLabel
            ? `扫描进行中：${stageLabel}。`
            : "扫描进行中，请稍候。"}
      </p>
      <p className="mt-2 text-sm leading-6 text-ink/65">
        {emailEnabled
          ? "可以关闭本页面，扫描会在后台继续，完成后将把结果简报加入邮件发送队列。"
          : "可以关闭本页面，扫描会在后台继续，稍后回来即可查看结果。"}
      </p>
      <p className="mt-1 font-mono text-[10px] tracking-[0.15em] text-ink/40">
        #{search.id.slice(0, 8)}
      </p>
      {pollError ? (
        <p
          role="status"
          className="mt-2 border-t border-line pt-2 font-mono text-[10px] tracking-wider text-accent"
        >
          {pollError}
        </p>
      ) : null}
    </div>
  );
}

function CompletedPanel({ search }: { search: CareerSearch }) {
  const matches = search.result ? visibleCareerMatches(search.result) : [];
  const digestStatus = careerDigestStatusLabel(search) ?? "历史任务无邮件状态";
  return (
    <div
      data-career-scan-status="completed"
      aria-live="polite"
      className="mt-6 max-w-3xl border border-ink px-4 py-4"
    >
      <div className="flex items-center justify-between gap-3">
        <p className="font-mono text-[10px] tracking-[0.2em] text-ink/45">TASK COMPLETED</p>
        <span className="border border-ink/40 px-2 py-0.5 font-mono text-[10px] tracking-widest text-ink/70">
          {careerSearchStatusLabel(search.status)}
        </span>
      </div>
      <p className="mt-2 text-sm leading-6 text-ink/75">
        {search.result?.summary ?? "扫描已完成，结果已保存。"}
      </p>
      <p className="mt-1 font-mono text-[10px] tracking-[0.15em] text-ink/40">
        #{search.id.slice(0, 8)} · {formatCareerSearchTime(search.created_at)}
        {` · ${digestStatus}`}
      </p>
      {search.result ? (
        <div className="mt-4 grid grid-cols-3 border-y border-line py-3 text-center font-mono text-[10px] tracking-wider text-ink/60">
          <p>来源<br /><strong className="text-base text-ink">{search.result.source_count}</strong></p>
          <p>岗位<br /><strong className="text-base text-ink">{search.result.job_count}</strong></p>
          <p>推荐<br /><strong className="text-base text-accent">{search.result.matched_count}</strong></p>
        </div>
      ) : null}
      {matches.length ? (
        <ol className="mt-4 space-y-3">
          {matches.map((job) => (
            <li key={`${job.source_key}:${job.url}`} className="border border-line p-3">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="font-display text-lg font-bold">{job.title}</p>
                  <p className="mt-1 text-xs text-ink/55">{job.company} · {job.location || "地点待确认"}</p>
                </div>
                <span className="border border-accent px-2 py-1 font-mono text-xs text-accent">{job.match_score}</span>
              </div>
              {job.match_reasons.length ? (
                <p className="mt-2 text-xs leading-5 text-ink/60">{job.match_reasons.join("；")}</p>
              ) : null}
              <a href={job.url} target="_blank" rel="noreferrer" className="mt-3 inline-block font-mono text-xs text-accent hover:underline">
                查看官方岗位 →
              </a>
            </li>
          ))}
        </ol>
      ) : search.result ? (
        <p className="mt-3 text-xs leading-5 text-ink/55">本次没有达到匹配阈值的岗位，可调整画像后重新扫描。</p>
      ) : null}
    </div>
  );
}

function FailedPanel({ search, onRetry }: { search: CareerSearch; onRetry: () => void }) {
  return (
    <div
      data-career-scan-status="failed"
      aria-live="polite"
      className="mt-6 max-w-xl border border-accent px-4 py-4"
    >
      <div className="flex items-center justify-between gap-3">
        <p className="font-mono text-[10px] tracking-[0.2em] text-accent">TASK FAILED</p>
        <span className="border border-accent/60 px-2 py-0.5 font-mono text-[10px] tracking-widest text-accent">
          {careerSearchStatusLabel(search.status)}
        </span>
      </div>
      <p className="mt-2 text-sm leading-6 text-ink/75">{careerScanFailedMessage()}。</p>
      <p className="mt-2 text-sm leading-6 text-ink/65">
        你可以重新发起一次扫描，之前的扫描记录不会丢失。
      </p>
      <button
        type="button"
        onClick={onRetry}
        className="mt-4 inline-flex min-h-11 items-center justify-center border border-ink px-5 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
      >
        重新扫描 →
      </button>
    </div>
  );
}
