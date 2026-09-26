import { ErrorBanner } from "@/components/data-state";
import BackLink from "@/components/back-link";

/** 资料详情的加载占位。 */
export function LibraryLoading() {
  return (
    <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
      <p className="font-mono text-xs leading-6 text-ink/70">加载中…</p>
      <BackLink className="mt-6 inline-block font-mono text-sm text-accent-text hover:underline" />
    </main>
  );
}

/**
 * 资料详情的 404 页。
 * 静态文案即「内容不存在或已下架」；error 仅承载额外诊断信息（如网络错误），不与 404 文案叠加。
 */
export function LibraryNotFound({ error }: { error?: string | null }) {
  return (
    <main className="mx-auto max-w-3xl px-5 py-24 text-center md:px-8">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/60">404 / NOT FOUND</p>
      <p className="mt-4 text-sm text-ink/60">
        内容不存在或已下架{error ? `（${error}）` : ""}。
      </p>
      <BackLink className="mt-6 inline-block font-mono text-sm text-accent-text hover:underline" />
    </main>
  );
}

/** Owner 暂时不可用时保留真实失败语义，并提供原地重试。 */
export function LibraryUnavailable({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <main className="mx-auto max-w-3xl px-5 py-24 md:px-8">
      <ErrorBanner message={message} onRetry={onRetry} />
      <BackLink className="mt-6 inline-block font-mono text-sm text-accent-text hover:underline" />
    </main>
  );
}
