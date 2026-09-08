import {
  ApiError,
  FeedbackService,
  FavoritesService,
  OpenAPI,
  PracticeService,
  type BankVersion,
  type ChapterPracticeSelection,
  type DifficultPracticeSelection,
  type PracticeQuestion,
  type QuestionFeedback,
  type RandomPracticeSelection,
} from '@/generated/quizcraft-api';
import type { PracticeSettings, Question, QuestionBank } from '@/types';
import {
  QUIZCRAFT_GO_READ_ENABLED,
  QUIZCRAFT_GO_WRITES_ENABLED,
} from '@/api/quizcraftRollout';

export { QUIZCRAFT_GO_READ_ENABLED, QUIZCRAFT_GO_WRITES_ENABLED };

// Compatibility name for pages whose complete workflow moves only at write cutover.
export const QUIZCRAFT_GO_SHADOW_ENABLED = QUIZCRAFT_GO_WRITES_ENABLED;

const bankRegistry = new Map<string, BankVersion>();
type QuizcraftFeedbackReference = Pick<
  QuestionFeedback,
  'bank_id' | 'question_id' | 'question_version_id'
>;

export type PendingQuizcraftFeedback = QuestionFeedback & {
  idempotencyKey: string;
};

type ShadowPracticeSession = {
  id: string;
  bankId: string;
  bankKey: string;
  versions: Map<string, string>;
  pendingKeys: Map<string, string>;
  pendingFeedback: Map<string, PendingQuizcraftFeedback>;
};

let activeSession: ShadowPracticeSession | null = null;

const persistActiveSession = (session: ShadowPracticeSession) => {
  activeSession = session;
  if (typeof window !== 'undefined') {
    window.sessionStorage.setItem('quizcraft_shadow_practice_session', JSON.stringify({
      id: session.id,
      bankId: session.bankId,
      bankKey: session.bankKey,
      versions: Array.from(session.versions.entries()),
      pendingKeys: Array.from(session.pendingKeys.entries()),
      pendingFeedback: Array.from(session.pendingFeedback.entries()),
    }));
  }
};

const restoreActiveSession = (): ShadowPracticeSession | null => {
  if (activeSession || typeof window === 'undefined') return activeSession;
  try {
    const stored = JSON.parse(window.sessionStorage.getItem('quizcraft_shadow_practice_session') || '{}') as {
      id?: string;
      bankId?: string;
      bankKey?: string;
      versions?: Array<[string, string]>;
      pendingKeys?: Array<[string, string]>;
      pendingFeedback?: Array<[string, PendingQuizcraftFeedback]>;
    };
    if (!stored.id || !stored.bankId || !stored.bankKey || !Array.isArray(stored.versions)) return null;
    activeSession = {
      id: stored.id,
      bankId: stored.bankId,
      bankKey: stored.bankKey,
      versions: new Map(stored.versions),
      pendingKeys: new Map(stored.pendingKeys || []),
      pendingFeedback: new Map(stored.pendingFeedback || []),
    };
  } catch {
    return null;
  }
  return activeSession;
};

const randomKey = () => {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `qc-${Date.now()}-${Math.random().toString(36).slice(2)}-request`;
};

export const createQuizcraftIdempotencyKey = () => randomKey();

const configureGeneratedClient = () => {
  OpenAPI.BASE = (import.meta.env.VITE_QUIZCRAFT_GO_API_BASE_URL || '').replace(/\/+$/, '');
  OpenAPI.WITH_CREDENTIALS = true;
  OpenAPI.CREDENTIALS = 'include';
  OpenAPI.TOKEN = undefined;
};

const toQuestionBank = (bank: BankVersion): QuestionBank => ({
  key: bank.bank_key,
  name: bank.name,
  color: '#2563eb',
  total: bank.question_count,
  chapters: bank.chapters,
  bank_id: bank.bank_id,
  bank_version_id: bank.bank_version_id,
});

const toQuestion = (question: PracticeQuestion): Question => ({
  id: question.question_id,
  type: question.type,
  chapter: question.chapter,
  chapter_id: question.chapter_id,
  content: question.content,
  options: 'options' in question ? question.options : undefined,
  answer: null as unknown as Question['answer'],
});

