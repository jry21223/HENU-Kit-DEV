"use client";

/**
 * Portal login — restored dual-mode UI (password + email code) from the
 * original HENUKIT account page, with real Platform Core mail channel for codes.
 *
 * Existing Portal UI; every production credential flow is owned by Platform
 * Core through the same-origin /account-auth route.
 */

import Link from "next/link";
import { Suspense, useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useSyncExternalStore } from "react";
import { HenuEmailField } from "@/components/account/henu-email-field";
import { useReveal } from "@/components/account/use-reveal";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  AccountCenterError,
  bootstrapAccountLogin,
  bootstrapAccountRegister,
  passwordLogin,
  portalOAuthStartUrl,
  registerAccount,
  requestLoginCode,
  requestRegistrationCode,
  verifyLoginCode,
} from "@/lib/auth/account-center";
import { fetchSession, hasGateway } from "@/lib/api/client";
import {
  isValidHenuLocalPart,
  toHenuEmail,
} from "@/lib/auth/henu-email";
import { authStore } from "@/lib/auth/store";
import { cn } from "@/lib/cn";

function Field({
  label,
  id,
  type = "text",
  value,
  onChange,
  error,
  placeholder,
  autoComplete,
}: {
  label: string;
  id: string;
  type?: string;
  value: string;
  onChange: (v: string) => void;
  error?: string;
  placeholder?: string;
  autoComplete?: string;
}) {
  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type={type}
        value={value}
        autoComplete={autoComplete}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={error ? "border-accent focus:border-accent" : undefined}
      />
      {error ? (
        <p className="mt-1 font-mono text-[10px] text-accent">{error}</p>
      ) : null}
    </div>
  );
}

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const requestedNext = params.get("next");

  const { user, ready } = useSyncExternalStore(
    authStore.subscribe,
    authStore.get,
    authStore.getServer
  );
  useReveal();

  const [tab, setTab] = useState<"login" | "register">("login");
  const [mode, setMode] = useState<"password" | "code">("code");
  const [name, setName] = useState("");
  const [localPart, setLocalPart] = useState("");
  const [code, setCode] = useState("");
  const [pwd, setPwd] = useState("");
  const [pwd2, setPwd2] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [info, setInfo] = useState("");
  const [cd, setCd] = useState(0);
  const [csrf, setCsrf] = useState("");
  const csrfBootstrap = useRef<Promise<string> | null>(null);
  const defaultNext = tab === "register" ? "/account/security" : "/account";
  const nextPath =
    requestedNext?.startsWith("/") ? requestedNext : defaultNext;
  const oauthReturnTo = portalOAuthStartUrl(nextPath);

  const fullEmail = toHenuEmail(localPart);
  const needCode = tab === "register" || mode === "code";
  const passwordLength = Array.from(pwd).length;

  useEffect(() => {
    if (!ready || !user) return;
    // The cached user can be stale: signed out, session expired, or a fresh
    // module copy re-initialised from a server session. Only bounce to the
    // account console when the Gateway still reports this session; otherwise
    // drop the stale cache and stay put (the #412 loop).
    if (!hasGateway) {
      router.replace(nextPath);
      return;
    }
    let cancelled = false;
    void fetchSession().then(
      (session) => {
        if (cancelled) return;
        if (session) {
          router.replace(nextPath);
        } else {
          authStore.clear();
        }
      },
      () => {
        // Gateway unreachable: keep the login page as-is.
      }
    );
    return () => {
      cancelled = true;
    };
  }, [ready, user, nextPath, router]);

  useEffect(() => {
    if (cd <= 0) return;
    const t = window.setTimeout(() => setCd((c) => c - 1), 1000);
    return () => window.clearTimeout(t);
  }, [cd]);

  const ensureCsrf = async () => {
    if (csrf) return csrf;
    if (!csrfBootstrap.current) {
      csrfBootstrap.current = (
        tab === "register"
          ? bootstrapAccountRegister(oauthReturnTo)
          : bootstrapAccountLogin(oauthReturnTo)
      ).then((result) => result.csrfToken);
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
    setErrors({});
    setInfo("");
    if (!isValidHenuLocalPart(localPart)) {
      setErrors({ email: "请输入邮箱前缀（自动补全 @henu.edu.cn）" });
      return;
    }
    setPending(true);
    try {
      const token = await ensureCsrf();
      const result =
        tab === "register"
          ? await requestRegistrationCode({
              csrfToken: token,
              email: fullEmail,
              returnTo: oauthReturnTo,
            })
          : await requestLoginCode({
              csrfToken: token,
              email: fullEmail,
              returnTo: oauthReturnTo,
            });
      setCsrf(result.csrfToken);
      setCd(60);
      setInfo(`验证码已进入发送队列（${fullEmail}），请查收学校邮箱。`);
    } catch (e) {
      if (e instanceof AccountCenterError && e.code === "CSRF") {
        setCsrf("");
      }
      setErrors({
        email:
          e instanceof AccountCenterError
            ? e.message
            : "验证码暂时发不出去，请稍后再试；仍不行请联系管理员",
      });
    } finally {
      setPending(false);
    }
  };

  const submit = async () => {
    const errs: Record<string, string> = {};
    setInfo("");

    if (!isValidHenuLocalPart(localPart)) {
      errs.email = "请输入邮箱前缀（自动补全 @henu.edu.cn）";
    }
    if (needCode && !/^\d{6}$/.test(code.trim())) {
      errs.code = "请输入 6 位数字验证码";
    }
    if ((tab === "register" || mode === "password") && passwordLength < 10) {
      errs.pwd = "密码至少 10 个字符";
    }
    if (tab === "register" && !name.trim()) {
      errs.name = "请输入展示名";
    }
    if (tab === "register" && pwd2 !== pwd) {
      errs.pwd2 = "两次输入的密码不一致";
    }
    setErrors(errs);
    if (Object.keys(errs).length) return;

    setPending(true);

    try {
      const token = await ensureCsrf();
      if (tab === "register") {
        await registerAccount({
          csrfToken: token,
          displayName: name.trim(),
          email: fullEmail,
          code: code.trim(),
          password: pwd,
          returnTo: oauthReturnTo,
        });
      } else if (mode === "code") {
        await verifyLoginCode({
          csrfToken: token,
          email: fullEmail,
          code: code.trim(),
          returnTo: oauthReturnTo,
        });
      } else {
        await passwordLogin({
          csrfToken: token,
          email: fullEmail,
          password: pwd,
          returnTo: oauthReturnTo,
        });
      }
      window.location.assign(oauthReturnTo);
    } catch (e) {
      if (e instanceof AccountCenterError && e.code === "CSRF") {
        setCsrf("");
      }
      const message =
        e instanceof AccountCenterError
          ? e.message
          : "认证暂不可用，请稍后重试";
      setErrors(
        tab === "login" && mode === "password"
          ? { pwd: message }
          : { code: message }
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
            <span className="text-accent">ACC-01</span>
            <span className="mx-2">/</span>
            AUTH
          </p>
          <Link
            href="/"
            className="font-mono text-[10px] tracking-widest text-ink/40 hover:text-accent"
          >
            ← henukit
          </Link>
        </div>
        <h1 className="mt-4 font-display text-4xl font-bold tracking-tight">
          {tab === "login" ? "登录" : "注册"}
        </h1>
        <p className="mt-2 font-mono text-[11px] leading-5 tracking-wider text-ink/50">
          首次注册需验证学校邮箱并设置密码；之后可用密码或验证码登录。
        </p>

        {/* 登录 / 注册 */}
        <div className="mt-6 flex border border-line">
          {(["login", "register"] as const).map((t) => (
            <button
              key={t}
              type="button"
              disabled={pending}
              onClick={() => {
                setTab(t);
                setCsrf("");
                if (t === "register") setMode("code");
                setErrors({});
                setInfo("");
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

        {/* 密码 / 验证码 */}
        {tab === "login" && (
          <div className="mt-4 flex gap-2">
            {(["password", "code"] as const).map((m) => (
              <button
                key={m}
                type="button"
                onClick={() => {
                  setMode(m);
                  setErrors({});
                  setInfo("");
                }}
                className={cn(
                  "border px-3 py-1.5 font-mono text-[11px] transition-colors",
                  mode === m
                    ? "border-ink bg-ink text-paper"
                    : "border-line text-ink/60 hover:border-ink/40"
                )}
              >
                {m === "password" ? "密码登录" : "验证码登录"}
              </button>
            ))}
          </div>
        )}

        <div className="mt-6 space-y-5">
          {tab === "register" && (
            <Field
              id="reg-name"
              label="账号 / NAME"
              value={name}
              onChange={setName}
              error={errors.name}
              placeholder="展示名（可选，可先填邮箱前缀）"
              autoComplete="username"
            />
          )}
          <HenuEmailField
            id="auth-email"
            value={localPart}
            onChange={setLocalPart}
          />
          {errors.email ? (
            <p className="-mt-3 font-mono text-[10px] text-accent">
              {errors.email}
            </p>
          ) : null}

          {needCode && (
            <div>
              <Label htmlFor="auth-code">邮箱验证码</Label>
              <div className="flex flex-col items-stretch gap-2 sm:flex-row sm:items-end sm:gap-3">
                <Input
                  id="auth-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  placeholder="6 位数字"
                  value={code}
                  onChange={(e) =>
                    setCode(e.target.value.replace(/\D/g, "").slice(0, 6))
                  }
                  className={cn(
                    "tracking-[0.35em]",
                    errors.code ? "border-accent focus:border-accent" : undefined
                  )}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={pending || cd > 0}
                  onClick={() => void sendCode()}
                  className="w-full shrink-0 sm:w-auto"
                >
                  {cd > 0 ? `${cd}s` : "发送验证码"}
                </Button>
              </div>
              {errors.code ? (
                <p className="mt-1 font-mono text-[10px] text-accent">
                  {errors.code}
                </p>
              ) : null}
              {info ? (
                <p className="mt-1 font-mono text-[10px] leading-5 text-ink/45">
                  {info}
                </p>
              ) : null}
            </div>
          )}

          {(tab === "register" || mode === "password") && (
            <Field
              id="auth-pwd"
              label="密码 / PASSWORD"
              type="password"
              value={pwd}
              onChange={setPwd}
              error={errors.pwd}
              placeholder="至少 10 个字符"
              autoComplete={
                tab === "register" ? "new-password" : "current-password"
              }
            />
          )}
          {tab === "register" && (
            <Field
              id="auth-pwd-confirm"
              label="确认密码 / CONFIRM"
              type="password"
              value={pwd2}
              onChange={setPwd2}
              error={errors.pwd2}
              autoComplete="new-password"
            />
          )}
        </div>

        <Button
          type="button"
          className="mt-8 w-full"
          disabled={pending}
          onClick={() => void submit()}
        >
          {pending ? "处理中…" : tab === "login" ? "登 录" : "注 册"}
        </Button>

        <div className="mt-4 flex flex-col gap-2 font-mono text-[10px] tracking-wider text-ink/50 sm:flex-row sm:items-center sm:justify-between">
          <Link href="/account/recover" className="hover:text-accent">
            忘记密码 / 收不到验证码 →
          </Link>
          <span className="text-ink/35">@henu.edu.cn 固定后缀</span>
        </div>
      </div>
    </main>
  );
}

export default function LoginPage() {
  return (
    <Suspense
      fallback={
        <main className="bg-blueprint flex min-h-svh items-center justify-center">
          <p className="font-mono text-xs tracking-widest text-ink/40">
            加载中…
          </p>
        </main>
      }
    >
      <LoginForm />
    </Suspense>
  );
}
