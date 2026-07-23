"use client";

import Link from "next/link";
import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useSyncExternalStore } from "react";
import { authStore } from "@/lib/auth/store";
import { EMAIL_DEMO_CODE } from "@/lib/auth/mock";
import { useReveal } from "@/components/account/use-reveal";
import CodeField from "@/components/account/code-field";
import { cn } from "@/lib/cn";

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function Field({
  label,
  type = "text",
  value,
  onChange,
  error,
  placeholder,
}: {
  label: string;
  type?: string;
  value: string;
  onChange: (v: string) => void;
  error?: string;
  placeholder?: string;
}) {
  return (
    <div>
      <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
        {label}
      </label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={cn(
          "w-full border-b bg-transparent py-2 font-mono text-sm outline-none transition-colors placeholder:text-ink/30",
          error ? "border-accent" : "border-ink/30 focus:border-ink"
        )}
      />
      {error && <p className="mt-1 font-mono text-[10px] text-accent">{error}</p>}
    </div>
  );
}

/** 验证码行（共享组件见 components/account/code-field.tsx） */

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const next = params.get("next");
  const { user, ready } = useSyncExternalStore(
    authStore.subscribe,
    authStore.get,
    authStore.getServer
  );
  useReveal();

  const [tab, setTab] = useState<"login" | "register">("login");
  const [mode, setMode] = useState<"password" | "code">("password");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [pwd, setPwd] = useState("");
  const [pwd2, setPwd2] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);

  // 已登录访问登录页 → 回控制台
  useEffect(() => {
    if (ready && user) router.replace(next || "/account");
  }, [ready, user, next, router]);

  const submit = () => {
    const errs: Record<string, string> = {};
    const needCode = tab === "register" || mode === "code";
    if (tab === "login" && mode === "password" && !name.trim()) errs.name = "请输入账号名";
    if (needCode && !EMAIL_RE.test(email)) errs.email = "邮箱格式不正确";
    if (needCode && code !== EMAIL_DEMO_CODE) errs.code = "验证码不正确（演示码 427819）";
    if ((tab === "register" || mode === "password") && pwd.length < 6) errs.pwd = "密码至少 6 位";
    if (tab === "register") {
      if (!name.trim()) errs.name = "请输入账号名";
      if (pwd2 !== pwd) errs.pwd2 = "两次输入的密码不一致";
    }
    setErrors(errs);
    if (Object.keys(errs).length) return;

    setPending(true);
    // mock 假延迟
    setTimeout(() => {
      if (tab === "register") authStore.register(name.trim(), email.trim());
      else authStore.login(mode === "code" ? email.split("@")[0] : name.trim());
      router.push(next || "/account");
    }, 500);
  };

  return (
    <main className="bg-blueprint flex min-h-svh items-center justify-center px-5 py-16">
      <div data-enter className="w-full max-w-md border border-ink bg-paper p-8 md:p-10">
        <div className="flex items-baseline justify-between">
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">ACC-01</span>
            <span className="mx-2">/</span>
            AUTH
          </p>
          <Link href="/" className="font-mono text-[10px] tracking-widest text-ink/40 hover:text-accent">
            ← henukit
          </Link>
        </div>
        <h1 className="mt-4 font-display text-4xl font-bold tracking-tight">
          {tab === "login" ? "登录" : "注册"}
        </h1>

        {/* 页签 */}
        <div className="mt-6 flex border border-line">
          {(["login", "register"] as const).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => {
                setTab(t);
                setErrors({});
              }}
              className={cn(
                "flex-1 py-2 font-mono text-xs tracking-widest transition-colors",
                tab === t ? "bg-ink text-paper" : "text-ink/50 hover:text-ink"
              )}
            >
              {t === "login" ? "登录" : "注册"}
            </button>
          ))}
        </div>

        {/* 登录形态切换 */}
        {tab === "login" && (
          <div className="mt-4 flex gap-2">
            {(["password", "code"] as const).map((m) => (
              <button
                key={m}
                type="button"
                onClick={() => {
                  setMode(m);
                  setErrors({});
                }}
                className={cn(
                  "border px-3 py-1.5 font-mono text-[11px] transition-colors",
                  mode === m ? "border-ink bg-ink text-paper" : "border-line text-ink/60 hover:border-ink/40"
                )}
              >
                {m === "password" ? "密码登录" : "验证码登录"}
              </button>
            ))}
          </div>
        )}

        <div className="mt-6 space-y-5">
          {tab === "register" && (
            <Field label="账号 / NAME" value={name} onChange={setName} error={errors.name} placeholder="学号或昵称" />
          )}
          {tab === "login" && mode === "password" && (
            <Field label="账号 / NAME" value={name} onChange={setName} error={errors.name} placeholder="学号或昵称" />
          )}
          {(tab === "register" || mode === "code") && (
            <Field label="邮箱 / EMAIL" type="email" value={email} onChange={setEmail} error={errors.email} placeholder="name@stu.henu.edu.cn" />
          )}
          {(tab === "register" || mode === "code") && (
            <CodeField email={email} value={code} onChange={setCode} error={errors.code} />
          )}
          {(tab === "register" || mode === "password") && (
            <Field label="密码 / PASSWORD" type="password" value={pwd} onChange={setPwd} error={errors.pwd} placeholder="至少 6 位" />
          )}
          {tab === "register" && (
            <Field label="确认密码 / CONFIRM" type="password" value={pwd2} onChange={setPwd2} error={errors.pwd2} />
          )}
        </div>

        <button
          type="button"
          onClick={submit}
          disabled={pending}
          className={cn(
            "mt-8 w-full border py-3.5 font-mono text-sm tracking-widest transition-colors",
            pending
              ? "cursor-wait border-line text-ink/40"
              : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
          )}
        >
          {pending ? "处理中…" : tab === "login" ? "登 录" : "注 册"}
        </button>

        <div className="mt-4 flex justify-between font-mono text-[10px] tracking-wider text-ink/50">
          <Link href="/account/recover" className="hover:text-accent">
            忘记密码 →
          </Link>
          <span>v1 预览 · 任意账号可登录</span>
        </div>
      </div>
    </main>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}
