"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import type { PortalPracticeAnswerResponse, PortalPracticeQuestion } from "@/lib/api/types";
import { cn } from "@/lib/cn";

type AnswerResult = PortalPracticeAnswerResponse["data"];

function questionKey(question: PortalPracticeQuestion) {
  return `${question.question_id}:${question.question_version_id}`;
}

interface SheetGridProps {
  questions: PortalPracticeQuestion[];
  currentIndex: number;
  answers: Record<string, AnswerResult>;
  onSelect: (index: number) => void;
  columnsClass?: string;
}

function AnswerSheetGrid({
  questions,
  currentIndex,
  answers,
  onSelect,
  columnsClass = "grid-cols-4",
}: SheetGridProps) {
  return (
    <>
      <div className={cn("mt-3 grid gap-1.5", columnsClass)}>
        {questions.map((item, itemIndex) => {
          const itemResult = answers[questionKey(item)];
          return (
            <button
              key={questionKey(item)}
              type="button"
              onClick={() => onSelect(itemIndex)}
              aria-current={itemIndex === currentIndex ? "step" : undefined}
              aria-label={`第 ${itemIndex + 1} 题${
                itemResult ? (itemResult.correct ? "，答对" : "，答错") : "，未答"
              }`}
              className={cn(
                "flex min-h-9 items-center justify-center border font-mono text-xs transition-colors",
                !itemResult && itemIndex === currentIndex && "border-accent text-accent-text",
                !itemResult && itemIndex !== currentIndex && "border-line text-ink/60 hover:border-ink/40",
                itemResult?.correct && "border-ink bg-ink text-paper",
                itemResult && !itemResult.correct && "border-accent bg-accent text-ink",
                itemIndex === currentIndex && itemResult && "outline-2 outline-offset-2 outline-ink"
              )}
            >
              {String(itemIndex + 1).padStart(2, "0")}
            </button>
          );
        })}
      </div>
      <div className="mt-4 flex flex-wrap gap-x-4 gap-y-1.5 font-mono text-xs text-ink/60">
        <p><span className="mr-2 inline-block h-2 w-2 border border-line align-middle" />未答</p>
        <p><span className="mr-2 inline-block h-2 w-2 bg-ink align-middle" />答对</p>
        <p><span className="mr-2 inline-block h-2 w-2 bg-accent align-middle" />答错</p>
      </div>
    </>
  );
}

export function QuizDesktopAnswerSheet({
  questions,
  currentIndex,
  answers,
  onSelect,
}: Omit<SheetGridProps, "columnsClass">) {
  return (
    <aside data-enter className="hidden lg:block">
      <p className="font-mono text-xs text-ink/60">
        <span className="tracking-[0.25em]">INDEX</span> / 跳题
      </p>
      <AnswerSheetGrid
        questions={questions}
        currentIndex={currentIndex}
        answers={answers}
        onSelect={onSelect}
      />
    </aside>
  );
}

export function QuizMobileAnswerSheetDialog({
  questions,
  currentIndex,
  answers,
  disabled,
  onSelect,
}: Omit<SheetGridProps, "columnsClass"> & { disabled?: boolean }) {
  const [open, setOpen] = useState(false);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <button
          type="button"
          disabled={disabled}
          className="min-h-11 border border-ink/30 px-3 font-mono text-xs text-ink/75 transition-colors hover:border-ink hover:text-ink disabled:cursor-not-allowed disabled:opacity-40"
        >
          答题卡 ({Object.keys(answers).length}/{questions.length})
        </button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>题目导航</DialogTitle>
          <DialogDescription>
            点击题号即可跳转，颜色标记当前进度与作答结果。
          </DialogDescription>
        </DialogHeader>
        <AnswerSheetGrid
          questions={questions}
          currentIndex={currentIndex}
          answers={answers}
          columnsClass="grid-cols-5 sm:grid-cols-6"
          onSelect={(targetIndex) => {
            setOpen(false);
            onSelect(targetIndex);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}
