"use client";

import Collapse from "@/components/ui/collapse";
import { Textarea } from "@/components/ui/textarea";
import type {
  PortalPracticeFeedbackStatusResponse,
  PracticeFeedbackCategory,
} from "@/lib/api/types";
import { cn } from "@/lib/cn";

export const FEEDBACK_CATEGORIES: Array<{ value: PracticeFeedbackCategory; label: string }> = [
  { value: "wrong_answer", label: "答案有误" },
  { value: "ambiguous", label: "表述歧义" },
  { value: "typo", label: "错别字" },
  { value: "outdated", label: "内容过时" },
  { value: "other", label: "其他" },
];

const FEEDBACK_STATUS_LABEL: Record<
  PortalPracticeFeedbackStatusResponse["data"]["status"],
  string
> = {
  pending: "已受理",
  in_progress: "处理中",
  blocked: "暂时受阻",
  resolved: "已解决",
  archived: "已归档",
};

export interface QuizFeedbackPanelProps {
  open: boolean;
  onToggle: () => void;
  category: PracticeFeedbackCategory;
  onCategoryChange: (value: PracticeFeedbackCategory) => void;
  detail: string;
  onDetailChange: (value: string) => void;
  submitting: boolean;
  message: string | null;
  status: PortalPracticeFeedbackStatusResponse["data"]["status"] | null;
  error: string | null;
  feedbackId: string | null;
  onSubmit: () => void;
  onRefreshStatus: (id: string) => void;
}

export function QuizFeedbackPanel({
  open,
  onToggle,
  category,
  onCategoryChange,
  detail,
  onDetailChange,
  submitting,
  message,
  status,
  error,
  feedbackId,
  onSubmit,
  onRefreshStatus,
}: QuizFeedbackPanelProps) {
  return (
    <div data-feedback className="mt-5 border-t border-line pt-4">
      {/* 文字按钮的点击区撑到 44px 高，等量负外边距让文字和下面的面板都不挪位置。 */}
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="-my-3.5 inline-flex min-h-11 items-center font-mono text-xs text-ink/60 transition-colors hover:text-accent-text"
      >
        {open ? "收起纠错 −" : "这道题有问题？提交纠错 +"}
      </button>

      <Collapse open={open}>
        <div className="mt-4 border border-dashed border-ink/25 p-5">
          <p className="font-mono text-xs text-ink/60">
            <span className="tracking-[0.25em]">CORRECTION</span> / 题内纠错
          </p>
          <div className="mt-4 flex flex-wrap gap-2">
            {FEEDBACK_CATEGORIES.map((item) => (
              <button
                key={item.value}
                type="button"
                onClick={() => onCategoryChange(item.value)}
                aria-pressed={category === item.value}
                className={cn(
                  "min-h-11 border px-3 py-1.5 font-mono text-xs transition-colors",
                  category === item.value
                    ? "border-ink bg-ink text-paper"
                    : "border-line text-ink/60 hover:border-ink/40"
                )}
              >
                {item.label}
              </button>
            ))}
          </div>
          <Textarea
            value={detail}
            onChange={(event) => onDetailChange(event.target.value)}
            rows={3}
            maxLength={4000}
            placeholder="简单描述问题，例如：第 2 题解析里的公式有笔误。"
            className="mt-4"
          />
          {message && (
            <p className="mt-3 text-sm text-ink/70">
              {message}
              {status && (
                <span className="ml-2 border border-accent/60 px-1.5 py-0.5 font-mono text-xs text-accent-text">
                  {FEEDBACK_STATUS_LABEL[status]}
                </span>
              )}
            </p>
          )}
          {error && (
            <p role="alert" className="mt-3 text-sm text-accent-text">
              {error}
            </p>
          )}
          <div className="mt-4 flex gap-3">
            <button
              type="button"
              onClick={onSubmit}
              disabled={submitting}
              className={cn(
                "min-h-11 border px-5 py-2 font-mono text-xs transition-colors",
                submitting
                  ? "cursor-not-allowed border-line text-ink/30"
                  : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent hover:text-ink"
              )}
            >
              {submitting ? "提交中…" : "提交纠错"}
            </button>
            {feedbackId && (
              <button
                type="button"
                onClick={() => onRefreshStatus(feedbackId)}
                className="min-h-11 border border-ink/30 px-4 py-2 font-mono text-xs transition-colors hover:border-ink"
              >
                刷新状态
              </button>
            )}
          </div>
          <p className="mt-3 font-mono text-xs leading-5 text-ink/60">
            纠错需要登录后提交；内容会连同题目版本引用交给题库维护者处理。
          </p>
        </div>
      </Collapse>
    </div>
  );
}
