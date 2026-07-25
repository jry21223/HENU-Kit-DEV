"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { AuthShell } from "@/components/account/auth-shell";
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

const EMAIL_RE = /^[^\s@]+@henu\.edu\.cn$/i;
const STEPS = ["验证邮箱", "输入验证码", "完成"] as const;

/**
 * Recover is email-code re-auth (no password in this product).
 * Same Platform Core verification path as login; after success we open
 * Portal OAuth so the user lands in account console.
 */
export default function RecoverPage() {
  const [step, setStep] = useState(0);
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [csrf, setCsrf] = useState("");
  const [returnTo] = useState(() => portalOAuthStartUrl("/account"));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [cd, setCd] = useState(0);

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
    const normalized = email.trim().toLowerCase();
    if (!EMAIL_RE.test(normalized)) {
      setError("请使用 @henu.edu.cn 学校邮箱");
      return;
    }
    setPending(true);
    try {
      const token = await ensureCsrf();
      const result = await requestLoginCode({
        csrfToken: token,
        email: normalized,
        returnTo,
      });
      setCsrf(result.csrfToken);
      setStep(1);
      setCd(60);
    } catch (e) {
      setError(
        e instanceof AccountCenterError ? e.message : "发送失败，请稍后重试"
      );
    } finally {
      setPending(false);
    }
  };

  const onVerify = async () => {
    setError("");
    setPending(true);
    try {
      const token = await ensureCsrf();
      const { redirectedTo } = await verifyLoginCode({
        csrfToken: token,
        email: email.trim().toLowerCase(),
        code: code.trim(),
        returnTo,
      });
      setStep(2);
      const target =
        redirectedTo && redirectedTo.startsWith("/")
          ? redirectedTo
          : returnTo;
      window.setTimeout(() => window.location.assign(target), 600);
    } catch (e) {
      setError(
        e instanceof AccountCenterError ? e.message : "验证失败，请重试"
      );
      setPending(false);
    }
  };

  return (
    <AuthShell code="ACC-02" title="找回 / 重新验证">
      <p className="mt-3 font-mono text-xs leading-6 tracking-wider text-ink/55">
        本站使用邮箱验证码登录，无独立密码。通过验证码即可重新进入账号。
      </p>

      <ol className="mt-6 flex gap-2">
        {STEPS.map((label, i) => (
          <li
            key={label}
            className={
              i === step
                ? "font-mono text-[10px] tracking-widest text-accent"
                : i < step
                  ? "font-mono text-[10px] tracking-widest text-ink/50"
                  : "font-mono text-[10px] tracking-widest text-ink/25"
            }
          >
            {String(i + 1).padStart(2, "0")} {label}
            {i < STEPS.length - 1 ? <span className="mx-2 text-ink/20">/</span> : null}
          </li>
        ))}
      </ol>

      <div className="mt-8 space-y-5">
        {step === 0 && (
          <>
            <div>
              <Label htmlFor="recover-email">学校邮箱</Label>
              <Input
                id="recover-email"
                type="email"
                placeholder="name@henu.edu.cn"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            {error && (
              <p className="font-mono text-[10px] text-accent">{error}</p>
            )}
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
              已发送至 <span className="text-ink">{email}</span>
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
                  className="tracking-[0.4em]"
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
            {error && (
              <p className="font-mono text-[10px] text-accent">{error}</p>
            )}
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
            <p className="font-mono text-sm text-ink">验证成功，正在进入账号…</p>
            <p className="font-mono text-[10px] tracking-wider text-ink/40">
              若未自动跳转，请点击下方按钮。
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

      <Link
        href="/account/login"
        className="mt-8 inline-block font-mono text-[10px] tracking-widest text-ink/40 hover:text-accent"
      >
        ← 返回登录
      </Link>
    </AuthShell>
  );
}
