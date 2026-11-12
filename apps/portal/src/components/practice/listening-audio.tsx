"use client";

import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import { ErrorBanner } from "@/components/data-state";
import { Button } from "@/components/ui/button";
import type { TrainingMedia, TrainingSentenceSegment } from "@/lib/api/generated/training";

export type EvidenceSelection = { segment: TrainingSentenceSegment; loop: boolean };
export type ListeningAudioHandle = { playSentence: (segment: TrainingSentenceSegment, loop: boolean) => void };

const ListeningAudio = forwardRef<ListeningAudioHandle, { media: TrainingMedia; onSelection: (selection: EvidenceSelection | null) => void }>(function ListeningAudio({ media, onSelection }, ref) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const selection = useRef<EvidenceSelection | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [active, setActive] = useState<EvidenceSelection | null>(null);
  const [speed, setSpeed] = useState(1);
  const [error, setError] = useState<string | null>(null);

  function clearTimer() {
    if (timer.current !== null) clearTimeout(timer.current);
    timer.current = null;
  }

  function select(next: EvidenceSelection | null) {
    clearTimer();
    selection.current = next;
    setActive(next);
    onSelection(next);
  }

  function armBoundary() {
    clearTimer();
    const audio = audioRef.current;
    const current = selection.current;
    if (!audio || !current || audio.paused || audio.seeking) return;
    const end = current.segment.end_ms / 1000;
    const remaining = (end - audio.currentTime) / audio.playbackRate;
    // Media timeupdate is intentionally coarse (often 250ms). Re-arm against
    // the actual media clock after play, seek and speed changes. A small guard
    // absorbs timer jitter before the next sentence can start.
    if (remaining <= 0.012) {
      if (current.loop) {
        audio.currentTime = current.segment.start_ms / 1000;
      } else {
        audio.pause();
        audio.currentTime = end;
        select(null);
      }
      return;
    }
    timer.current = setTimeout(armBoundary, Math.max(1, (remaining - 0.01) * 1000));
  }

  useImperativeHandle(ref, () => ({
    playSentence(segment, loop) {
      const audio = audioRef.current;
      if (!audio) return;
      if (segment.start_ms < 0 || segment.end_ms <= segment.start_ms || segment.end_ms > media.duration_ms) {
        setError("这句音频暂时无法定位，请重新打开训练后重试。");
        return;
      }
      const chosen = { segment, loop };
      select(chosen);
      audio.currentTime = segment.start_ms / 1000;
      void audio.play().then(() => { if (selection.current === chosen) { setError(null); armBoundary(); } }).catch(() => {
        if (selection.current !== chosen) return;
        clearTimer();
        setError("音频未能开始播放，请使用播放器的播放按钮重试。");
      });
    },
  }));

  useEffect(() => {
    const audio = audioRef.current;
    if (audio && !audio.getAttribute("src")) { audio.src = media.url; audio.load(); }
    const hide = () => { if (document.hidden) audio?.pause(); };
    document.addEventListener("visibilitychange", hide);
    return () => {
      document.removeEventListener("visibilitychange", hide);
      if (timer.current !== null) clearTimeout(timer.current);
      selection.current = null;
      if (audio) { audio.pause(); audio.removeAttribute("src"); audio.load(); }
    };
  }, [media.url]);

  function failed() {
    clearTimer();
    const unsupported = audioRef.current?.error?.code === MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED || audioRef.current?.error?.code === MediaError.MEDIA_ERR_DECODE;
    setError(unsupported ? "音频暂时无法播放。请检查网络后重试；若仍失败，请使用支持 MP3 的浏览器打开这次训练。" : "音频暂时无法播放，请检查网络后重试。");
  }

  return <section aria-label="听力音频" className="mt-6 border-y border-line py-5">
    <audio ref={audioRef} controls preload="metadata" src={media.url} className="w-full" aria-label="听力音频播放器"
      onPlay={armBoundary} onPause={clearTimer} onEnded={() => { if (selection.current?.loop) { const audio = audioRef.current!; audio.currentTime = selection.current.segment.start_ms / 1000; void audio.play().catch(failed); } else select(null); }}
      onSeeking={clearTimer} onSeeked={() => {
        const audio = audioRef.current!;
        const current = selection.current;
        if (current && (audio.currentTime < current.segment.start_ms / 1000 - 0.02 || audio.currentTime >= current.segment.end_ms / 1000)) select(null);
        armBoundary();
      }} onRateChange={() => { setSpeed(audioRef.current?.playbackRate || 1); armBoundary(); }} onTimeUpdate={armBoundary}
      onError={failed} onCanPlay={() => setError(null)} />
    <div className="mt-4 flex flex-wrap items-center gap-3">
      <label className="flex min-h-11 items-center gap-3 text-sm">播放速度
        <select aria-label="播放速度" value={speed} onChange={event => { if (audioRef.current) audioRef.current.playbackRate = Number(event.target.value); }} className="min-h-11 border border-ink/40 bg-paper px-3 font-mono text-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring">
          {[0.75, 1, 1.25, 1.5, 2].map(rate => <option key={rate} value={rate}>{rate}×</option>)}
        </select>
      </label>
      {active?.loop && <Button type="button" size="sm" variant="outline" onClick={() => select(null)}>停止单句循环</Button>}
    </div>
    {active && <p role="status" className="mt-2 text-xs leading-6 text-ink/70">{active.loop ? "单句循环已开启，可暂停或停止循环。" : "本句播放将在句末暂停。"}</p>}
    {error && <ErrorBanner className="mt-4" message={error} onRetry={() => { setError(null); select(null); audioRef.current?.load(); }} />}
  </section>;
});

export default ListeningAudio;
