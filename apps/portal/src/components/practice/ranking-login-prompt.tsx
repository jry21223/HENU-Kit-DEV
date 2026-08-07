"use client";

import Link from "next/link";

/**
 * 排行身份登录引导块。排行身份写接口只接受已登录用户
 * （Portal Session 缺失时 Gateway 返回 401），页面用明确的登录入口
 * 代替静默失败。与 #266 收藏夹的 favorites-login-prompt.tsx 同构；
 * 两个 PR 合并后由后续任务统一为一个共享组件。
 */
export default function RankingLoginPrompt({ next }: { next: string }) {
  return (
    <div className="mt-8 border border-ink/25 p-8">
      <p className="font-display text-xl font-bold">设置排行身份需要登录</p>
      <p className="mt-3 text-sm leading-7 text-ink/65">
        登录后即可设置昵称、选择系统头像，或退出排行榜。未登录的作答不会进入公开排行。
      </p>
      <Link
        href={`/account/login?next=${encodeURIComponent(next)}`}
        className="mt-6 inline-flex border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
      >
        去登录 →
      </Link>
    </div>
  );
}
