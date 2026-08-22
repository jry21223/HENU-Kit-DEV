"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import FavoritesLoginPrompt from "@/components/practice/favorites-login-prompt";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";
import {
  createFavoritesSession,
  fetchBankFavorites,
  formatPortalError,
  PortalUnauthorizedError,
  redirectToLogin,
  unfavoriteQuestion,
} from "@/lib/api/client";
import type { FavoriteQuestion } from "@/lib/api/types";
import { useFetchState } from "@/lib/api/use-fetch-state";
import { useIdempotencyKey } from "@/lib/practice/use-idempotency-key";
import { writePracticeSessionHandoff } from "@/lib/practice/session-handoff";
import { cn } from "@/lib/cn";

function FavoriteRow({
  item,
  index,
  removing,
  onRemove,
}: {
  item: FavoriteQuestion;
  index: number;
  removing: boolean;
  onRemove: () => void;
}) {
  return (
    <li>
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 py-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-xs text-ink/40">
              Q-{String(index + 1).padStart(2, "0")}
            </span>
            {item.available ? (
              <span className="border border-ink/40 px-1.5 py-0.5 font-mono text-[10px] text-ink/70">
                可用
              </span>
            ) : (
              <span className="border border-accent/50 px-1.5 py-0.5 font-mono text-[10px] text-accent">
                暂不可用
              </span>
            )}
          </div>
          <p className="mt-1.5 truncate font-mono text-xs text-ink/70">{item.question_id}</p>
          {item.available && item.question_version_id ? (
            <p className="mt-0.5 font-mono text-[10px] tracking-wider text-ink/45">
              内容版本 {item.question_version_id.slice(0, 8)}
            </p>
          ) : (
            <p className="mt-0.5 font-mono text-[10px] tracking-wider text-ink/45">
              题目内容暂不可用，保留收藏关系
            </p>
          )}
        </div>
        <button
          type="button"
          onClick={onRemove}
          disabled={removing}
          className={cn(
            "border px-3 py-1.5 font-mono text-xs tracking-widest transition-colors",
            removing
              ? "cursor-not-allowed border-line text-ink/30"
              : "border-ink/30 text-ink/70 hover:border-accent hover:text-accent"
          )}
        >
          {removing ? "移除中…" : "取消收藏"}
        </button>
      </div>
    </li>
  );
}

