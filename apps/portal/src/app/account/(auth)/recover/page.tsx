"use client";

/**
 * Existing recovery shell wired to Platform Core password recovery.
 */

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { HenuEmailField } from "@/components/account/henu-email-field";
import { useReveal } from "@/components/account/use-reveal";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  AccountCenterError,
  bootstrapPasswordRecovery,
  portalOAuthStartUrl,
  recoverPassword,
  requestRecoveryCode,
} from "@/lib/auth/account-center";
import {
  isValidHenuLocalPart,
  toHenuEmail,
} from "@/lib/auth/henu-email";
import { cn } from "@/lib/cn";

const STEPS = ["验证邮箱", "重置密码", "完成"] as const;

export default function RecoverPage() {
  useReveal();
  const [step, setStep] = useState(0);
  const [localPart, setLocalPart] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [passwordConfirm, setPasswordConfirm] = useState("");
  const [csrf, setCsrf] = useState("");
  const csrfBootstrap = useRef<Promise<string> | null>(null);
  const [returnTo] = useState(() => portalOAuthStartUrl("/account"));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [cd, setCd] = useState(0);
  const fullEmail = toHenuEmail(localPart);

  useEffect(() => {
    if (cd <= 0) return;
    const t = window.setTimeout(() => setCd((c) => c - 1), 1000);
    return () => window.clearTimeout(t);
  }, [cd]);

  const ensureCsrf = async () => {
    if (csrf) return csrf;
    if (!csrfBootstrap.current) {
      csrfBootstrap.current = bootstrapPasswordRecovery().then(
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

  const onSend = async () => {
    setError("");
    if (!isValidHenuLocalPart(localPart)) {
      setError("请输入邮箱前缀（自动补全 @henu.edu.cn）");
      return;
    }
    setPending(true);
    try {
      const token = await ensureCsrf();
      const result = await requestRecoveryCode({
        csrfToken: token,
        email: fullEmail,
      });
      setCsrf(result.csrfToken);
      setStep(1);
      setCd(60);
    } catch (e) {
      if (e instanceof AccountCenterError && e.code === "CSRF") {
        setCsrf("");
      }
      setError(
        e instanceof AccountCenterError
          ? e.message
          : "发送失败（请确认邮件通道已配置）"
      );
    } finally {
      setPending(false);
    }
  };

  const onVerify = async () => {
    setError("");
    if (!/^\d{6}$/.test(code.trim())) {
      setError("请输入 6 位数字验证码");
      return;
    }
    if (Array.from(password).length < 10) {
      setError("新密码至少 10 个字符");
      return;
    }
    if (password !== passwordConfirm) {
      setError("两次输入的新密码不一致");
      return;
    }
    setPending(true);
    try {
      const token = await ensureCsrf();
      await recoverPassword({
        csrfToken: token,
        email: fullEmail,
        code: code.trim(),
        password,
      });
      setStep(2);
      // Always continue via Portal Gateway OAuth after core session is set.
      window.setTimeout(() => window.location.assign(returnTo), 500);
    } catch (e) {
      if (e instanceof AccountCenterError && e.code === "CSRF") {
        setCsrf("");
      }
      setError(
        e instanceof AccountCenterError ? e.message : "验证失败，请重试"
      );
      setPending(false);
    }
  };

  return (
    <main className="bg-blueprint flex min-h-svh items-center justify-center px-4 py-10 sm:px-5 sm:py-16">
      <div
        data-enter
        className="w-full max-w-md border border-ink bg-paper p-5 sm:p-8 md:p-10"
      >
        <div className="flex items-baseline justify-between">
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">ACC-02</span>
            <span className="mx-2">/</span>
            RECOVER
          </p>
          <Link
            href="/account/login"
            className="font-mono text-[10px] tracking-widest text-ink/40 hover:text-accent"
          >
            ← 登录
          </Link>
        </div>
        <h1 className="mt-4 font-display text-4xl font-bold tracking-tight">
          找回密码
        </h1>
        <p className="mt-3 font-mono text-xs leading-6 tracking-wider text-ink/55">
          验证学校邮箱后设置新密码；完成后旧会话会全部失效，并自动登录当前设备。
        </p>

        <ol className="mt-6 flex flex-wrap gap-y-1">
          {STEPS.map((label, i) => (
            <li
              key={label}
              className={cn(
                "font-mono text-[10px] tracking-widest",
                i === step
                  ? "text-accent"
                  : i < step
                    ? "text-ink/50"
                    : "text-ink/25"
              )}
            >
              {String(i + 1).padStart(2, "0")} {label}
              {i < STEPS.length - 1 ? (
                <span className="mx-2 text-ink/20">/</span>
              ) : null}
            </li>
          ))}
        </ol>

        <div className="mt-8 space-y-5">
          {step === 0 && (
            <>
              <HenuEmailField
                id="recover-email"
                value={localPart}
                onChange={setLocalPart}
                autoFocus
              />
              {error ? (
                <p className="font-mono text-[10px] text-accent">{error}</p>
              ) : null}
              <Button
                type="button"
                className="w-full"
                disabled={pending}
                onClick={() => void onSend()}
              >
                {pending ? "发送中…" : "发送验证码"}
              </Button>
            </>
          )}

          {step === 1 && (
            <>
              <p className="font-mono text-[11px] text-ink/50">
                已发送至 <span className="text-ink">{fullEmail}</span>
              </p>
              <div>
                <Label htmlFor="recover-code">6 位验证码</Label>
                <div className="flex flex-col items-stretch gap-2 sm:flex-row sm:items-end sm:gap-3">
                  <Input
                    id="recover-code"
                    inputMode="numeric"
                    maxLength={6}
                    value={code}
                    onChange={(e) =>
                      setCode(e.target.value.replace(/\D/g, "").slice(0, 6))
                    }
                    className="tracking-[0.35em]"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={cd > 0 || pending}
                    onClick={() => void onSend()}
                    className="w-full sm:w-auto"
                  >
                    {cd > 0 ? `${cd}s` : "重发"}
                  </Button>
                </div>
              </div>
              <div>
                <Label htmlFor="recover-password">新密码</Label>
                <Input
                  id="recover-password"
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="至少 10 个字符"
                />
              </div>
              <div>
                <Label htmlFor="recover-password-confirm">确认新密码</Label>
                <Input
                  id="recover-password-confirm"
                  type="password"
                  autoComplete="new-password"
                  value={passwordConfirm}
                  onChange={(e) => setPasswordConfirm(e.target.value)}
                />
              </div>
              {error ? (
                <p className="font-mono text-[10px] text-accent">{error}</p>
              ) : null}
              <Button
                type="button"
                className="w-full"
                disabled={pending}
                onClick={() => void onVerify()}
              >
                {pending ? "重置中…" : "重置密码并登录"}
              </Button>
            </>
          )}

          {step === 2 && (
            <div className="space-y-3">
              <p className="font-mono text-sm text-ink">
                验证成功，正在进入账号…
              </p>
              <Button
                type="button"
                className="w-full"
                onClick={() => window.location.assign(returnTo)}
              >
                继续 →
              </Button>
            </div>
          )}
        </div>
      </div>
    </main>
  );
}
