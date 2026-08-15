"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { useReveal } from "@/components/account/use-reveal";
import CareerScanStatusPanel from "@/components/career/career-scan-status-panel";
import { formatPortalError } from "@/lib/api/client";
import type { CareerJobType, CareerProfile, CareerSearch } from "@/lib/api/types";
import { careerSearchStatusLabel, formatCareerSearchTime } from "@/lib/career/career-scan-state";
import {
  careerLifetimeRequiredMessage,
  isCareerLifetimeRequiredError,
  requestCareerSearch,
} from "@/lib/career/gateway";
import { useCareerSearchPolling } from "@/lib/career/use-career-search-polling";

const JOB_TYPE_LABELS: Record<CareerJobType, string> = {
  "": "不限",
  daily_intern: "日常实习",
  summer_intern: "暑期实习",
  campus_recruit: "校招",
};

type ScanState =
  | { kind: "idle" }
  | { kind: "starting" }
  | { kind: "error"; message: string };

/** Lifetime 且画像就绪：画像摘要 + 开始扫描 + 异步状态跟踪（#402）+ 历史入口。 */
export default function CareerReadyView({
  profile,
  searches,
}: {
  profile: CareerProfile;
  searches: CareerSearch[];
}) {
  useReveal();
  const [scan, setScan] = useState<ScanState>({ kind: "idle" });
  // 幂等键：创建成功即消费置空；失败保留供重试复用，网关只会创建一次任务。
  const idempotencyKey = useRef<string | null>(null);
  const { state: pollState, isPolling, start: startPolling } = useCareerSearchPolling();

  const latest = searches[0] ?? null;

  // 刷新 / 重开恢复：本地无进行中任务时，取最近一条搜索恢复状态跟踪。
  // runner 的 start 幂等：终态直接落展示、进行中恢复轮询，重复调用不会叠加循环。
  useEffect(() => {
    if (latest) startPolling(latest);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const startScan = useCallback(async () => {
    if (scan.kind === "starting" || isPolling) return;
    setScan({ kind: "starting" });
    if (!idempotencyKey.current) {
      idempotencyKey.current = `career:scan-${crypto.randomUUID()}`;
    }
    try {
      const response = await requestCareerSearch(
        {
          target_roles: profile.target_roles,
          tech_stack: profile.tech_stack,
          locations: profile.locations,
          job_type: profile.job_type,
          graduation_year: profile.graduation_year,
          resume_text: profile.resume_text,
          email_notification_enabled: profile.email_notification_enabled,
        },
        idempotencyKey.current
      );
      // 创建成功即消费幂等键：之后的重试 / 重新扫描会生成新键，创建新任务。
      idempotencyKey.current = null;
      setScan({ kind: "idle" });
      startPolling(response.search);
    } catch (error) {
      // 创建失败保留幂等键：网关只会创建一次，重试不会产生重复任务。
      setScan({
        kind: "error",
        message: isCareerLifetimeRequiredError(error)
          ? careerLifetimeRequiredMessage()
          : formatPortalError(error),
      });
    }
  }, [profile, scan.kind, isPolling, startPolling]);

  return (
    <section data-career-state="lifetime-ready" className="mt-10">
      <div className="grid gap-10 lg:grid-cols-5">
        <div className="lg:col-span-3">
          <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">R-01</span>
            <span className="mx-2">/</span>
            READY TO SCAN
          </p>
          <h2 data-enter className="mt-3 font-display text-3xl font-bold tracking-tight md:text-4xl">
            求职画像已就绪
          </h2>

          <dl data-enter className="mt-8 grid gap-x-10 gap-y-5 border-t border-line pt-6 sm:grid-cols-2">
            <div>
              <dt className="font-mono text-[10px] tracking-[0.18em] text-ink/45">目标岗位</dt>
              <dd className="mt-1.5 text-sm leading-6">{profile.target_roles ?? "—"}</dd>
            </div>
            <div>
              <dt className="font-mono text-[10px] tracking-[0.18em] text-ink/45">技术栈</dt>
              <dd className="mt-1.5 text-sm leading-6">{profile.tech_stack || "—"}</dd>
            </div>
            <div>
              <dt className="font-mono text-[10px] tracking-[0.18em] text-ink/45">目标城市</dt>
              <dd className="mt-1.5 text-sm leading-6">{profile.locations || "—"}</dd>
            </div>
            <div>
              <dt className="font-mono text-[10px] tracking-[0.18em] text-ink/45">求职类型</dt>
              <dd className="mt-1.5 text-sm leading-6">
                {profile.job_type ? JOB_TYPE_LABELS[profile.job_type] : "不限"}
              </dd>
            </div>
          </dl>

          <div data-enter className="mt-8 flex flex-wrap items-center gap-4">
            <button
              type="button"
              disabled={scan.kind === "starting" || isPolling}
              onClick={() => void startScan()}
              className="inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper disabled:cursor-wait disabled:opacity-50"
            >
              {scan.kind === "starting"
                ? "正在创建任务…"
                : isPolling
                  ? "扫描进行中…"
                  : "开始扫描 →"}
            </button>
            <Link
              href="/account/profile"
              className="inline-flex min-h-11 items-center border border-line px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:border-ink"
            >
              修改画像
            </Link>
          </div>

          {scan.kind === "error" ? (
            <p
              data-career-scan-error
              role="alert"
              className="mt-6 max-w-xl border border-accent px-4 py-3 text-sm leading-6 text-ink/75"
            >
              {scan.message}
            </p>
          ) : null}

          {pollState ? (
            <div data-enter>
              <CareerScanStatusPanel
                state={pollState}
                emailEnabled={profile.email_notification_enabled ?? false}
                onRetry={() => void startScan()}
              />
            </div>
          ) : null}
        </div>

        <aside data-enter className="lg:col-span-2">
          <div className="border border-line p-5">
            <div className="flex items-center justify-between font-mono text-[10px] tracking-[0.22em] text-ink/50">
              <span>SCAN HISTORY</span>
              <Link href="/career/history" className="transition-colors hover:text-accent">
                全部历史 →
              </Link>
            </div>
            {latest ? (
              <div className="mt-4 border-t border-line pt-4">
                <div className="flex items-center justify-between gap-3">
                  <span className="font-mono text-[11px] tracking-wider text-ink/70">
                    最近一次
                  </span>
                  <span className="border border-ink/30 px-2 py-0.5 font-mono text-[10px] tracking-widest text-ink/60">
                    {careerSearchStatusLabel(latest.status)}
                  </span>
                </div>
                <p className="mt-2 text-xs leading-5 text-ink/55">
                  {formatCareerSearchTime(latest.created_at)}
                  {latest.has_email ? " · 邮件简报已开启" : ""}
                </p>
              </div>
            ) : (
              <p className="mt-4 border-t border-line pt-4 text-sm leading-6 text-ink/55">
                还没有扫描记录，发起第一次扫描后在此查看历史。
              </p>
            )}
          </div>
        </aside>
      </div>
    </section>
  );
}
