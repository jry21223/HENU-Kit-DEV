import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { isQuizcraftAuthenticationError, quizcraftLoginHref, shadowWorkshopApi } from '@/api/quizcraftShadowClient';
import type { WorkshopFeedback } from '@/generated/quizcraft-api';

export default function WorkshopFeedbackPage() {
  const { feedbackId = '' } = useParams();
  const [feedback, setFeedback] = useState<WorkshopFeedback | null>(null);
  const [error, setError] = useState('');
  const [loginRequired, setLoginRequired] = useState(false);

  useEffect(() => {
    void shadowWorkshopApi.feedback(feedbackId).then(setFeedback).catch((failure) => { setLoginRequired(isQuizcraftAuthenticationError(failure)); setError((failure as Error).message || '反馈读取失败'); });
  }, [feedbackId]);

  return <div className="mx-auto max-w-3xl space-y-5 animate-fade-in">
    <header><h1 className="text-2xl font-bold">QuizCraft 纠错反馈</h1><p className="mt-2 text-sm text-gray-500">正文仅从 QuizCraft 读取；Operations Inbox 只保存此资源引用。</p></header>
    {error && <p role="alert" className="rounded-xl bg-red-50 p-4 text-red-700">{error}</p>}
    {loginRequired && <a href={quizcraftLoginHref(`/workshop/feedback/${feedbackId}`)} className="inline-flex rounded-lg bg-primary-500 px-4 py-2 text-white">通过 Platform Core 登录并返回反馈</a>}
    {!feedback && !error && <p role="status">正在读取反馈…</p>}
    {feedback && <article className="rounded-2xl border bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
      <dl className="grid gap-3 text-sm sm:grid-cols-2"><div><dt className="text-gray-500">分类</dt><dd>{feedback.category}</dd></div><div><dt className="text-gray-500">提交时间</dt><dd>{new Date(feedback.created_at).toLocaleString('zh-CN')}</dd></div><div><dt className="text-gray-500">题库</dt><dd className="font-mono text-xs">{feedback.bank_id}</dd></div><div><dt className="text-gray-500">题目版本</dt><dd className="font-mono text-xs">{feedback.question_version_id}</dd></div></dl>
      <h2 className="mt-5 font-semibold">反馈正文</h2><p className="mt-2 whitespace-pre-wrap leading-7">{feedback.detail}</p>
    </article>}
  </div>;
}
