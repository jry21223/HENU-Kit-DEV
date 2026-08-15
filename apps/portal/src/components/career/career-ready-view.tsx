"use client";

import Link from "next/link";
import { useCallback, useRef, useState } from "react";
import { useReveal } from "@/components/account/use-reveal";
import { formatPortalError } from "@/lib/api/client";
import type { CareerJobType, CareerProfile, CareerSearch } from "@/lib/api/types";
import {
  careerLifetimeRequiredMessage,
  isCareerLifetimeRequiredError,
  requestCareerSearch,
} from "@/lib/career/gateway";

const JOB_TYPE_LABELS: Record<CareerJobType, string> = {
  "": "不限",
  daily_intern: "日常实习",
  summer_intern: "暑期实习",
  campus_recruit: "校招",
};

const STATUS_LABELS: Record<CareerSearch["status"], string> = {
  queued: "排队中",
  running: "扫描中",
  completed: "已完成",
  failed: "失败",
};

type ScanState =
  | { kind: "idle" }
  | { kind: "starting" }
  | { kind: "created"; search: CareerSearch }
  | { kind: "error"; message: string };

function formatTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** Lifetime 且画像就绪：画像摘要 + 开始扫描 + 历史入口；状态区为 #402 异步状态机预留。 */
export default function CareerReadyView({
  profile,
  searches,
}: {
  profile: CareerProfile;
  searches: CareerSearch[];
}) {
  useReveal();
  const [scan, setScan] = useState<ScanState>({ kind: "idle" });
  // 保留 key：重试复用同一幂等键，网关只创建一次。
  const idempotencyKey = useRef<string | null>(null);

  const latest = searches[0] ?? null;

  const startScan = useCallback(async () => {
    if (scan.kind === "starting" || scan.kind === "created") return;
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
      setScan({ kind: "created", search: response.search });
    } catch (error) {
      setScan({
        kind: "error",
        message: isCareerLifetimeRequiredError(error)
          ? careerLifetimeRequiredMessage()
          : formatPortalError(error),
      });
    }
  }, [profile, scan.kind]);

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
              disabled={scan.kind === "starting" || scan.kind === "created"}
              onClick={() => void startScan()}
              className="inline-flex min-h-11 items-center justify-center border border-ink px-6 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper disabled:cursor-wait disabled:opacity-50"
            >
              {scan.kind === "starting"
                ? "正在创建任务…"
                : scan.kind === "created"
                  ? "扫描任务已创建"
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

          {scan.kind === "created" ? (
            <div
              data-career-scan-status="created"
              aria-live="polite"
              className="mt-6 max-w-xl border border-ink px-4 py-4"
            >
              <p className="font-mono text-[10px] tracking-[0.2em] text-ink/45">TASK CREATED</p>
              <p className="mt-2 text-sm leading-6 text-ink/75">
                扫描任务已创建（{STATUS_LABELS[scan.search.status]}）。
                任务在后台异步执行，完成后会向账户邮箱发送结果简报。
              </p>
              <p className="mt-1 font-mono text-[10px] tracking-[0.15em] text-ink/40">
                #{scan.search.id.slice(0, 8)}
              </p>
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
                    {STATUS_LABELS[latest.status]}
                  </span>
                </div>
                <p className="mt-2 text-xs leading-5 text-ink/55">
                  {formatTime(latest.created_at)}
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
