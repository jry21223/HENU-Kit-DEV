"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

type SubmissionReviewActionsProps = {
  submissionId: string;
};

type ReviewResponse = {
  material_id?: string;
  error?: string;
};

export function SubmissionReviewActions({ submissionId }: SubmissionReviewActionsProps) {
  const router = useRouter();
  const [comment, setComment] = useState("");
  const [pendingAction, setPendingAction] = useState<"approve" | "reject" | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function review(action: "approve" | "reject") {
    setPendingAction(action);
    setError("");
    setMessage("");

    try {
      const response = await fetch(`/api/admin/submissions/${submissionId}/review`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          action,
          review_comment: comment,
        }),
      });
      const payload = (await response.json()) as ReviewResponse;

      if (!response.ok) {
        setError(payload.error ?? "审核操作失败。");
        return;
      }

      setMessage(action === "approve" ? "已通过并生成资料。" : "已驳回投稿。");
      router.refresh();
    } catch {
      setError("网络异常，请稍后重试。");
    } finally {
      setPendingAction(null);
    }
  }

  return (
    <div className="mt-4 grid gap-3">
      <textarea
        value={comment}
        onChange={(event) => setComment(event.target.value)}
        rows={3}
        maxLength={500}
        className="rounded-md border border-line px-3 py-2 text-sm leading-6 text-ink focus-ring"
        placeholder="审核说明；驳回时必须填写原因。"
      />
      <div className="grid gap-2 sm:grid-cols-2">
        <button
          type="button"
          onClick={() => review("approve")}
          disabled={Boolean(pendingAction)}
          className="inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] disabled:cursor-not-allowed disabled:bg-line disabled:text-muted focus-ring"
        >
          {pendingAction === "approve" ? "处理中" : "通过并发布"}
        </button>
        <button
          type="button"
          onClick={() => review("reject")}
          disabled={Boolean(pendingAction)}
          className="inline-flex h-10 items-center justify-center rounded-md border border-line bg-white px-4 text-sm font-semibold text-ink hover:bg-panel disabled:cursor-not-allowed disabled:text-muted focus-ring"
        >
          {pendingAction === "reject" ? "处理中" : "驳回"}
        </button>
      </div>
      {error ? (
        <div className="rounded-lg border border-[#e7c5b5] bg-[#fff7f3] p-3 text-sm font-semibold text-[#8b3f24]">
          {error}
        </div>
      ) : null}
      {message ? (
        <div className="rounded-lg border border-[#b7d7ca] bg-[#f2fbf7] p-3 text-sm font-semibold text-[#1d5a45]">
          {message}
        </div>
      ) : null}
    </div>
  );
}
