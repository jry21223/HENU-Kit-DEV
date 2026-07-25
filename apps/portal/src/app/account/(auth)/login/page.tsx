"use client";

/**
 * Portal login — restored dual-mode UI (password + email code) from the
 * original HENUKIT account page, with real Platform Core mail channel for codes.
 *
 * - 密码登录 / 注册：仍走本地 authStore（后端尚无密码登录 API）
 * - 验证码登录 / 注册发码：走 /account-auth → Platform Core → mail outbox
 */

import Link from "next/link";
import { Suspense, useEffect, useState } from "react";
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
  portalOAuthStartUrl,
  requestLoginCode,
  verifyLoginCode,
} from "@/lib/auth/account-center";
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
  const nextRaw = params.get("next") || "/account";
  const nextPath = nextRaw.startsWith("/") ? nextRaw : "/account";

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
  const [oauthReturnTo, setOauthReturnTo] = useState(() =>
    portalOAuthStartUrl(nextPath)
  );

  const fullEmail = toHenuEmail(localPart);
  const needCode = tab === "register" || mode === "code";

  useEffect(() => {
    if (ready && user) router.replace(nextPath);
  }, [ready, user, nextPath, router]);

  useEffect(() => {
    if (cd <= 0) return;
    const t = window.setTimeout(() => setCd((c) => c - 1), 1000);
    return () => window.clearTimeout(t);
  }, [cd]);

  // Warm CSRF when email-code path may be used
  useEffect(() => {
    if (!needCode) return;
    let cancelled = false;
    const returnTo = portalOAuthStartUrl(nextPath);
    setOauthReturnTo(returnTo);
    bootstrapAccountLogin(returnTo)
      .then((b) => {
        if (!cancelled) setCsrf(b.csrfToken);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [needCode, nextPath]);

  const ensureCsrf = async () => {
    if (csrf) return csrf;
    const b = await bootstrapAccountLogin(oauthReturnTo);
    setCsrf(b.csrfToken);
    return b.csrfToken;
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
      const result = await requestLoginCode({
        csrfToken: token,
        email: fullEmail,
        returnTo: oauthReturnTo,
      });
      setCsrf(result.csrfToken);
      setCd(60);
      setInfo(`验证码已进入发送队列（${fullEmail}），请查收学校邮箱。`);
    } catch (e) {
      setErrors({
        email:
          e instanceof AccountCenterError
            ? e.message
            : "发送失败，请稍后重试（需配置邮件通道）",
      });
    } finally {
      setPending(false);
    }
  };

  const submit = async () => {
    const errs: Record<string, string> = {};
    setInfo("");

    if (tab === "login" && mode === "password" && !name.trim()) {
      errs.name = "请输入账号名";
    }
    if (needCode && !isValidHenuLocalPart(localPart)) {
      errs.email = "请输入邮箱前缀（自动补全 @henu.edu.cn）";
    }
    if (needCode && !/^\d{6}$/.test(code.trim())) {
      errs.code = "请输入 6 位数字验证码";
    }
    // 密码仅用于“密码登录预览”；注册/验证码登录不要求密码后端。
    if (tab === "login" && mode === "password" && pwd.length < 6) {
      errs.pwd = "密码至少 6 位（预览校验）";
    }
    if (tab === "register" && !name.trim()) {
      errs.name = "请输入账号名（展示用）";
    }
    setErrors(errs);
    if (Object.keys(errs).length) return;

    setPending(true);

    // —— 验证码路径：真实 Platform Core 发信 + 校验 + Portal OAuth ——
    if (needCode) {
      try {
        const token = await ensureCsrf();
        await verifyLoginCode({
          csrfToken: token,
          email: fullEmail,
          code: code.trim(),
          // Platform Core only preserves OAuth authorize return_to; portal login
          // path is rewritten to "/". Always continue via Portal Gateway OAuth.
          returnTo: oauthReturnTo,
        });
        // Optional: keep local display name for account shell
        try {
          authStore.register(name.trim() || localPart, fullEmail);
        } catch {
          /* store may be gateway-only */
        }
        // Establish Portal session (PKCE) after core session cookie is set.
        window.location.assign(oauthReturnTo);
        return;
      } catch (e) {
        setErrors({
          code:
            e instanceof AccountCenterError
              ? e.message
              : "验证码无效或邮件通道未就绪",
        });
        setPending(false);
        return;
      }
    }

    // —— 密码登录路径：UI 保留；后端尚无密码 API，仅本地会话（预览）——
    // 注册 / 验证码模式已在 needCode 分支返回；此处仅可能是 login+password。
    window.setTimeout(() => {
      authStore.login(name.trim());
      router.push(nextPath);
    }, 400);
  };

  return (
    <main className="bg-blueprint flex min-h-svh items-center justify-center px-5 py-16">
      <div
        data-enter
        className="w-full max-w-md border border-ink bg-paper p-8 md:p-10"
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
          注册/登录均用邮箱验证码（真实通道）。密码登录仅为界面预览，后端尚未接入。
        </p>

        {/* 登录 / 注册 */}
        <div className="mt-6 flex border border-line">
          {(["login", "register"] as const).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => {
                setTab(t);
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
          {tab === "login" && mode === "password" && (
            <Field
              id="login-name"
              label="账号 / NAME"
              value={name}
              onChange={setName}
              error={errors.name}
              placeholder="学号或昵称"
              autoComplete="username"
            />
          )}

          {needCode && (
            <HenuEmailField
              id="auth-email"
              value={localPart}
              onChange={setLocalPart}
            />
          )}
          {errors.email ? (
            <p className="-mt-3 font-mono text-[10px] text-accent">
              {errors.email}
            </p>
          ) : null}

          {needCode && (
            <div>
              <Label htmlFor="auth-code">邮箱验证码</Label>
              <div className="flex items-end gap-3">
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
                  className="shrink-0"
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

          {tab === "login" && mode === "password" && (
            <Field
              id="auth-pwd"
              label="密码 / PASSWORD（仅预览）"
              type="password"
              value={pwd}
              onChange={setPwd}
              error={errors.pwd}
              placeholder="至少 6 位（后端暂未接密码）"
              autoComplete="current-password"
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

        <div className="mt-4 flex justify-between font-mono text-[10px] tracking-wider text-ink/50">
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
