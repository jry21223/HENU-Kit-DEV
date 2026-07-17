import { FormEvent, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { MessageSquare, ArrowLeft, Send } from 'lucide-react';
import { feedbackApi } from '@/api/client';

type FeedbackLocationState = {
  questionIndex?: unknown;
  question_index?: unknown;
  questionBank?: unknown;
  question_bank?: unknown;
};

const parseQuestionIndex = (value: unknown): number | null => {
  const raw = typeof value === 'string' ? Number(value) : Number(value);
  return Number.isInteger(raw) && raw > 0 ? raw : null;
};

const parseQuestionBank = (value: unknown): string => {
  return typeof value === 'string' ? value.trim() : '';
};

const readLastFeedbackQuestionIndex = () => {
  try {
    return parseQuestionIndex(localStorage.getItem('quizcraft_last_feedback_question_index'));
  } catch {
    return null;
  }
};

export default function Feedback() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const location = useLocation();
  const [suggestion, setSuggestion] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  const locationState = (location.state as FeedbackLocationState | null) || {};
  const questionIndex =
    parseQuestionIndex(locationState.questionIndex) ??
    parseQuestionIndex(locationState.question_index) ??
    parseQuestionIndex(searchParams.get('questionIndex')) ??
    parseQuestionIndex(searchParams.get('question_index')) ??
    readLastFeedbackQuestionIndex();
  const questionBank =
    parseQuestionBank(locationState.questionBank) ||
    parseQuestionBank(locationState.question_bank) ||
    parseQuestionBank(searchParams.get('questionBank')) ||
    parseQuestionBank(searchParams.get('question_bank'));

  const submitFeedback = async (event: FormEvent) => {
    event.preventDefault();

    const normalizedSuggestion = suggestion.trim();
    if (!questionIndex || questionIndex <= 0) {
      setError('未获取到有效题目索引，请从题目页点击“反馈本题”提交');
      return;
    }
    if (!normalizedSuggestion) {
      setError('建议改正内容不能为空');
      return;
    }

    setSubmitting(true);
    setError('');
    setMessage('');
    try {
      await feedbackApi.submit({
        question_index: questionIndex,
        suggestion: normalizedSuggestion,
        question_bank: questionBank || undefined,
      });
      setMessage('反馈提交成功，感谢你的建议！');
      setSuggestion('');
    } catch (err) {
      setError((err as Error).message || '提交失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto animate-fade-in">
      <div className="rounded-2xl border border-gray-100 dark:border-slate-700 bg-white dark:bg-slate-800 shadow-sm dark:shadow-slate-900/30 p-6">
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-3">
            <MessageSquare className="w-6 h-6 text-primary-500 dark:text-primary-400" />
            <div>
              <h1 className="text-xl font-semibold text-gray-800 dark:text-slate-100">题目反馈</h1>
              <p className="mt-1 text-xs text-gray-500 dark:text-slate-400 dark:text-slate-500">
                推荐从刷题页点击“反馈本题”，可自动带上题目 ID 和题干快照。
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={() => navigate(-1)}
            className="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg border border-gray-200 dark:border-slate-700 text-sm text-gray-600 dark:text-slate-300 hover:bg-gray-50 dark:hover:bg-slate-700"
          >
            <ArrowLeft className="w-4 h-4" />
            返回
          </button>
        </div>

        <form onSubmit={submitFeedback} className="space-y-4">
          <div>
            <label htmlFor="feedback-question-index" className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
              题目索引
            </label>
            <input
              id="feedback-question-index"
              type="number"
              min={1}
              value={questionIndex ?? ''}
              readOnly
              className="w-full rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-700 px-3 py-2 text-sm text-gray-700 dark:text-slate-200"
              placeholder="无法获取题目索引"
            />
          </div>

          <div>
            <label htmlFor="feedback-suggestion" className="block text-sm font-medium text-gray-700 dark:text-slate-200 mb-2">
              建议改正内容
            </label>
            <textarea
              id="feedback-suggestion"
              rows={8}
              value={suggestion}
              onChange={(event) => setSuggestion(event.target.value)}
              placeholder="请输入题目纠错建议，例如：选项 B 的表述应为..."
              className="w-full rounded-lg border border-gray-200 dark:border-slate-700 px-3 py-2 text-sm text-gray-700 dark:text-slate-200 min-h-[140px] resize-y"
              maxLength={2000}
            />
            <p className="text-xs text-gray-400 dark:text-slate-500 mt-1">
              最多 2000 字，目前输入 {suggestion.length} 字
            </p>
          </div>

          {error && <p className="text-sm text-red-600">{error}</p>}
          {message && <p className="text-sm text-green-600">{message}</p>}

          <div className="flex items-center gap-3">
            <button
              type="submit"
              disabled={submitting || !questionIndex}
              className="inline-flex items-center justify-center gap-2 flex-1 py-3 px-4 bg-primary-50 dark:bg-primary-900/300 text-white rounded-xl font-medium hover:bg-primary-600 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Send className="w-4 h-4" />
              {submitting ? '提交中...' : '提交反馈'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
