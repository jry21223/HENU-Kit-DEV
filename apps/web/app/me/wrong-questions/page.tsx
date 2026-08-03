"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, BarChart3, BookOpenCheck, RotateCcw, Trash2 } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { apiBaseUrl, type Course, type QuizQuestion, type WeaknessPoint, type WrongQuestion } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type PageState = {
  wrongQuestions: WrongQuestion[];
  weakness: WeaknessPoint[];
  courses: Record<string, Course>;
  questions: Record<string, QuizQuestion | null>;
};

const copy = {
  back: "返回个人中心",
  eyebrow: "我的错题本",
  title: "错题记录与薄弱课程",
  intro:
    "这里只读取当前登录账号自己的错题记录。题目详情来自公开题目，不包含标准答案；删除错题只会移出错题本，不会影响题库或练习记录。",
  loading: "正在加载错题本...",
  login: "去登录",
  empty: "暂无错题。完成课程练习后，答错的题目会自动进入这里。",
  fallbackError: "错题本暂时不可用",
  weakCourses: "薄弱课程",
  totalWrong: "累计错题次数",
  wrongCount: "错误次数",
  lastAnswer: "上次答案",
  updatedAt: "更新时间",
  noAnswer: "未记录",
  questionUnavailable: "题目已归档或暂不可见",
  practiceAgain: "重新练习",
  remove: "移出错题本",
  removing: "移除中...",
};