export default function FavoritesFolder({ bankID }: { bankID: string }) {
  const heroRef = usePageEnter<HTMLDivElement>("list", bankID);
  const router = useRouter();
  const { state, setState, retry } = useFetchState<FavoriteQuestion[]>(() => fetchBankFavorites(bankID), [bankID]);
  const [removing, setRemoving] = useState<Set<string>>(new Set());
  const [removeError, setRemoveError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState<string | null>(null);
  const favoriteKey = useIdempotencyKey("practice-unfavorite");
  const sessionKeys = useIdempotencyKey("practice-favorites-session");
  const [bankName, setBankName] = useState("");

  useEffect(() => {
    const timer = window.setTimeout(
      () => setBankName(new URLSearchParams(window.location.search).get("name")?.trim() || ""),
      0
    );
    return () => window.clearTimeout(timer);
  }, []);

  useEffect(() => {
    if (state.status !== "loading") return;
    const timer = window.setTimeout(() => setRemoveError(null), 0);
    return () => window.clearTimeout(timer);
  }, [state.status]);

  const removeFavorite = async (item: FavoriteQuestion) => {
    if (state.status !== "ready" || removing.has(item.question_id)) return;
    const scope = `unfavorite:${bankID}:${item.question_id}`;
    const idempotencyKey = favoriteKey.obtain(scope);
    setRemoving((current) => new Set(current).add(item.question_id));
    setRemoveError(null);
    try {
      await unfavoriteQuestion(bankID, item.question_id, idempotencyKey);
      // A successful write consumed the key; the next toggle mints a fresh one.
      favoriteKey.clear(scope);
      setState((current) =>
        current.status === "ready"
          ? {
              status: "ready",
              data: current.data.filter((entry) => entry.question_id !== item.question_id),
            }
          : current
      );
    } catch (error) {
      if (error instanceof PortalUnauthorizedError) {
        redirectToLogin(window.location.pathname + window.location.search);
        return;
      }
      setRemoveError(formatPortalError(error));
    } finally {
      setRemoving((current) => {
        const next = new Set(current);
        next.delete(item.question_id);
        return next;
      });
    }
  };

  const startSession = async () => {
    if (starting) return;
    const idempotencyKey = sessionKeys.obtain("create-favorites-session");
    setStarting(true);
    setStartError(null);
    try {
      const response = await createFavoritesSession(bankID, idempotencyKey);
      sessionKeys.clear("create-favorites-session");
      const payload = response.data;
      try {
        // The quiz page has no endpoint to re-read an existing session by id,
        // so the created session travels through sessionStorage.
        writePracticeSessionHandoff(payload);
      } catch {
        setStartError("无法在本浏览器保存练习会话，请检查隐私设置后重试。");
        return;
      }
      router.push(`/practice/quiz?session_id=${encodeURIComponent(payload.session_id)}`);
    } catch (error) {
      if (error instanceof PortalUnauthorizedError) {
        redirectToLogin(window.location.pathname + window.location.search);
        return;
      }
      setStartError(formatPortalError(error));
    } finally {
      setStarting(false);
    }
  };

  const items = state.status === "ready" ? state.data : [];
  const availableCount = items.filter((item) => item.available).length;
  const canStart = state.status === "ready" && availableCount > 0 && !starting;

  return (
    <main className="mx-auto max-w-6xl px-5 py-12 md:px-8 md:py-16">
      <div data-block ref={heroRef} className="flex flex-wrap items-end justify-between gap-6">
        <div>
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">FAV</span>
            <span className="mx-2">/</span>
            BANK {bankID.slice(0, 8)}
          </p>
          <h1 className="mt-3 font-display text-4xl font-bold tracking-tight md:text-5xl">
            {bankName || "题库收藏夹"}
          </h1>
          <p className="mt-3 max-w-2xl text-sm leading-7 text-ink/65">
            {state.status === "ready" && items.length > 0
              ? `${items.length} 条收藏${
                  availableCount === 0 ? "，暂无可练习题目" : `，其中 ${availableCount} 条可练习`
                }。收藏保存的是稳定的题目引用，正文只在练习时展示。`
              : "收藏保存的是稳定的题目引用；不可用的收藏会保留关系，但不展示题目内容。"}
          </p>
        </div>
        <div className="flex flex-col items-end gap-2">
          <button
            type="button"
            onClick={() => void startSession()}
            disabled={!canStart}
            className={cn(
              "border px-6 py-3 font-mono text-sm tracking-widest transition-colors",
              canStart
                ? "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
                : "cursor-not-allowed border-line text-ink/30"
            )}
          >
            {starting ? "正在发起…" : "发起收藏练习 →"}
          </button>
          {state.status === "ready" && items.length > 0 && availableCount === 0 && (
            <p className="font-mono text-[10px] tracking-wider text-ink/45">
              没有可练习的收藏，练习按钮不可用
            </p>
          )}
          {startError && (
            <p role="alert" className="font-mono text-[10px] leading-5 text-accent">
              {startError}
            </p>
          )}
        </div>
      </div>

      {state.status === "loading" && (
        <section data-testid="practice-favorites-folder-loading" className="mt-10">
          <LoadingBlock label="正在读取收藏" />
        </section>
      )}

      {state.status === "anonymous" && (
        <section data-testid="practice-favorites-folder-anonymous">
          <FavoritesLoginPrompt next={window.location.pathname + window.location.search} />
        </section>
      )}

      {state.status === "error" && (
        <section data-testid="practice-favorites-folder-error" className="mt-10">
          <ErrorBanner
            message={state.message}
            onRetry={() => retry()}
          />
        </section>
      )}

      {state.status === "ready" && (
        <section data-testid="practice-favorites-folder-list" data-block className="mt-10">
          {items.length === 0 ? (
            <EmptyBlock label="这个题库还没有收藏题目" />
          ) : (
            <>
              <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 border-b border-ink/40 pb-2 font-mono text-[10px] tracking-[0.25em] text-ink/40">
                <span>题目引用</span>
                <span className="text-right">操作</span>
              </div>
              <ul data-enter className="divide-y divide-ink/15">
                {items.map((item, index) => (
                  <FavoriteRow
                    key={item.question_id}
                    item={item}
                    index={index}
                    removing={removing.has(item.question_id)}
                    onRemove={() => void removeFavorite(item)}
                  />
                ))}
              </ul>
              {removeError && (
                <p role="alert" className="mt-4 font-mono text-xs text-accent">
                  {removeError}
                </p>
              )}
              <p className="mt-4 font-mono text-[10px] leading-5 text-ink/45">
                不可用的收藏会保留在这里，但不会进入收藏练习，也不展示题目内容。
              </p>
              <Link
                href="/practice/favorites"
                className="mt-6 inline-block font-mono text-xs tracking-widest text-ink/60 transition-colors hover:text-accent"
              >
                ← 返回收藏夹概览
              </Link>
            </>
          )}
        </section>
      )}
    </main>
  );
}
