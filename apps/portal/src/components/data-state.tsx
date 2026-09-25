"use client";

import { cn } from "@/lib/cn";

export function LoadingBlock({ label = "加载中…" }: { label?: string }) {
  return (
    <p className="border border-dashed border-ink/30 px-5 py-16 text-center font-mono text-xs tracking-[0.3em] text-ink/40">
      {label} / LOADING
    </p>
  );
}

export function EmptyBlock({ label = "暂无数据" }: { label?: string }) {
  return (
    <p className="border border-dashed border-ink/30 px-5 py-16 text-center font-mono text-xs tracking-[0.3em] text-ink/40">
      {label} / EMPTY
    </p>
  );
}

/**
 * 加载失败：一条主信息（发生了什么、可以怎么做）加「重试」。message 应来自
 * formatPortalError 或调用方自己的中文文案；requestId 显示为错误编号，方便提工单时附上。
 */
export function ErrorBanner({
  message,
  requestId,
  onRetry,
  className,
}: {
  message: string;
  requestId?: string | null;
  onRetry?: () => void;
  className?: string;
}) {
  return (
    <div
      role="alert"
      className={cn(
        "border border-accent/60 bg-accent/5 px-5 py-4 font-mono text-xs leading-6 text-ink",
        className
      )}
    >
      <p className="text-ink/80">{message}</p>
      {requestId ? (
        <p className="mt-1 text-ink/70">
          错误编号：<span className="select-all break-all">{requestId}</span>（提交工单时请附上）
        </p>
      ) : null}
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-3 inline-flex min-h-11 min-w-11 items-center justify-center border border-ink px-4 tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          重试
        </button>
      )}
    </div>
  );
}
