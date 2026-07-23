import Link from "next/link";

/**
 * 子路由占位页：同一套工业极简视觉语言。
 */
export default function Placeholder({
  index,
  en,
  title,
}: {
  index: string;
  en: string;
  title: string;
}) {
  return (
    <main className="snap-screen bg-blueprint relative flex min-h-svh flex-col items-center justify-center bg-paper px-6">
      <span aria-hidden className="absolute left-6 top-6 font-mono text-lg text-ink/40">+</span>
      <span aria-hidden className="absolute right-6 top-6 font-mono text-lg text-ink/40">+</span>
      <span aria-hidden className="absolute bottom-6 left-6 font-mono text-lg text-accent">+</span>
      <span aria-hidden className="absolute bottom-6 right-6 font-mono text-lg text-ink/40">+</span>

      <p className="font-mono text-xs tracking-[0.3em] text-ink/60 uppercase">
        <span className="text-accent">{index}</span>
        <span className="mx-2">/</span>
        {en}
      </p>
      <h1 className="mt-4 font-display text-5xl font-bold tracking-tight md:text-7xl">
        {title}
      </h1>
      <p className="mt-6 border border-ink/25 px-4 py-2 font-mono text-xs tracking-[0.3em] text-ink/60">
        模块建设中 / UNDER CONSTRUCTION
      </p>
      <Link
        href="/"
        className="group mt-10 inline-flex items-center gap-2 font-mono text-sm tracking-widest text-ink transition-colors hover:text-accent"
      >
        <span aria-hidden className="transition-transform group-hover:-translate-x-1">←</span>
        返回首页
      </Link>
    </main>
  );
}
