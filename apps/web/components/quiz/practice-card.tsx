"use client";

import { useState } from "react";
import { QuizQuestion, postApi } from "@/lib/api";

type SubmitResult = {
  isCorrect: boolean;
  score: number;
  explanation: string;
};

export function PracticeCard({ question }: { question: QuizQuestion }) {
  const [answer, setAnswer] = useState("");
  const [result, setResult] = useState<SubmitResult | null>(null);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submitAnswer() {
    setSubmitting(true);
    setError("");
    setResult(null);
    try {
      const response = await postApi<SubmitResult>(`/questions/${question.id}/submit`, { answer });
      setResult(response.data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "提交失败");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="rounded-lg border border-line bg-white p-5 shadow-sm">
      <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500">
        <span className="rounded-md bg-paper px-2 py-1">{question.type}</span>
        <span className="rounded-md bg-paper px-2 py-1">难度 {question.difficulty}</span>
      </div>
      <h2 className="mt-4 text-base font-semibold leading-7">{question.stem}</h2>

      {question.options?.length ? (
        <div className="mt-4 grid gap-2">
          {question.options.map((option) => (
            <button
              className={`rounded-md border px-3 py-2 text-left text-sm ${answer === option.label ? "border-sage bg-paper" : "border-line bg-white"}`}
              key={option.id}
              onClick={() => setAnswer(option.label)}
              type="button"
            >
              <span className="font-semibold">{option.label}.</span> {option.content}
            </button>
          ))}
        </div>
      ) : (
        <textarea
          className="mt-4 min-h-24 w-full rounded-md border border-line px-3 py-2 text-sm"
          onChange={(event) => setAnswer(event.target.value)}
          placeholder="输入答案"
          value={answer}
        />
      )}

      <div className="mt-4 flex items-center gap-3">
        <button
          className="rounded-md bg-sage px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
          disabled={submitting || !answer}
          onClick={submitAnswer}
          type="button"
        >
          {submitting ? "提交中" : "提交答案"}
        </button>
        {result ? <span className={`text-sm font-medium ${result.isCorrect ? "text-green-700" : "text-red-700"}`}>{result.isCorrect ? "回答正确" : "回答错误"}</span> : null}
      </div>

      {result ? (
        <div className="mt-4 rounded-md border border-line bg-paper p-4 text-sm leading-6 text-slate-700">
          <p>得分：{result.score}</p>
          <p className="mt-2">{result.explanation || "暂无解析"}</p>
        </div>
      ) : null}
      {error ? <p className="mt-4 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</p> : null}
    </div>
  );
}
