"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";

type CourseOption = {
  id: string;
  name: string;
};

type SubmissionFormProps = {
  courses: CourseOption[];
};

type SubmissionResponse = {
  submission?: {
    id: string;
    title: string;
  };
  error?: string;
};

export function SubmissionForm({ courses }: SubmissionFormProps) {
  const router = useRouter();
  const formRef = useRef<HTMLFormElement>(null);
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setMessage("");
    setError("");

    try {
      const response = await fetch("/api/submissions", {
        method: "POST",
        body: new FormData(event.currentTarget),
      });
      const payload = (await response.json()) as SubmissionResponse;

      if (!response.ok) {
        setError(payload.error ?? "投稿失败。");
        return;
      }

      setMessage("投稿已提交，进入人工审核队列。");
      formRef.current?.reset();
      router.refresh();
    } catch {
      setError("网络异常，请稍后重试。");
    } finally {
      setPending(false);
    }
  }

  return (
    <form ref={formRef} onSubmit={submit} className="grid gap-4 rounded-lg border border-line bg-white p-5 shadow-soft">
      <div className="grid gap-2">
        <label htmlFor="course_id" className="text-sm font-semibold text-ink">
          课程
        </label>
        <select
          id="course_id"
          name="course_id"
          required
          className="h-11 rounded-md border border-line bg-white px-3 text-sm text-ink focus-ring"
        >
          <option value="">选择课程</option>
          {courses.map((course) => (
            <option key={course.id} value={course.id}>
              {course.name}
            </option>
          ))}
        </select>
      </div>
      <div className="grid gap-2">
        <label htmlFor="title" className="text-sm font-semibold text-ink">
          资料标题
        </label>
        <input
          id="title"
          name="title"
          required
          minLength={2}
          maxLength={100}
          className="h-11 rounded-md border border-line px-3 text-sm text-ink focus-ring"
          placeholder="例如：离散数学第 3 章课堂整理"
        />
      </div>
      <div className="grid gap-2">
        <label htmlFor="description" className="text-sm font-semibold text-ink">
          资料说明
        </label>
        <textarea
          id="description"
          name="description"
          required
          maxLength={1000}
          rows={4}
          className="rounded-md border border-line px-3 py-2 text-sm leading-6 text-ink focus-ring"
          placeholder="说明资料来源、覆盖章节、适用范围或注意事项。"
        />
      </div>
      <div className="grid gap-2">
        <label htmlFor="file" className="text-sm font-semibold text-ink">
          文件
        </label>
        <input
          id="file"
          name="file"
          type="file"
          required
          accept="application/pdf,text/plain,.pdf,.txt"
          className="block w-full rounded-md border border-line bg-white px-3 py-2 text-sm text-ink file:mr-3 file:rounded-md file:border-0 file:bg-panel file:px-3 file:py-2 file:text-sm file:font-semibold file:text-ink focus-ring"
        />
        <p className="text-xs leading-5 text-muted">仅支持 PDF 或 TXT，单个文件不超过 10MB。</p>
      </div>
      {error ? (
        <div className="rounded-lg border border-[#e7c5b5] bg-[#fff7f3] p-3 text-sm font-semibold text-[#8b3f24]">
          {error}
        </div>
      ) : null}
      {message ? (
        <div className="rounded-lg border border-[#b7d7ca] bg-[#f2fbf7] p-3 text-sm font-semibold text-[#1d5a45]">
          {message}
        </div>
      ) : null}
      <button
        type="submit"
        disabled={pending || courses.length === 0}
        className="inline-flex h-11 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] disabled:cursor-not-allowed disabled:bg-line disabled:text-muted focus-ring"
      >
        {pending ? "提交中" : "提交审核"}
      </button>
    </form>
  );
}
