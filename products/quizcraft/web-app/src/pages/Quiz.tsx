import {
  type FormEvent,
  type TouchEvent,
  memo,
  useEffect,
  useLayoutEffect,
  useMemo,
  useReducer,
  useRef,
} from "react";
import { useNavigate } from "react-router-dom";
import {
  LazyMotion,
  animate,
  domAnimation,
  m,
  useMotionValue,
  useReducedMotion,
} from "framer-motion";
import {
  ChevronLeft,
  ChevronRight,
  Flag,
  CheckCircle2,
  XCircle,
  BookOpen,
  Timer,
  MessageCircle,
  Send,
  X,
} from "lucide-react";
import { useQuizStore } from "@/stores/quizStore";
import { feedbackApi, practiceApi } from "@/api/client";
import {
  QUIZCRAFT_GO_SHADOW_ENABLED,
  clearPendingShadowFeedback,
  createQuizcraftIdempotencyKey,
  getActiveShadowBankId,
  getActiveShadowQuestionVersionId,
  getPendingShadowFeedback,
  isQuizcraftAuthenticationError,
  persistPendingShadowFeedback,
  redirectToHenuKitFavorites,
  shadowFavoritesApi,
  shadowFeedbackApi,
} from "@/api/quizcraftShadowClient";
import { RichText } from "@/components/RichText";
import {
  formatQuestionType,
  formatAnswer,
  formatTime,
  getTypeColor,
  getDifficultyLabel,
} from "@/utils/format";
import { shouldAutoAdvanceAfterAnswer } from "@/utils/quizAutoAdvance";
import {
  runAnswerSubmission,
  type AnswerSubmissionFailure,
} from "@/utils/answerSubmission";
import { feedbackErrorMessage, feedbackStatusLabel } from "@/utils/feedbackStatus";
import { getQuestionOptionKey } from "./quizCardState";
import type { FeedbackStatus, QuestionFeedback } from "@/generated/quizcraft-api";
import type { PracticeState, Question, QuestionType } from "@/types";

type FeedbackCategory = QuestionFeedback["category"];

const normalizeBlankAnswer = (answer: unknown): string =>
  String(answer ?? "").replace(/\s+/g, " ").trim();

const normalizeOptionIndex = (value: unknown): number | null => {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }

  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) return null;

    if (/^[A-Za-z]$/.test(trimmed)) {
      return trimmed.toUpperCase().charCodeAt(0) - 65;
    }

    const parsed = Number(trimmed);
    return Number.isFinite(parsed) ? parsed : null;
  }

  return null;
};

const isOptionSelected = (
  answer: unknown,
  type: QuestionType,
  optionIndex: number,
) => {
  if (type === "multi") {
    return Array.isArray(answer)
      ? answer.some((item) => normalizeOptionIndex(item) === optionIndex)
      : false;
  }

  return normalizeOptionIndex(answer) === optionIndex;
};

const normalizeJudgeAnswer = (answer: unknown): boolean | null => {
  if (typeof answer === "boolean") return answer;
  if (typeof answer === "string") {
    const normalized = answer.trim().toLowerCase();
    if (["true", "1", "yes", "y", "对", "正确"].includes(normalized)) {
      return true;
    }
    if (["false", "0", "no", "n", "错", "错误"].includes(normalized)) {
      return false;
    }
  }
  if (typeof answer === "number") {
    if (answer === 1) return true;
    if (answer === 0) return false;
  }
  return null;
};

const hasSelectedAnswer = (answer: unknown, type: QuestionType) => {
  if (type === "blank") {
    return normalizeBlankAnswer(answer).length > 0;
  }
  if (type === "multi") {
    return Array.isArray(answer) && answer.length > 0;
  }
  if (type === "judge") {
    return normalizeJudgeAnswer(answer) !== null;
  }
  return normalizeOptionIndex(answer) !== null;
};

const SWIPE_THRESHOLD = 50;
const SWIPE_DIRECTION_RATIO = 1.25;
const SWIPE_MAX_DRAG = 120;
const PROGRESS_DOT_WINDOW_RADIUS = 40;
const SLIDE_DURATION = 0.22;
const SLIDE_EASE: [number, number, number, number] = [0.22, 1, 0.36, 1];
const CORRECT_ANSWER_AUTO_ADVANCE_DELAY_MS = 450;

// SWIPE_MAX_DRAG 保留常量便于参数统一管理，但默认实现不再做位移截断，避免影响拖拽跟手效果。

type SwipeStart = {
  x: number;
  y: number;
  currentX: number;
  tracking: boolean;
  horizontal: boolean;
};

const shouldIgnoreSwipeTarget = (target: EventTarget | null) => {
  return (
    target instanceof HTMLElement &&
    Boolean(target.closest('[data-swipe-ignore="true"]'))
  );
};

const clampSwipeDrag = (value: number) => {
  if (SWIPE_MAX_DRAG <= 0) {
    return value;
  }

  return value;
};

const parsePositiveInt = (value: unknown): number | null => {
  if (value === null || value === undefined) {
    return null;
  }

  const text = typeof value === "string" ? value.trim() : String(value);
  if (!text) {
    return null;
  }

  const parsed = Number(text);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : null;
};

const getQuestionMetadataIndex = (
  question: Question | null | undefined,
): number | null => {
  if (!question) {
    return null;
  }

  const fromNumber = parsePositiveInt(question.number);
  if (fromNumber !== null) {
    return fromNumber;
  }

  const match = question.id.match(/(\d+)\s*$/);
  if (!match) return null;
  return parsePositiveInt(match[1]);
};

const resolveFeedbackQuestionIndex = (
  question: Question | null | undefined,
  fallbackIndex: number,
): number => getQuestionMetadataIndex(question) || fallbackIndex;

const JUDGE_OPTIONS = [
  { value: true, label: "对", icon: CheckCircle2 },
  { value: false, label: "错", icon: XCircle },
];

const getProgressDotClass = ({
  current,
  answered,
  correct,
  starred,
}: {
  current: boolean;
  answered: boolean;
  correct?: boolean;
  starred: boolean;
}) => {
  const baseClass = "w-2 h-2 rounded-full transition-all transform-gpu";

  if (starred) {
    return `${baseClass} ${current ? "scale-125 bg-yellow-500 dark:bg-yellow-400" : "bg-yellow-400"}`;
  }

  if (current) {
    return `${baseClass} bg-primary-500 dark:bg-primary-400 scale-125`;
  }

  if (answered) {
    return `${baseClass} ${correct ? "bg-green-400" : "bg-red-400"}`;
  }

  return `${baseClass} bg-gray-200 dark:bg-slate-500`;
};

type ProgressDotsProps = {
  questions: Question[];
  currentIndex: number;
  answers: Record<string, any>;
  results: Record<string, boolean>;
  favoriteQuestionIds: string[];
  disabled: boolean;
  onJump: (index: number) => void;
};

type ProgressDotButtonProps = {
  question: Question;
  index: number;
  currentIndex: number;
  answers: Record<string, any>;
  results: Record<string, boolean>;
  starredSet: Set<string>;
  disabled: boolean;
  onJump: (index: number) => void;
};

const ProgressDotButton = memo(function ProgressDotButton({
  question,
  index,
  currentIndex,
  answers,
  results,
  starredSet,
  disabled,
  onJump,
}: ProgressDotButtonProps) {
  const answered = answers[question.id] !== undefined;

  return (
    <button
      type="button"
      onClick={() => onJump(index)}
      disabled={disabled}
      aria-label={`跳到第 ${index + 1} 题`}
      className={getProgressDotClass({
        current: index === currentIndex,
        answered,
        correct: results[question.id],
        starred: starredSet.has(question.id),
      })}
    />
  );
});

