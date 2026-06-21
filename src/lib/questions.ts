import type { QuestionType } from "@prisma/client";
import type { PracticeOption, PracticeQuestion, PracticeQuestionType } from "@/types";

type RawQuestion = {
  id: string;
  courseId: string;
  knowledgePointId?: string | null;
  knowledgePointTitle?: string | null;
  type: QuestionType | PracticeQuestionType;
  stem: string;
  options?: unknown;
  difficulty: number;
};

const questionTypeMap: Record<QuestionType, PracticeQuestionType> = {
  SINGLE_CHOICE: "single_choice",
  MULTIPLE_CHOICE: "multiple_choice",
  TRUE_FALSE: "true_false",
  BLANK: "blank",
  CALCULATION: "calculation",
  PROOF: "proof",
};

function normalizeQuestionType(type: QuestionType | PracticeQuestionType): PracticeQuestionType {
  if (typeof type === "string" && type in questionTypeMap) {
    return questionTypeMap[type as QuestionType];
  }
  return type as PracticeQuestionType;
}

export function normalizeOptions(value: unknown): PracticeOption[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return value
    .map((item) => {
      if (!item || typeof item !== "object") {
        return null;
      }
      const option = item as { id?: unknown; text?: unknown };
      if (typeof option.id !== "string" || typeof option.text !== "string") {
        return null;
      }
      return {
        id: option.id,
        text: option.text,
      };
    })
    .filter((item): item is PracticeOption => Boolean(item));
}

export function toPublicQuestion(question: RawQuestion): PracticeQuestion {
  return {
    id: question.id,
    courseId: question.courseId,
    knowledgePointId: question.knowledgePointId ?? undefined,
    knowledgePointTitle: question.knowledgePointTitle ?? undefined,
    type: normalizeQuestionType(question.type),
    stem: question.stem,
    options: normalizeOptions(question.options),
    difficulty: question.difficulty,
  };
}

function normalizeAnswerTokens(value: unknown): string[] {
  const values = Array.isArray(value) ? value : [value];

  return values
    .flatMap((item) => {
      if (item === null || item === undefined) {
        return [];
      }
      return String(item).trim().toLowerCase();
    })
    .filter(Boolean)
    .sort();
}

export function isSupportedAnswerInput(value: unknown) {
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return String(value).trim().length > 0;
  }

  return (
    Array.isArray(value) &&
    value.length > 0 &&
    value.every(
      (item) =>
        typeof item === "string" || typeof item === "number" || typeof item === "boolean",
    )
  );
}

export function getNormalizedSubmittedAnswer(value: unknown) {
  return normalizeAnswerTokens(value);
}

export function isAnswerCorrect(submittedAnswer: unknown, correctAnswer: unknown) {
  const submitted = normalizeAnswerTokens(submittedAnswer);
  const correct = normalizeAnswerTokens(correctAnswer);

  if (submitted.length === 0 || submitted.length !== correct.length) {
    return false;
  }

  return submitted.every((token, index) => token === correct[index]);
}

export function formatAnswerForDisplay(answer: unknown, options: unknown) {
  const normalizedOptions = normalizeOptions(options);
  const optionMap = new Map(
    normalizedOptions.map((option) => [option.id.trim().toLowerCase(), option]),
  );

  return normalizeAnswerTokens(answer)
    .map((token) => {
      const option = optionMap.get(token);
      if (option) {
        return `${option.id}. ${option.text}`;
      }
      if (token === "true") {
        return "正确";
      }
      if (token === "false") {
        return "错误";
      }
      return token;
    })
    .join(" / ");
}
