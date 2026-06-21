"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { aiOutputTypeOptions } from "@/lib/ai-jobs";

type CourseOption = {
  id: string;
  name: string;
};

type AiJobFormProps = {
  courses: CourseOption[];
};

type AiJobResponse = {
  job?: {
    id: string;
    status: string;
    result: unknown;
  };
  error?: string;
};

function getMaterialId(result: unknown) {
  if (result && typeof result === "object" && "materialId" in result) {
    const materialId = (result as { materialId?: unknown }).materialId;
    return typeof materialId === "string" ? materialId : "";
  }
  return "";
}

export function AiJobForm({ courses }: AiJobFormProps) {
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setMessage("");
    setError("");

    const formData = new FormData(event.currentTarget);
    const body = {
      course_id: String(formData.get("course_id") ?? ""),
      output_type: String(formData.get("output_type") ?? ""),
      input_material_ids: String(formData.get("input_material_ids") ?? ""),
      simulate_failure: formData.get("simulate_failure") === "on",
    };

    try {
      const response = await fetch("/api/admin/ai-jobs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const payload = (await response.json()) as AiJobResponse;

      if (!response.ok) {
        setError(payload.error ?? "创建 AI 任务失败。");
        return;
      }

      const materialId = getMaterialId(payload.job?.result);
      setMessage(
        materialId
          ? `任务已完成，生成草稿资料：${materialId}。`
          : `任务已记录，状态：${payload.job?.status ?? "unknown"}。`,
      );
      router.refresh();
    } catch {
      setError("网络异常，请稍后重试。");
    } finally {
      setPending(false);
    }
  }

  return (
    <form onSubmit={submit} className="grid gap-4 rounded-lg border border-line bg-white p-5 shadow-soft">
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
        <label htmlFor="output_type" className="text-sm font-semibold text-ink">
          生成类型
        </label>
        <select
          id="output_type"
          name="output_type"
          required
          className="h-11 rounded-md border border-line bg-white px-3 text-sm text-ink focus-ring"
          defaultValue="knowledge_note"
        >
          {aiOutputTypeOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </div>
      <div className="grid gap-2">
        <label htmlFor="input_material_ids" className="text-sm font-semibold text-ink">
          来源资料 ID
        </label>
        <textarea
          id="input_material_ids"
          name="input_material_ids"
          rows={3}
          className="rounded-md border border-line px-3 py-2 text-sm leading-6 text-ink focus-ring"
          placeholder="可选，多个 ID 用英文逗号分隔。来源资料必须属于所选课程且已发布。"
        />
      </div>
      <label className="flex items-center gap-2 text-sm font-semibold text-ink">
        <input type="checkbox" name="simulate_failure" className="h-4 w-4 rounded border-line" />
        模拟失败
      </label>
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
        {pending ? "创建中" : "创建 AI 任务"}
      </button>
    </form>
  );
}
