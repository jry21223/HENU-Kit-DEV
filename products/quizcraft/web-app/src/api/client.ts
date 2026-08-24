import axios, { type AxiosInstance, type AxiosError } from 'axios';
import { ApiRequestError, classifyApiErrorKind } from '@/api/errors';
import {
  QUIZCRAFT_GO_READ_ENABLED,
  QUIZCRAFT_GO_WRITES_ENABLED,
  shadowPracticeApi,
} from '@/api/quizcraftShadowClient';
import type { 
  QuestionBank, 
  Question, 
  PracticeSettings,
  UserStats,
} from '@/types';

type FeedbackPayload = {
  question_index: number;
  suggestion: string;
  question_bank?: string;
  question_id?: string;
  question_content?: string;
};

type FeedbackResponse = {
  ok: boolean;
  feedback_id: number;
  created_at: string;
};

type UserStatsResponse = {
  user_id: string;
  name?: string;
  correct?: number;
  total?: number;
  rate?: number;
};

const isElectron =
  typeof navigator !== 'undefined' &&
  navigator.userAgent.toLowerCase().includes('electron');

const isFileProtocol =
  typeof window !== 'undefined' &&
  window.location.protocol === 'file:';

const trimTrailingSlash = (value: string) => value.replace(/\/+$/, '');

const rawApiBaseURL = import.meta.env.VITE_API_BASE_URL?.trim();

const defaultApiBaseURL =
  isElectron || isFileProtocol
    ? 'http://127.0.0.1:10086/api'
    : '/api';

const apiBaseURL = trimTrailingSlash(rawApiBaseURL || defaultApiBaseURL);

const getAbsoluteApiOrigin = () => {
  if (/^https?:\/\//.test(apiBaseURL)) {
    return new URL(apiBaseURL).origin;
  }

  if (typeof window !== 'undefined') {
    return window.location.origin;
  }

  return 'http://127.0.0.1:10086';
};

export const buildBrowserURL = (path: string) => {
  if (/^https?:\/\//.test(path)) {
    return path;
  }

  if (path.startsWith('/')) {
    return /^https?:\/\//.test(apiBaseURL)
      ? `${getAbsoluteApiOrigin()}${path}`
      : path;
  }

  return `${apiBaseURL}/${path.replace(/^\/+/, '')}`;
};

export const buildWebSocketURL = (path: string) => {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  const rawWsBaseURL = import.meta.env.VITE_WS_BASE_URL?.trim();

  if (rawWsBaseURL) {
    return `${trimTrailingSlash(rawWsBaseURL)}${normalizedPath}`;
  }

  if (isElectron || isFileProtocol) {
    return `ws://127.0.0.1:10086${normalizedPath}`;
  }

  const apiOrigin = getAbsoluteApiOrigin();
  const apiUrl = new URL(apiOrigin);
  const wsProtocol = apiUrl.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${wsProtocol}//${apiUrl.host}${normalizedPath}`;
};

const getStoredUserId = () => {
  if (typeof window === 'undefined') {
    return '';
  }

  return localStorage.getItem('user_id')?.trim() || '';
};

const persistUserId = (userId: string) => {
  if (typeof window !== 'undefined') {
    localStorage.setItem('user_id', userId);
  }
};

const normalizeUserStats = (user: UserStatsResponse): UserStats => {
  const correct = user.correct ?? 0;
  const total = user.total ?? 0;
  const rate =
    user.rate ??
    (total > 0 ? Math.round((correct / total) * 1000) / 10 : 0);

  return {
    userId: user.user_id,
    name: user.name?.trim() || user.user_id,
    correct,
    total,
    rate,
  };
};

// 创建 axios 实例
const api: AxiosInstance = axios.create({
  baseURL: apiBaseURL,
  timeout: 30000,
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json',
  },
});

// 响应拦截器
api.interceptors.response.use(
  (response) => response.data,
  (error: AxiosError) => {
    const responseData = error.response?.data as {
      message?: string;
      detail?: string;
    } | undefined;
    const rawMessage =
      responseData?.message ||
      responseData?.detail ||
      error.message ||
      '请求失败';
    const status = error.response?.status;
    let message: string;
    if (!error.response) {
      message = '网络不太顺畅，请检查网络后重试';
    } else if (status !== undefined && status >= 500) {
      message = '服务暂时不可用，请稍后再来';
    } else {
      message = '操作没有成功，请检查填写内容后重试';
    }
    const kind = classifyApiErrorKind(error);
    console.error('API Error:', rawMessage);
    return Promise.reject(
      new ApiRequestError(kind, message, status),
    );
  }
);

// 题库 API
export const bankApi = {
  // 获取题库列表
  getList: (): Promise<{ banks: QuestionBank[] }> => {
    if (QUIZCRAFT_GO_READ_ENABLED) {
      return shadowPracticeApi.listBanks();
    }
    return api.get('/banks');
  },
};

// 练习 API
export const practiceApi = {
  // 开始练习
  start: (bank: string, settings: PracticeSettings): Promise<{ 
    questions: Question[]; 
    total: number;
    avg_rate?: number;
  }> => {
    if (QUIZCRAFT_GO_WRITES_ENABLED) {
      return shadowPracticeApi.start(bank, settings);
    }
    return api.post('/practice/start', {
      bank,
      mode: settings.mode,
      params: {
        count: settings.count,
        chapter_id: settings.chapterId,
        threshold: settings.threshold,
      },
    });
  },
  
  // 提交答案
  submitAnswer: (bank: string, questionId: string, answer: any): Promise<{
    correct: boolean;
    correct_answer: any;
    analysis?: string;
    stats?: any;
    user_stats?: UserStats;
  }> => {
    if (QUIZCRAFT_GO_WRITES_ENABLED) {
      return shadowPracticeApi.submitAnswer(bank, questionId, answer);
    }
    return api.post('/practice/submit', {
      bank,
      question_id: questionId,
      answer,
      user_id: getStoredUserId() || undefined,
    }).then((res: any) => ({
      ...res,
      user_stats: res.user_stats
        ? normalizeUserStats(res.user_stats as UserStatsResponse)
        : undefined,
    }));
  },
};

// 用户 API
export const userApi = {
  // 获取或创建用户 ID
  ensureUser: (name = ''): Promise<UserStats> => {
    return api.post('/user', { name }).then((res: any) => {
      const user = normalizeUserStats(res as UserStatsResponse);
      persistUserId(user.userId);
      return user;
    });
  },
};

export const feedbackApi = {
  submit: (payload: FeedbackPayload): Promise<FeedbackResponse> => {
    return api.post('/feedback', payload);
  },
};
