import Link from "next/link";

/** 详情/阅读页共享的加载占位。 */
export function LibraryLoading() {
  return (
    <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/40">LOADING / 加载中</p>
      <Link href="/library" className="mt-6 inline-block font-mono text-sm text-accent hover:underline">
        ← 返回书库
      </Link>
    </main>
  );
}

/**
 * 详情/阅读页共享的 404 页。
 * 静态文案即「内容不存在或已下架」；error 仅承载额外诊断信息（如网络错误），不与 404 文案叠加。
 */
export function LibraryNotFound({ error }: { error?: string | null }) {
  return (
    <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/40">404 / NOT FOUND</p>
      <p className="mt-4 text-sm text-ink/60">
        内容不存在或已下架{error ? `（${error}）` : ""}。
      </p>
      <Link href="/library" className="mt-6 inline-block font-mono text-sm text-accent hover:underline">
        ← 返回书库
      </Link>
    </main>
  );
}