export const shadowPracticeApi = {
  async listBanks(): Promise<{ banks: QuestionBank[] }> {
    configureGeneratedClient();
    const response = await PracticeService.listPracticeBanks();
    bankRegistry.clear();
    for (const bank of response.data) {
      bankRegistry.set(bank.bank_key, bank);
    }
    return { banks: response.data.map(toQuestionBank) };
  },

  async start(bankKey: string, settings: PracticeSettings) {
    configureGeneratedClient();
    const bank = bankRegistry.get(bankKey);
    if (!bank) {
      throw new Error(`题库 ${bankKey} 缺少稳定版本信息，请刷新题库列表`);
    }
    const questionCount = settings.count <= 0 ? 500 : settings.count;
    let requestBody:
      | RandomPracticeSelection
      | DifficultPracticeSelection
      | ChapterPracticeSelection;
    if (settings.mode === 'chapter') {
      if (!settings.chapterId) throw new Error('章节练习需要选择章节');
      requestBody = {
        bank_id: bank.bank_id,
        bank_version_id: bank.bank_version_id,
        mode: 'chapter',
        chapter_id: settings.chapterId,
        question_count: questionCount,
      };
    } else if (settings.mode === 'hard') {
      requestBody = {
        bank_id: bank.bank_id,
        bank_version_id: bank.bank_version_id,
        mode: 'difficult',
        question_count: questionCount,
      };
    } else {
      requestBody = {
        bank_id: bank.bank_id,
        bank_version_id: bank.bank_version_id,
        mode: 'random',
        question_count: questionCount,
      };
    }
    const response = await PracticeService.createPracticeSession({
      idempotencyKey: randomKey(),
      requestBody,
    });
    persistActiveSession({
      id: response.data.session_id,
      bankId: bank.bank_id,
      bankKey,
      versions: new Map(
        response.data.questions.map((question) => [
          question.question_id,
          question.question_version_id,
        ]),
      ),
      pendingKeys: new Map(),
      pendingFeedback: new Map(),
    });
    return {
      questions: response.data.questions.map(toQuestion),
      total: response.data.questions.length,
    };
  },

  async submitAnswer(bankKey: string, questionId: string, answer: unknown) {
    configureGeneratedClient();
    const session = restoreActiveSession();
    if (!session || session.bankKey !== bankKey) {
      throw new Error('当前影子练习 Session 不存在，请重新开始练习');
    }
    const questionVersionId = session.versions.get(questionId);
    if (!questionVersionId) {
      throw new Error('题目不属于当前影子练习 Session');
    }
    const idempotencyKey =
      session.pendingKeys.get(questionId) || randomKey();
    session.pendingKeys.set(questionId, idempotencyKey);
    persistActiveSession(session);
    const response = await PracticeService.submitPracticeAnswer({
      sessionId: session.id,
      idempotencyKey,
      requestBody: {
        question_id: questionId,
        question_version_id: questionVersionId,
        answer,
      },
    });
    session.pendingKeys.delete(questionId);
    persistActiveSession(session);
    return {
      correct: response.data.correct,
      correct_answer: response.data.expected_answer,
      analysis: response.data.analysis,
    };
  },
};

export const isQuizcraftAuthenticationError = (error: unknown) =>
  error instanceof ApiError && error.status === 401;

export const getActiveShadowBankId = () => restoreActiveSession()?.bankId;
export const getActiveShadowQuestionVersionId = (questionId: string) => restoreActiveSession()?.versions.get(questionId);

const pendingFeedbackKey = (reference: QuizcraftFeedbackReference) => [
  reference.bank_id,
  reference.question_id,
  reference.question_version_id,
].join(':');

export const getPendingShadowFeedback = (
  reference: QuizcraftFeedbackReference,
): PendingQuizcraftFeedback | null => {
  const session = restoreActiveSession();
  if (!session || session.bankId !== reference.bank_id) return null;
  return session.pendingFeedback.get(pendingFeedbackKey(reference)) || null;
};

export const persistPendingShadowFeedback = (
  pending: PendingQuizcraftFeedback,
): boolean => {
  const session = restoreActiveSession();
  if (
    !session
    || session.bankId !== pending.bank_id
    || session.versions.get(pending.question_id) !== pending.question_version_id
  ) {
    return false;
  }
  session.pendingFeedback.set(pendingFeedbackKey(pending), pending);
  persistActiveSession(session);
  return true;
};

export const clearPendingShadowFeedback = (
  reference: QuizcraftFeedbackReference,
) => {
  const session = restoreActiveSession();
  if (!session || session.bankId !== reference.bank_id) return;
  session.pendingFeedback.delete(pendingFeedbackKey(reference));
  persistActiveSession(session);
};

export const shadowFeedbackApi = {
  async submit(input: QuestionFeedback, idempotencyKey = randomKey()) {
    configureGeneratedClient();
    return FeedbackService.createQuestionFeedback({ idempotencyKey, requestBody: input });
  },
  async status(feedbackId: string) {
    configureGeneratedClient();
    return (await FeedbackService.getQuestionFeedbackStatus({ feedbackId })).data;
  },
  async listStatuses() {
    configureGeneratedClient();
    return (await FeedbackService.listQuestionFeedbackStatuses()).data.items;
  },
};

export const redirectToHenuKitFavorites = () => {
  window.location.assign('https://henukit.cn/practice/favorites');
};

export const shadowFavoritesApi = {
  async list(bankId: string) {
    configureGeneratedClient();
    return (await FavoritesService.listFavoriteQuestions({ bankId })).data;
  },

  async set(bankId: string, questionId: string, favorite: boolean) {
    configureGeneratedClient();
    const input = { bankId, questionId, idempotencyKey: randomKey() };
    if (favorite) {
      await FavoritesService.favoriteQuestion(input);
    } else {
      await FavoritesService.unfavoriteQuestion(input);
    }
  },

  async overview() {
    configureGeneratedClient();
    return (await FavoritesService.getFavoritesOverview()).data;
  },

  async start(bankId: string, bankKey: string) {
    configureGeneratedClient();
    const response = await FavoritesService.createFavoritesPracticeSession({
      bankId,
      idempotencyKey: randomKey(),
    });
    persistActiveSession({
      id: response.data.session_id,
      bankId,
      bankKey,
      versions: new Map(
        response.data.questions.map((question) => [
          question.question_id,
          question.question_version_id,
        ]),
      ),
      pendingKeys: new Map(),
      pendingFeedback: new Map(),
    });
    return {
      questions: response.data.questions.map(toQuestion),
      excludedUnavailableCount: response.data.excluded_unavailable_count,
    };
  },
};
