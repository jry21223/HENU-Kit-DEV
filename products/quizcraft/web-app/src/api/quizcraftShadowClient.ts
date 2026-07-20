import {
  ApiError,
  FeedbackService,
  FavoritesService,
  OpenAPI,
  PracticeService,
  RankingService,
  WorkshopService,
  type BankImportRequest,
  type BankVersion,
  type ChapterPracticeSelection,
  type DifficultPracticeSelection,
  type PracticeQuestion,
  type QuestionFeedback,
  type RandomPracticeSelection,
  type WorkshopBank,
  type WorkshopFeedback,
  type WorkshopVersionDetail,
} from '@/generated/quizcraft-api';
import type { PracticeSettings, Question, QuestionBank } from '@/types';
import {
  QUIZCRAFT_GO_READ_ENABLED,
  QUIZCRAFT_GO_READ_PERCENT,
  QUIZCRAFT_GO_WRITES_ENABLED,
} from '@/api/quizcraftRollout';

export { QUIZCRAFT_GO_READ_ENABLED, QUIZCRAFT_GO_READ_PERCENT, QUIZCRAFT_GO_WRITES_ENABLED };

// Compatibility name for pages whose complete workflow moves only at write cutover.
export const QUIZCRAFT_GO_SHADOW_ENABLED = QUIZCRAFT_GO_WRITES_ENABLED;

const bankRegistry = new Map<string, BankVersion>();
type ShadowPracticeSession = {
  id: string;
  bankId: string;
  bankKey: string;
  versions: Map<string, string>;
  pendingKeys: Map<string, string>;
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
    };
    if (!stored.id || !stored.bankId || !stored.bankKey || !Array.isArray(stored.versions)) return null;
    activeSession = {
      id: stored.id,
      bankId: stored.bankId,
      bankKey: stored.bankKey,
      versions: new Map(stored.versions),
      pendingKeys: new Map(),
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
    return {
      correct: response.data.correct,
      correct_answer: response.data.expected_answer,
      analysis: response.data.analysis,
    };
  },
};

export const isQuizcraftAuthenticationError = (error: unknown) =>
  error instanceof ApiError && error.status === 401;

export const quizcraftLoginHref = (returnTo: string) => {
  const loginEntry = import.meta.env.VITE_QUIZCRAFT_LOGIN_URL?.trim() || '/auth/login';
  const loginURL = new URL(loginEntry, window.location.origin);
  loginURL.searchParams.set('return_to', returnTo);
  return loginURL.toString();
};

export const getActiveShadowBankId = () => restoreActiveSession()?.bankId;
export const getActiveShadowQuestionVersionId = (questionId: string) => restoreActiveSession()?.versions.get(questionId);

export const shadowFeedbackApi = {
  async submit(input: QuestionFeedback) {
    configureGeneratedClient();
    return FeedbackService.createQuestionFeedback({ idempotencyKey: randomKey(), requestBody: input });
  },
};

export const shadowWorkshopApi = {
  async list(): Promise<WorkshopBank[]> {
    configureGeneratedClient();
    return (await WorkshopService.listWorkshopCatalog()).data;
  },
  async createBank(bank_key: string, name: string) {
    configureGeneratedClient();
    return WorkshopService.createWorkshopBank({ idempotencyKey: randomKey(), requestBody: { bank_key, name } });
  },
  async detail(bankId: string, bankVersionId: string): Promise<WorkshopVersionDetail> {
    configureGeneratedClient();
    return (await WorkshopService.getWorkshopBankVersion({ bankId, bankVersionId })).data;
  },
  async feedback(feedbackId: string): Promise<WorkshopFeedback> {
    configureGeneratedClient();
    return (await WorkshopService.getWorkshopFeedback({ feedbackId })).data;
  },
  async createVersion(bankId: string, expectedVersion: number, questions: BankImportRequest['questions']) {
    configureGeneratedClient();
    return WorkshopService.createWorkshopBankVersion({ bankId, idempotencyKey: randomKey(), requestBody: { expected_version: expectedVersion, questions } });
  },
  async importVersion(bankId: string, expectedVersion: number, sourceSHA256: string, questions: BankImportRequest['questions']) {
    configureGeneratedClient();
    return WorkshopService.importWorkshopBank({ bankId, idempotencyKey: randomKey(), requestBody: { expected_version: expectedVersion, source_sha256: sourceSHA256, questions } });
  },
  async validate(bankId: string, bankVersionId: string, expectedVersion: number) {
    configureGeneratedClient();
    return WorkshopService.validateWorkshopBankVersion({ bankId, bankVersionId, idempotencyKey: randomKey(), requestBody: { expected_version: expectedVersion, note: '题库工坊人工逐题校验' } });
  },
  async publish(bankId: string, bankVersionId: string, expectedVersion: number) {
    configureGeneratedClient();
    return WorkshopService.publishWorkshopBankVersion({ bankId, bankVersionId, idempotencyKey: randomKey(), requestBody: { expected_version: expectedVersion, note: '人工校验后发布' } });
  },
  async unpublish(bankId: string, bankVersionId: string, expectedVersion: number) {
    configureGeneratedClient();
    return WorkshopService.unpublishWorkshopBankVersion({ bankId, bankVersionId, idempotencyKey: randomKey(), requestBody: { expected_version: expectedVersion, note: '下架保留不可变版本' } });
  },
  async rollback(bankId: string, bankVersionId: string, expectedVersion: number) {
    configureGeneratedClient();
    return WorkshopService.rollbackWorkshopBank({ bankId, idempotencyKey: randomKey(), requestBody: { expected_version: expectedVersion, target_bank_version_id: bankVersionId, note: '回滚至已校验版本' } });
  },
};

export const redirectToQuizcraftLogin = (bankId: string, questionId: string, favorite: boolean) => {
  const loginEntry = import.meta.env.VITE_QUIZCRAFT_LOGIN_URL?.trim();
  if (!loginEntry) {
    throw new Error('未配置 QuizCraft 登录入口');
  }
  const returnURL = new URL(window.location.href);
  window.sessionStorage.setItem(
    'quizcraft_pending_favorite',
    JSON.stringify({ bankId, questionId, favorite }),
  );
  returnURL.searchParams.set('favorite_question_id', questionId);
  returnURL.searchParams.set('favorite_bank_id', bankId);
  const loginURL = new URL(loginEntry, window.location.origin);
  loginURL.searchParams.set(
    'return_to',
    `${returnURL.pathname}${returnURL.search}${returnURL.hash}`,
  );
  window.location.assign(loginURL.toString());
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
    });
    return {
      questions: response.data.questions.map(toQuestion),
      excludedUnavailableCount: response.data.excluded_unavailable_count,
    };
  },
};

export const shadowRankingApi = {
  async get(scope: 'overall' | 'bank', period: 'weekly' | 'lifetime', bankId?: string) {
    configureGeneratedClient();
    if (scope === 'bank') {
      if (!bankId) throw new Error('请选择题库');
      return (await RankingService.getBankRanking({ bankId, period })).data;
    }
    return (await RankingService.getOverallRanking({ period })).data;
  },

  async updateProfile(input: { visible: boolean; nickname: string; system_avatar: string }) {
    configureGeneratedClient();
    return RankingService.updateRankingProfile({ idempotencyKey: randomKey(), requestBody: input });
  },
};