export default function WrongQuestionsPage() {
  const [state, setState] = useState<PageState>({
    wrongQuestions: [],
    weakness: [],
    courses: {},
    questions: {},
  });
  const [loading, setLoading] = useState(true);
  const [savingId, setSavingId] = useState("");
  const [error, setError] = useState("");

  const totalWrongCount = useMemo(
    () => state.wrongQuestions.reduce((total, item) => total + item.wrongCount, 0),
    [state.wrongQuestions],
  );

  async function loadWrongQuestions() {
    setLoading(true);
    setError("");
    try {
      const [wrongResponse, weaknessResponse, courseResult] = await Promise.all([
        request<{ wrongQuestions: WrongQuestion[] }>("/me/wrong-questions"),
        request<{ weakness: WeaknessPoint[] }>("/me/weakness-report"),
        request<{ courses: Course[] }>("/courses").catch(() => ({
          code: 0,
          message: "ok",
          data: { courses: [] },
        })),
      ]);

      const wrongQuestions = wrongResponse.data?.wrongQuestions ?? [];
      const questionPairs = await Promise.all(
        wrongQuestions.map(async (wrong) => {
          try {
            const response = await request<{ question: QuizQuestion }>(`/questions/${wrong.questionId}`);
            return [wrong.questionId, response.data?.question ?? null] as const;
          } catch {
            return [wrong.questionId, null] as const;
          }
        }),
      );

      setState({
        wrongQuestions,
        weakness: weaknessResponse.data?.weakness ?? [],
        courses: Object.fromEntries((courseResult.data?.courses ?? []).map((course) => [course.id, course])),
        questions: Object.fromEntries(questionPairs),
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.fallbackError);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadWrongQuestions();
  }, []);

  async function removeWrongQuestion(id: string) {
    setSavingId(id);
    setError("");
    try {
      await request<{ deleted: boolean }>(`/me/wrong-questions/${id}`, { method: "DELETE" });
      await loadWrongQuestions();
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.fallbackError);
    } finally {
      setSavingId("");
    }
  }

  return (
    <SiteShell>
      <nav className="flex flex-wrap items-center justify-between gap-3 text-sm">
        <Link className="font-semibold text-primary" href="/me">
          {copy.back}
        </Link>
        <Link className="rounded-xl border border-border px-3 py-2 font-medium hover:bg-muted" href="/courses">
          课程练习
        </Link>
      </nav>

      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
            <h1 className="mt-2 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{copy.title}</h1>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <StatCard label="错题条目" value={state.wrongQuestions.length} />
            <StatCard label={copy.totalWrong} value={totalWrongCount} />
          </div>
        </div>
      </section>

      {state.weakness.length > 0 ? (
        <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
          <div className="flex items-center gap-2">
            <BarChart3 className="size-5 text-primary" aria-hidden="true" />
            <h2 className="text-lg font-semibold tracking-tight">{copy.weakCourses}</h2>
          </div>
          <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {state.weakness.map((item) => (
              <Link
                className="rounded-2xl border border-border bg-background p-4 transition hover:border-primary/60"
                href={`/courses/${item.courseId}/quiz`}
                key={item.courseId}
              >
                <p className="break-words text-sm font-semibold">{state.courses[item.courseId]?.name ?? item.courseId}</p>
                <p className="mt-2 text-xs text-muted-foreground">
                  {copy.wrongCount}: {item.wrongCount}
                </p>
              </Link>
            ))}
          </div>
        </section>
      ) : null}

      {loading ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.loading}</p> : null}
      {error ? (
        <div className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">
          <p>{error}</p>
          <Link className="mt-3 inline-flex rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground" href="/login">
            {copy.login}
          </Link>
        </div>
      ) : null}

      {!loading && !error ? (
        <section className="grid gap-3">
          {state.wrongQuestions.map((wrong) => {
            const question = state.questions[wrong.questionId];
            const course = state.courses[wrong.courseId];
            return (
              <article className="rounded-3xl border border-border bg-card p-5 shadow-sm" key={wrong.id}>
                <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge tone="muted">{course?.name ?? "暂无课程信息"}</Badge>
                      <Badge tone="muted">{questionTypeLabel(question?.type)}</Badge>
                      <Badge tone="success">
                        {copy.wrongCount}: {wrong.wrongCount}
                      </Badge>
                    </div>
                    <h2 className="mt-3 whitespace-pre-wrap break-words text-base font-semibold leading-7">
                      {question?.stem ?? copy.questionUnavailable}
                    </h2>
                    <dl className="mt-4 grid gap-3 text-sm sm:grid-cols-2">
                      <Info label={copy.lastAnswer}>{wrong.lastAnswer || copy.noAnswer}</Info>
                      <Info label={copy.updatedAt}>{formatDate(wrong.updatedAt)}</Info>
                    </dl>
                  </div>
                  <div className="flex shrink-0 flex-col gap-2 sm:w-36">
                    <Link
                      className="inline-flex items-center justify-center rounded-xl border border-border px-3 py-2 text-sm font-medium hover:bg-muted"
                      href={`/courses/${wrong.courseId}/quiz`}
                    >
                      <RotateCcw className="mr-2 size-4 text-primary" aria-hidden="true" />
                      {copy.practiceAgain}
                    </Link>
                    <button
                      className="inline-flex items-center justify-center rounded-xl border border-border px-3 py-2 text-sm font-medium hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
                      disabled={savingId === wrong.id}
                      onClick={() => removeWrongQuestion(wrong.id)}
                      type="button"
                    >
                      <Trash2 className="mr-2 size-4 text-primary" aria-hidden="true" />
                      {savingId === wrong.id ? copy.removing : copy.remove}
                    </button>
                  </div>
                </div>
              </article>
            );
          })}
          {state.wrongQuestions.length === 0 ? (
            <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.empty}</p>
          ) : null}
        </section>
      ) : null}
    </SiteShell>
  );
}

function StatCard({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-2xl border border-border bg-background p-4 text-sm">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 flex items-center text-2xl font-semibold">
        <AlertTriangle className="mr-2 size-5 text-primary" aria-hidden="true" />
        {value}
      </p>
    </div>
  );
}

function Info({ children, label }: { children: string; label: string }) {
  return (
    <div className="rounded-2xl border border-border bg-background p-3">
      <dt className="flex items-center text-xs text-muted-foreground">
        <BookOpenCheck className="mr-1.5 size-3.5" aria-hidden="true" />
        {label}
      </dt>
      <dd className="mt-1 min-w-0 whitespace-pre-wrap break-words font-medium">{children}</dd>
    </div>
  );
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

async function request<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
  const headers = new Headers(init.headers);
  if (init.body && !(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || "网络不太顺畅，请检查网络后重试");
  }
  return payload;
}

function questionTypeLabel(type?: string) {
  const labels: Record<string, string> = {
    single: "单选",
    multiple: "多选",
    judge: "判断",
  };
  if (!type) return "题目";
  return labels[type] ?? type;
}
