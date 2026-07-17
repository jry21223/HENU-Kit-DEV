import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { 
  Question, 
  QuestionBank, 
  PracticeState, 
  UserStats,
  AnswerRecord 
} from '@/types';

interface QuizState {
  // 题库
  currentBank: string;
  banks: QuestionBank[];
  setBanks: (banks: QuestionBank[]) => void;
  setCurrentBank: (bank: string) => void;
  
  // 用户
  user: UserStats | null;
  setUser: (user: UserStats) => void;
  updateUserStats: (correct: number, total: number) => void;
  
  // 练习状态
  practice: PracticeState | null;
  startPractice: (questions: Question[], bankKey: string) => void;
  answerQuestion: (payload: { questionId: string; answer: any; isCorrect: boolean; correctAnswer?: any; analysis?: string }) => void;
  nextQuestion: () => void;
  prevQuestion: () => void;
  finishPractice: () => void;
  resetPractice: () => void;
  jumpToQuestion: (index: number) => void;
  
  // 历史记录
  history: AnswerRecord[];
  addToHistory: (record: AnswerRecord) => void;
  
  // 错题本
  wrongQuestions: string[];
  addWrongQuestion: (questionId: string) => void;
  removeWrongQuestion: (questionId: string) => void;
  
  // 收藏
  starredQuestions: string[];
  toggleStar: (questionId: string) => void;
  isStarred: (questionId: string) => boolean;
}

const valueEquals = (left: any, right: any): boolean => {
  if (Array.isArray(left) || Array.isArray(right)) {
    if (!Array.isArray(left) || !Array.isArray(right)) return false;
    return (
      left.length === right.length &&
      left.every((value, index) => Object.is(value, right[index]))
    );
  }

  return Object.is(left, right);
};

export const useQuizStore = create<QuizState>()(
  persist(
    (set, get) => ({
      // 初始状态
      currentBank: 'sixiu',
      banks: [],
      user: null,
      practice: null,
      history: [],
      wrongQuestions: [],
      starredQuestions: [],
      
      // 题库操作
      setBanks: (banks) => set({ banks }),
      setCurrentBank: (bank) => set({ currentBank: bank }),
      
      // 用户操作
      setUser: (user) => set({ user }),
      updateUserStats: (correct, total) => {
        const { user } = get();
        if (user) {
          set({
            user: {
              ...user,
              correct: user.correct + correct,
              total: user.total + total,
          rate:
            user.total + total > 0
              ? Math.round(((user.correct + correct) / (user.total + total)) * 100)
              : 0,
            },
          });
        }
      },
      
      // 练习操作
      startPractice: (questions, bankKey) => set({
        practice: {
          bankKey,
          questions,
          currentIndex: 0,
          answers: {},
          results: {},
          correctAnswers: {},
          analyses: {},
          createdAt: Date.now(),
          startTime: Date.now(),
          isFinished: false,
        },
      }),

      answerQuestion: ({ questionId, answer, isCorrect, correctAnswer, analysis }) => {
        const { practice } = get();
        if (!practice) return;

        const answerUnchanged = valueEquals(practice.answers[questionId], answer);
        const resultUnchanged = practice.results[questionId] === isCorrect;
        const correctAnswerUnchanged =
          correctAnswer === undefined ||
          valueEquals(practice.correctAnswers[questionId], correctAnswer);
        const analysisUnchanged =
          analysis === undefined || practice.analyses[questionId] === analysis;

        if (
          answerUnchanged &&
          resultUnchanged &&
          correctAnswerUnchanged &&
          analysisUnchanged
        ) {
          return;
        }

        const newCorrectAnswers = correctAnswer !== undefined
          ? { ...practice.correctAnswers, [questionId]: correctAnswer }
          : practice.correctAnswers;
        const newAnalyses = analysis !== undefined
          ? { ...practice.analyses, [questionId]: analysis }
          : practice.analyses;

        set({
          practice: {
            ...practice,
            answers: { ...practice.answers, [questionId]: answer },
            results: { ...practice.results, [questionId]: isCorrect },
            correctAnswers: newCorrectAnswers,
            analyses: newAnalyses,
          },
        });
        
        // 如果答错，加入错题本
        if (!isCorrect) {
          get().addWrongQuestion(questionId);
        }
      },
      
      nextQuestion: () => {
        const { practice } = get();
        if (!practice) return;
        
        const nextIndex = Math.min(practice.currentIndex + 1, practice.questions.length - 1);
        set({
          practice: { ...practice, currentIndex: nextIndex },
        });
      },
      
      prevQuestion: () => {
        const { practice } = get();
        if (!practice) return;
        
        const prevIndex = Math.max(practice.currentIndex - 1, 0);
        set({
          practice: { ...practice, currentIndex: prevIndex },
        });
      },
      
      jumpToQuestion: (index) => {
        const { practice } = get();
        if (!practice) return;
        
        set({
          practice: { ...practice, currentIndex: index },
        });
      },
      
      finishPractice: () => {
        const { practice } = get();
        if (!practice) return;
        
        set({
          practice: { ...practice, isFinished: true },
        });
      },
      
      resetPractice: () => set({ practice: null }),
      
      // 历史记录
      addToHistory: (record) => {
        const { history } = get();
        set({ history: [record, ...history].slice(0, 1000) });  // 最多保留1000条
      },
      
      // 错题本
      addWrongQuestion: (questionId) => {
        const { wrongQuestions } = get();
        if (!wrongQuestions.includes(questionId)) {
          set({ wrongQuestions: [...wrongQuestions, questionId] });
        }
      },
      
      removeWrongQuestion: (questionId) => {
        const { wrongQuestions } = get();
        set({ wrongQuestions: wrongQuestions.filter((id) => id !== questionId) });
      },
      
      // 收藏
      toggleStar: (questionId) => {
        const { starredQuestions } = get();
        if (starredQuestions.includes(questionId)) {
          set({ starredQuestions: starredQuestions.filter((id) => id !== questionId) });
        } else {
          set({ starredQuestions: [...starredQuestions, questionId] });
        }
      },
      
      isStarred: (questionId) => {
        return get().starredQuestions.includes(questionId);
      },
    }),
    {
      name: 'quiz-storage',
      partialize: (state) => ({
        currentBank: state.currentBank,
        user: state.user,
        history: state.history,
        wrongQuestions: state.wrongQuestions,
        starredQuestions: state.starredQuestions,
        practice: state.practice,
      }),
    }
  )
);
