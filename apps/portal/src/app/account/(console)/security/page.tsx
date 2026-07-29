"use client";

import { useEffect, useRef, useState } from "react";
import {
  AccountCenterError,
  bootstrapAccountSecurity,
  changePassword,
  requestSecurityCode,
} from "@/lib/auth/account-center";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

const EMAIL_RE = /^[^@\s]+@henu\.edu\.cn$/i;

export default function SecurityPage() {
  useReveal();

  const [oldPwd, setOldPwd] = useState("");
  const [newPwd, setNewPwd] = useState("");
  const [newPwd2, setNewPwd2] = useState("");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [ok, setOk] = useState(false);
  const [pending, setPending] = useState(false);
  const [csrf, setCsrf] = useState("");
  const csrfBootstrap = useRef<Promise<string> | null>(null);
  const [cd, setCd] = useState(0);

  useEffect(() => {
    if (cd <= 0) return;
    const timer = window.setTimeout(() => setCd((value) => value - 1), 1000);
    return () => window.clearTimeout(timer);
  }, [cd]);

  const ensureCsrf = async () => {
    if (csrf) return csrf;
    if (!csrfBootstrap.current) {
      csrfBootstrap.current = bootstrapAccountSecurity().then(
        (result) => result.csrfToken
      );
    }
    const pendingBootstrap = csrfBootstrap.current;
    try {
      const token = await pendingBootstrap;
      setCsrf(token);
      return token;
    } finally {
      if (csrfBootstrap.current === pendingBootstrap) {
        csrfBootstrap.current = null;
      }
    }
  };

  const sendCode = async () => {
    setError("");
    if (!EMAIL_RE.test(email)) {
      setError("请输入绑定的 @henu.edu.cn 邮箱");
      return;
    }
    setPending(true);
    try {
      const token = await ensureCsrf();
      const result = await requestSecurityCode({
        csrfToken: token,
        email,
      });
      setCsrf(result.csrfToken);
      setCd(60);
    } catch (cause) {
      if (cause instanceof AccountCenterError && cause.code === "CSRF") {
        setCsrf("");
      }
      setError(
        cause instanceof AccountCenterError
          ? cause.message
          : "验证码发送失败，请稍后重试"
      );
    } finally {
      setPending(false);
    }
  };

  const submit = async () => {
    setError("");
    setOk(false);
    if (!oldPwd) return setError("请输入当前密码");
    if (Array.from(newPwd).length < 10) return setError("新密码至少 10 个字符");
    if (newPwd2 !== newPwd) return setError("两次输入的新密码不一致");
    if (!EMAIL_RE.test(email)) return setError("请输入绑定的 @henu.edu.cn 邮箱");
    if (!/^\d{6}$/.test(code)) return setError("请输入 6 位验证码");
    setPending(true);
    try {
      const token = await ensureCsrf();
      await changePassword({
        csrfToken: token,
        email,
        code,
        currentPassword: oldPwd,
        newPassword: newPwd,
      });
      setPending(false);
      setOk(true);
      setOldPwd("");
      setNewPwd("");
      setNewPwd2("");
      setCode("");
    } catch (cause) {
      if (cause instanceof AccountCenterError && cause.code === "CSRF") {
        setCsrf("");
      }
      setPending(false);
      setError(
        cause instanceof AccountCenterError
          ? cause.message
          : "密码修改失败，请稍后重试"
      );
    }
  };

  return (
    <div>
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">A-02</span>
        <span className="mx-2">/</span>
        SECURITY
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">安全设置</h1>

      <section data-enter className="mt-8 max-w-md border border-ink/25 p-6">
        <p className="font-mono text-xs tracking-[0.25em] text-ink/60">修改密码</p>
        <p className="mt-3 font-mono text-[10px] leading-5 tracking-wider text-ink/50">
          修改密码需当前密码与学校邮箱验证码；成功后其他设备会自动下线。
        </p>
        <div className="mt-5 space-y-4">
          {[
            { label: "当前密码", v: oldPwd, set: setOldPwd },
            { label: "新密码", v: newPwd, set: setNewPwd },
            { label: "确认新密码", v: newPwd2, set: setNewPwd2 },
          ].map((f) => (
            <div key={f.label}>
              <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
                {f.label}
              </label>
              <input
                type="password"
                value={f.v}
                onChange={(e) => f.set(e.target.value)}
                autoComplete={f.label === "当前密码" ? "current-password" : "new-password"}
                className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none focus:border-ink"
              />
            </div>
          ))}
          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
              绑定邮箱
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="name@henu.edu.cn"
              autoComplete="email"
              className="w-full border-b border-ink/30 bg-transparent py-2 font-mono text-sm outline-none placeholder:text-ink/30 focus:border-ink"
            />
          </div>
          <div>
            <label className="mb-1 block font-mono text-[10px] tracking-[0.25em] text-ink/50">
              邮箱验证码
            </label>
            <div className="flex gap-3">
              <input
                value={code}
                onChange={(event) =>
                  setCode(event.target.value.replace(/\D/g, "").slice(0, 6))
                }
                placeholder="6 位数字"
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                className="min-w-0 flex-1 border-b border-ink/30 bg-transparent py-2 font-mono text-sm tracking-[0.4em] outline-none placeholder:tracking-normal placeholder:text-ink/30 focus:border-ink"
              />
              <button
                type="button"
                disabled={pending || cd > 0}
                onClick={() => void sendCode()}
                className="shrink-0 border border-ink/40 px-3 py-1.5 font-mono text-[11px] transition-colors hover:border-ink disabled:cursor-not-allowed disabled:border-line disabled:text-ink/40"
              >
                {cd > 0 ? `${cd}s 后重发` : "发送验证码"}
              </button>
            </div>
          </div>
        </div>
        {error && <p className="mt-3 font-mono text-xs text-accent">{error}</p>}
        {ok && (
          <p className="mt-3 border border-ink bg-ink px-3 py-2 font-mono text-xs text-paper">
            ✓ 密码已更新，其他设备已下线
          </p>
        )}
        <button
          type="button"
          onClick={() => void submit()}
          disabled={pending}
          className={cn(
            "mt-5 border px-6 py-2.5 font-mono text-xs tracking-widest transition-colors",
            pending
              ? "cursor-wait border-line text-ink/40"
              : "border-ink bg-ink text-paper hover:border-accent hover:bg-accent"
          )}
        >
          {pending ? "提交中…" : "确认修改"}
        </button>
      </section>

      <section data-enter className="mt-10">
        <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
          会话安全
        </p>
        <div className="mt-4 border-y border-line py-4">
          <p className="font-mono text-xs leading-6 text-ink/60">
            不展示推测的设备名称、位置或 IP。修改密码会保留当前会话并撤销其余所有会话。
          </p>
        </div>
      </section>
    </div>
  );
}
