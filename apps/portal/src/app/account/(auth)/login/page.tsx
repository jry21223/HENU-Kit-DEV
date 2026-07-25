"use client";

import Link from "next/link";
import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useSyncExternalStore } from "react";
import { AuthShell } from "@/components/account/auth-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  AccountCenterError,
  bootstrapAccountLogin,
  portalOAuthStartUrl,
  requestLoginCode,
  verifyLoginCode,
} from "@/lib/auth/account-center";
import { authStore } from "@/lib/auth/store";
import { cn } from "@/lib/cn";

const EMAIL_RE = /^[^\s@]+@henu\.edu\.cn$/i;

function FieldError({ message }: { message?: string }) {
  if (!message) return null;
  return <p className="mt-1 font-mono text-[10px] text-accent">{message}</p>;
}

function EmailCodeAuth({
  mode,
  nextPath,
}: {
  mode: "login" | "register";
  nextPath: string;
}) {
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [step, setStep] = useState<"email" | "code">("email");
  const [csrf, setCsrf] = useState("");
  const [oauthReturnTo, setOauthReturnTo] = useState(() =>
    portalOAuthStartUrl(nextPath)
  );
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const [cd, setCd] = useState(0);

  useEffect(() => {
    if (cd <= 0) return;
    const t = window.setTimeout(() => setCd((c) => c - 1), 1000);
    return () => window.clearTimeout(t);
  }, [cd]);

  useEffect(() => {
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
  }, [nextPath]);

  const ensureCsrf = async () => {
    if (csrf) return csrf;
    const b = await bootstrapAccountLogin(oauthReturnTo);
    setCsrf(b.csrfToken);
    return b.csrfToken;
  };

  const onSend = async () => {
    setError("");
    setInfo("");
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
        returnTo: oauthReturnTo,
      });
      setCsrf(result.csrfToken);
      setStep("code");
      setCd(60);
      setInfo(
        mode === "register"
          ? "验证码已发送。首次验证将自动创建账号。"
          : "验证码已进入发送队列，请查收学校邮箱。"
      );
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
    setInfo("");
    if (!/^\d{6}$/.test(code.trim())) {
      setError("请输入 6 位数字验证码");
      return;
    }
    setPending(true);
    try {
      const token = await ensureCsrf();
      const { redirectedTo } = await verifyLoginCode({
        csrfToken: token,
        email: email.trim().toLowerCase(),
        code: code.trim(),
        returnTo: oauthReturnTo,
      });
      const target =
        redirectedTo && redirectedTo.startsWith("/")
          ? redirectedTo
          : oauthReturnTo;
      window.location.assign(target);
    } catch (e) {
      setError(
        e instanceof AccountCenterError ? e.message : "验证失败，请重试"
      );
      setPending(false);
    }
  };

  return (
    <div className="space-y-5">
      <p className="font-mono text-xs leading-6 tracking-wider text-ink/55">
        {mode === "register"
          ? "使用河南大学邮箱验证码完成注册并登录。无密码，首次验证即开通账号。"
          : "使用河南大学邮箱验证码登录。无密码，验证码单次有效。"}
      </p>

      <div>
        <Label htmlFor="auth-email">学校邮箱</Label>
        <Input
          id="auth-email"
          type="email"
          autoComplete="email"
          placeholder="name@henu.edu.cn"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
      </div>

      {step === "code" && (
        <div>
          <Label htmlFor="auth-code">6 位验证码</Label>
          <div className="flex items-end gap-3">
            <Input
              id="auth-code"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              placeholder="••••••"
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
              className="shrink-0"
            >
              {cd > 0 ? `${cd}s` : "重发"}
            </Button>
          </div>
        </div>
      )}

      {info ? (
        <p className="font-mono text-[11px] leading-5 tracking-wider text-ink/50">
          {info}
        </p>
      ) : null}
      <FieldError message={error} />

      {step === "email" ? (
        <Button
          type="button"
          className="w-full"
          disabled={pending}
          onClick={() => void onSend()}
        >
          {pending ? "发送中…" : "发送验证码"}
        </Button>
      ) : (
        <Button
          type="button"
          className="w-full"
          disabled={pending}
          onClick={() => void onVerify()}
        >
          {pending
            ? "验证中…"
            : mode === "register"
              ? "验证并注册"
              : "登录并继续"}
        </Button>
      )}

      <p className="font-mono text-[10px] leading-5 tracking-wider text-ink/40">
        仅允许 <span className="text-ink/60">henu.edu.cn</span> 邮箱 · 会话 15
        天 · 学生自主运营 · 非河南大学官方项目
      </p>
    </div>
  );
}

function LoginRegisterForm() {
  const router = useRouter();
  const params = useSearchParams();
  const nextRaw = params.get("next") || "/account";
  const nextPath = nextRaw.startsWith("/") ? nextRaw : "/account";
  const initialTab = params.get("tab") === "register" ? "register" : "login";

  const { user, ready } = useSyncExternalStore(
    authStore.subscribe,
    authStore.get,
    authStore.getServer
  );

  useEffect(() => {
    if (ready && user) router.replace(nextPath);
  }, [ready, user, nextPath, router]);

  return (
    <AuthShell code="ACC-01" title="统一登录">
      <Tabs defaultValue={initialTab} className="mt-6">
        <TabsList className="w-full">
          <TabsTrigger value="login" className="flex-1">
            登录
          </TabsTrigger>
          <TabsTrigger value="register" className="flex-1">
            注册
          </TabsTrigger>
        </TabsList>
        <TabsContent value="login">
          <EmailCodeAuth mode="login" nextPath={nextPath} />
        </TabsContent>
        <TabsContent value="register">
          <EmailCodeAuth mode="register" nextPath={nextPath} />
        </TabsContent>
      </Tabs>

      <div className="mt-8 flex items-center justify-between border-t border-line pt-4">
        <Link
          href="/account/recover"
          className={cn(
            "font-mono text-[10px] tracking-widest text-ink/40 hover:text-accent"
          )}
        >
          无法收到验证码？
        </Link>
        <Link
          href="/"
          className="font-mono text-[10px] tracking-widest text-ink/40 hover:text-accent"
        >
          回首页
        </Link>
      </div>
    </AuthShell>
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
      <LoginRegisterForm />
    </Suspense>
  );
}