const ProgressDots = memo(function ProgressDots({
  questions,
  currentIndex,
  answers,
  results,
  favoriteQuestionIds,
  disabled,
  onJump,
}: ProgressDotsProps) {
  const starredSet = useMemo(
    () => new Set(favoriteQuestionIds),
    [favoriteQuestionIds],
  );

  if (!questions.length) {
    return null;
  }

  const start = Math.max(0, currentIndex - PROGRESS_DOT_WINDOW_RADIUS);
  const end = Math.min(
    questions.length,
    currentIndex + PROGRESS_DOT_WINDOW_RADIUS + 1,
  );

  return (
    <div
      className="mt-3 w-full overflow-x-auto pb-1"
      data-swipe-ignore="true"
    >
      <div className="flex items-center gap-1 min-w-max px-0.5">
        {start > 0 && (
          <>
            <ProgressDotButton
              key={questions[0].id}
              question={questions[0]}
              index={0}
              currentIndex={currentIndex}
              answers={answers}
              results={results}
              starredSet={starredSet}
              disabled={disabled}
              onJump={onJump}
            />
            <span className="px-1 text-xs text-gray-300">…</span>
          </>
        )}

        {questions.slice(start, end).map((question, offset) => (
          <ProgressDotButton
            key={question.id}
            question={question}
            index={start + offset}
            currentIndex={currentIndex}
            answers={answers}
            results={results}
            starredSet={starredSet}
            disabled={disabled}
            onJump={onJump}
          />
        ))}

        {end < questions.length && (
          <>
            <span className="px-1 text-xs text-gray-300">…</span>
            <ProgressDotButton
              key={questions[questions.length - 1].id}
              question={questions[questions.length - 1]}
              index={questions.length - 1}
              currentIndex={currentIndex}
              answers={answers}
              results={results}
              starredSet={starredSet}
              disabled={disabled}
              onJump={onJump}
            />
          </>
        )}
      </div>
    </div>
  );
});

// 选项组件
function OptionButton({
  label,
  text,
  selected,
  correct,
  missed = false,
  showResult,
  disabled = false,
  onClick,
}: {
  label: string;
  text: string;
  selected: boolean;
  correct?: boolean;
  missed?: boolean;
  showResult: boolean;
  disabled?: boolean;
  onClick: () => void;
}) {
  let bgClass = "bg-white dark:bg-slate-900 border-gray-200 dark:border-slate-600 hover:border-primary-400";

  if (showResult) {
    if (correct && selected) {
      bgClass = "bg-green-50 dark:bg-green-900/20 border-green-500 text-green-800 dark:text-green-200";
    } else if (missed) {
      bgClass = "bg-green-50 dark:bg-green-900/20 border-green-200 text-green-700 dark:text-green-300";
    } else if (selected) {
      bgClass = "bg-red-50 dark:bg-red-900/20 border-red-500 text-red-800 dark:text-red-200";
    } else {
      bgClass = "bg-gray-50 dark:bg-slate-700 border-gray-200 dark:border-slate-700 text-gray-400 dark:text-slate-500";
    }
  } else if (selected) {
    bgClass = "bg-primary-50 dark:bg-slate-500 dark:border-primary-400 border-primary-500 text-primary-700 dark:text-slate-100";
  }

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={showResult || disabled}
      className={`grid grid-cols-[44px_minmax(0,1fr)_auto] items-center gap-3 w-full min-h-[72px] h-auto rounded-xl border-2 p-4 box-border overflow-hidden text-left transition-colors disabled:cursor-not-allowed ${bgClass}`}
    >
      <span
        className={`w-11 h-11 rounded-xl flex items-center justify-center text-sm font-bold ${
          showResult
            ? correct && selected
              ? "bg-green-500 dark:bg-green-600 text-white"
              : missed
                ? "bg-green-200 dark:bg-green-800/60 text-green-700 dark:text-green-100"
                : selected
                  ? "bg-red-500 dark:bg-red-600 text-white"
                  : "bg-gray-200 dark:bg-slate-600 text-gray-500 dark:text-slate-200"
            : selected
              ? "bg-primary-500 dark:bg-primary-600 text-white"
              : "bg-gray-100 dark:bg-slate-800 text-gray-600 dark:text-slate-300"
        }`}
      >
        {label}
      </span>
      <span className="min-w-0 max-w-full whitespace-normal break-words [overflow-wrap:anywhere] text-lg leading-normal">
        <RichText text={text} />
      </span>
      {showResult && correct && selected && (
        <CheckCircle2 className="w-5 h-5 text-green-500 shrink-0 justify-self-end" />
      )}
      {showResult && missed && (
        <CheckCircle2 className="w-5 h-5 text-green-400 shrink-0 justify-self-end" />
      )}
      {showResult && selected && !correct && (
        <XCircle className="w-5 h-5 text-red-500 shrink-0 justify-self-end" />
      )}
    </button>
  );
}

