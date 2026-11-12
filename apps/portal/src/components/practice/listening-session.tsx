"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { Button } from "@/components/ui/button";
import { PortalUnauthorizedError } from "@/lib/api/client";
import { fetchTrainingReview, fetchTrainingSession, submitTrainingAnswer } from "@/lib/api/training";
import type { TrainingAnswerSubmission, TrainingListeningReview, TrainingSession } from "@/lib/api/generated/training";
import { createIdempotencyKey } from "@/lib/practice/session-handoff";
import TrainingAttributionList from "./training-attribution";
import { usePageEnter } from "./transition/use-page-enter";

type Draft = { input: TrainingAnswerSubmission; key: string; locked: boolean };
type Drafts = Record<string, Draft>;
const storageKey = (sessionID: string) => `training-unsaved:${sessionID}`;
const outcomes = { independent_correct: "独立答对", assisted_completion: "提示后答对", review_required: "需要复习" };

// Only unsaved input and its retry identity live in this tab. Saved results always come from Core.
function readDrafts(session: TrainingSession): Drafts {
  try {
    const value: unknown = JSON.parse(sessionStorage.getItem(storageKey(session.session_id)) || "{}");
    if (!value || typeof value !== "object" || Array.isArray(value)) return {};
    const drafts: Drafts = {};
    for (const question of session.unit.questions) {
      const draft = (value as Record<string, Draft>)[question.question_id];
      if (draft && draft.input?.unit_version_id === session.unit.unit_version_id && draft.input.question_id === question.question_id && draft.input.question_version_id === question.question_version_id && Number.isInteger(draft.input.answer) && draft.input.answer >= 0 && draft.input.answer < question.options.length && /^[A-Za-z0-9._:-]{16,160}$/.test(draft.key) && typeof draft.locked === "boolean") drafts[question.question_id] = draft;
    }
    return drafts;
  } catch { return {}; }
}

function rememberDrafts(sessionID: string, drafts: Drafts) {
  try { sessionStorage.setItem(storageKey(sessionID), JSON.stringify(drafts)); } catch { /* Storage is optional; server recovery still works. */ }
}

function timeLabel(ms: number) {
  return `${Math.floor(ms / 60_000)}:${String(Math.floor(ms / 1000) % 60).padStart(2, "0")}`;
}

