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

export function ErrorBanner({
  message,
  onRetry,
  className,
}: {
  message: string;
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
      <p className="tracking-[0.2em] text-accent">ERROR / 数据源不可用</p>
      <p className="mt-2 text-ink/80">{message}</p>
      <p className="mt-2 text-ink/50">
        生产环境不使用本地 mock 数据。请配置 NEXT_PUBLIC_PORTAL_GATEWAY_URL 并确保 portal-api / portal-gateway 在线。
      </p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-3 border border-ink px-3 py-1.5 tracking-widest transition-colors hover:bg-ink hover:text-paper"
        >
          重试
        </button>
      )}
    </div>
  );
}
