import { useEffect, useMemo, useState } from 'react';
import { CheckCircle2, PackagePlus, RefreshCw, Rocket, RotateCcw, Upload, XCircle } from 'lucide-react';
import { isQuizcraftAuthenticationError, quizcraftLoginHref, shadowWorkshopApi } from '@/api/quizcraftShadowClient';
import { ApiError, type ImportedQuestionInput, type WorkshopBank, type WorkshopVersionDetail } from '@/generated/quizcraft-api';

const sampleQuestions = JSON.stringify([{
  source_question_id: '',
  type: 'single',
  chapter_id: '',
  chapter: '',
  content: '',
  options: [''],
  answer: '',
  analysis: '',
}], null, 2);

export default function Workshop() {
  const [banks, setBanks] = useState<WorkshopBank[]>([]);
  const [selectedID, setSelectedID] = useState('');
  const [bankKey, setBankKey] = useState('');
  const [bankName, setBankName] = useState('');
  const [questionsText, setQuestionsText] = useState(sampleQuestions);
  const [sourceSHA, setSourceSHA] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [review, setReview] = useState<WorkshopVersionDetail | null>(null);
  const [loginRequired, setLoginRequired] = useState(false);
  const selected = useMemo(() => banks.find((bank) => bank.bank_id === selectedID) || banks[0], [banks, selectedID]);

  const handleFailure = (failure: unknown, fallback: string) => {
    const needsLogin = isQuizcraftAuthenticationError(failure);
    setLoginRequired(needsLogin);
    console.error('题库工坊操作失败:', failure);
    if (needsLogin || (failure instanceof ApiError && failure.status === 403)) {
      setError('当前账号没有工坊权限，请联系管理员');
    } else if (failure instanceof ApiError && failure.status >= 500) {
      setError('服务暂时不可用，请稍后再来');
    } else if (
      !(failure instanceof ApiError) &&
      failure instanceof Error &&
      (!failure.message ||
        failure.message.includes('Failed to fetch') ||
        failure.message.includes('Network Error'))
    ) {
      setError('网络不通，请检查网络后重试');
    } else {
      setError(fallback);
    }
  };

  const refresh = async () => {
    const result = await shadowWorkshopApi.list();
    setBanks(result);
    setSelectedID((current) => result.some((bank) => bank.bank_id === current) ? current : (result[0]?.bank_id || ''));
  };

  useEffect(() => { void refresh().catch((failure) => handleFailure(failure, '登录后返回工坊')); }, []);

  const run = async (work: () => Promise<unknown>, success: string) => {
    if (busy) return;
    setBusy(true);
    setError('');
    setMessage('');
    try {
      await work();
      await refresh();
      setReview(null);
      setMessage(success);
    } catch (failure) {
      handleFailure(failure, '操作没有成功，请刷新后重试；如仍失败请联系管理员');
    } finally {
      setBusy(false);
    }
  };

  const questions = () => {
    const parsed = JSON.parse(questionsText) as ImportedQuestionInput[];
    if (!Array.isArray(parsed) || parsed.length === 0) throw new Error('请至少提供一道已人工核对的题目');
    return parsed;
  };

  return (
    <div className="mx-auto max-w-5xl space-y-6 animate-fade-in">
      <header>
        <h1 className="text-2xl font-bold text-gray-800 dark:text-slate-100">题库工坊</h1>
        <p className="mt-2 text-gray-500 dark:text-slate-400">需要登录管理账号后使用；导入后必须人工校验才能发布。</p>
      </header>

      {error && <p role="alert" className="rounded-xl bg-red-50 p-3 text-red-700">{error}</p>}
      {loginRequired && <a href={quizcraftLoginHref('/extract')} className="inline-flex rounded-lg bg-primary-500 px-4 py-2 text-white">登录管理账号后即可使用</a>}
      {message && <p role="status" className="rounded-xl bg-emerald-50 p-3 text-emerald-700">{message}</p>}

      <section className="rounded-2xl border bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
        <h2 className="font-semibold">创建稳定题库</h2>
        <div className="mt-3 grid gap-3 sm:grid-cols-[1fr_1fr_auto]">
          <input aria-label="题库标识" value={bankKey} onChange={(event) => setBankKey(event.target.value)} placeholder="computer-basics" className="rounded-lg border p-3" />
          <input aria-label="题库名称" value={bankName} onChange={(event) => setBankName(event.target.value)} placeholder="计算机基础" className="rounded-lg border p-3" />
          <button type="button" disabled={busy || !bankKey || !bankName} onClick={() => void run(() => shadowWorkshopApi.createBank(bankKey, bankName), '稳定题库已创建')} className="rounded-lg bg-primary-500 px-4 py-2 text-white disabled:opacity-50"><PackagePlus className="mr-2 inline h-4 w-4" />创建</button>
        </div>
      </section>

      <section className="rounded-2xl border bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="font-semibold">版本与导入</h2>
          <button type="button" onClick={() => void run(refresh, '已刷新最新版本')} disabled={busy} className="rounded-lg border px-3 py-2"><RefreshCw className="mr-2 inline h-4 w-4" />刷新</button>
        </div>
        <select aria-label="选择工坊题库" value={selected?.bank_id || ''} onChange={(event) => setSelectedID(event.target.value)} className="mt-3 w-full rounded-lg border p-3">
          {banks.map((bank) => <option key={bank.bank_id} value={bank.bank_id}>{bank.name} · 版本 v{bank.lifecycle_version}</option>)}
        </select>
        <textarea aria-label="题目 JSON" value={questionsText} onChange={(event) => setQuestionsText(event.target.value)} rows={12} className="mt-3 w-full rounded-lg border p-3 font-mono text-sm" />
        <input aria-label="导入来源 SHA256" value={sourceSHA} onChange={(event) => setSourceSHA(event.target.value)} className="mt-3 w-full rounded-lg border p-3 font-mono text-sm" />
        <div className="mt-3 flex flex-wrap gap-2">
          <button type="button" disabled={busy || !selected} onClick={() => void run(() => shadowWorkshopApi.createVersion(selected!.bank_id, selected!.lifecycle_version, questions()), '手工编辑版本已封存为草稿')} className="rounded-lg bg-primary-500 px-4 py-2 text-white disabled:opacity-50">创建草稿版本</button>
          <button type="button" disabled={busy || !selected} onClick={() => void run(() => shadowWorkshopApi.importVersion(selected!.bank_id, selected!.lifecycle_version, sourceSHA, questions()), '导入报告已通过，等待人工校验')} className="rounded-lg border px-4 py-2 disabled:opacity-50"><Upload className="mr-2 inline h-4 w-4" />导入为草稿</button>
        </div>
      </section>

      <section className="space-y-3">
        {selected?.versions.map((version) => (
          <article key={version.bank_version_id} className="rounded-2xl border bg-white p-5 dark:border-slate-700 dark:bg-slate-800">
            <div className="flex flex-wrap items-start justify-between gap-3"><div><p className="font-mono text-xs text-gray-500">{version.bank_version_id}</p><h3 className="mt-1 font-semibold">{version.question_count} 题 · {version.state}{version.active ? ' · 已发布' : ''}</h3></div><span className="rounded-full bg-gray-100 px-3 py-1 text-xs">版本 v{selected.lifecycle_version}</span></div>
            <div className="mt-4 flex flex-wrap gap-2">
              {version.state === 'draft' && <button type="button" disabled={busy} onClick={() => void shadowWorkshopApi.detail(selected.bank_id, version.bank_version_id).then(setReview).catch((failure) => handleFailure(failure, '版本详情读取失败'))} className="rounded-lg border px-3 py-2">查看并校验题目</button>}
              {version.state === 'validated' && !version.active && <button type="button" disabled={busy} onClick={() => void run(() => shadowWorkshopApi.publish(selected.bank_id, version.bank_version_id, selected.lifecycle_version), '已发布校验版本')} className="rounded-lg bg-primary-500 px-3 py-2 text-white"><Rocket className="mr-2 inline h-4 w-4" />发布</button>}
              {version.active && <button type="button" disabled={busy} onClick={() => void run(() => shadowWorkshopApi.unpublish(selected.bank_id, version.bank_version_id, selected.lifecycle_version), '已下架，版本和审计保留')} className="rounded-lg border px-3 py-2"><XCircle className="mr-2 inline h-4 w-4" />下架</button>}
              {version.state === 'validated' && !version.active && <button type="button" disabled={busy} onClick={() => void run(() => shadowWorkshopApi.rollback(selected.bank_id, version.bank_version_id, selected.lifecycle_version), '已回滚至所选不可变版本')} className="rounded-lg border px-3 py-2"><RotateCcw className="mr-2 inline h-4 w-4" />回滚到此版本</button>}
            </div>
            {review?.bank_version_id === version.bank_version_id && <div className="mt-4 rounded-xl bg-slate-50 p-4 dark:bg-slate-900"><h4 className="font-semibold">逐题人工校验</h4>{review.questions.map((question) => <div key={question.question_version_id} className="mt-3 border-t pt-3"><p className="font-medium">{question.position}. {question.content}</p><p className="mt-1 text-sm">题型：{question.type} · 章节：{question.chapter || question.chapter_id}</p>{question.options?.length ? <ol className="mt-2 list-inside list-decimal text-sm">{question.options.map((option) => <li key={option}>{option}</li>)}</ol> : null}<p className="mt-1 text-sm">答案：{JSON.stringify(question.answer)}</p><p className="text-sm text-gray-500">解析：{question.analysis || '（无）'}</p></div>)}<button type="button" disabled={busy} onClick={() => void run(() => shadowWorkshopApi.validate(selected.bank_id, version.bank_version_id, selected.lifecycle_version), '已记录人工校验')} className="mt-4 rounded-lg border px-3 py-2"><CheckCircle2 className="mr-2 inline h-4 w-4" />人工校验通过</button></div>}
          </article>
        ))}
      </section>
    </div>
  );
}
