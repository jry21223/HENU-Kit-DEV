"use client";

import Link from "next/link";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { Loader2, PenLine, Send, Trophy } from "lucide-react";
import { apiBaseUrl, type ForumBoard, type ForumPost, type User } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type PostType = ForumPost["type"];

type FormState = {
  boardId: string;
  type: PostType;
  title: string;
  content: string;
  rewardPoints: string;
};

const postTypes: Array<{ value: PostType; label: string; description: string }> = [
  { value: "normal", label: "\u8ba8\u8bba", description: "\u5206\u4eab\u590d\u4e60\u65b9\u6cd5\u6216\u8d44\u6599\u7ebf\u7d22" },
  { value: "question", label: "\u95ee\u7b54", description: "\u9488\u5bf9\u4e00\u4e2a\u5177\u4f53\u95ee\u9898\u6c42\u89e3" },
  { value: "reward", label: "\u60ac\u8d4f", description: "\u51bb\u7ed3\u79ef\u5206\uff0c\u6700\u4f73\u7b54\u6848\u7ed3\u7b97" },
];

const copy = {
  title: "\u53d1\u8d77\u65b0\u8ba8\u8bba",
  intro: "\u65b0\u5e16\u4f1a\u5148\u8fdb\u5165\u5ba1\u6838\u961f\u5217\uff0c\u901a\u8fc7\u540e\u624d\u4f1a\u5728\u8bba\u575b\u516c\u5f00\u5c55\u793a\u3002",
  loginRequired: "\u767b\u5f55\u5b66\u751f\u90ae\u7bb1\u540e\u53ef\u4ee5\u53d1\u5e03\u8ba8\u8bba\u3002",
  login: "\u53bb\u767b\u5f55",
  loading: "\u6b63\u5728\u8bfb\u53d6\u767b\u5f55\u72b6\u6001...",
  board: "\u7248\u5757",
  chooseBoard: "\u9009\u62e9\u7248\u5757",
  postTitle: "\u6807\u9898",
  titlePlaceholder: "\u4f8b\u5982\uff1a\u79bb\u6563\u6570\u5b66\u56fe\u8bba\u600e\u4e48\u590d\u4e60\uff1f",
  content: "\u5185\u5bb9",
  contentPlaceholder: "\u5199\u6e05\u8bfe\u7a0b\u3001\u9898\u76ee\u80cc\u666f\u3001\u5df2\u7ecf\u5c1d\u8bd5\u7684\u65b9\u6cd5\u548c\u5e0c\u671b\u522b\u4eba\u5e2e\u4ec0\u4e48\u3002",
  rewardPoints: "\u60ac\u8d4f\u79ef\u5206",
  rewardHint: "\u60ac\u8d4f\u5e16\u63d0\u4ea4\u65f6\u7531\u670d\u52a1\u7aef\u51bb\u7ed3\u79ef\u5206\uff0c\u5ba1\u6838\u62d2\u7edd\u4f1a\u81ea\u52a8\u9000\u56de\u3002",
  submit: "\u63d0\u4ea4\u5ba1\u6838",
  submitting: "\u63d0\u4ea4\u4e2d...",
  submitted: "\u5e16\u5b50\u5df2\u63d0\u4ea4\u5ba1\u6838\uff0c\u901a\u8fc7\u540e\u4f1a\u516c\u5f00\u5c55\u793a\u3002",
  noBoards: "\u6682\u65e0\u53ef\u7528\u7248\u5757\uff0c\u8bf7\u5148\u5728 seed \u6216\u7ba1\u7406\u540e\u53f0\u51c6\u5907\u7248\u5757\u6570\u636e\u3002",
  loadFailed: "\u53d1\u5e16\u529f\u80fd\u6682\u65f6\u4e0d\u53ef\u7528",
};

