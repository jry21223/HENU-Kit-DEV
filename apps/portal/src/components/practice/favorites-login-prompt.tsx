"use client";

import { redirectToLogin } from "@/lib/api/client";

/**
 * 收藏夹登录引导块。收藏读接口对未登录用户返回 401
 * （PortalUnauthorizedError），页面用明确的登录入口代替静默失败。
 */
export default function FavoritesLoginPrompt({ next }: { next: string }) {
  return (
    <div data-enter className="mt-8 border border-ink/25 p-8">
      <p className="font-display text-xl font-bold">收藏夹需要登录</p>
      <p className="mt-3 text-sm leading-7 text-ink/65">
        登录后即可在刷题时收藏题目，并在收藏夹里发起练习。收藏保存在你的账号下，未登录的作答不会写入收藏。
      </p>
      <button
        type="button"
        onClick={() => redirectToLogin(next)}
        className="mt-6 inline-flex border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
      >
        去登录 →
      </button>
    </div>
  );
}
