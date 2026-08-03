"use client";

import Link from "next/link";
import { useReveal } from "@/components/account/use-reveal";

export default function ShelfPage() {
  useReveal();

  return (
    <main data-library-shelf-state="unavailable" className="mx-auto max-w-2xl px-5 py-10 md:px-8">
      <p data-enter className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">L-02</span>
        <span className="mx-2">/</span>
        MY SHELF
      </p>
      <h1 data-enter className="mt-3 font-display text-4xl font-bold tracking-tight">我的书架</h1>
      <p data-enter className="mt-6 border border-dashed border-ink/30 px-5 py-12 text-center text-sm leading-7 text-ink/65">
        书架功能即将上线，敬请期待。
      </p>
      <Link
        href="/library"
        className="mt-6 inline-flex border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
      >
        返回书库
      </Link>
    </main>
  );
}
