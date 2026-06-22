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
        <div className="rounded-md border border-line bg-paper p-4">
          <p className="text-sm text-slate-500">当前登录</p>
          <p className="mt-1 font-semibold">{currentUser.email}</p>
          <p className="mt-1 text-sm text-slate-600">
            {currentUser.name} · {currentUser.role} · {currentUser.emailVerified ? "邮箱已验证" : "邮箱未验证"}
          </p>
          <button className="mt-4 rounded-md border border-line bg-white px-3 py-2 text-sm font-medium" disabled={loading} onClick={logout} type="button">
            退出登录
          </button>
        </div>
      ) : (
        <form className="space-y-4" onSubmit={login}>
          <label className="block text-sm font-medium">
            学生邮箱
            <input
              className="mt-2 w-full rounded-md border border-line px-3 py-2"
              onChange={(event) => setEmail(event.target.value)}
              placeholder="name@stu.henu.edu.cn"
              type="email"
              value={email}
            />
          </label>
          <label className="block text-sm font-medium">
            昵称
            <input className="mt-2 w-full rounded-md border border-line px-3 py-2" onChange={(event) => setName(event.target.value)} placeholder="可选" value={name} />
          </label>
          <label className="block text-sm font-medium">
            年级
            <input className="mt-2 w-full rounded-md border border-line px-3 py-2" onChange={(event) => setGrade(event.target.value)} placeholder="2023" value={grade} />
          </label>
          <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
            <label className="block text-sm font-medium">
              验证码
              <input className="mt-2 w-full rounded-md border border-line px-3 py-2" onChange={(event) => setCode(event.target.value)} placeholder="123456" value={code} />
            </label>
            <button className="mt-7 rounded-md border border-line px-3 py-2 text-sm font-medium disabled:opacity-60" disabled={loading || !email} onClick={sendCode} type="button">
              发送验证码
            </button>
          </div>
          <button className="w-full rounded-md bg-sage px-4 py-2 text-sm font-medium text-white disabled:opacity-60" disabled={loading || !email || !code} type="submit">
            {loading ? "处理中" : "登录"}
          </button>
        </form>
      )}

      {devCode ? <p className="mt-3 rounded-md bg-paper p-3 text-xs text-slate-600">开发环境验证码：{devCode}</p> : null}
      {message ? <p className="mt-3 rounded-md border border-line bg-white p-3 text-sm text-slate-700">{message}</p> : null}
      {error ? <p className="mt-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</p> : null}
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
