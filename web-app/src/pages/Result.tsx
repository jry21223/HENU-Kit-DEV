import { useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { LazyMotion, domAnimation, m } from 'framer-motion';
import { 
  RotateCcw, 
  Home, 
  Target,
  Clock,
  CheckCircle2,
  XCircle,
  HelpCircle
} from 'lucide-react';
import { useQuizStore } from '@/stores/quizStore';
import { formatTime } from '@/utils/format';

const getRating = (accuracy: number) => {
  if (accuracy >= 90) return { label: '优秀', color: 'text-green-500', emoji: '🎉' };
  if (accuracy >= 70) return { label: '良好', color: 'text-blue-500', emoji: '👍' };
  if (accuracy >= 60) return { label: '及格', color: 'text-yellow-500', emoji: '💪' };
  return { label: '需努力', color: 'text-red-500', emoji: '📚' };
};

export default function Result() {
  const navigate = useNavigate();
  const { practice, resetPractice, updateUserStats } = useQuizStore();
  const statsUpdatedRef = useRef(false);
  
  useEffect(() => {
    if (!practice?.isFinished) {
      navigate('/practice');
      return;
    }
    
    // 防重复：同一完成结果只更新一次统计
    if (statsUpdatedRef.current) return;
    statsUpdatedRef.current = true;
    
    // 更新用户统计
    const correct = Object.values(practice.results).filter(Boolean).length;
    const total = practice.questions.length;
    updateUserStats(correct, total);
  }, [practice, navigate, updateUserStats]);
  
  if (!practice) return null;
  
  const correct = Object.values(practice.results).filter(Boolean).length;
  const wrong = Object.values(practice.results).filter((r) => r === false).length;
  const unanswered = practice.questions.length - correct - wrong;
  const accuracy = Math.round((correct / practice.questions.length) * 100);
  const duration = Math.floor((Date.now() - practice.startTime) / 1000);
  
  const rating = getRating(accuracy);
  
  return (
    <LazyMotion features={domAnimation}>
    <div className="max-w-2xl mx-auto animate-fade-in">
      {/* 成绩卡片 */}
      <m.div
        initial={{ scale: 0.9, opacity: 0 }}
        animate={{ scale: 1, opacity: 1 }}
        className="bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 shadow-lg p-8 mb-6 text-center"
      >
        <div className="text-6xl mb-4">{rating.emoji}</div>
        <h1 className="text-2xl font-bold text-gray-800 dark:text-slate-100 mb-2">练习完成！</h1>
        <div className={`text-4xl font-bold ${rating.color} mb-1`}>{accuracy}%</div>
        <div className="text-gray-500 dark:text-slate-400">{rating.label}</div>
        
        {/* 统计 */}
        <div className="grid grid-cols-3 gap-4 mt-8">
          <div className="bg-green-50 rounded-xl p-4">
            <div className="flex items-center justify-center gap-1 text-green-600 mb-1">
              <CheckCircle2 className="w-4 h-4" />
              <span className="text-sm font-medium">正确</span>
            </div>
            <div className="text-2xl font-bold text-green-700">{correct}</div>
          </div>
          <div className="bg-red-50 rounded-xl p-4">
            <div className="flex items-center justify-center gap-1 text-red-600 mb-1">
              <XCircle className="w-4 h-4" />
              <span className="text-sm font-medium">错误</span>
            </div>
            <div className="text-2xl font-bold text-red-700">{wrong}</div>
          </div>
          <div className="bg-gray-50 dark:bg-slate-700 rounded-xl p-4">
            <div className="flex items-center justify-center gap-1 text-gray-600 dark:text-slate-300 mb-1">
              <HelpCircle className="w-4 h-4" />
              <span className="text-sm font-medium">未答</span>
            </div>
            <div className="text-2xl font-bold text-gray-700 dark:text-slate-200">{unanswered}</div>
          </div>
        </div>
        
        {/* 额外信息 */}
        <div className="flex justify-center gap-6 mt-6 text-sm text-gray-500 dark:text-slate-400">
          <div className="flex items-center gap-1">
            <Target className="w-4 h-4" />
            <span>共 {practice.questions.length} 题</span>
          </div>
          <div className="flex items-center gap-1">
            <Clock className="w-4 h-4" />
            <span>用时 {formatTime(duration)}</span>
          </div>
        </div>
      </m.div>
      
      {/* 题目回顾 */}
      <div className="bg-white dark:bg-slate-800 rounded-2xl border border-gray-100 dark:border-slate-700 shadow-sm dark:shadow-slate-900/30 p-6 mb-6">
        <h2 className="text-lg font-semibold text-gray-800 dark:text-slate-100 mb-4">题目回顾</h2>
        <div className="grid grid-cols-5 sm:grid-cols-10 gap-2">
          {practice.questions.map((q, idx) => {
            const isCorrect = practice.results[q.id];
            const hasAnswer = practice.answers[q.id] !== undefined;
            
            return (
              <button
                type="button"
                key={q.id}
                onClick={() => {
                  // 可以跳转到对应题目查看
                }}
                className={`aspect-square rounded-lg flex items-center justify-center text-sm font-medium transition-colors ${
                  isCorrect
                    ? 'bg-green-100 text-green-700'
                    : hasAnswer
                      ? 'bg-red-100 text-red-700'
                      : 'bg-gray-100 dark:bg-slate-700 text-gray-500 dark:text-slate-400 dark:text-slate-500'
                }`}
              >
                {idx + 1}
              </button>
            );
          })}
        </div>
        <div className="flex justify-center gap-4 mt-4 text-xs text-gray-500 dark:text-slate-400">
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 rounded bg-green-100" />
            <span>正确</span>
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 rounded bg-red-100" />
            <span>错误</span>
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 rounded bg-gray-100 dark:bg-slate-700" />
            <span>未答</span>
          </div>
        </div>
      </div>
      
      {/* 操作按钮 */}
      <div className="grid grid-cols-2 gap-4">
        <button
          type="button"
          onClick={() => {
            resetPractice();
            navigate('/practice');
          }}
          className="flex items-center justify-center gap-2 py-3 bg-primary-500 text-white font-medium rounded-xl hover:bg-primary-600 transition-colors"
        >
          <RotateCcw className="w-5 h-5" />
          再练一次
        </button>
        <button
          type="button"
          onClick={() => {
            resetPractice();
            navigate('/');
          }}
          className="flex items-center justify-center gap-2 py-3 bg-gray-100 dark:bg-slate-700 text-gray-700 dark:text-slate-200 font-medium rounded-xl hover:bg-gray-200 transition-colors"
        >
          <Home className="w-5 h-5" />
          返回首页
        </button>
      </div>
    </div>
    </LazyMotion>
  );
}
