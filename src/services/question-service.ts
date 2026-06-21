import { RecordStatus } from "@prisma/client";
import {
  findPublishedMockQuestion,
  getPublishedMockQuestionsByCourse,
} from "@/constants/mock-questions";
import { isDatabaseConfigured, prisma } from "@/lib/db";
import {
  formatAnswerForDisplay,
  getNormalizedSubmittedAnswer,
  isAnswerCorrect,
  toPublicQuestion,
} from "@/lib/questions";
import { buildWeakPointStats } from "@/lib/wrong-questions";
import type {
  PracticeQuestion,
  QuestionSubmitResult,
  WeakPointItem,
  WrongQuestionFilters,
  WrongQuestionItem,
} from "@/types";

type CurrentUserForPractice = {
  id: string;
} | null;

type DbQuestion = Awaited<ReturnType<typeof prisma.question.findMany>>[number] & {
  knowledgePoint?: { title: string } | null;
};

function mapDbQuestion(question: DbQuestion): PracticeQuestion {
  return toPublicQuestion({
    id: question.id,
    courseId: question.courseId,
    knowledgePointId: question.knowledgePointId,
    knowledgePointTitle: question.knowledgePoint?.title,
    type: question.type,
    stem: question.stem,
    options: question.options,
    difficulty: question.difficulty,
  });
}

function mapMockQuestion(question: ReturnType<typeof findPublishedMockQuestion>): PracticeQuestion | null {
  if (!question) {
    return null;
  }

  return toPublicQuestion({
    id: question.id,
    courseId: question.courseId,
    knowledgePointId: question.knowledgePointId,
    knowledgePointTitle: question.knowledgePointTitle,
    type: question.type,
    stem: question.stem,
    options: question.options,
    difficulty: question.difficulty,
  });
}

export async function listQuestionsByCourse(courseId: string): Promise<PracticeQuestion[]> {
  if (!isDatabaseConfigured()) {
    return getPublishedMockQuestionsByCourse(courseId).map(
      (question) => mapMockQuestion(question)!,
    );
  }

  const questions = await prisma.question.findMany({
    where: {
      courseId,
      status: RecordStatus.PUBLISHED,
    },
    include: {
      knowledgePoint: {
        select: { title: true },
      },
    },
    orderBy: [{ difficulty: "asc" }, { createdAt: "asc" }],
  });

  return questions.map(mapDbQuestion);
}

export async function getQuestionById(questionId: string): Promise<PracticeQuestion | null> {
  if (!isDatabaseConfigured()) {
    return mapMockQuestion(findPublishedMockQuestion(questionId));
  }

  const question = await prisma.question.findFirst({
    where: {
      id: questionId,
      status: RecordStatus.PUBLISHED,
    },
    include: {
      knowledgePoint: {
        select: { title: true },
      },
    },
  });

  return question ? mapDbQuestion(question) : null;
}

export async function submitQuestionAnswer(
  questionId: string,
  submittedAnswer: unknown,
  user: CurrentUserForPractice,
): Promise<QuestionSubmitResult | null> {
  if (!isDatabaseConfigured()) {
    const question = findPublishedMockQuestion(questionId);
    if (!question) {
      return null;
    }

    const isCorrect = isAnswerCorrect(submittedAnswer, question.answer);
    return {
      questionId: question.id,
      isCorrect,
      submittedAnswer: getNormalizedSubmittedAnswer(submittedAnswer),
      correctAnswer: getNormalizedSubmittedAnswer(question.answer),
      correctAnswerLabel: formatAnswerForDisplay(question.answer, question.options),
      explanation: question.explanation,
      wrongQuestionSaved: false,
    };
  }

  const question = await prisma.question.findFirst({
    where: {
      id: questionId,
      status: RecordStatus.PUBLISHED,
    },
  });

  if (!question) {
    return null;
  }

  const isCorrect = isAnswerCorrect(submittedAnswer, question.answer);
  let wrongQuestionSaved = false;

  if (!isCorrect && user) {
    await prisma.wrongQuestion.upsert({
      where: {
        userId_questionId: {
          userId: user.id,
          questionId: question.id,
        },
      },
      update: {},
      create: {
        userId: user.id,
        questionId: question.id,
      },
    });
    wrongQuestionSaved = true;
  }

  return {
    questionId: question.id,
    isCorrect,
    submittedAnswer: getNormalizedSubmittedAnswer(submittedAnswer),
    correctAnswer: getNormalizedSubmittedAnswer(question.answer),
    correctAnswerLabel: formatAnswerForDisplay(question.answer, question.options),
    explanation: question.explanation ?? "暂无解析。",
    wrongQuestionSaved,
  };
}

export async function listWrongQuestionsForUser(
  userId: string,
  filters: WrongQuestionFilters = {},
): Promise<WrongQuestionItem[]> {
  if (!isDatabaseConfigured()) {
    return [];
  }

  const wrongQuestions = await prisma.wrongQuestion.findMany({
    where: {
      userId,
      question: {
        status: RecordStatus.PUBLISHED,
        courseId: filters.courseId,
        knowledgePointId: filters.knowledgePointId,
      },
    },
    include: {
      question: {
        include: {
          course: {
            select: { id: true, name: true },
          },
          knowledgePoint: {
            select: { title: true },
          },
        },
      },
    },
    orderBy: { createdAt: "desc" },
  });

  return wrongQuestions.map((item) => ({
    id: item.id,
    createdAt: item.createdAt.toISOString(),
    course: item.question.course,
    knowledgePointTitle: item.question.knowledgePoint?.title,
    question: toPublicQuestion({
      id: item.question.id,
      courseId: item.question.courseId,
      knowledgePointId: item.question.knowledgePointId,
      knowledgePointTitle: item.question.knowledgePoint?.title,
      type: item.question.type,
      stem: item.question.stem,
      options: item.question.options,
      difficulty: item.question.difficulty,
    }),
  }));
}

export async function deleteWrongQuestionForUser(userId: string, wrongQuestionId: string) {
  if (!isDatabaseConfigured()) {
    return false;
  }

  const result = await prisma.wrongQuestion.deleteMany({
    where: {
      id: wrongQuestionId,
      userId,
    },
  });

  return result.count > 0;
}

export async function listWeakPointsForUser(userId: string): Promise<WeakPointItem[]> {
  if (!isDatabaseConfigured()) {
    return [];
  }

  const wrongQuestions = await prisma.wrongQuestion.findMany({
    where: {
      userId,
      question: {
        status: RecordStatus.PUBLISHED,
      },
    },
    include: {
      question: {
        include: {
          course: {
            select: { id: true, name: true },
          },
          knowledgePoint: {
            select: { id: true, title: true },
          },
        },
      },
    },
  });

  return buildWeakPointStats(
    wrongQuestions.map((item) => ({
      course: item.question.course,
      knowledgePointId: item.question.knowledgePoint?.id,
      knowledgePointTitle: item.question.knowledgePoint?.title,
    })),
  );
}