// 判断题选项
function JudgeButtons({
  selected,
  correct,
  showResult,
  disabled = false,
  onSelect,
}: {
  selected: boolean | null;
  correct?: boolean;
  showResult: boolean;
  disabled?: boolean;
  onSelect: (value: boolean) => void;
}) {
  return (
    <div className="flex gap-4">
      {JUDGE_OPTIONS.map((opt) => {
        const isSelected = selected === opt.value;
        const isCorrect = showResult && correct === opt.value;

        let btnClass = "bg-white dark:bg-slate-900 border-gray-200 dark:border-slate-600 hover:border-primary-400";
        if (showResult) {
          if (correct === opt.value) {
            btnClass = "bg-green-50 dark:bg-green-900/20 border-green-500 text-green-800 dark:text-green-200";
          } else if (isSelected) {
            btnClass = "bg-red-50 dark:bg-red-900/20 border-red-500 text-red-800 dark:text-red-200";
          }
        } else if (isSelected) {
          btnClass = "bg-primary-50 dark:bg-slate-500 dark:border-primary-400 border-primary-500 text-primary-700 dark:text-slate-100";
        }

        return (
          <button
            type="button"
            key={opt.label}
            onClick={() => onSelect(opt.value)}
            disabled={showResult || disabled}
            className={`flex-1 flex items-center justify-center gap-2 py-4 rounded-xl border-2 font-medium transition-colors ${btnClass}`}
          >
            <opt.icon
              className={`w-5 h-5 ${isCorrect ? "text-green-500" : isSelected && showResult ? "text-red-500" : ""}`}
            />
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

function BlankAnswerInput({
  value,
  correctAnswer,
  showResult,
  disabled = false,
  onChange,
}: {
  value: unknown;
  correctAnswer?: unknown;
  showResult: boolean;
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  const textValue = typeof value === "string" ? value : "";

  return (
    <div className="space-y-3" data-swipe-ignore="true">
      <input
        aria-label="填空题答案"
        type="text"
        value={textValue}
        onChange={(event) => onChange(event.target.value)}
        disabled={showResult || disabled}
        placeholder="请输入答案"
        className={`w-full rounded-xl border-2 px-4 py-3 text-base outline-none transition-colors ${
          showResult
            ? "border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-700 text-gray-500 dark:text-slate-400 dark:text-slate-500"
            : "border-gray-200 dark:border-slate-700 bg-white dark:bg-slate-800 text-gray-800 dark:text-slate-100 focus:border-primary-400 focus:ring-2 focus:ring-primary-100"
        }`}
      />
      {showResult && (
        <div className="text-sm text-gray-500 dark:text-slate-400 dark:text-slate-500">
          你的答案：{normalizeBlankAnswer(value) || "未作答"}
          {correctAnswer !== undefined && (
            <span className="ml-3">
              标准答案：{formatAnswer(correctAnswer, "blank")}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

type QuestionCardProps = {
  question: Question | null;
  mode:
    | { kind: "preview" }
    | {
        kind: "answering";
        submitDisabled: boolean;
        submitting: boolean;
        submitError: AnswerSubmissionFailure | null;
      }
    | { kind: "result" };
  practice: PracticeState;
  selectedAnswer: any;
  onBlankAnswerChange: (value: string) => void;
  onJudgeSelect: (value: boolean) => void;
  onOptionSelect: (index: number) => void;
  onSubmitCurrent: () => void;
  onRetrySubmit: () => void;
  onCancelSubmit: () => void;
  onTouchStart: (event: TouchEvent<HTMLDivElement>) => void;
  onTouchMove: (event: TouchEvent<HTMLDivElement>) => void;
  onTouchEnd: (event: TouchEvent<HTMLDivElement>) => void;
  onTouchCancel: () => void;
};

type QuizUiState = {
  selectedAnswer: any;
  showResult: boolean;
  answerSubmitting: boolean;
  answerSubmitError: AnswerSubmissionFailure | null;
  pendingAnswer: { questionId: string; answer: any } | null;
  elapsedTime: number;
  isSliding: boolean;
  visualCurrentIndex: number;
  feedbackCategory: FeedbackCategory;
  feedbackSuggestion: string;
  feedbackRequestKey: string | null;
  feedbackID: string | null;
  feedbackStatus: FeedbackStatus | null;
  feedbackSubmitting: boolean;
  feedbackMessage: string;
  feedbackError: string;
  serverFavoriteIds: string[];
  favoriteSubmitting: boolean;
  favoriteError: string;
};

const initialQuizUiState: QuizUiState = {
  selectedAnswer: null,
  showResult: false,
  answerSubmitting: false,
  answerSubmitError: null,
  pendingAnswer: null,
  elapsedTime: 0,
  isSliding: false,
  visualCurrentIndex: 0,
  feedbackCategory: "other",
  feedbackSuggestion: "",
  feedbackRequestKey: null,
  feedbackID: null,
  feedbackStatus: null,
  feedbackSubmitting: false,
  feedbackMessage: "",
  feedbackError: "",
  serverFavoriteIds: [],
  favoriteSubmitting: false,
  favoriteError: "",
};

const mergeQuizUiState = (
  state: QuizUiState,
  updates: Partial<QuizUiState>,
) => ({
  ...state,
  ...updates,
});

const ignoreBlankAnswerChange = () => undefined;
const ignoreJudgeSelect = () => undefined;
const ignoreOptionSelect = () => undefined;

function QuestionCard({
  question,
  mode,
  practice,
  selectedAnswer,
  onBlankAnswerChange,
  onJudgeSelect,
  onOptionSelect,
  onSubmitCurrent,
  onRetrySubmit,
  onCancelSubmit,
  onTouchStart,
  onTouchMove,
  onTouchEnd,
  onTouchCancel,
}: QuestionCardProps) {
  if (!question) {
    return (
      <div className="w-full flex-shrink-0 min-w-full px-0.5">
        <div className="bg-white dark:bg-slate-800 rounded-2xl border border-transparent p-6 min-h-[300px]" />
      </div>
    );
  }

  const isCurrent = mode.kind !== "preview";
  const savedAnswer = practice.answers[question.id];
  const hasSavedAnswer = savedAnswer !== undefined;
  const cardAnswer = isCurrent
    ? selectedAnswer ?? (hasSavedAnswer ? savedAnswer : null)
    : hasSavedAnswer
      ? savedAnswer
      : null;
  const cardShowResult = isCurrent ? mode.kind === "result" : hasSavedAnswer;
  const answerLocked = Boolean(
    isCurrent &&
      mode.kind === "answering" &&
      (mode.submitting || mode.submitError),
  );
  const result =
    hasSavedAnswer
      ? {
          correct: practice.results[question.id],
          correctAnswer: practice.correctAnswers[question.id],
          analysis: practice.analyses[question.id],
        }
      : null;

  const difficulty = question.stats
    ? getDifficultyLabel(question.stats.rate)
    : null;

  return (
    <div className="w-full flex-shrink-0 min-w-full px-0.5">
      <div
        onTouchStart={isCurrent ? onTouchStart : undefined}
        onTouchMove={isCurrent ? onTouchMove : undefined}
        onTouchEnd={isCurrent ? onTouchEnd : undefined}
        onTouchCancel={isCurrent ? onTouchCancel : undefined}
        className={`bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 shadow-sm dark:shadow-slate-900/30 p-6 touch-pan-y will-change-transform ${
          isCurrent ? "" : "pointer-events-none"
        }`}
      >
        <div className="flex items-center gap-2 mb-4">
          <span
            className={`px-2.5 py-1 rounded-lg text-xs font-medium ${getTypeColor(question.type)}`}
          >
            {formatQuestionType(question.type)}
          </span>
          {difficulty && (
            <span
              className={`px-2.5 py-1 rounded-lg text-xs font-medium ${difficulty.color}`}
            >
              {difficulty.label} · 正确率 {question.stats?.rate}%
            </span>
          )}
          <span className="text-xs text-gray-400 dark:text-slate-500">{question.chapter}</span>
        </div>

        <h2 className="text-lg font-medium text-gray-800 dark:text-slate-100 mb-6 leading-relaxed">
          <RichText text={question.content} />
        </h2>

        <div className="space-y-3 mb-6">
          {question.type === "blank" ? (
            <BlankAnswerInput
              value={cardAnswer}
              correctAnswer={cardShowResult ? result?.correctAnswer : undefined}
              showResult={cardShowResult}
              disabled={answerLocked}
              onChange={isCurrent ? onBlankAnswerChange : ignoreBlankAnswerChange}
            />
          ) : question.type === "judge" ? (
            <JudgeButtons
              selected={normalizeJudgeAnswer(cardAnswer)}
              correct={
                cardShowResult
                  ? normalizeJudgeAnswer(result?.correctAnswer) ?? undefined
                  : undefined
              }
              showResult={cardShowResult}
              disabled={answerLocked}
              onSelect={isCurrent ? onJudgeSelect : ignoreJudgeSelect}
            />
          ) : (
            question.options?.map((option, index) => {
              const isSelected = isOptionSelected(
                cardAnswer,
                question.type,
                index,
              );

              const isCorrect = cardShowResult
                ? isOptionSelected(result?.correctAnswer, question.type, index)
                : false;

              const isMissed = Boolean(
                cardShowResult &&
                  question.type === "multi" &&
                  isCorrect &&
                  !isSelected,
              );

              return (
                <OptionButton
                  key={getQuestionOptionKey(question.id, index)}
                  label={String.fromCharCode(65 + index)}
                  text={option}
                  selected={Boolean(isSelected)}
                  correct={isCorrect}
                  missed={isMissed}
                  showResult={cardShowResult}
                  disabled={answerLocked}
                  onClick={
                    isCurrent ? () => onOptionSelect(index) : ignoreOptionSelect
                  }
                />
              );
            })
          )}
        </div>

        {mode.kind === "answering" && (
          <div className="min-h-[52px] mb-4 flex items-end">
            <button
              type="button"
              onClick={onSubmitCurrent}
              disabled={mode.submitDisabled}
              className="w-full py-3 bg-primary-500 text-white font-medium rounded-xl hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {mode.submitting ? "正在提交..." : "提交答案"}
            </button>
          </div>
        )}

        {mode.kind === "answering" && mode.submitError && (
          <div
            role="alert"
            className="mb-4 rounded-xl border border-amber-200 bg-amber-50 p-4 text-amber-900 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-100"
          >
            <div className="font-medium">{mode.submitError.title}</div>
            <p className="mt-1 text-sm leading-6">{mode.submitError.message}</p>
            <div className="mt-3 flex flex-wrap gap-2">
              <button
                type="button"
                onClick={onRetrySubmit}
                className="rounded-lg bg-amber-600 px-3 py-2 text-sm font-medium text-white hover:bg-amber-700"
              >
                重试提交
              </button>
              <button
                type="button"
                onClick={onCancelSubmit}
                className="rounded-lg border border-amber-300 px-3 py-2 text-sm font-medium text-amber-800 hover:bg-amber-100 dark:border-amber-700 dark:text-amber-100 dark:hover:bg-amber-900/40"
              >
                修改答案
              </button>
            </div>
          </div>
        )}

        {mode.kind === "result" && result && (
          <div
            className={`rounded-xl p-4 mb-4 ${result.correct ? "bg-green-50 dark:bg-green-900/20 border border-green-100 dark:border-green-800" : "bg-red-50 dark:bg-red-900/20 border border-red-100 dark:border-red-800"}`}
          >
            <div className="flex items-center gap-2 mb-2">
              {result.correct ? (
                <>
                  <CheckCircle2 className="w-5 h-5 text-green-500" />
                  <span className="font-medium text-green-800 dark:text-green-200">
                    回答正确！
                  </span>
                </>
              ) : (
                <>
                  <XCircle className="w-5 h-5 text-red-500" />
                  <span className="font-medium text-red-800 dark:text-red-200">回答错误</span>
                </>
              )}
            </div>

            {!result.correct && (
              <div className="text-sm text-gray-700 dark:text-slate-200 mb-2">
                正确答案：
                <span className="font-medium text-green-700 dark:text-green-300">
                  {formatAnswer(result.correctAnswer, question.type)}
                </span>
              </div>
            )}

            {result.analysis && (
              <div className="mt-3 pt-3 border-t border-gray-200 dark:border-slate-700/50">
                <div className="text-sm font-medium text-gray-700 dark:text-slate-200 mb-1">
                  解析
                </div>
                <div className="text-sm text-gray-600 dark:text-slate-300 leading-relaxed">
                  <RichText text={result.analysis} />
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function useQuizController() {
  const navigate = useNavigate();
  const {
    practice,
    currentBank,
    answerQuestion,
    jumpToQuestion,
    finishPractice,
    banks,
    setUser,
  } = useQuizStore();

  const [ui, setUi] = useReducer(mergeQuizUiState, initialQuizUiState);
  const {
    selectedAnswer,
    showResult,
    answerSubmitting,
    answerSubmitError,
    pendingAnswer,
    elapsedTime,
    isSliding,
    visualCurrentIndex,
    feedbackCategory,
    feedbackSuggestion,
    feedbackRequestKey,
    feedbackID,
    feedbackStatus,
    favoriteSubmitting,
    favoriteError,
    feedbackSubmitting,
    feedbackMessage,
    feedbackError,
  } = ui;
  const prefersReducedMotion = useReducedMotion();
  const trackX = useMotionValue(0);
  const feedbackDialogRef = useRef<HTMLDialogElement | null>(null);
  const autoAdvanceTimerRef = useRef<number | null>(null);
  const viewportWidthRef = useRef(0);
  const dragXRef = useRef(0);
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const swipeStartRef = useRef<SwipeStart | null>(null);
  const favoriteReturnHandledRef = useRef(false);
  const favoriteMutationVersionRef = useRef(0);
  const serverFavoriteIdsRef = useRef<string[]>([]);


  const currentIndex = practice?.currentIndex ?? -1;
  const hasPractice = Boolean(practice);
  const activeQuestion = practice?.questions[currentIndex];
  const activeQuestionId = activeQuestion?.id;
  const activeBankKey = practice?.bankKey || currentBank;
  const activeBankId = banks.find((bank) => bank.key === activeBankKey)?.bank_id
    || (QUIZCRAFT_GO_SHADOW_ENABLED ? getActiveShadowBankId() : undefined);
  const activeAnswer = activeQuestionId
    ? practice?.answers[activeQuestionId]
    : undefined;

  useLayoutEffect(() => {
    if (!viewportRef.current) return;

    const node = viewportRef.current;
    const updateWidth = () => {
      const width = Math.floor(node.getBoundingClientRect().width);
      const nextWidth = width > 0 ? width : 0;
      viewportWidthRef.current = nextWidth;
      if (nextWidth > 0) {
        trackX.set(-nextWidth);
      }
    };

    updateWidth();

    const ro = new ResizeObserver(() => {
      updateWidth();
    });

    ro.observe(node);
    return () => ro.disconnect();
  }, [hasPractice, trackX]);

  useEffect(() => {
    if (!hasPractice) {
      navigate("/practice");
    }
  }, [hasPractice, navigate]);

  useEffect(() => {
    if (!QUIZCRAFT_GO_SHADOW_ENABLED || !activeBankId) return;
    let cancelled = false;
    const mutationVersion = favoriteMutationVersionRef.current;
    serverFavoriteIdsRef.current = [];
    setUi({ serverFavoriteIds: [], favoriteError: "" });
    void shadowFavoritesApi.list(activeBankId).then((items) => {
      if (!cancelled && mutationVersion === favoriteMutationVersionRef.current) {
        const favoriteIds = items.filter((item) => item.available).map((item) => item.question_id);
        serverFavoriteIdsRef.current = favoriteIds;
        setUi({
          serverFavoriteIds: favoriteIds,
          favoriteError: "",
        });
      }
    }).catch((error) => {
      if (!cancelled && mutationVersion === favoriteMutationVersionRef.current) {
        serverFavoriteIdsRef.current = [];
        setUi({
          serverFavoriteIds: [],
          favoriteError: isQuizcraftAuthenticationError(error)
            ? "登录后才能查看和管理收藏"
            : "收藏状态加载失败，请检查网络后重试",
        });
      }
    });
    return () => { cancelled = true; };
  }, [activeBankId]);

  useEffect(() => {
    if (!QUIZCRAFT_GO_SHADOW_ENABLED || !activeQuestionId || favoriteReturnHandledRef.current) return;
    const url = new URL(window.location.href);
    const pendingQuestionId = url.searchParams.get('favorite_question_id');
    const pendingBankId = url.searchParams.get('favorite_bank_id') || activeBankId;
    let browserIntent: { bankId?: string; questionId?: string; favorite?: boolean } = {};
    try {
      browserIntent = JSON.parse(window.sessionStorage.getItem('quizcraft_pending_favorite') || '{}');
    } catch {
      browserIntent = {};
    }
    if (!pendingBankId || !pendingQuestionId || pendingQuestionId !== activeQuestionId || browserIntent.bankId !== pendingBankId || browserIntent.questionId !== pendingQuestionId || typeof browserIntent.favorite !== 'boolean') return;
    const pendingFavorite = browserIntent.favorite;
    favoriteReturnHandledRef.current = true;
    favoriteMutationVersionRef.current += 1;
    window.sessionStorage.removeItem('quizcraft_pending_favorite');
    setUi({ favoriteSubmitting: true });
    void shadowFavoritesApi.set(pendingBankId, pendingQuestionId, pendingFavorite).then(() => {
      favoriteMutationVersionRef.current += 1;
      const mutationVersion = favoriteMutationVersionRef.current;
      const confirmedIds = pendingFavorite
        ? Array.from(new Set([...serverFavoriteIdsRef.current, pendingQuestionId]))
        : serverFavoriteIdsRef.current.filter((id) => id !== pendingQuestionId);
      serverFavoriteIdsRef.current = confirmedIds;
      setUi({
        serverFavoriteIds: confirmedIds,
        favoriteSubmitting: false,
      });
      url.searchParams.delete('favorite_question_id');
      url.searchParams.delete('favorite_bank_id');
      window.history.replaceState({}, '', `${url.pathname}${url.search}${url.hash}`);

      // The write is already confirmed, so never keep the UI or return URL
      // blocked on a read. Reconcile the rest of the bank in the background;
      // a later mutation generation always wins over this refresh.
      void shadowFavoritesApi.list(pendingBankId).then((items) => {
        if (mutationVersion !== favoriteMutationVersionRef.current) return;
        const authoritativeIds = items.filter((item) => item.available).map((item) => item.question_id);
        const favoriteIds = pendingFavorite
          ? Array.from(new Set([...authoritativeIds, pendingQuestionId]))
          : authoritativeIds.filter((id) => id !== pendingQuestionId);
        serverFavoriteIdsRef.current = favoriteIds;
        setUi({ serverFavoriteIds: favoriteIds });
      }).catch(() => {
        // Keep the already-confirmed current-question state. A later page or
        // bank refresh will retry the authoritative list read.
      });
    }).catch((error) => {
      setUi({
        favoriteSubmitting: false,
        favoriteError: isQuizcraftAuthenticationError(error)
          ? "请前往练习服务使用收藏"
          : "收藏失败，请稍后重试",
      });
      if (isQuizcraftAuthenticationError(error)) {
        try {
          redirectToHenuKitFavorites();
        } catch {
          setUi({ favoriteError: "暂时无法打开练习服务，请稍后重试" });
        }
      }
    });
  }, [activeBankId, activeQuestionId]);

  useEffect(() => () => {
    if (autoAdvanceTimerRef.current !== null) {
      window.clearTimeout(autoAdvanceTimerRef.current);
      autoAdvanceTimerRef.current = null;
    }
  }, [activeQuestionId]);

  useLayoutEffect(() => {
    if (!hasPractice) return;

    if (activeAnswer !== undefined) {
      setUi({
        selectedAnswer: activeAnswer,
        showResult: true,
        answerSubmitting: false,
        answerSubmitError: null,
        pendingAnswer: null,
      });
    } else {
      setUi({
        selectedAnswer: null,
        showResult: false,
        answerSubmitting: false,
        answerSubmitError: null,
        pendingAnswer: null,
      });
    }
  }, [hasPractice, activeQuestionId, activeAnswer]);

  useLayoutEffect(() => {
    if (!hasPractice || !practice) return;

    setUi({ visualCurrentIndex: practice.currentIndex });
  }, [hasPractice, practice?.currentIndex, practice]);

  useLayoutEffect(() => {
    const viewportWidth = viewportWidthRef.current;
    if (!hasPractice || !viewportWidth) {
      return;
    }

    trackX.set(-viewportWidth);
    dragXRef.current = 0;
    swipeStartRef.current = null;
    setUi({ isSliding: false });
  }, [hasPractice, currentIndex, practice?.questions.length, trackX]);

  const startTime = practice?.startTime;
  const isFinished = practice?.isFinished ?? false;

  useEffect(() => {
    if (!startTime || isFinished) return;

    const timer = setInterval(() => {
      setUi({ elapsedTime: Math.floor((Date.now() - startTime) / 1000) });
    }, 1000);

    return () => clearInterval(timer);
  }, [startTime, isFinished, practice?.startTime, practice?.isFinished]);

  const practiceQuestionCount = practice?.questions.length ?? 0;
  const visualIndex = practice && practiceQuestionCount > 0
    ? Math.min(
      Math.max(visualCurrentIndex, 0),
      practiceQuestionCount - 1,
    )
    : 0;
  const feedbackQuestionIndex = resolveFeedbackQuestionIndex(
    activeQuestion,
    visualIndex + 1,
  );

  useEffect(() => {
    if (!hasPractice || practiceQuestionCount === 0) {
      return;
    }
    try {
      // 与题目索引脱钩，优先使用题库元数据 index（如 question.number 或 id 后缀）
      localStorage.setItem(
        "quizcraft_last_feedback_question_index",
        String(feedbackQuestionIndex),
      );
    } catch {
      // ignore storage errors
    }
  }, [
    hasPractice,
    practiceQuestionCount,
    feedbackQuestionIndex,
    practice?.questions.length,
  ]);

  if (!hasPractice || !practice || !activeQuestion) return null;

  const progress = ((visualIndex + 1) / practice.questions.length) * 100;

  const prevQuestion = currentIndex > 0 ? practice.questions[currentIndex - 1] : null;
  const nextQuestionRef =
    currentIndex < practice.questions.length - 1
      ? practice.questions[currentIndex + 1]
      : null;

  const canSubmitCurrent = Boolean(
    activeQuestion && hasSelectedAnswer(selectedAnswer, activeQuestion.type),
  );
  const submissionLocked = answerSubmitting || pendingAnswer !== null;

  const openFeedbackModal = () => {
    const questionVersionId = QUIZCRAFT_GO_SHADOW_ENABLED
      ? getActiveShadowQuestionVersionId(activeQuestion.id)
      : undefined;
    const pendingFeedback = activeBankId && questionVersionId
      ? getPendingShadowFeedback({
          bank_id: activeBankId,
          question_id: activeQuestion.id,
          question_version_id: questionVersionId,
        })
      : null;
    setUi({
      feedbackCategory: pendingFeedback?.category || "other",
      feedbackSuggestion: pendingFeedback?.detail || "",
      feedbackRequestKey: pendingFeedback?.idempotencyKey || null,
      feedbackID: null,
      feedbackStatus: null,
      feedbackMessage: "",
      feedbackError: "",
    });
    feedbackDialogRef.current?.showModal();
  };

  const closeFeedbackModal = () => {
    if (feedbackSubmitting) return;
    feedbackDialogRef.current?.close();
  };

  const handleFeedbackSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (!activeQuestion) return;

    const normalizedSuggestion = feedbackSuggestion.trim();
    if (!normalizedSuggestion) {
      setUi({ feedbackError: "建议改正内容不能为空" });
      return;
    }

    if (!QUIZCRAFT_GO_SHADOW_ENABLED) {
      setUi({
        feedbackSubmitting: true,
        feedbackError: "",
        feedbackMessage: "",
      });
      try {
        await feedbackApi.submit({
          question_index: feedbackQuestionIndex,
          question_bank: activeBankKey || undefined,
          question_id: activeQuestion.id,
          question_content: activeQuestion.content,
          suggestion: normalizedSuggestion,
        });
        setUi({
          feedbackMessage: "反馈提交成功，感谢你的建议！",
          feedbackSuggestion: "",
        });
      } catch (error) {
        setUi({ feedbackError: feedbackErrorMessage(error, "提交反馈暂时失败，请保持页面并重试。") });
      } finally {
        setUi({ feedbackSubmitting: false });
      }
      return;
    }

    const questionVersionId = getActiveShadowQuestionVersionId(activeQuestion.id);
    if (!activeBankId || !questionVersionId) {
      setUi({ feedbackError: "题目数据暂时不完整，请重新进入练习" });
      return;
    }
    const idempotencyKey = feedbackRequestKey || createQuizcraftIdempotencyKey();
    const pendingFeedback = {
      bank_id: activeBankId,
      question_id: activeQuestion.id,
      question_version_id: questionVersionId,
      category: feedbackCategory,
      detail: normalizedSuggestion,
      idempotencyKey,
    };
    try {
      if (!persistPendingShadowFeedback(pendingFeedback)) {
        throw new Error("无法保存当前影子练习的安全重试凭据");
      }
    } catch {
      setUi({ feedbackError: "浏览器存储不可用，请刷新页面后重试" });
      return;
    }
    setUi({
      feedbackSubmitting: true,
      feedbackRequestKey: idempotencyKey,
      feedbackError: "",
      feedbackMessage: "",
    });
    try {
      const feedbackPayload = {
        bank_id: pendingFeedback.bank_id,
        question_id: pendingFeedback.question_id,
        question_version_id: pendingFeedback.question_version_id,
        category: pendingFeedback.category,
        detail: pendingFeedback.detail,
      };
      const accepted = await shadowFeedbackApi.submit(feedbackPayload, idempotencyKey);
      const feedbackId = accepted.data.resource_id;
      if (!feedbackId) {
        setUi({
          feedbackSubmitting: false,
          feedbackSuggestion: "",
          feedbackMessage: "反馈已收到，状态稍后更新",
        });
        return;
      }
      try {
        clearPendingShadowFeedback(pendingFeedback);
      } catch {
        // The accepted server write is authoritative even if local retry cleanup fails.
      }
      setUi({
        feedbackID: feedbackId,
        feedbackStatus: null,
        feedbackSuggestion: "",
        feedbackRequestKey: null,
      });
      try {
        const feedback = await shadowFeedbackApi.status(feedbackId);
        setUi({
          feedbackStatus: feedback,
          feedbackMessage: `反馈已保存，当前状态：${feedbackStatusLabel(feedback.status)}。`,
        });
      } catch {
        // The accepted write is durable even if the follow-up status read is unavailable.
        setUi({
          feedbackMessage: "反馈已保存，但处理状态暂时不可用。",
          feedbackError: "请重试读取状态，或到“我的反馈”查看已保存的记录。",
        });
      }
    } catch (error) {
      setUi({ feedbackError: feedbackErrorMessage(error, "提交反馈暂时失败，请保持页面并重试。") });
    } finally {
      setUi({ feedbackSubmitting: false });
    }
  };

  const retryFeedbackStatus = async () => {
    if (!feedbackID || feedbackSubmitting) return;
    setUi({ feedbackSubmitting: true, feedbackError: "", feedbackMessage: "" });
    try {
      const feedback = await shadowFeedbackApi.status(feedbackID);
      setUi({
        feedbackStatus: feedback,
        feedbackMessage: `当前状态：${feedbackStatusLabel(feedback.status)}。`,
      });
    } catch {
      setUi({
        feedbackMessage: "反馈已保存，但处理状态暂时不可用。",
        feedbackError: "请稍后重试，或到“我的反馈”查看已保存的记录。",
      });
    } finally {
      setUi({ feedbackSubmitting: false });
    }
  };

  const openFeedbackHistory = () => {
    if (feedbackSubmitting) return;
    feedbackDialogRef.current?.close();
    navigate("/feedback");
  };

  const clearPendingFeedbackForCurrentQuestion = () => {
    if (!QUIZCRAFT_GO_SHADOW_ENABLED || !activeBankId || !activeQuestion) return;
    const questionVersionId = getActiveShadowQuestionVersionId(activeQuestion.id);
    if (!questionVersionId) return;
    try {
      clearPendingShadowFeedback({
        bank_id: activeBankId,
        question_id: activeQuestion.id,
        question_version_id: questionVersionId,
      });
    } catch {
      // A changed draft will overwrite the same session entry before its next write.
    }
  };

  const alignTrackToCenter = (animateBack = true) => {
    const viewportWidth = viewportWidthRef.current;
    if (!viewportWidth) {
      return;
    }

    if (animateBack) {
      animate(trackX, -viewportWidth, {
        duration: prefersReducedMotion ? 0 : 0.12,
        ease: "easeOut",
      });
    } else {
      trackX.set(-viewportWidth);
    }
  };

  const setTrackDragPosition = (value: number) => {
    const viewportWidth = viewportWidthRef.current;
    if (!viewportWidth) {
      return;
    }

    const processed = clampSwipeDrag(value);
    dragXRef.current = processed;
    trackX.set(-viewportWidth + processed);
  };

  const resetSwipeState = (animateBack = true) => {
    swipeStartRef.current = null;
    dragXRef.current = 0;
    alignTrackToCenter(animateBack);
  };

  const getQuestionViewState = (question: Question | null | undefined) => {
    const questionId = question?.id;
    if (!questionId) {
      return { selectedAnswer: null, showResult: false };
    }

    const savedAnswer = practice.answers[questionId];
    return savedAnswer !== undefined
      ? { selectedAnswer: savedAnswer, showResult: true }
      : { selectedAnswer: null, showResult: false };
  };

  const slideToIndex = (
    targetIndex: number,
    allowSubmissionTransition = false,
  ) => {
    if (
      !practice ||
      isSliding ||
      (submissionLocked && !allowSubmissionTransition)
    ) {
      return;
    }

    const { currentIndex, questions } = practice;
    const maxIndex = questions.length - 1;
    const nextIndex = Math.max(0, Math.min(maxIndex, targetIndex));

    if (nextIndex === currentIndex) return;

    const viewportWidth = viewportWidthRef.current;
    const nextQuestion = questions[nextIndex] ?? null;
    const nextQuestionViewState = getQuestionViewState(nextQuestion);

    if (!viewportWidth || Math.abs(nextIndex - currentIndex) > 1) {
      setUi({ visualCurrentIndex: nextIndex, ...nextQuestionViewState });
      jumpToQuestion(nextIndex);
      trackX.set(-viewportWidth);
      return;
    }

    const targetX = nextIndex > currentIndex ? -2 * viewportWidth : 0;

    setUi({ visualCurrentIndex: nextIndex, isSliding: true });
    const controls = animate(trackX, targetX, {
      duration: prefersReducedMotion ? 0 : SLIDE_DURATION,
      ease: SLIDE_EASE,
    });

    void controls.then(() => {
      jumpToQuestion(nextIndex);
      trackX.set(-viewportWidth);
      setUi({ ...nextQuestionViewState, isSliding: false });
    });
  };

  const handleJump = (index: number) => {
    if (!practice) return;
    if (index === practice.currentIndex) return;
    if (index < 0 || index >= practice.questions.length) return;

    slideToIndex(index);
  };

  const handleNext = () => {
    if (!practice || isSliding || submissionLocked) return;

    if (practice.currentIndex < practice.questions.length - 1) {
      slideToIndex(practice.currentIndex + 1);
    } else {
      finishPractice();
      navigate("/result");
    }
  };

  const handlePrev = () => {
    if (!practice || isSliding || submissionLocked) return;
    if (practice.currentIndex <= 0) {
      return;
    }

    slideToIndex(practice.currentIndex - 1);
  };

  const scheduleCorrectAnswerAutoAdvance = (
    questionId: string,
    isCorrect: boolean,
  ) => {
    const latestPractice = useQuizStore.getState().practice;
    if (!latestPractice) return;

    const latestQuestion = latestPractice.questions[latestPractice.currentIndex];
    if (latestQuestion?.id !== questionId) return;
    if (
      !shouldAutoAdvanceAfterAnswer({
        isCorrect,
        currentIndex: latestPractice.currentIndex,
        questionCount: latestPractice.questions.length,
      })
    ) {
      return;
    }

    if (autoAdvanceTimerRef.current !== null) {
      window.clearTimeout(autoAdvanceTimerRef.current);
    }

    autoAdvanceTimerRef.current = window.setTimeout(() => {
      autoAdvanceTimerRef.current = null;
      const currentPractice = useQuizStore.getState().practice;
      if (!currentPractice) return;

      const currentQuestion =
        currentPractice.questions[currentPractice.currentIndex];
      if (currentQuestion?.id !== questionId) return;
      if (
        shouldAutoAdvanceAfterAnswer({
          isCorrect,
          currentIndex: currentPractice.currentIndex,
          questionCount: currentPractice.questions.length,
        })
      ) {
        slideToIndex(currentPractice.currentIndex + 1, true);
      }
    }, CORRECT_ANSWER_AUTO_ADVANCE_DELAY_MS);
  };

  const handleCardTouchStart = (event: React.TouchEvent<HTMLDivElement>) => {
    const viewportWidth = viewportWidthRef.current;
    if (
      isSliding ||
      submissionLocked ||
      !viewportWidth ||
      event.touches.length !== 1 ||
      shouldIgnoreSwipeTarget(event.target)
    ) {
      return;
    }

    const touch = event.touches[0];
    swipeStartRef.current = {
      x: touch.clientX,
      y: touch.clientY,
      currentX: 0,
      tracking: true,
      horizontal: false,
    };

    dragXRef.current = 0;
    trackX.set(-viewportWidth);
  };

  const handleCardTouchMove = (event: React.TouchEvent<HTMLDivElement>) => {
    const start = swipeStartRef.current;
    const viewportWidth = viewportWidthRef.current;
    if (!start?.tracking || event.touches.length !== 1 || !viewportWidth) return;

    const touch = event.touches[0];
    const deltaX = touch.clientX - start.x;
    const deltaY = touch.clientY - start.y;
    const absX = Math.abs(deltaX);
    const absY = Math.abs(deltaY);

    if (!start.horizontal) {
      if (absY > 12 && absY > absX) {
        resetSwipeState();
        return;
      }
      if (absX < 12 || absX < absY * SWIPE_DIRECTION_RATIO) {
        return;
      }
      start.horizontal = true;
    }

    start.currentX = deltaX;
    setTrackDragPosition(deltaX);
  };

  const handleCardTouchEnd = (event: React.TouchEvent<HTMLDivElement>) => {
    const start = swipeStartRef.current;
    const viewportWidth = viewportWidthRef.current;
    if (!start?.tracking || !viewportWidth) {
      resetSwipeState();
      return;
    }

    const changedTouch = event.changedTouches[0];
    const fallbackDeltaX = changedTouch
      ? changedTouch.clientX - start.x
      : dragXRef.current;
    const fallbackDeltaY = changedTouch ? changedTouch.clientY - start.y : 0;
    const finalX = clampSwipeDrag(start.currentX || fallbackDeltaX);
    const finalIsHorizontal =
      start.horizontal ||
      (Math.abs(fallbackDeltaX) >= SWIPE_THRESHOLD &&
        Math.abs(fallbackDeltaX) >=
          Math.abs(fallbackDeltaY) * SWIPE_DIRECTION_RATIO);

    if (!finalIsHorizontal || Math.abs(finalX) < SWIPE_THRESHOLD) {
      resetSwipeState();
      return;
    }

    if (finalX < 0) {
      if (practice.currentIndex < practice.questions.length - 1) {
        slideToIndex(practice.currentIndex + 1);
      } else {
        resetSwipeState();
      }

      return;
    }

    if (practice.currentIndex > 0) {
      slideToIndex(practice.currentIndex - 1);
      return;
    }

    resetSwipeState();
  };

  const handleOptionSelect = (index: number) => {
    if (showResult || submissionLocked) return;

    if (activeQuestion.type === "multi") {
      // 多选题：切换选择
      const current = (selectedAnswer as number[]) || [];
      const newAnswer = current.includes(index)
        ? current.filter((i) => i !== index)
        : [...current, index].sort();
      setUi({ selectedAnswer: newAnswer });
    } else {
      // 单选题：只更新选中
      setUi({ selectedAnswer: index });
    }
  };

  const handleSubmitCurrent = () => {
    if (!activeQuestion || showResult || !canSubmitCurrent || isSliding || submissionLocked) return;

    if (
      activeQuestion.type === "multi" &&
      (!Array.isArray(selectedAnswer) || selectedAnswer.length === 0)
    ) {
      return;
    }

    void submitAnswer(
      activeQuestion.type === "blank"
        ? normalizeBlankAnswer(selectedAnswer)
        : selectedAnswer,
    );
  };

  const submitAnswer = async (
    answer: any,
    questionId = activeQuestion.id,
  ) => {
    if (!activeBankKey || activeQuestion.id !== questionId) return;

    setUi({
      answerSubmitting: true,
      answerSubmitError: null,
      pendingAnswer: { questionId, answer },
    });

    const result = await runAnswerSubmission(() =>
      practiceApi.submitAnswer(activeBankKey, questionId, answer),
    );

    if (!result.ok) {
      const latestPractice = useQuizStore.getState().practice;
      const latestQuestion =
        latestPractice?.questions[latestPractice.currentIndex];
      setUi(
        latestQuestion?.id === questionId
          ? { answerSubmitting: false, answerSubmitError: result.error }
          : {
              answerSubmitting: false,
              answerSubmitError: null,
              pendingAnswer: null,
            },
      );
      return;
    }

    const res = result.response;
    if (res.user_stats?.userId) {
      localStorage.setItem("user_id", res.user_stats.userId);
      setUser(res.user_stats);
    }
    // 以后如果后端隐藏答案或规则调整，以后端结果为准进行一次轻量校正。
    answerQuestion({
      questionId,
      answer,
      isCorrect: res.correct,
      correctAnswer: res.correct_answer,
      analysis: res.analysis,
    });
    const latestPractice = useQuizStore.getState().practice;
    const latestQuestion =
      latestPractice?.questions[latestPractice.currentIndex];
    setUi({
      answerSubmitting: false,
      answerSubmitError: null,
      pendingAnswer: null,
      showResult: latestQuestion?.id === questionId,
    });
    scheduleCorrectAnswerAutoAdvance(questionId, res.correct);
  };

  const retryAnswerSubmission = () => {
    if (!pendingAnswer || answerSubmitting) return;
    if (pendingAnswer.questionId !== activeQuestion.id) {
      setUi({ answerSubmitError: null, pendingAnswer: null });
      return;
    }
    void submitAnswer(pendingAnswer.answer, pendingAnswer.questionId);
  };

  const cancelAnswerSubmission = () => {
    if (answerSubmitting) return;
    setUi({ answerSubmitError: null, pendingAnswer: null });
  };

  const handleJudgeSelect = (value: boolean) => {
    if (showResult || submissionLocked) return;
    setUi({ selectedAnswer: value });
  };

  const favorited = ui.serverFavoriteIds.includes(activeQuestion.id);

  const handleToggleFavorite = () => {
    if (!QUIZCRAFT_GO_SHADOW_ENABLED) return;
    if (!activeBankId || ui.favoriteSubmitting) return;
    const nextFavorite = !favorited;
    favoriteMutationVersionRef.current += 1;
    setUi({ favoriteSubmitting: true, favoriteError: "" });
    void shadowFavoritesApi.set(activeBankId, activeQuestion.id, nextFavorite).then(() => {
      const favoriteIds = nextFavorite
        ? Array.from(new Set([...serverFavoriteIdsRef.current, activeQuestion.id]))
        : serverFavoriteIdsRef.current.filter((id) => id !== activeQuestion.id);
      serverFavoriteIdsRef.current = favoriteIds;
      setUi({
        favoriteSubmitting: false,
        favoriteError: "",
        serverFavoriteIds: favoriteIds,
      });
    }).catch((error) => {
      setUi({
        favoriteSubmitting: false,
        favoriteError: isQuizcraftAuthenticationError(error)
          ? "请前往练习服务使用收藏"
          : "收藏失败，请稍后重试",
      });
      if (isQuizcraftAuthenticationError(error)) {
        try {
          redirectToHenuKitFavorites();
        } catch {
          setUi({ favoriteError: "暂时无法打开练习服务，请稍后重试" });
        }
      }
    });
  };

  return {
    activeBankKey,
    activeQuestion,
    answerSubmitting,
    answerSubmitError,
    cancelAnswerSubmission,
    canSubmitCurrent,
    closeFeedbackModal,
    elapsedTime,
    feedbackDialogRef,
    feedbackError,
    feedbackID,
    feedbackMessage,
    feedbackQuestionIndex,
    feedbackStatus,
    feedbackSubmitting,
    feedbackCategory,
    feedbackSuggestion,
    favoriteError,
    favoriteQuestionIds: ui.serverFavoriteIds,
    favoriteSubmitting,
    handleCardTouchEnd,
    handleCardTouchMove,
    handleCardTouchStart,
    handleFeedbackSubmit,
    handleJudgeSelect,
    handleJump,
    handleNext,
    handleOptionSelect,
    handlePrev,
    handleSubmitCurrent,
    isSliding,
    nextQuestionRef,
    openFeedbackModal,
    openFeedbackHistory,
    practice,
    prevQuestion,
    progress,
    resetSwipeState,
    retryFeedbackStatus,
    retryAnswerSubmission,
    selectedAnswer,
    setFeedbackSuggestion: (value: string) => {
      if (value !== feedbackSuggestion) clearPendingFeedbackForCurrentQuestion();
      setUi({
        feedbackSuggestion: value,
        feedbackRequestKey: null,
        feedbackID: null,
        feedbackStatus: null,
        feedbackError: "",
        feedbackMessage: "",
      });
    },
    setFeedbackCategory: (value: FeedbackCategory) => {
      if (value !== feedbackCategory) clearPendingFeedbackForCurrentQuestion();
      setUi({
        feedbackCategory: value,
        feedbackRequestKey: null,
        feedbackID: null,
        feedbackStatus: null,
        feedbackError: "",
        feedbackMessage: "",
      });
    },
    setSelectedAnswer: (value: any) => {
      if (showResult || submissionLocked) return;
      setUi({ selectedAnswer: value });
    },
    showResult,
    submissionLocked,
    favorited,
    toggleFavorite: handleToggleFavorite,
    trackX,
    visualIndex,
    viewportRef,
  };
}

type QuizController = NonNullable<ReturnType<typeof useQuizController>>;

export default function Quiz() {
  const controller = useQuizController();

  if (!controller) return null;

  return <QuizView controller={controller} />;
}

function QuizView({ controller }: { controller: QuizController }) {
  return (
    <LazyMotion features={domAnimation}>
      <div className="max-w-3xl mx-auto">
        <QuizFeedbackDialog controller={controller} />
        <QuizProgressHeader controller={controller} />
        <QuizTrack controller={controller} />
        <QuizFooterControls controller={controller} />
    </div>
    </LazyMotion>
  );
}

function QuizFeedbackDialog({ controller }: { controller: QuizController }) {
  return (
    <dialog
      ref={controller.feedbackDialogRef}
      aria-labelledby="feedback-modal-title"
      onCancel={(event) => {
        if (controller.feedbackSubmitting) {
          event.preventDefault();
        }
      }}
      className="w-[calc(100%-2rem)] max-w-lg rounded-2xl bg-white dark:bg-slate-800 p-5 shadow-xl backdrop:bg-black/50"
      data-swipe-ignore="true"
    >
      <div className="mb-4 flex items-start justify-between gap-4">
        <div>
          <h2 id="feedback-modal-title" className="text-lg font-semibold text-gray-800 dark:text-slate-100">反馈本题</h2>
          <p className="mt-1 text-xs text-gray-500 dark:text-slate-400 dark:text-slate-500">
            {controller.activeBankKey} · 第 {controller.feedbackQuestionIndex} 题
          </p>
        </div>
        <button
          type="button"
          onClick={controller.closeFeedbackModal}
          disabled={controller.feedbackSubmitting}
          className="rounded-lg p-1.5 text-gray-500 dark:text-slate-400 dark:text-slate-500 transition-colors hover:bg-gray-100 dark:hover:bg-slate-700 disabled:opacity-50"
          aria-label="关闭反馈弹窗"
        >
          <X className="h-5 w-5" />
        </button>
      </div>

      <div className="mb-4 rounded-xl border border-gray-100 dark:border-slate-700 bg-gray-50 dark:bg-slate-700 p-3 text-sm leading-relaxed text-gray-600 dark:text-slate-300">
        <RichText text={controller.activeQuestion.content} />
      </div>

      <form onSubmit={controller.handleFeedbackSubmit} className="space-y-3">
        {QUIZCRAFT_GO_SHADOW_ENABLED && (
          <div>
            <label htmlFor="quiz-feedback-category" className="block text-sm font-medium text-gray-700 dark:text-slate-200">
              反馈类型
            </label>
            <select
              id="quiz-feedback-category"
              value={controller.feedbackCategory}
              onChange={(event) => controller.setFeedbackCategory(event.target.value as FeedbackCategory)}
              disabled={controller.feedbackSubmitting}
              className="mt-1 w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 outline-none transition-colors focus:border-primary-300 focus:ring-2 focus:ring-primary-100 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200"
            >
              <option value="wrong_answer">正确答案或解析错误</option>
              <option value="ambiguous">题意或选项歧义</option>
              <option value="typo">错别字或排版问题</option>
              <option value="outdated">内容已过时</option>
              <option value="other">其他建议</option>
            </select>
          </div>
        )}
        <label htmlFor="quiz-feedback-suggestion" className="sr-only">
          反馈建议
        </label>
        <textarea
          id="quiz-feedback-suggestion"
          rows={6}
          value={controller.feedbackSuggestion}
          onChange={(event) => controller.setFeedbackSuggestion(event.target.value)}
          placeholder="例如：正确答案应为 B；选项 C 有错别字；解析和答案不一致..."
          className="w-full resize-y rounded-xl border border-gray-200 dark:border-slate-700 px-3 py-2 text-sm text-gray-700 dark:text-slate-200 outline-none transition-colors focus:border-primary-300 focus:ring-2 focus:ring-primary-100"
          maxLength={2000}
          disabled={controller.feedbackSubmitting}
        />
        <div className="flex items-center justify-between text-xs text-gray-400 dark:text-slate-500">
          <span>最多 2000 字，提交后不会离开当前刷题进度</span>
          <span>{controller.feedbackSuggestion.length}/2000</span>
        </div>

        {controller.feedbackError && <p role="alert" className="text-sm text-red-600">{controller.feedbackError}</p>}
        {controller.feedbackMessage && <p role="status" className="text-sm text-green-600">{controller.feedbackMessage}</p>}

        {controller.feedbackID && (
          <div className="rounded-xl border border-primary-100 bg-primary-50 p-3 text-xs text-primary-800 dark:border-primary-900/70 dark:bg-primary-900/20 dark:text-primary-200">
            <p className="break-all">
              已保存反馈编号：{controller.feedbackID}
              {controller.feedbackStatus && ` · ${feedbackStatusLabel(controller.feedbackStatus.status)}`}
            </p>
            <div className="mt-2 flex flex-wrap gap-2">
              <button
                type="button"
                onClick={controller.retryFeedbackStatus}
                disabled={controller.feedbackSubmitting}
                className="rounded-lg border border-primary-200 bg-white px-2.5 py-1.5 font-medium text-primary-700 transition-colors hover:bg-primary-100 disabled:cursor-not-allowed disabled:opacity-50 dark:border-primary-700 dark:bg-slate-800 dark:text-primary-200 dark:hover:bg-slate-700"
              >
                重试读取状态
              </button>
              <button
                type="button"
                onClick={controller.openFeedbackHistory}
                disabled={controller.feedbackSubmitting}
                className="rounded-lg border border-primary-200 bg-white px-2.5 py-1.5 font-medium text-primary-700 transition-colors hover:bg-primary-100 disabled:cursor-not-allowed disabled:opacity-50 dark:border-primary-700 dark:bg-slate-800 dark:text-primary-200 dark:hover:bg-slate-700"
              >
                查看我的反馈
              </button>
            </div>
          </div>
        )}

        <div className="flex items-center gap-3 pt-1">
          <button
            type="button"
            onClick={controller.closeFeedbackModal}
            disabled={controller.feedbackSubmitting}
            className="flex-1 rounded-xl border border-gray-200 dark:border-slate-700 px-4 py-3 text-sm font-medium text-gray-600 dark:text-slate-300 transition-colors hover:bg-gray-50 dark:hover:bg-slate-700 disabled:opacity-50"
          >
            继续刷题
          </button>
          <button
            type="submit"
            disabled={controller.feedbackSubmitting || !controller.feedbackSuggestion.trim()}
            className="inline-flex flex-1 items-center justify-center gap-2 rounded-xl bg-primary-500 text-white px-4 py-3 text-sm font-medium transition-colors hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Send className="h-4 w-4" />
            {controller.feedbackSubmitting ? "提交中..." : "提交反馈"}
          </button>
        </div>
      </form>
    </dialog>
  );
}

function QuizProgressHeader({ controller }: { controller: QuizController }) {
  return (
    <div className="sticky top-0 z-10 bg-white dark:bg-slate-800/80 backdrop-blur-md border-b border-gray-100 dark:border-slate-700 -mx-4 px-4 py-3 mb-6">
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-slate-400 dark:text-slate-500">
          <BookOpen className="w-4 h-4" />
          <span>
            题目 {controller.visualIndex + 1} / {controller.practice.questions.length}
          </span>
        </div>
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={controller.openFeedbackModal}
            className="p-1.5 rounded-lg text-gray-500 dark:text-slate-400 dark:text-slate-500 hover:bg-gray-100 dark:hover:bg-slate-700 transition-colors"
            aria-label="反馈本题"
          >
            <MessageCircle className="w-4 h-4" />
          </button>
          <div className="flex items-center gap-1 text-sm text-gray-500 dark:text-slate-400 dark:text-slate-500">
            <Timer className="w-4 h-4" />
            <span>{formatTime(controller.elapsedTime)}</span>
          </div>
          {QUIZCRAFT_GO_SHADOW_ENABLED && (
            <button
              type="button"
              onClick={controller.toggleFavorite}
              disabled={controller.favoriteSubmitting}
              className={`p-1.5 rounded-lg transition-colors ${controller.favorited ? "text-yellow-500 bg-yellow-50 dark:bg-yellow-900/20" : "text-gray-400 dark:text-slate-500 hover:bg-gray-100 dark:hover:bg-slate-700"}`}
              aria-label={controller.favorited ? "取消收藏本题" : "收藏本题"}
            >
              <Flag className={`w-4 h-4 ${controller.favorited ? "fill-current" : ""}`} />
            </button>
          )}
        </div>
      </div>
      {controller.favoriteError && (
        <p role="status" className="mt-2 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-200">
          {controller.favoriteError}
        </p>
      )}
      <div className="h-1.5 bg-gray-100 dark:bg-slate-700 rounded-full overflow-hidden">
        <div
          className="h-full bg-primary-500 dark:bg-primary-400 transition-all duration-[120ms]"
          style={{ width: `${controller.progress}%` }}
        />
      </div>
    </div>
  );
}

function QuizTrack({ controller }: { controller: QuizController }) {
  return (
    <div className="overflow-hidden" ref={controller.viewportRef}>
      <m.div style={{ x: controller.trackX }} className="flex">
        <QuizTrackCard
          key={`prev-${controller.prevQuestion?.id ?? "empty"}`}
          controller={controller}
          question={controller.prevQuestion}
          mode={{ kind: "preview" }}
        />
        <QuizTrackCard
          key={`current-${controller.activeQuestion.id}`}
          controller={controller}
          question={controller.activeQuestion}
          mode={
            controller.showResult
              ? { kind: "result" }
              : {
                  kind: "answering",
                  submitDisabled:
                    !controller.canSubmitCurrent ||
                    controller.isSliding ||
                    controller.submissionLocked,
                  submitting: controller.answerSubmitting,
                  submitError: controller.answerSubmitError,
                }
          }
        />
        <QuizTrackCard
          key={`next-${controller.nextQuestionRef?.id ?? "empty"}`}
          controller={controller}
          question={controller.nextQuestionRef}
          mode={{ kind: "preview" }}
        />
      </m.div>
    </div>
  );
}

function QuizTrackCard({
  controller,
  mode,
  question,
}: {
  controller: QuizController;
  mode: QuestionCardProps["mode"];
  question: Question | null;
}) {
  return (
    <QuestionCard
      question={question}
      mode={mode}
      practice={controller.practice}
      selectedAnswer={controller.selectedAnswer}
      onBlankAnswerChange={controller.setSelectedAnswer}
      onJudgeSelect={controller.handleJudgeSelect}
      onOptionSelect={controller.handleOptionSelect}
      onSubmitCurrent={controller.handleSubmitCurrent}
      onRetrySubmit={controller.retryAnswerSubmission}
      onCancelSubmit={controller.cancelAnswerSubmission}
      onTouchStart={controller.handleCardTouchStart}
      onTouchMove={controller.handleCardTouchMove}
      onTouchEnd={controller.handleCardTouchEnd}
      onTouchCancel={() => controller.resetSwipeState()}
    />
  );
}

function QuizFooterControls({ controller }: { controller: QuizController }) {
  return (
    <div className="pt-4 border-t border-gray-100 dark:border-slate-700">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={controller.handlePrev}
          disabled={
            controller.isSliding ||
            controller.submissionLocked ||
            controller.practice.currentIndex === 0
          }
          className="flex items-center justify-center gap-1 px-4 py-2 rounded-lg text-gray-600 dark:text-slate-300 hover:bg-gray-100 dark:hover:bg-slate-700 disabled:opacity-30 disabled:cursor-not-allowed transition-colors flex-1"
        >
          <ChevronLeft className="w-5 h-5" />
          上一题
        </button>

        <button
          type="button"
          onClick={controller.handleNext}
          disabled={controller.isSliding || controller.submissionLocked}
          className="flex items-center justify-center gap-1 px-4 py-2 bg-primary-500 text-white rounded-lg hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500 disabled:opacity-30 disabled:cursor-not-allowed transition-colors flex-1"
        >
          {controller.visualIndex === controller.practice.questions.length - 1
            ? "查看结果"
            : "下一题"}
          <ChevronRight className="w-5 h-5" />
        </button>
      </div>

      <ProgressDots
        questions={controller.practice.questions}
        currentIndex={controller.visualIndex}
        answers={controller.practice.answers}
        results={controller.practice.results}
        favoriteQuestionIds={controller.favoriteQuestionIds}
        disabled={controller.submissionLocked}
        onJump={controller.handleJump}
      />
    </div>
  );
}
