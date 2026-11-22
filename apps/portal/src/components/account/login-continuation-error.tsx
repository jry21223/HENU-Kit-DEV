"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

export default function LoginContinuationError({
  serviceUnavailable, unsupportedApplication, canRetryContinuation, displayedRequestID, onRetry,
}: {
  serviceUnavailable: boolean;
  unsupportedApplication: boolean;
  canRetryContinuation: boolean;
  displayedRequestID?: string;
  onRetry: () => void;
}) {
  const router = useRouter();
  return (
    <main className="bg-blueprint flex min-h-svh items-center justify-center px-4 py-10 sm:px-5 sm:py-16">
      <div data-enter className="w-full max-w-md border border-ink bg-paper p-5 sm:p-8 md:p-10">
        <p className="font-mono text-xs text-ink/60">
          账号中心
        </p>
        <h1 className="mt-4 font-display text-3xl font-bold tracking-tight">
          {serviceUnavailable
            ? "登录暂时不可用"
            : unsupportedApplication
              ? "此应用暂不支持统一登录"
              : "登录链接已过期或不可继续"}
        </h1>
        <p className="mt-3 text-sm leading-6 text-ink/65">
          {serviceUnavailable
            ? canRetryContinuation
              ? "暂时无法验证这次登录，请稍后重试。"
              : "这次登录暂时无法继续。请重新开始登录；如仍失败，请稍后再试。"
            : unsupportedApplication
              ? "请返回原应用；如需继续使用，请联系该应用的维护者。"
              : "这次登录无法继续。请重新开始登录，我们会为你创建一条新的安全链接。"}
        </p>
        {displayedRequestID ? (
          <p className="mt-4 font-mono text-xs text-ink/60">
            请求编号：{displayedRequestID}
          </p>
        ) : null}
        {unsupportedApplication ? (
          <Button
            type="button"
            className="mt-7 w-full"
            onClick={() => {
              if (window.history.length > 1) window.history.back();
              else router.push("/");
            }}
          >
            返回上一步
          </Button>
        ) : canRetryContinuation ? (
          <Button
            type="button"
            className="mt-7 w-full"
            onClick={onRetry}
          >
            重新尝试
          </Button>
        ) : (
          <Button asChild className="mt-7 w-full">
            <a href="/api/v1/auth/login?return_to=%2Faccount">重新开始登录</a>
          </Button>
        )}
        <Link
          href="/"
          className="mt-1 flex min-h-11 items-center justify-center font-mono text-xs text-ink/60 hover:text-accent-text"
        >
          返回 HENU Kit 首页
        </Link>
      </div>
    </main>
  );
}
