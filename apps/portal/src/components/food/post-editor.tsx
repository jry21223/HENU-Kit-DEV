"use client";

import Link from "next/link";
import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { useRouter } from "next/navigation";
import FoodPostForm from "./post-form";
import { PortalHttpError, PortalUnauthorizedError } from "@/lib/api/client";
import { authStore } from "@/lib/auth/store";
import { fetchFoodPostForEdit, type FoodPostEditData } from "@/lib/food/submit";

type State =
  | { kind: "loading" }
  | { kind: "ready"; post: FoodPostEditData }
  | { kind: "error"; message: string; retry: boolean };

export default function FoodPostEditor({ id }: { id: string }) {
  const router = useRouter();
  const { user, ready } = useSyncExternalStore(
    authStore.subscribe,
    authStore.get,
    authStore.getServer,
  );
  const [state, setState] = useState<State>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);
  const loginPath = `/account/login?next=${encodeURIComponent(`/food/post/${id}/edit`)}`;
  const reload = useCallback(() => {
    setState({ kind: "loading" });
    setAttempt((value) => value + 1);
  }, []);

  useEffect(() => {
    if (!ready) return;
    if (!user) {
      router.replace(loginPath);
      return;
    }
    let cancelled = false;
    void fetchFoodPostForEdit(id).then(
      (response) => {
        if (!cancelled) setState({ kind: "ready", post: response.post });
      },
      (error: unknown) => {
        if (cancelled) return;
        if (error instanceof PortalUnauthorizedError) {
          router.replace(loginPath);
          return;
        }
        const unavailable =
          error instanceof PortalHttpError &&
          (error.status === 404 || error.errorCode === "POST_HIDDEN");
        setState({
          kind: "error",
          message: unavailable
            ? "这条投稿不存在或目前无法编辑。请返回我的投稿查看。"
            : "投稿加载失败，请重试。",
          retry: !unavailable,
        });
      },
    );
    return () => {
      cancelled = true;
    };
  }, [id, ready, user, attempt, router, loginPath]);

  if (state.kind === "ready")
    return (
      <FoodPostForm
        key={`${id}:${attempt}`}
        initial={state.post}
        onReload={reload}
      />
    );
  return (
    <main className="mx-auto max-w-site px-5 py-12 md:px-8">
      <h1 className="font-display text-4xl font-bold">编辑我的投稿</h1>
      {state.kind === "loading" ? (
        <p aria-live="polite" className="mt-6 text-ink/65">
          投稿加载中…
        </p>
      ) : (
        <div role="alert" className="mt-6">
          <p className="text-ink/65">{state.message}</p>
          {state.retry && (
            <button
              type="button"
              onClick={reload}
              className="mt-4 min-h-11 border border-ink px-4 py-2"
            >
              重新加载
            </button>
          )}
        </div>
      )}
      <Link
        href="/account/posts"
        className="mt-6 inline-flex min-h-11 items-center border border-ink px-4 py-2 text-sm"
      >
        返回我的投稿
      </Link>
    </main>
  );
}
