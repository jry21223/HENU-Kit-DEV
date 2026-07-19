import {
  OpenAPI,
  PracticeService,
  type BankVersion,
  type ChapterPracticeSelection,
  type DifficultPracticeSelection,
  type PracticeQuestion,
  type RandomPracticeSelection,
} from '@/generated/quizcraft-api';
import type { PracticeSettings, Question, QuestionBank } from '@/types';

export const QUIZCRAFT_GO_SHADOW_ENABLED =
  import.meta.env.VITE_QUIZCRAFT_GO_SHADOW === '1';

const bankRegistry = new Map<string, BankVersion>();
let activeSession: {
  id: string;
  bankKey: string;
  versions: Map<string, string>;
  pendingKeys: Map<string, string>;
} | null = null;

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
    activeSession = {
      id: response.data.session_id,
      bankKey,
      versions: new Map(
        response.data.questions.map((question) => [
          question.question_id,
          question.question_version_id,
        ]),
      ),
      pendingKeys: new Map(),
    };
    return {
      questions: response.data.questions.map(toQuestion),
      total: response.data.questions.length,
    };
  },

  async submitAnswer(bankKey: string, questionId: string, answer: unknown) {
    configureGeneratedClient();
    if (!activeSession || activeSession.bankKey !== bankKey) {
      throw new Error('当前影子练习 Session 不存在，请重新开始练习');
    }
    const questionVersionId = activeSession.versions.get(questionId);
    if (!questionVersionId) {
      throw new Error('题目不属于当前影子练习 Session');
    }
    const idempotencyKey =
      activeSession.pendingKeys.get(questionId) || randomKey();
    activeSession.pendingKeys.set(questionId, idempotencyKey);
    const response = await PracticeService.submitPracticeAnswer({
      sessionId: activeSession.id,
      idempotencyKey,
      requestBody: {
        question_id: questionId,
        question_version_id: questionVersionId,
        answer,
      },
    });
    activeSession.pendingKeys.delete(questionId);
    return {
      correct: response.data.correct,
      correct_answer: response.data.expected_answer,
      analysis: response.data.analysis,
    };
  },
};
