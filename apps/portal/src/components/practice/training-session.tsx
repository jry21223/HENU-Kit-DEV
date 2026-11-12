"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { ErrorBanner, LoadingBlock } from "@/components/data-state";
import { Button } from "@/components/ui/button";
import { PortalHttpError, PortalUnauthorizedError } from "@/lib/api/client";
import { discloseTrainingHint, fetchTrainingReview, fetchTrainingSession, revealTrainingLayer, submitTrainingAnswer } from "@/lib/api/training";
import type { TrainingAnswerSubmission, TrainingReview, TrainingSession } from "@/lib/api/generated/training";
import { createIdempotencyKey } from "@/lib/practice/session-handoff";
import ClozePassage, { ClozeEvidence } from "./cloze-passage";
import TrainingAttributionList from "./training-attribution";
import ListeningAudio, { type EvidenceSelection, type ListeningAudioHandle } from "./listening-audio";
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

export default function TrainingSessionView({ sessionID }: { sessionID: string }) {
  usePageEnter(null);
  const [session, setSession] = useState<TrainingSession | null>(null);
  const [drafts, setDrafts] = useState<Drafts>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sessionMissing, setSessionMissing] = useState(false);
  const [review, setReview] = useState<TrainingReview | null>(null);
  const [activeQuestion, setActiveQuestion] = useState("");
  const [hinting, setHinting] = useState<string | null>(null);
  const [hintError, setHintError] = useState<string | null>(null);
  const hintKeys = useRef<Record<string, string>>({});
  const [reviewError, setReviewError] = useState(false);
  const [evidence, setEvidence] = useState<EvidenceSelection | null>(null);
  const [revealing, setRevealing] = useState<"english" | "chinese" | null>(null);
  const [revealError, setRevealError] = useState<string | null>(null);
  const revealKeys = useRef<Partial<Record<"english" | "chinese", string>>>({});
  const revealBusy = useRef(false);
  const audioRef = useRef<ListeningAudioHandle>(null);
  const resultRef = useRef<HTMLElement>(null);
  const retryRef = useRef<HTMLButtonElement>(null);
  const submittedHere = useRef(false);
  const busy = useRef(false);
  const alive = useRef(true);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setError(null);
    setSessionMissing(false);
    try {
      const response = await fetchTrainingSession(sessionID, signal);
      if (signal?.aborted) return;
      setSession(response.data);
      setDrafts(readDrafts(response.data));
    } catch (cause) {
      if (!signal?.aborted) {
        if (cause instanceof PortalHttpError && cause.status === 404) setSessionMissing(true);
        else setError(cause instanceof PortalUnauthorizedError ? "登录状态已失效，请重新登录后打开这次训练。" : "这次训练暂时无法读取，请重试；若仍无法打开，可返回训练目录。");
      }
    } finally { if (!signal?.aborted) setLoading(false); }
  }, [sessionID]);

  useEffect(() => {
    alive.current = true;
    const controller = new AbortController();
    const timer = window.setTimeout(() => void load(controller.signal), 0);
    return () => { alive.current = false; window.clearTimeout(timer); controller.abort(); };
  }, [load]);

  const loadReview = useCallback(async (signal?: AbortSignal) => {
    setReviewError(false);
    try {
      const response = await fetchTrainingReview(sessionID, signal);
      if (response.data.session_id !== sessionID) throw new Error("Unexpected training review");
      if (!signal?.aborted && alive.current) setReview(response.data);
    } catch { if (!signal?.aborted && alive.current) setReviewError(true); }
  }, [sessionID]);

  useEffect(() => {
    if (session?.phase !== "completed") return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => void loadReview(controller.signal), 0);
    return () => { window.clearTimeout(timer); controller.abort(); };
  }, [session?.phase, loadReview]);

  useEffect(() => {
    if (!submittedHere.current) return;
    if (session?.phase === "completed") {
      resultRef.current?.focus({ preventScroll: true });
      resultRef.current?.scrollIntoView({ behavior: "auto", block: "start" });
      submittedHere.current = false;
    } else if (error) {
      retryRef.current?.focus({ preventScroll: true });
      retryRef.current?.scrollIntoView({ behavior: "auto", block: "end" });
    }
  }, [session?.phase, error]);

  async function reveal(layer: "english" | "chinese") {
    if (!session || session.phase !== "completed" || revealBusy.current) return;
    revealBusy.current = true;
    setRevealing(layer);
    setRevealError(null);
    const key = revealKeys.current[layer] ??= createIdempotencyKey("training-reveal");
    try {
      // Recover first: a previous response may have been lost after committing.
      const recovered = (await fetchTrainingSession(sessionID)).data;
      if (!alive.current) return;
      setSession(recovered);
      if (!recovered.revealed_layers.includes(layer)) {
        const disclosure = (await revealTrainingLayer(sessionID, { unit_version_id: recovered.unit.unit_version_id, layer }, key)).data;
        if (!alive.current) return;
        if (disclosure.content.kind !== recovered.unit.kind || disclosure.content.layer !== layer) throw new Error("Unexpected training layer");
      }
      const [confirmed, fullReview] = await Promise.all([fetchTrainingSession(sessionID), fetchTrainingReview(sessionID)]);
      if (!alive.current) return;
      if (fullReview.data.kind !== recovered.unit.kind || fullReview.data.session_id !== sessionID) throw new Error("Unexpected training review");
      setSession(confirmed.data);
      setReview(fullReview.data);
      setReviewError(false);
    } catch (cause) {
      if (!alive.current) return;
      let confirmed = false;
      try {
        const recovered = (await fetchTrainingSession(sessionID)).data;
        if (!alive.current) return;
        setSession(recovered);
        if (recovered.revealed_layers.includes(layer)) {
          const fullReview = (await fetchTrainingReview(sessionID)).data;
          if (!alive.current) return;
          if (fullReview.kind === recovered.unit.kind && fullReview.session_id === sessionID) { setReview(fullReview); confirmed = true; }
        }
      } catch { /* Keep the last confirmed content; retry reads Core again. */ }
      if (!confirmed && alive.current) setRevealError(cause instanceof PortalUnauthorizedError ? "登录状态已失效，请重新登录后继续查看文字。" : "文字暂时无法读取，请重试。");
    } finally {
      revealBusy.current = false;
      if (alive.current) setRevealing(null);
    }
  }

  function selectQuestion(questionID: string) {
    setActiveQuestion(questionID);
    if (session?.unit.kind === "cloze") {
      const blank = session.unit.blanks.find(item => item.question_id === questionID);
      const target = blank && document.getElementById(`cloze-${blank.blank_id}`);
      const passage = target?.closest<HTMLElement>("[data-cloze-scroll]");
      if (target && passage) passage.scrollBy({ top: target.getBoundingClientRect().top - passage.getBoundingClientRect().top - passage.clientHeight / 2 + target.clientHeight / 2, behavior: "auto" });
    }
    window.setTimeout(() => {
      const fieldset = document.getElementById(`training-question-${questionID}`);
      fieldset?.focus({ preventScroll: true });
      fieldset?.scrollIntoView({ behavior: "auto", block: "nearest" });
    }, 0);
  }

  async function requestHint(hintID: string, level: number) {
    if (!session || session.phase === "completed" || busy.current) return;
    busy.current = true;
    const identity = `${hintID}:${level}`;
    setHinting(identity);
    setHintError(null);
    const commandStorage = `training-hint-command:${sessionID}:${identity}`;
    let remembered: string | null = null;
    try { remembered = sessionStorage.getItem(commandStorage); } catch { /* Retry identity also survives within this mounted page. */ }
    const key = hintKeys.current[identity] ??= remembered && /^[A-Za-z0-9._:-]{16,160}$/.test(remembered) ? remembered : createIdempotencyKey("training-hint");
    try { sessionStorage.setItem(commandStorage, key); } catch { /* No learning facts are stored in this tab. */ }
    try {
      const recovered = (await fetchTrainingSession(sessionID)).data;
      if (!alive.current) return;
      setSession(recovered);
      if (!recovered.hint_disclosures.some(item => item.hint_id === hintID && item.level === level)) {
        await discloseTrainingHint(sessionID, { unit_version_id: recovered.unit.unit_version_id, hint_id: hintID, level }, key);
        if (!alive.current) return;
      }
      const confirmed = (await fetchTrainingSession(sessionID)).data;
      if (alive.current) setSession(confirmed);
    } catch (cause) {
      if (!alive.current) return;
      let confirmed = false;
      try {
        const recovered = (await fetchTrainingSession(sessionID)).data;
        if (!alive.current) return;
        setSession(recovered);
        confirmed = recovered.hint_disclosures.some(item => item.hint_id === hintID && item.level === level);
      } catch { /* Retry recovers from Core before sending the same command. */ }
      if (!confirmed && alive.current) setHintError(cause instanceof PortalUnauthorizedError ? "登录状态已失效，请重新登录后继续查看提示。" : "提示暂时无法确认，请检查网络后重试。已记录的提示不会重复保存。");
    } finally {
      busy.current = false;
      if (alive.current) setHinting(null);
    }
  }

  function choose(questionID: string, answer: number) {
    if (!session || saving || busy.current || drafts[questionID]?.locked) return;
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
    submittedHere.current = true;
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

  if (loading) return <main className="mx-auto max-w-site px-5 py-10 md:px-8"><LoadingBlock label="恢复英语训练" /></main>;
  if (!session && sessionMissing) return (
    <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/60">404 / NOT FOUND</p>
      <h1 className="mt-4 font-display text-2xl font-bold">无法找到这次训练</h1>
      <p className="mt-4 text-sm leading-7 text-ink/70">训练记录不存在，或当前身份无法访问。请返回训练目录选择训练。</p>
      <div className="mt-6"><Button asChild variant="outline"><Link href="/practice/training">返回训练目录</Link></Button></div>
    </main>
  );
  if (!session) return <main className="mx-auto max-w-site px-5 py-10 md:px-8"><ErrorBanner message={error || "无法读取这次训练。"} onRetry={() => void load()} /><div className="mt-6"><Button asChild variant="outline"><Link href="/practice/training">返回训练目录</Link></Button></div></main>;

  const activeQuestionID = activeQuestion || session.unit.questions[0]?.question_id;
  const cloze = session.unit.kind === "cloze" ? session.unit : null;
  const activeReview = review?.questions.find(item => item.question_id === activeQuestionID);
  const complete = session.phase === "completed";
  const receipts = new Map(session.receipts.map(receipt => [receipt.question_id, receipt]));
  const ready = session.unit.questions.every(question => receipts.has(question.question_id) || drafts[question.question_id]);
  return (
    <main className="mx-auto max-w-site px-5 py-8 md:px-8 md:py-12">
      <div className={cloze ? "max-w-none" : "max-w-3xl"}>
        <p className="font-mono text-xs tracking-[0.2em] text-accent-text">{cloze ? "CLOZE" : "LISTENING"}</p>
        <h1 className="mt-3 break-words font-display text-3xl font-bold md:text-4xl">{session.unit.title}</h1>
        <p className="mt-4 text-sm leading-7 text-ink/70">{session.recovery_scope === "account" ? "已保存的答案记录在你的账号中。" : "同一浏览器保留站点数据时，可恢复已保存的答案。"} 提交后首次答案不可更改。</p>
        <div className={cloze ? "lg:grid lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)] lg:gap-10" : undefined}>
        {session.unit.kind === "listening" && <ListeningAudio key={sessionID} ref={audioRef} media={session.unit.media} onSelection={setEvidence} />}
        {cloze && <ClozePassage unit={cloze} activeQuestionID={activeQuestionID} selected={Object.fromEntries(session.unit.questions.map(question => [question.question_id, session.receipts.find(item => item.question_id === question.question_id)?.answer ?? drafts[question.question_id]?.input.answer]))} onSelect={selectQuestion} evidenceParts={complete && activeReview?.evidence.kind === "cloze" ? activeReview.evidence.part_ids : []} />}
        <div className="min-w-0">
        <p role="status" className="mt-5 font-mono text-sm text-ink/70">{complete ? "已完成" : "已保存"} {session.saved_count}/{session.total_count} 题{saving ? " · 正在保存…" : ""}</p>
        {complete && <section ref={resultRef} tabIndex={-1} aria-label="训练结果" className="mt-8 scroll-mt-32 border-y border-line py-6 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus-ring">
          <h2 className="font-display text-2xl font-bold">本组训练已完成</h2>
          <p className="mt-3 text-sm leading-7">独立答对 {session.receipts.filter(item => item.outcome === "independent_correct").length} 题 · 提示后答对 {session.receipts.filter(item => item.outcome === "assisted_completion").length} 题 · 答错 {session.receipts.filter(item => !item.correct).length} 题</p>
          <p className="mt-2 text-sm leading-7">需要独立复习 {session.receipts.filter(item => item.review_required).length} 题（含提示后答对的题目）。</p>
          <p className="mt-2 text-xs leading-6 text-ink/70">训练结果不计入普通刷题统计与排行榜。</p>
          {reviewError ? <ErrorBanner className="mt-4" message="答案已保存，解析暂时无法读取，请重试。" onRetry={() => void loadReview()} /> : !review ? <div className="mt-4"><LoadingBlock label="加载答案解析" /></div> : null}
          <div className="mt-5 flex flex-wrap gap-3"><Button asChild variant="outline"><Link href="/practice/training/review">查看待复习训练</Link></Button><Button asChild variant="ghost"><Link href="/practice/training">{session?.unit.kind === "listening" ? "返回听力目录" : "返回训练目录"}</Link></Button></div>
        </section>}
        {cloze && <div className="mt-6" aria-label="空格导航">
          <p className="text-sm leading-7 text-ink/70">{complete ? "选择题号核对语境依据。橙色标记为当前题的上下文。" : "选择题号切换空格；可用 Tab 和方向键选择选项。"}</p>
          <div className="mt-3 flex flex-wrap gap-2">{cloze.questions.map((question, index) => <Button key={question.question_id} type="button" size="sm" className="min-w-11" variant={activeQuestionID === question.question_id ? "accent" : "outline"} aria-pressed={activeQuestionID === question.question_id} aria-label={`第 ${index + 1} 题${receipts.has(question.question_id) ? "，已保存" : drafts[question.question_id] ? "，已选择" : "，未选择"}`} onClick={() => selectQuestion(question.question_id)}>{index + 1}</Button>)}</div>
        </div>}
        <form className="mt-6 space-y-8" onSubmit={event => { event.preventDefault(); void saveUnit(); }}>
          {session.unit.questions.map((question, index) => {
            const receipt = receipts.get(question.question_id);
            const selected = receipt?.answer ?? drafts[question.question_id]?.input.answer;
            const detail = complete ? review?.questions.find(item => item.question_id === question.question_id && item.question_version_id === question.question_version_id) : undefined;
            if (cloze && question.question_id !== activeQuestionID) return null;
            return <fieldset id={`training-question-${question.question_id}`} tabIndex={-1} key={question.question_id} className="min-w-0 border-t border-line pt-5">
              <legend className="max-w-full pr-3 text-base font-semibold leading-7"><span className="mr-2 font-mono text-sm text-accent-text">{String(index + 1).padStart(2, "0")}</span>{question.content}</legend>
              {cloze && <Button type="button" variant="ghost" size="sm" className="mt-3 -ml-3" onClick={() => {
                const blank = cloze.blanks.find(item => item.question_id === question.question_id);
                const target = blank && document.getElementById(`cloze-${blank.blank_id}`);
                if (target) { target.focus({ preventScroll: true }); target.scrollIntoView({ block: "center", behavior: "auto" }); }
              }}>回到第 {index + 1} 空的上下文</Button>}
              <div className="mt-4 space-y-2">
                {question.options.map((option, optionIndex) => <label key={optionIndex} className={`flex min-h-12 cursor-pointer items-start gap-3 border px-4 py-3 text-sm leading-6 transition-colors motion-reduce:transition-none ${selected === optionIndex ? "border-ink bg-ink/5" : "border-ink/25 hover:border-ink/60"}`}>
                  <input type="radio" disabled={!!receipt || saving || hinting !== null || drafts[question.question_id]?.locked} name={question.question_id} value={optionIndex} checked={selected === optionIndex} onChange={() => choose(question.question_id, optionIndex)} className="mt-1 size-4 shrink-0 accent-ink focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus-ring" />
                  <span className="min-w-0 break-words"><span className="mr-2 font-mono">{String.fromCharCode(65 + optionIndex)}.</span>{option}</span>
                </label>)}
              </div>
              {receipt && <p className="mt-3 text-sm text-ink/70">{complete ? outcomes[receipt.outcome] : "答案已保存"}</p>}
              {detail && <div className="mt-4 border-l-2 border-accent pl-4 text-sm leading-7">
                <p className="font-semibold">参考答案：{String.fromCharCode(65 + detail.expected_answer)}. {question.options[detail.expected_answer]}</p>
                <p className="mt-2 whitespace-pre-wrap break-words">{detail.analysis}</p>
                <p className="mt-2 text-ink/70">本题考点：{({ vocabulary: "词义", collocation: "搭配", logic: "逻辑", grammar: "语法", detail: "细节", inference: "推断" })[detail.error_category]}</p>
                {detail.evidence.kind === "cloze" && cloze && <section className="mt-3" aria-label="本题语境依据"><p className="font-semibold">上下文依据</p><ClozeEvidence unit={cloze} partIDs={detail.evidence.part_ids} /><p className="mt-2 text-xs text-ink/70">分类是作者对本题考点的说明，供复习参考。</p></section>}
                {detail.evidence.kind === "listening" && review?.kind === "listening" && <div className="mt-3 space-y-3" aria-label={`第 ${index + 1} 题音频依据`}>{detail.evidence.segment_ids.map(id => review?.segments.find(segment => segment.segment_id === id)).filter(segment => !!segment).map(segment => {
                  const number = (review?.segments.findIndex(item => item.segment_id === segment.segment_id) ?? 0) + 1;
                  return <div key={segment.segment_id}>
                    <p className="font-mono text-xs text-ink/70">第 {number} 句 · {timeLabel(segment.start_ms)}–{timeLabel(segment.end_ms)}</p>
                    <div className="mt-2 flex flex-wrap gap-2"><Button type="button" size="sm" variant="outline" onClick={() => audioRef.current?.playSentence(segment, false)}>播放第 {number} 句</Button><Button type="button" size="sm" variant="outline" aria-pressed={evidence?.segment.segment_id === segment.segment_id && evidence.loop} onClick={() => audioRef.current?.playSentence(segment, true)}>循环第 {number} 句</Button></div>
                  </div>;
                })}</div>}
              </div>}
            </fieldset>;
          })}
          {error && !complete && <ErrorBanner message={error} />}
          {!complete && <Button ref={retryRef} type="submit" variant="accent" disabled={!ready || saving || hinting !== null}>{saving ? "正在保存…" : error || session.saved_count > 0 || Object.values(drafts).some(draft => draft.locked) ? "继续保存答案" : "提交整组答案"}</Button>}
        </form>

        {cloze && <section className="mt-8 border-t border-line pt-5" aria-label="分层提示">
          <h2 className="font-display text-xl font-bold">分层提示</h2>
          <p className="mt-3 text-sm leading-7 text-ink/70">{complete ? "查看提示时尚未保存的题目已记为使用过提示，首次结果不变。提示后答对仍需独立复习。" : "查看任一提示，会把本组所有尚未保存的题目标记为使用过提示；已保存的首次结果不变。提示后答对仍需独立复习。"}</p>
          {hintError && <ErrorBanner className="mt-4" message={hintError} />}
          <details className="mt-4"><summary className="min-h-11 cursor-pointer py-2 text-sm font-semibold focus-visible:outline-2 focus-visible:outline-focus-ring">提示列表 · 已查看 {session.hint_disclosures.length} 层</summary>{complete && session.hint_disclosures.length === 0 && <p className="py-3 text-sm text-ink/70">本次未查看提示。</p>}<div className="divide-y divide-line">{cloze.hints.map((hint, index) => {
            const disclosed = session.hint_disclosures.filter(item => item.hint_id === hint.hint_id);
            if (complete && disclosed.length === 0) return null;
            const nextLevel = Math.max(0, ...disclosed.map(item => item.level)) + 1;
            return <div key={hint.hint_id} className="py-4"><h3 className="text-sm font-semibold">提示 {index + 1}</h3>{disclosed.map(item => <p key={item.disclosure_id} className="mt-2 text-sm leading-7"><span className="mr-2 font-mono text-xs text-accent-text">第 {item.level} 层</span>{item.text}</p>)}{!complete && nextLevel <= hint.level_count && <Button type="button" size="sm" variant="outline" className="mt-3" disabled={saving || hinting !== null} onClick={() => void requestHint(hint.hint_id, nextLevel)}>{hinting === `${hint.hint_id}:${nextLevel}` ? "正在读取提示…" : `查看提示 ${index + 1} 第 ${nextLevel} 层`}</Button>}</div>;
          })}</div></details>
        </section>}
        {complete && review && <section aria-label="复盘文字" className="mt-8 border-t border-line pt-6">
          <h2 className="font-display text-2xl font-bold">原文与译文</h2>
          <p className="mt-3 text-sm leading-7 text-ink/70">{cloze ? "先核对上下文依据，再按需查看完整英文与中文。" : "先复听，再按需查看文字。"}复盘不改变本次首次作答结果。</p>
          {revealError && <ErrorBanner className="mt-4" message={revealError} />}
          <Button type="button" variant="outline" className="mt-5" aria-disabled={revealing !== null || review.layers.some(item => item.content.layer === "chinese")} onClick={() => { if (!revealing && !review.layers.some(item => item.content.layer === "chinese")) void reveal(review.layers.some(item => item.content.layer === "english") ? "chinese" : "english"); }}>{revealing ? "正在读取文字…" : review.layers.some(item => item.content.layer === "chinese") ? "文字已全部展开" : review.layers.some(item => item.content.layer === "english") ? "查看中文译文" : "查看英文原文"}</Button>
          {review.layers.map(disclosure => <section key={disclosure.disclosure_id} aria-label={disclosure.content.layer === "english" ? "英文原文" : "中文译文"} className="mt-5">
            <h3 className="font-semibold">{disclosure.content.layer === "english" ? "英文原文" : "中文译文"}</h3>
            {disclosure.content.kind === "cloze" && <p className="mt-3 whitespace-pre-wrap break-words text-sm leading-8" lang={disclosure.content.layer === "english" ? "en" : "zh-CN"}>{disclosure.content.parts.map(part => part.text).join("")}</p>}
            {disclosure.content.kind === "listening" && <ol className="mt-3 space-y-4 text-sm leading-7">{disclosure.content.segments.map((segment, index) => <li key={segment.segment_id} className="flex gap-3"><span aria-hidden="true" className="font-mono text-xs text-ink/60">{String(index + 1).padStart(2, "0")}</span><p className="min-w-0 break-words" lang={disclosure.content.layer === "english" ? "en" : "zh-CN"}>{segment.text}</p></li>)}</ol>}
          </section>)}

        </section>}
        </div>
        </div>
        <TrainingAttributionList records={session.unit.attributions} />
      </div>
    </main>
  );
}
