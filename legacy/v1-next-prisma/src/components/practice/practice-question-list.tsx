"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { questionTypeLabels } from "@/constants/enums";
import type { PracticeQuestion, QuestionSubmitResult } from "@/types";

type PracticeQuestionListProps = {
  courseId: string;
  courseName: string;
  questions: PracticeQuestion[];
};

type ApiSubmitResult = {
  question_id: string;
  is_correct: boolean;
  submitted_answer: string[];
  correct_answer: string[];
  correct_answer_label: string;
  explanation: string;
  wrong_question_saved: boolean;
};

type StoredPracticeResult = {
  courseId: string;
  courseName: string;
  total: number;
  answered: number;
  correct: number;
  wrong: number;
  updatedAt: string;
};

function mapSubmitResult(result: ApiSubmitResult): QuestionSubmitResult {
  return {
    questionId: result.question_id,
    isCorrect: result.is_correct,
    submittedAnswer: result.submitted_answer,
    correctAnswer: result.correct_answer,
    correctAnswerLabel: result.correct_answer_label,
    explanation: result.explanation,
    wrongQuestionSaved: result.wrong_question_saved,
  };
}

export function PracticeQuestionList({
  courseId,
  courseName,
  questions,
}: PracticeQuestionListProps) {
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const [results, setResults] = useState<Record<string, QuestionSubmitResult>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [pendingId, setPendingId] = useState<string | null>(null);

  const summary = useMemo(() => {
    const resultList = Object.values(results);
    const correct = resultList.filter((result) => result.isCorrect).length;
    return {
      total: questions.length,
      answered: resultList.length,
      correct,
      wrong: resultList.length - correct,
    };
  }, [questions.length, results]);

  useEffect(() => {
    if (summary.total === 0 || summary.answered !== summary.total) {
      return;
    }

    const storedResult: StoredPracticeResult = {
      courseId,
      courseName,
      total: summary.total,
      answered: summary.answered,
      correct: summary.correct,
      wrong: summary.wrong,
      updatedAt: new Date().toISOString(),
    };

    window.localStorage.setItem(`practice-result:${courseId}`, JSON.stringify(storedResult));
  }, [courseId, courseName, summary]);

  async function submitAnswer(question: PracticeQuestion) {
    const answer = answers[question.id];
    if (!answer) {
      setErrors((current) => ({ ...current, [question.id]: "请选择答案。" }));
      return;
    }

    setPendingId(question.id);
    setErrors((current) => ({ ...current, [question.id]: "" }));

    try {
      const response = await fetch(`/api/questions/${question.id}/submit`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ answer }),
      });
      const payload = (await response.json()) as {
        result?: ApiSubmitResult;
        error?: string;
      };

      if (!response.ok || !payload.result) {
        setErrors((current) => ({
          ...current,
          [question.id]: payload.error ?? "提交失败，请稍后重试。",
        }));
        return;
      }

      setResults((current) => ({
        ...current,
        [question.id]: mapSubmitResult(payload.result!),
      }));
    } catch {
      setErrors((current) => ({
        ...current,
        [question.id]: "网络异常，请稍后重试。",
      }));
    } finally {
      setPendingId(null);
    }
  }

  if (questions.length === 0) {
    return (
      <div className="rounded-lg border border-line bg-white p-6 text-sm text-muted shadow-soft">
        当前课程暂无已发布题目。
      </div>
    );
  }

  return (
    <div className="grid gap-5">
      <div className="rounded-lg border border-line bg-white p-5 shadow-soft">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-sm font-semibold text-ink">练习进度</p>
            <p className="mt-1 text-sm text-muted">
              已答 {summary.answered}/{summary.total}，正确 {summary.correct}，错误{" "}
              {summary.wrong}
            </p>
          </div>
          <Link
            href={`/practice/result/${courseId}`}
            className="inline-flex h-10 items-center justify-center rounded-md border border-line bg-panel px-4 text-sm font-semibold text-ink hover:bg-white focus-ring"
          >
            查看结果
          </Link>
        </div>
      </div>

      {questions.map((question, index) => {
        const result = results[question.id];
        const error = errors[question.id];

        return (
          <article
            key={question.id}
            className="rounded-lg border border-line bg-white p-5 shadow-soft"
          >
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex flex-wrap items-center gap-2 text-xs">
                <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                  第 {index + 1} 题
                </span>
                <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                  {questionTypeLabels[question.type]}
                </span>
                {question.knowledgePointTitle ? (
                  <span className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted">
                    {question.knowledgePointTitle}
                  </span>
                ) : null}
              </div>
              <span className="text-xs font-semibold text-muted">
                难度 {question.difficulty}
              </span>
            </div>

            <p className="mt-4 text-base font-semibold leading-7 text-ink">
              {question.stem}
            </p>

            <div className="mt-4 grid gap-3">
              {question.options.map((option) => (
                <label
                  key={option.id}
                  className="flex cursor-pointer items-start gap-3 rounded-lg border border-line bg-panel p-3 text-sm text-ink hover:bg-white"
                >
                  <input
                    type="radio"
                    name={`question-${question.id}`}
                    value={option.id}
                    checked={answers[question.id] === option.id}
                    onChange={(event) =>
                      setAnswers((current) => ({
                        ...current,
                        [question.id]: event.target.value,
                      }))
                    }
                    className="mt-1 size-5 accent-brand"
                  />
                  <span>
                    <span className="font-semibold">{option.id}.</span> {option.text}
                  </span>
                </label>
              ))}
            </div>

            <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <button
                type="button"
                onClick={() => submitAnswer(question)}
                disabled={pendingId === question.id}
                className="inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] disabled:cursor-not-allowed disabled:bg-line disabled:text-muted focus-ring"
              >
                {pendingId === question.id ? "提交中" : result ? "重新提交" : "提交答案"}
              </button>
              {error ? <p className="text-sm font-semibold text-accent">{error}</p> : null}
            </div>

            {result ? (
              <div
                className={`mt-4 rounded-lg border p-4 text-sm ${
                  result.isCorrect
                    ? "border-[#b8dccf] bg-[#f1faf6] text-[#185c48]"
                    : "border-[#e7c5b5] bg-[#fff7f3] text-[#8b3f24]"
                }`}
              >
                <p className="font-semibold">
                  {result.isCorrect ? "答对了" : "答错了"}，正确答案：
                  {result.correctAnswerLabel}
                </p>
                <p className="mt-2 leading-6">{result.explanation}</p>
                {!result.isCorrect ? (
                  <p className="mt-2 text-xs">
                    {result.wrongQuestionSaved
                      ? "已保存到错题本。"
                      : "未登录或当前环境不保存错题。"}
                  </p>
                ) : null}
              </div>
            ) : null}
          </article>
        );
      })}
    </div>
  );
}
