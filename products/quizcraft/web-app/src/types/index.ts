// 题目类型
export type QuestionType = 'single' | 'multi' | 'judge' | 'blank';

// 题目
export interface Question {
  id: string;
  number?: string;
  type: QuestionType;
  chapter: string;
  chapter_id: string;
  content: string;
  options?: string[];  // 单选/多选有选项，判断题没有
  answer: string | number | boolean | number[];
  analysis?: string;
  stats?: {
    total: number;
    correct: number;
    rate: number;
  };
}

// 题库
export interface QuestionBank {
  key: string;
  name: string;
  color: string;
  total: number;
  chapters: Chapter[];
}

// 章节
export interface Chapter {
  id: string;
  name: string;
  short_name?: string;
  order?: number;
}

// 练习模式
export type PracticeMode = 'random' | 'chapter' | 'hard' | 'exam';

// 练习设置
export interface PracticeSettings {
  mode: PracticeMode;
  count: number;
  chapterId?: string;
  threshold?: number;  // 难题模式阈值
}

// 练习状态
export interface PracticeState {
  bankKey: string;
  questions: Question[];
  currentIndex: number;
  answers: Record<string, any>;  // questionId -> answer
  results: Record<string, boolean>;  // questionId -> isCorrect
  correctAnswers: Record<string, any>;  // questionId -> correctAnswer
  analyses: Record<string, string>;  // questionId -> analysis
  createdAt: number;
  startTime: number;
  isFinished: boolean;
}

// 用户统计
export interface UserStats {
  userId: string;
  name: string;
  correct: number;
  total: number;
  rate: number;
}

// 排行榜项
export interface RankItem {
  user_id: string;
  name: string;
  correct: number;
  total: number;
  accuracy: number;
}

export interface FeedbackBoardItem {
  feedback_id: number;
  question_index: number;
  question_id?: string | null;
  question_content?: string | null;
  question_bank?: string | null;
  suggestion: string;
  user_id?: string | null;
  source_page: string;
  created_at: string;
  status: 'pending' | 'resolved' | 'archived';
  resolved_at?: string | null;
  resolution_note: string;
}

export interface FeedbackDashboard {
  summary: {
    today_total: number;
    pending_total: number;
    resolved_total: number;
    archived_total: number;
  };
  pending_items: FeedbackBoardItem[];
  resolved_items: FeedbackBoardItem[];
  archived_items: FeedbackBoardItem[];
}

// 答题记录
export interface AnswerRecord {
  questionId: string;
  answer: any;
  correct: boolean;
  timestamp: string;
}

// 解析生成配置
export interface AnalysisConfig {
  provider: 'openai' | 'deepseek' | 'siliconflow';
  apiKey: string;
  apiUrl?: string;
  model?: string;
  apiConfigs?: Array<{
    provider: 'openai' | 'deepseek' | 'siliconflow';
    apiKey: string;
    apiUrl?: string;
    model?: string;
  }>;
}

// 文件处理状态
export interface FileProcessState {
  file: File | null;
  content: string;
  parsedQuestions: ParsedQuestion[];
  isProcessing: boolean;
  progress: number;
}

// 解析的题目（用于提取工具）
export interface ParsedQuestion {
  id: string;
  number: string;
  type: QuestionType;
  content: string;
  options?: string[];
  answer: string;
  analysis?: string;
  chapter?: string;
}
