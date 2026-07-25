"use client";

import Link from "next/link";
import { useReveal } from "@/components/account/use-reveal";
import { cn } from "@/lib/cn";

/** Shared blueprint-framed shell for login / register / recover. */
export function AuthShell({
  code,
  title,
  children,
  className,
}: {
  code: string;
  title: string;
  children: React.ReactNode;
  className?: string;
}) {
  useReveal();

  return (
    <main className="bg-blueprint flex min-h-svh items-center justify-center px-5 py-16">
      <div
        data-enter
        className={cn(
          "w-full max-w-md border border-ink bg-paper p-8 md:p-10",
          className
        )}
      >
        <div className="flex items-baseline justify-between">
          <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
            <span className="text-accent">{code}</span>
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
          {title}
        </h1>
        {children}
      </div>
    </main>
  );
}
