import { type FormEvent, useCallback, useEffect, useState } from 'react';
import { AlertCircle, ArrowLeft, CheckCircle2, Loader2, MessageSquare, RefreshCw, Send } from 'lucide-react';
import type { FeedbackStatus } from '@/generated/quizcraft-api';
import { feedbackApi } from '@/api/client';
import { QUIZCRAFT_GO_SHADOW_ENABLED, shadowFeedbackApi } from '@/api/quizcraftShadowClient';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { feedbackErrorMessage, feedbackStatusLabel } from '@/utils/feedbackStatus';

type FeedbackLocationState = {
  questionIndex?: unknown;
  question_index?: unknown;
  questionBank?: unknown;
  question_bank?: unknown;
};

const parseQuestionIndex = (value: unknown): number | null => {
  const raw = Number(value);
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

const statusClass = (status: FeedbackStatus['status']) => {
  switch (status) {
    case 'resolved':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300';
    case 'blocked':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300';
    case 'archived':
      return 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-300';
    default:
      return 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300';
  }
};

const categoryLabel = (category: FeedbackStatus['category']) => {
  switch (category) {
    case 'wrong_answer':
      return '答案或解析错误';
    case 'ambiguous':
      return '题意或选项歧义';
    case 'typo':
      return '错别字或排版问题';
    case 'outdated':
      return '内容已过时';
    case 'other':
      return '其他建议';
  }
};

const formatTime = (value: string) => {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(parsed);
};

export default function Feedback() {
  if (!QUIZCRAFT_GO_SHADOW_ENABLED) {
    return <LegacyFeedbackForm />;
  }

  return <FeedbackHistory />;
}

function FeedbackHistory() {
  const [items, setItems] = useState<FeedbackStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [refreshingID, setRefreshingID] = useState<string | null>(null);
  const [itemErrors, setItemErrors] = useState<Record<string, string>>({});

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError('');
    try {
      setItems(await shadowFeedbackApi.listStatuses());
    } catch (error) {
      setLoadError(feedbackErrorMessage(error, '反馈记录暂时无法读取，请稍后重试。'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const refreshStatus = async (feedbackID: string) => {
    if (refreshingID) return;
    setRefreshingID(feedbackID);
    setItemErrors((current) => {
      const next = { ...current };
      delete next[feedbackID];
      return next;
    });
    try {
      const current = await shadowFeedbackApi.status(feedbackID);
      setItems((existing) => existing.map((item) => (
        item.feedback_id === feedbackID ? current : item
      )));
    } catch (error) {
      setItemErrors((current) => ({
        ...current,
        [feedbackID]: feedbackErrorMessage(error, '处理状态暂时无法读取，已保留上次保存的状态。'),
      }));
    } finally {
      setRefreshingID(null);
    }
  };

  return (
    <section className="mx-auto max-w-3xl space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold text-gray-900 dark:text-slate-100">
            <MessageSquare className="h-6 w-6 text-primary-500" />
            我的反馈
          </h1>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">
            这里显示已保存的纠错记录；可按条刷新处理状态。
          </p>
        </div>
        <button
          type="button"
          onClick={() => void load()}
          disabled={loading}
          className="inline-flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200 dark:hover:bg-slate-700"
        >
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          重新加载
        </button>
      </header>

      {loading && (
        <div role="status" className="flex items-center justify-center gap-2 rounded-2xl border border-gray-100 bg-white p-10 text-sm text-gray-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400">
          <Loader2 className="h-5 w-5 animate-spin" />
          正在读取已保存的反馈…
        </div>
      )}

      {!loading && loadError && (
        <div role="alert" className="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300">
          <div className="flex items-start gap-2">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
            <p>{loadError}</p>
          </div>
        </div>
      )}

      {!loading && !loadError && items.length === 0 && (
        <div className="rounded-2xl border border-dashed border-gray-200 bg-white p-10 text-center dark:border-slate-700 dark:bg-slate-800">
          <CheckCircle2 className="mx-auto h-7 w-7 text-gray-400" />
          <h2 className="mt-3 font-semibold text-gray-800 dark:text-slate-100">还没有已保存的反馈</h2>
          <p className="mt-1 text-sm text-gray-500 dark:text-slate-400">提交题目纠错后，会在这里显示处理进度。</p>
        </div>
      )}

      {!loading && !loadError && items.length > 0 && (
        <ol className="space-y-3" aria-label="已保存的反馈">
          {items.map((item) => (
            <li key={item.feedback_id} className="rounded-2xl border border-gray-100 bg-white p-4 shadow-sm dark:border-slate-700 dark:bg-slate-800">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="font-medium text-gray-800 dark:text-slate-100">{categoryLabel(item.category)}</p>
                  <p className="mt-1 break-all text-xs text-gray-500 dark:text-slate-400">反馈编号：{item.feedback_id}</p>
                </div>
                <span className={`rounded-full px-2.5 py-1 text-xs font-semibold ${statusClass(item.status)}`}>
                  {feedbackStatusLabel(item.status)}
                </span>
              </div>
              <dl className="mt-3 grid gap-1 text-xs text-gray-500 dark:text-slate-400 sm:grid-cols-2">
                <div><dt className="inline">提交时间：</dt><dd className="inline">{formatTime(item.created_at)}</dd></div>
                <div><dt className="inline">状态更新：</dt><dd className="inline">{formatTime(item.updated_at)}</dd></div>
              </dl>
              {itemErrors[item.feedback_id] && (
                <p role="alert" className="mt-3 text-xs text-red-600 dark:text-red-300">{itemErrors[item.feedback_id]}</p>
              )}
              <div className="mt-3">
                <button
                  type="button"
                  aria-label={`刷新反馈 ${item.feedback_id}`}
                  onClick={() => void refreshStatus(item.feedback_id)}
                  disabled={refreshingID !== null}
                  className="inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-600 dark:text-slate-200 dark:hover:bg-slate-700"
                >
                  <RefreshCw className={`h-3.5 w-3.5 ${refreshingID === item.feedback_id ? 'animate-spin' : ''}`} />
                  刷新处理状态
                </button>
              </div>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

function LegacyFeedbackForm() {
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
    if (!questionIndex) {
      setError('未获取到题目编号，请从刷题页点“反馈本题”提交');
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
    } catch (submitError) {
      setError(feedbackErrorMessage(submitError, '提交反馈暂时失败，请保持页面并重试。'));
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
                推荐从刷题页点击“反馈本题”，可自动带上题目编号和题干快照。
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
              题目编号
            </label>
            <input
              id="feedback-question-index"
              type="number"
              min={1}
              value={questionIndex ?? ''}
              readOnly
              className="w-full rounded-lg border border-gray-200 dark:border-slate-700 bg-gray-50 dark:bg-slate-700 px-3 py-2 text-sm text-gray-700 dark:text-slate-200"
              placeholder="无法获取题目编号"
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

          {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
          {message && <p role="status" className="text-sm text-green-600">{message}</p>}

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
