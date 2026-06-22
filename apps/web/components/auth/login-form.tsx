"use client";

import { FormEvent, useEffect, useState } from "react";
import { apiBaseUrl } from "@/lib/api";

type User = {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  grade: string;
  emailVerified: boolean;
};

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
  details?: unknown;
};

type SendCodeData = {
  expiresInSeconds: number;
  devCode?: string;
};

type LoginData = {
  user: User;
  accessToken: string;
  tokenType: string;
  expiresAt: string;
};

export function LoginForm() {
  const [email, setEmail] = useState("student@stu.henu.edu.cn");
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const [grade, setGrade] = useState("2023");
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [devCode, setDevCode] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    void loadMe();
  }, []);

  async function loadMe() {
    try {
      const response = await request<{ user?: User } | User>("/auth/me", { method: "GET" });
      const data = response.data;
      if (data && "user" in data && data.user) {
        setCurrentUser(data.user);
      } else if (data && "email" in data) {
        setCurrentUser(data as User);
      }
    } catch {
      setCurrentUser(null);
    }
  }

  async function sendCode() {
    setLoading(true);
    setError("");
    setMessage("");
    try {
      const response = await request<SendCodeData>("/auth/send-code", {
        method: "POST",
        body: JSON.stringify({ email }),
      });
      setDevCode(response.data?.devCode ?? "");
      setMessage(`验证码已发送，${response.data?.expiresInSeconds ?? 600} 秒内有效。`);
      if (response.data?.devCode) {
        setCode(response.data.devCode);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "发送验证码失败");
    } finally {
      setLoading(false);
    }
  }

  async function login(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoading(true);
    setError("");
    setMessage("");
    try {
      const response = await request<LoginData>("/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, code, name, grade }),
      });
      setCurrentUser(response.data?.user ?? null);
      setMessage("登录成功，后续下载和错题保存会使用该会话。");
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setLoading(false);
    }
  }

  async function logout() {
    setLoading(true);
    setError("");
    setMessage("");
    try {
      await request<{ ok: boolean }>("/auth/logout", { method: "POST" });
      setCurrentUser(null);
      setMessage("已退出登录。");
    } catch (err) {
      setError(err instanceof Error ? err.message : "退出失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="mt-6">
      {currentUser ? (
        <div className="rounded-2xl border border-border bg-muted p-4">
          <p className="text-sm text-muted-foreground">当前登录</p>
          <p className="mt-1 break-words font-semibold text-foreground">{currentUser.email}</p>
          <p className="mt-1 text-sm text-muted-foreground">
            {currentUser.name} · {currentUser.role} · {currentUser.emailVerified ? "邮箱已验证" : "邮箱未验证"}
          </p>
          <button
            className="mt-4 rounded-xl border border-border bg-card px-3 py-2 text-sm font-medium text-foreground shadow-sm transition hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
            disabled={loading}
            onClick={logout}
            type="button"
          >
            退出登录
          </button>
        </div>
      ) : (
        <form className="space-y-4" onSubmit={login}>
          <label className="block text-sm font-medium text-foreground">
            学生邮箱
            <input
              className="mt-2 w-full rounded-xl border border-border bg-card px-3 py-2 text-sm shadow-sm"
              onChange={(event) => setEmail(event.target.value)}
              placeholder="name@stu.henu.edu.cn"
              type="email"
              value={email}
            />
          </label>
          <label className="block text-sm font-medium text-foreground">
            昵称
            <input
              className="mt-2 w-full rounded-xl border border-border bg-card px-3 py-2 text-sm shadow-sm"
              onChange={(event) => setName(event.target.value)}
              placeholder="可选"
              value={name}
            />
          </label>
          <label className="block text-sm font-medium text-foreground">
            年级
            <input
              className="mt-2 w-full rounded-xl border border-border bg-card px-3 py-2 text-sm shadow-sm"
              onChange={(event) => setGrade(event.target.value)}
              placeholder="2023"
              value={grade}
            />
          </label>
          <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
            <label className="block text-sm font-medium text-foreground">
              验证码
              <input
                className="mt-2 w-full rounded-xl border border-border bg-card px-3 py-2 text-sm shadow-sm"
                onChange={(event) => setCode(event.target.value)}
                placeholder="123456"
                value={code}
              />
            </label>
            <button
              className="rounded-xl border border-border bg-card px-3 py-2 text-sm font-medium text-foreground shadow-sm transition hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60 sm:mt-7"
              disabled={loading || !email}
              onClick={sendCode}
              type="button"
            >
              发送验证码
            </button>
          </div>
          <button
            className="w-full rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] disabled:cursor-not-allowed disabled:opacity-60"
            disabled={loading || !email || !code}
            type="submit"
          >
            {loading ? "处理中" : "登录"}
          </button>
        </form>
      )}

      {devCode ? (
        <p className="mt-3 rounded-xl border border-border bg-muted p-3 text-xs text-muted-foreground" role="status" aria-live="polite">
          开发环境验证码：{devCode}
        </p>
      ) : null}
      {message ? (
        <p className="mt-3 rounded-xl border border-border bg-card p-3 text-sm text-foreground" role="status" aria-live="polite">
          {message}
        </p>
      ) : null}
      {error ? (
        <p className="mt-3 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

async function request<T>(path: string, init: RequestInit): Promise<Envelope<T>> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init.headers,
    },
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `API request failed with ${response.status}`);
  }
  return payload;
}