export default function ListeningSession({ sessionID }: { sessionID: string }) {
  usePageEnter(null);
  const [session, setSession] = useState<TrainingSession | null>(null);
  const [drafts, setDrafts] = useState<Drafts>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [review, setReview] = useState<TrainingListeningReview | null>(null);
  const [reviewError, setReviewError] = useState(false);
  const [audioError, setAudioError] = useState(false);
  const audioRef = useRef<HTMLAudioElement>(null);
  const busy = useRef(false);
  const alive = useRef(true);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetchTrainingSession(sessionID, signal);
      if (signal?.aborted) return;
      setSession(response.data);
      setDrafts(readDrafts(response.data));
    } catch (cause) {
      if (!signal?.aborted) setError(cause instanceof PortalUnauthorizedError ? "登录状态已失效，请重新登录后打开这次训练。" : "这次训练暂时无法读取，请重试；若仍无法打开，可返回听力目录。");
    } finally { if (!signal?.aborted) setLoading(false); }
  }, [sessionID]);

  useEffect(() => {
    alive.current = true;
    const controller = new AbortController();
    const timer = window.setTimeout(() => void load(controller.signal), 0);
    return () => { alive.current = false; window.clearTimeout(timer); controller.abort(); };
  }, [load]);

  useEffect(() => {
    const audio = audioRef.current;
    return () => { audio?.pause(); };
  }, [session?.unit.unit_version_id]);

  const loadReview = useCallback(async (signal?: AbortSignal) => {
    setReviewError(false);
    try {
      const response = await fetchTrainingReview(sessionID, signal);
      if (response.data.kind !== "listening" || response.data.session_id !== sessionID) throw new Error("Unexpected training review");
      if (!signal?.aborted && alive.current) setReview(response.data);
    } catch { if (!signal?.aborted && alive.current) setReviewError(true); }
  }, [sessionID]);

  useEffect(() => {
    if (session?.phase !== "completed") return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => void loadReview(controller.signal), 0);
    return () => { window.clearTimeout(timer); controller.abort(); };
  }, [session?.phase, loadReview]);

  function choose(questionID: string, answer: number) {
    if (!session || saving || drafts[questionID]?.locked) return;
    const question = session.unit.questions.find(item => item.question_id === questionID);
    if (!question) return;
    const next = { ...drafts, [questionID]: { input: { unit_version_id: session.unit.unit_version_id, question_id: questionID, question_version_id: question.question_version_id, answer }, key: createIdempotencyKey("training-answer"), locked: false } };
    setDrafts(next);
    rememberDrafts(sessionID, next);
  }

  async function saveUnit() {
    if (!session || busy.current) return;
    const saved = new Set(session.receipts.map(item => item.question_id));
    if (session.unit.questions.some(question => !saved.has(question.question_id) && !drafts[question.question_id])) return;
    busy.current = true;
    setSaving(true);
    setError(null);
    const frozen = Object.fromEntries(Object.entries(drafts).map(([id, draft]) => [id, { ...draft, locked: true }]));
    setDrafts(frozen);
    rememberDrafts(sessionID, frozen);
    try {
      // An uncertain prior response may already be committed. Recover before retrying missing first answers.
      let recovered = (await fetchTrainingSession(sessionID)).data;
      if (!alive.current) return;
      setSession(recovered);
      const committed = new Set(recovered.receipts.map(item => item.question_id));
      for (const question of recovered.unit.questions) {
        if (committed.has(question.question_id)) continue;
        const draft = frozen[question.question_id];
        if (!draft) throw new Error("Missing answer");
        const receipt = (await submitTrainingAnswer(sessionID, draft.input, draft.key)).data;
        if (!alive.current) return;
        committed.add(receipt.question_id);
        recovered = { ...recovered, receipts: [...recovered.receipts, receipt], saved_count: receipt.saved_count, phase: receipt.phase };
        setSession(recovered);
      }
      const confirmed = (await fetchTrainingSession(sessionID)).data;
      if (alive.current) {
        setSession(confirmed);
        setDrafts({});
        rememberDrafts(sessionID, {});
      }
    } catch (cause) {
      if (!alive.current) return;
      // A failed response is not a failed write. Never infer completion or resubmit already saved questions.
      try { const response = await fetchTrainingSession(sessionID); if (alive.current) setSession(response.data); } catch { /* Keep the last confirmed snapshot and frozen input. */ }
      if (alive.current) setError(cause instanceof PortalUnauthorizedError ? "登录状态已失效，请重新登录后继续保存。已确认保存的答案不会重复提交。" : "部分答案可能尚未保存。请检查网络后重试；会先读取已保存的答案，再继续缺失项。");
    } finally {
      busy.current = false;
      if (alive.current) setSaving(false);
    }
  }

  if (loading) return <main className="mx-auto max-w-site px-5 py-10 md:px-8"><LoadingBlock label="恢复听力训练" /></main>;
  if (!session) return <main className="mx-auto max-w-site px-5 py-10 md:px-8"><ErrorBanner message={error || "无法读取这次训练。"} onRetry={() => void load()} /><div className="mt-6"><Button asChild variant="outline"><Link href="/practice/training">返回听力目录</Link></Button></div></main>;
  if (session.unit.kind !== "listening") return <main className="mx-auto max-w-site px-5 py-10 md:px-8"><EmptyBlock label="请从听力目录选择材料。" action={{ href: "/practice/training", label: "返回听力目录" }} /></main>;

  const complete = session.phase === "completed";
  const receipts = new Map(session.receipts.map(receipt => [receipt.question_id, receipt]));
  const ready = session.unit.questions.every(question => receipts.has(question.question_id) || drafts[question.question_id]);
  return (
    <main className="mx-auto max-w-site px-5 py-8 md:px-8 md:py-12">
      <div className="max-w-3xl">
        <p className="font-mono text-xs tracking-[0.2em] text-accent-text">LISTENING</p>
        <h1 className="mt-3 break-words font-display text-3xl font-bold md:text-4xl">{session.unit.title}</h1>
        <p className="mt-4 text-sm leading-7 text-ink/70">{session.recovery_scope === "account" ? "已保存的答案记录在你的账号中。" : "同一浏览器保留站点数据时，可恢复已保存的答案。"} 提交后首次答案不可更改。</p>
        <section aria-label="听力音频" className="mt-6 border-y border-line py-5">
          <audio ref={audioRef} controls preload="metadata" src={session.unit.media.url} className="w-full" aria-label="听力音频播放器" onError={() => setAudioError(true)} onCanPlay={() => setAudioError(false)} />
          {audioError && <ErrorBanner className="mt-4" message="音频暂时无法播放，请检查网络后重试。" onRetry={() => { setAudioError(false); audioRef.current?.load(); }} />}
        </section>
        <p role="status" className="mt-5 font-mono text-sm text-ink/70">{complete ? "已完成" : "已保存"} {session.saved_count}/{session.total_count} 题{saving ? " · 正在保存…" : ""}</p>
        {error && !complete && <ErrorBanner className="mt-5" message={error} />}
        <form className="mt-6 space-y-8" onSubmit={event => { event.preventDefault(); void saveUnit(); }}>
          {session.unit.questions.map((question, index) => {
            const receipt = receipts.get(question.question_id);
            const selected = receipt?.answer ?? drafts[question.question_id]?.input.answer;
            const detail = complete ? review?.questions.find(item => item.question_id === question.question_id && item.question_version_id === question.question_version_id) : undefined;
            return <fieldset key={question.question_id} disabled={!!receipt || saving || drafts[question.question_id]?.locked} className="min-w-0 border-t border-line pt-5">
              <legend className="max-w-full pr-3 text-base font-semibold leading-7"><span className="mr-2 font-mono text-sm text-accent-text">{String(index + 1).padStart(2, "0")}</span>{question.content}</legend>
              <div className="mt-4 space-y-2">
                {question.options.map((option, optionIndex) => <label key={optionIndex} className={`flex min-h-12 cursor-pointer items-start gap-3 border px-4 py-3 text-sm leading-6 transition-colors motion-reduce:transition-none ${selected === optionIndex ? "border-ink bg-ink/5" : "border-ink/25 hover:border-ink/60"}`}>
                  <input type="radio" name={question.question_id} value={optionIndex} checked={selected === optionIndex} onChange={() => choose(question.question_id, optionIndex)} className="mt-1 size-4 shrink-0 accent-ink focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus-ring" />
                  <span className="min-w-0 break-words"><span className="mr-2 font-mono">{String.fromCharCode(65 + optionIndex)}.</span>{option}</span>
                </label>)}
              </div>
              {receipt && <p className="mt-3 text-sm text-ink/70">{complete ? outcomes[receipt.outcome] : "答案已保存"}</p>}
              {detail && <div className="mt-4 border-l-2 border-accent pl-4 text-sm leading-7">
                <p className="font-semibold">参考答案：{String.fromCharCode(65 + detail.expected_answer)}. {question.options[detail.expected_answer]}</p>
                <p className="mt-2 whitespace-pre-wrap break-words">{detail.analysis}</p>
                <p className="mt-2 text-ink/70">本题考点：{({ vocabulary: "词义", collocation: "搭配", logic: "逻辑", grammar: "语法", detail: "细节", inference: "推断" })[detail.error_category]}</p>
                {detail.evidence.kind === "listening" && <p className="mt-2 font-mono text-xs text-ink/70">音频依据：{detail.evidence.segment_ids.map(id => review?.segments.find(segment => segment.segment_id === id)).filter(segment => !!segment).map(segment => `${timeLabel(segment.start_ms)}–${timeLabel(segment.end_ms)}`).join("、")}</p>}
              </div>}
            </fieldset>;
          })}
          {!complete && <Button type="submit" variant="accent" disabled={!ready || saving}>{saving ? "正在保存…" : error || session.saved_count > 0 || Object.values(drafts).some(draft => draft.locked) ? "继续保存答案" : "提交整组答案"}</Button>}
        </form>
        {complete && <section aria-label="训练结果" className="mt-8 border-y border-line py-6">
          <h2 className="font-display text-2xl font-bold">本组训练已完成</h2>
          <p className="mt-3 text-sm leading-7">独立答对 {session.receipts.filter(item => item.outcome === "independent_correct").length} 题 · 提示后答对 {session.receipts.filter(item => item.outcome === "assisted_completion").length} 题 · 需要复习 {session.receipts.filter(item => item.review_required).length} 题</p>
          <p className="mt-2 text-xs leading-6 text-ink/70">训练结果不计入普通刷题统计与排行榜。</p>
          {reviewError ? <ErrorBanner className="mt-4" message="答案已保存，解析暂时无法读取，请重试。" onRetry={() => void loadReview()} /> : !review ? <div className="mt-4"><LoadingBlock label="加载答案解析" /></div> : null}
          <Button asChild variant="outline" className="mt-5"><Link href="/practice/training">返回听力目录</Link></Button>
        </section>}
        <TrainingAttributionList records={session.unit.attributions} />
      </div>
    </main>
  );
}
