"use client";

import { useState } from "react";
import type { Major, School } from "@/types";

type LoginFormProps = {
  schools: School[];
  majors: Major[];
  grades: string[];
};

type Step = "email" | "code";

export function LoginForm({ schools, majors, grades }: LoginFormProps) {
  const [step, setStep] = useState<Step>("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [schoolId, setSchoolId] = useState(schools[0]?.id ?? "henu");
  const [majorId, setMajorId] = useState(majors[0]?.id ?? "network-engineering");
  const [grade, setGrade] = useState(grades[0] ?? "2023级");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(false);

  async function sendCode(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setIsLoading(true);
    setError("");
    setMessage("");

    const response = await fetch("/api/auth/send-code", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    });
    const result = (await response.json()) as { message?: string; error?: string };

    setIsLoading(false);
    if (!response.ok) {
      setError(result.error ?? "验证码发送失败。");
      return;
    }

    setMessage(result.message ?? "验证码已发送。");
    setStep("code");
  }

  async function verifyCode(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setIsLoading(true);
    setError("");
    setMessage("");

    const response = await fetch("/api/auth/verify-code", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email,
        code,
        school_id: schoolId,
        major_id: majorId,
        grade,
      }),
    });
    const result = (await response.json()) as { error?: string };

    setIsLoading(false);
    if (!response.ok) {
      setError(result.error ?? "登录失败。");
      return;
    }

    window.location.href = "/courses";
  }

  return (
    <section className="max-w-xl rounded-lg border border-line bg-white p-6 shadow-soft">
      {step === "email" ? (
        <form className="grid gap-4" onSubmit={sendCode}>
          <label className="grid gap-2 text-sm font-semibold text-ink">
            学生邮箱
            <input
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="name@stu.henu.edu.cn"
              className="h-11 rounded-md border border-line bg-white px-3 text-sm text-ink focus-ring"
              required
            />
          </label>
          <button
            type="submit"
            disabled={isLoading}
            className="h-11 rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] disabled:cursor-not-allowed disabled:bg-line disabled:text-muted focus-ring"
          >
            {isLoading ? "发送中" : "发送验证码"}
          </button>
        </form>
      ) : (
        <form className="grid gap-4" onSubmit={verifyCode}>
          <label className="grid gap-2 text-sm font-semibold text-ink">
            学生邮箱
            <input
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              className="h-11 rounded-md border border-line bg-white px-3 text-sm text-ink focus-ring"
              required
            />
          </label>
          <label className="grid gap-2 text-sm font-semibold text-ink">
            验证码
            <input
              inputMode="numeric"
              value={code}
              onChange={(event) => setCode(event.target.value)}
              placeholder="6 位数字"
              className="h-11 rounded-md border border-line bg-white px-3 text-sm text-ink focus-ring"
              required
            />
          </label>
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="grid gap-2 text-sm font-semibold text-ink">
              学校
              <select
                value={schoolId}
                onChange={(event) => setSchoolId(event.target.value)}
                className="h-11 rounded-md border border-line bg-white px-3 text-sm font-normal text-ink focus-ring"
              >
                {schools.map((school) => (
                  <option key={school.id} value={school.id}>
                    {school.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="grid gap-2 text-sm font-semibold text-ink">
              专业
              <select
                value={majorId}
                onChange={(event) => setMajorId(event.target.value)}
                className="h-11 rounded-md border border-line bg-white px-3 text-sm font-normal text-ink focus-ring"
              >
                {majors.map((major) => (
                  <option key={major.id} value={major.id}>
                    {major.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="grid gap-2 text-sm font-semibold text-ink">
              年级
              <select
                value={grade}
                onChange={(event) => setGrade(event.target.value)}
                className="h-11 rounded-md border border-line bg-white px-3 text-sm font-normal text-ink focus-ring"
              >
                {grades.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <button
            type="submit"
            disabled={isLoading}
            className="h-11 rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] disabled:cursor-not-allowed disabled:bg-line disabled:text-muted focus-ring"
          >
            {isLoading ? "登录中" : "验证并登录"}
          </button>
          <button
            type="button"
            onClick={() => setStep("email")}
            className="h-10 rounded-md border border-line px-4 text-sm font-semibold text-ink hover:bg-panel focus-ring"
          >
            重新发送
          </button>
        </form>
      )}

      {message ? (
        <p className="mt-4 rounded-md border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-900">
          {message}
        </p>
      ) : null}
      {error ? (
        <p className="mt-4 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-900">
          {error}
        </p>
      ) : null}
      <p className="mt-5 rounded-lg border border-line bg-panel p-4 text-sm leading-6 text-muted">
        白名单邮箱域名：henu.edu.cn、stu.henu.edu.cn。开发环境验证码只输出到服务端日志，不会返回给前端。
      </p>
    </section>
  );
}