export function ForumPostComposer({ boards, selectedBoardId }: { boards: ForumBoard[]; selectedBoardId?: string }) {
  const defaultBoardId = useMemo(() => {
    if (selectedBoardId && boards.some((board) => board.id === selectedBoardId)) return selectedBoardId;
    return boards[0]?.id ?? "";
  }, [boards, selectedBoardId]);

  const [user, setUser] = useState<User | null>(null);
  const [loadingUser, setLoadingUser] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [form, setForm] = useState<FormState>({ boardId: defaultBoardId, type: "normal", title: "", content: "", rewardPoints: "" });
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    setForm((current) => ({ ...current, boardId: current.boardId || defaultBoardId }));
  }, [defaultBoardId]);

  useEffect(() => {
    async function loadMe() {
      try {
        const response = await request<User>("/auth/me", { method: "GET" });
        setUser(response.data ?? null);
      } catch {
        setUser(null);
      } finally {
        setLoadingUser(false);
      }
    }

    void loadMe();
  }, []);

  async function submitPost(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!form.boardId || !form.title.trim() || !form.content.trim()) return;
    setSubmitting(true);
    setMessage("");
    setError("");
    try {
      const rewardPoints = form.type === "reward" ? Number.parseInt(form.rewardPoints, 10) : 0;
      await request<{ post: ForumPost }>("/forum/posts", {
        method: "POST",
        body: JSON.stringify({
          boardId: form.boardId,
          title: form.title.trim(),
          content: form.content.trim(),
          type: form.type,
          rewardPoints: Number.isFinite(rewardPoints) ? rewardPoints : 0,
        }),
      });
      setForm({ boardId: form.boardId, type: "normal", title: "", content: "", rewardPoints: "" });
      setMessage(copy.submitted);
    } catch (err) {
      setError(formatError(err));
    } finally {
      setSubmitting(false);
    }
  }

  if (loadingUser) {
    return <p className="rounded-3xl border border-border bg-card p-5 text-sm text-muted-foreground shadow-sm">{copy.loading}</p>;
  }

  if (!user) {
    return (
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
        <p className="text-sm leading-6 text-muted-foreground">{copy.loginRequired}</p>
        <Link className="mt-4 inline-flex rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
          {copy.login}
        </Link>
      </section>
    );
  }

  if (boards.length === 0) {
    return <p className="rounded-3xl border border-border bg-card p-5 text-sm text-muted-foreground shadow-sm">{copy.noBoards}</p>;
  }

  return (
    <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
      <div className="flex items-start gap-3">
        <span className="grid size-10 shrink-0 place-items-center rounded-2xl bg-primary text-primary-foreground">
          <PenLine className="size-5" aria-hidden="true" />
        </span>
        <div>
          <h2 className="text-xl font-semibold tracking-tight">{copy.title}</h2>
          <p className="mt-1 text-sm leading-6 text-muted-foreground">{copy.intro}</p>
        </div>
      </div>

      <form className="mt-5 grid gap-4" onSubmit={submitPost}>
        <div className="grid gap-4 md:grid-cols-[1fr_1.2fr]">
          <label className="block text-sm font-medium text-foreground">
            {copy.board}
            <select
              className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2.5 text-sm shadow-sm"
              onChange={(event) => setForm((current) => ({ ...current, boardId: event.target.value }))}
              value={form.boardId}
            >
              <option value="">{copy.chooseBoard}</option>
              {boards.map((board) => (
                <option key={board.id} value={board.id}>
                  {board.name}
                </option>
              ))}
            </select>
          </label>
          <label className="block text-sm font-medium text-foreground">
            {copy.postTitle}
            <input
              className="mt-2 w-full rounded-xl border border-border bg-background px-3 py-2.5 text-sm shadow-sm"
              maxLength={200}
              onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))}
              placeholder={copy.titlePlaceholder}
              value={form.title}
            />
          </label>
        </div>

        <div className="grid gap-2 sm:grid-cols-3">
          {postTypes.map((item) => {
            const active = form.type === item.value;
            return (
              <button
                className={`rounded-2xl border p-3 text-left transition ${
                  active ? "border-primary bg-primary text-primary-foreground" : "border-border bg-background text-foreground hover:border-primary/70"
                }`}
                key={item.value}
                onClick={() => setForm((current) => ({ ...current, type: item.value }))}
                type="button"
              >
                <span className="block text-sm font-semibold">{item.label}</span>
                <span className={`mt-1 block text-xs leading-5 ${active ? "text-primary-foreground/80" : "text-muted-foreground"}`}>{item.description}</span>
              </button>
            );
          })}
        </div>

        {form.type === "reward" ? (
          <label className="block text-sm font-medium text-foreground">
            {copy.rewardPoints}
            <div className="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center">
              <div className="relative min-w-0 flex-1">
                <Trophy className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                <input
                  className="w-full rounded-xl border border-border bg-background py-2.5 pl-9 pr-3 text-sm shadow-sm"
                  min={1}
                  max={100000}
                  onChange={(event) => setForm((current) => ({ ...current, rewardPoints: event.target.value }))}
                  placeholder="50"
                  type="number"
                  value={form.rewardPoints}
                />
              </div>
              <p className="text-xs leading-5 text-muted-foreground sm:max-w-md">{copy.rewardHint}</p>
            </div>
          </label>
        ) : null}

        <label className="block text-sm font-medium text-foreground">
          {copy.content}
          <textarea
            className="mt-2 min-h-36 w-full rounded-2xl border border-border bg-background px-3 py-3 text-sm leading-6 shadow-sm"
            maxLength={20000}
            onChange={(event) => setForm((current) => ({ ...current, content: event.target.value }))}
            placeholder={copy.contentPlaceholder}
            value={form.content}
          />
        </label>

        <button
          className="inline-flex w-full items-center justify-center rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto"
          disabled={submitting || !form.boardId || !form.title.trim() || !form.content.trim() || (form.type === "reward" && !form.rewardPoints)}
          type="submit"
        >
          {submitting ? <Loader2 className="mr-2 size-4 animate-spin" aria-hidden="true" /> : <Send className="mr-2 size-4" aria-hidden="true" />}
          {submitting ? copy.submitting : copy.submit}
        </button>
      </form>

      {message ? <p className="mt-4 rounded-2xl border border-border bg-muted p-4 text-sm text-foreground">{message}</p> : null}
      {error ? <p className="mt-4 rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700">{error}</p> : null}
    </section>
  );
}

async function request<T>(path: string, init: RequestInit): Promise<Envelope<T>> {
  const headers = new Headers(init.headers);
  if (!(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `API request failed with ${response.status}`);
  }
  return payload;
}

function formatError(error: unknown) {
  const message = error instanceof Error ? error.message : copy.loadFailed;
  if (message === "insufficient_points") return "\u79ef\u5206\u4e0d\u8db3\uff0c\u65e0\u6cd5\u521b\u5efa\u60ac\u8d4f\u5e16\u3002";
  if (message === "invalid_reward_points") return "\u60ac\u8d4f\u79ef\u5206\u9700\u5927\u4e8e 0\uff0c\u4e14\u4e0d\u80fd\u8d85\u8fc7 100000\u3002";
  if (message === "board_not_found") return "\u7248\u5757\u4e0d\u5b58\u5728\u6216\u672a\u53d1\u5e03\u3002";
  return message;
}
