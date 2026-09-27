"use client";

import Link from "next/link";
import { cn } from "@/lib/cn";

/**
 * 加载中：label 说明正在读什么，末尾统一补省略号。
 * 它和空状态的说明都带 role="status"（礼貌播报），列表换成它们时焦点不用挪，支持的读屏软件
 * 会读出来。它们是跟着内容一起插进页面的，并非所有读屏软件都会读；一定要读出的结果（如验证码
 * 已发送）放在常驻的 status 区里。调用方不要再套一层 aria-live，否则会读两遍。
 */
export function LoadingBlock({ label = "加载中" }: { label?: string }) {
  return (
    <p
      role="status"
      className="border border-dashed border-ink/30 px-5 py-16 text-center font-mono text-xs leading-6 text-ink/70"
    >
      {label}…
    </p>
  );
}

/** 空状态的下一步：去别处用链接，就地改条件（如清除筛选）用按钮。 */
type EmptyAction =
  | { label: string; href: string }
  | { label: string; onClick: () => void };

const emptyActionClass =
  "mt-5 inline-flex min-h-11 min-w-11 items-center justify-center border border-ink px-4 font-mono text-xs text-ink transition-colors hover:bg-ink hover:text-paper focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

/**
 * 真实为空：label 说明为什么没有内容，action 给出一个可执行的下一步（不算在播报里）。
 * 详情页里随整页一起出现的固定段落占位（如“暂无学生补充”）不是结果变化，传 announce={false}
 * 不播报，免得页面一加载就连读几句。
 */
export function EmptyBlock({
  label = "暂无数据",
  action,
  announce = true,
}: {
  label?: string;
  action?: EmptyAction;
  announce?: boolean;
}) {
  return (
    <div className="border border-dashed border-ink/30 px-5 py-16 text-center">
      <p role={announce ? "status" : undefined} className="font-mono text-xs leading-6 text-ink/70">
        {label}
      </p>
      {action ? (
        "href" in action ? (
          <Link href={action.href} className={emptyActionClass}>
            {action.label}
          </Link>
        ) : (
          <button type="button" onClick={action.onClick} className={emptyActionClass}>
            {action.label}
          </button>
        )
      ) : null}
    </div>
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
          className="mt-3 inline-flex min-h-11 min-w-11 items-center justify-center border border-ink px-4 transition-colors hover:bg-ink hover:text-paper"
        >
          重试
        </button>
      )}
    </div>
  );
}
