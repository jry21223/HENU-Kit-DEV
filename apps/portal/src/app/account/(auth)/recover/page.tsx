"use client";

/**
 * Recover — original multi-step shell, email code via real mail channel.
 * Product has no password reset API yet; re-auth by henu email code then OAuth.
 */

import Link from "next/link";
import { useEffect, useState } from "react";
import { HenuEmailField } from "@/components/account/henu-email-field";
import { useReveal } from "@/components/account/use-reveal";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  AccountCenterError,
  bootstrapAccountLogin,
  portalOAuthStartUrl,
  requestLoginCode,
  verifyLoginCode,
} from "@/lib/auth/account-center";
import {
  isValidHenuLocalPart,
  toHenuEmail,
} from "@/lib/auth/henu-email";
import { cn } from "@/lib/cn";

const STEPS = ["验证邮箱", "输入验证码", "完成"] as const;

export default function RecoverPage() {
  useReveal();
  const [step, setStep] = useState(0);
  const [localPart, setLocalPart] = useState("");
  const [code, setCode] = useState("");
  const [csrf, setCsrf] = useState("");
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

  useEffect(() => {
    let cancelled = false;
    bootstrapAccountLogin(returnTo)
      .then((b) => {
        if (!cancelled) setCsrf(b.csrfToken);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [returnTo]);

  const ensureCsrf = async () => {
    if (csrf) return csrf;
    const b = await bootstrapAccountLogin(returnTo);
    setCsrf(b.csrfToken);
    return b.csrfToken;
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
      const result = await requestLoginCode({
        csrfToken: token,
        email: fullEmail,
        returnTo,
      });
      setCsrf(result.csrfToken);
      setStep(1);
      setCd(60);
    } catch (e) {
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
    setPending(true);
    try {
      const token = await ensureCsrf();
      await verifyLoginCode({
        csrfToken: token,
        email: fullEmail,
        code: code.trim(),
        returnTo,
      });
      setStep(2);
      // Always continue via Portal Gateway OAuth after core session is set.
      window.setTimeout(() => window.location.assign(returnTo), 500);
    } catch (e) {
      setError(
        e instanceof AccountCenterError ? e.message : "验证失败，请重试"
      );
      setPending(false);
    }
  };

  return (
    <main className="bg-blueprint flex min-h-svh items-center justify-center px-5 py-16">
      <div
        data-enter
        className="w-full max-w-md border border-ink bg-paper p-8 md:p-10"
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
          找回 / 重新验证
        </h1>
        <p className="mt-3 font-mono text-xs leading-6 tracking-wider text-ink/55">
          通过学校邮箱验证码重新进入账号（验证码走真实发信通道）。密码重置接口尚未提供。
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
                <div className="flex items-end gap-3">
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
                  >
                    {cd > 0 ? `${cd}s` : "重发"}
                  </Button>
                </div>
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
                {pending ? "验证中…" : "验证并进入"}
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
