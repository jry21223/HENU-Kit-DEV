import axios, { type AxiosInstance, type AxiosError } from 'axios';
import { ApiRequestError, classifyApiErrorKind } from '@/api/errors';
import type { 
  QuestionBank, 
  Question, 
  PracticeSettings,
  RankItem,
  UserStats,
  FeedbackDashboard,
  FeedbackBoardItem,
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

type WheelResponse = {
  wheels: Array<{
    id: number | string;
    owner_user_id: string;
    owner_name: string;
    items: string[];
    is_public: boolean;
    updated_at: string;
  }>;
};

type WheelSaveResponse = {
  id: number | string;
  owner_user_id: string;
  owner_name: string;
  items: string[];
  is_public: boolean;
  updated_at: string;
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
    const message =
      responseData?.message ||
      responseData?.detail ||
      error.message ||
      '请求失败';
    const kind = classifyApiErrorKind(error);
    console.error('API Error:', message);
    return Promise.reject(
      new ApiRequestError(kind, message, error.response?.status),
    );
  }
);

type AdminSessionResponse = {
  authenticated: boolean;
  expires_in?: number;
};

export const adminApi = {
  getSession: (): Promise<AdminSessionResponse> => {
    return api.get('/admin/session');
  },
  login: (token: string): Promise<AdminSessionResponse> => {
    return api.post('/admin/session', undefined, {
      headers: { 'X-Admin-Token': token.trim() },
    });
  },
  logout: (): Promise<AdminSessionResponse> => {
    return api.delete('/admin/session');
  },
};

// 题库 API
export const bankApi = {
  // 获取题库列表
  getList: (): Promise<{ banks: QuestionBank[] }> => {
    return api.get('/banks');
  },

  // 保存题库到服务端
  save: (payload: {
    name: string;
    key?: string;
    color?: string;
    questions: any[];
    overwrite?: boolean;
  }): Promise<{
    message: string;
    bank: QuestionBank;
    file: string;
  }> => {
    return api.post('/banks/save', payload);
  },
  
  // 获取题库统计
  getStats: (bankKey: string): Promise<any> => {
    return api.get('/stats/global', { params: { bank: bankKey } });
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
  
  // 获取排行榜
  getRanking: (): Promise<{ ranking: RankItem[] }> => {
    return api.get('/ranking');
  },
};

export const feedbackApi = {
  submit: (payload: FeedbackPayload): Promise<FeedbackResponse> => {
    return api.post('/feedback', payload);
  },
  getDashboard: (): Promise<FeedbackDashboard> => {
    return api.get('/feedback/dashboard');
  },
  updateStatus: (
    feedbackId: number,
    payload: {
      status: 'pending' | 'resolved' | 'archived';
      resolution_note?: string;
    },
  ): Promise<{ ok: boolean; item: FeedbackBoardItem }> => {
    return api.patch(`/feedback/${feedbackId}/status`, payload);
  },
};

export const wheelApi = {
  get: (): Promise<WheelResponse> => {
    return api.get('/wheel');
  },
  save: (
    items: string[],
    userId: string,
  ): Promise<WheelSaveResponse> => {
    return api.post('/wheel', { items, user_id: userId });
  },
};

// 解析生成 API
export const analysisApi = {
  // 上传文件并解析
  parseFile: (file: File): Promise<{
    content: string;
    questions: any[];
  }> => {
    const formData = new FormData();
    formData.append('file', file);
    
    return api.post('/extract/parse', formData);
  },
  
  // 生成解析
  generateAnalysis: (questions: any[], config: any): Promise<{
    questions: any[];
  }> => {
    return api.post('/extract/analyze', {
      questions,
      config,
    });
  },
  
  // 导出题库
  exportBank: (questions: any[], name: string): Promise<{
    download_url: string;
  }> => {
    return api.post('/extract/export', {
      questions,
      name,
    });
  },
};
