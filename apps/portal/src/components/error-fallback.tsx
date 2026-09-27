"use client";

import Link from "next/link";
import FallbackPage from "@/components/fallback-page";
import { Button, buttonVariants } from "@/components/ui/button";

/**
 * error.tsx 与 global-error.tsx 共用的出错兜底，只说明出错了和能做什么。
 * 错误原文、digest 与堆栈可能带内部细节，一律不上屏。
 */
export default function ErrorFallback({ retry }: { retry: () => void }) {
  return (
    <FallbackPage
      code="ERR"
      label="SOMETHING WENT WRONG"
      title="页面出错了"
      description="这个页面没能正常显示。可以点「重试」再加载一次；如果还是不行，先回首页，稍后再来。"
    >
      <div className="mt-8 flex flex-wrap gap-3">
        <Button type="button" onClick={() => retry()}>
          重试
        </Button>
        <Link href="/" className={buttonVariants({ variant: "outline" })}>
          回首页
        </Link>
      </div>
    </FallbackPage>
  );
}
