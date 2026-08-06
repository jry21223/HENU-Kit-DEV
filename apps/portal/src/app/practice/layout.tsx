import type { Metadata } from "next";
import Link from "next/link";
import PracticeNav from "@/components/practice/practice-nav";
import TransitionProvider from "@/components/practice/transition/transition-provider";
import { quizCraftV2ReadsEnabled } from "@/lib/api/env";

export const metadata: Metadata = {
  title: "刷题 — henukit",
};

// The mistakes entrance is a V2 personal-data surface like leaderboard and
// ranking profile: it only appears once the #166 cutover flag is on.
const learningStateEntryVisible = quizCraftV2ReadsEnabled();

export default function PracticeLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="min-h-svh bg-paper text-ink">
      <PracticeNav />
      {learningStateEntryVisible && (
        <div className="border-b border-line bg-paper">
          <div className="mx-auto flex max-w-6xl items-center gap-5 px-5 py-2 md:px-8">
            <Link
              href="/practice/mistakes"
              className="group relative py-1 font-mono text-xs tracking-widest text-ink/50 transition-colors hover:text-ink"
            >
              <span className="mr-1 text-accent">P-06</span>
              错题
              <span
                aria-hidden
                className="absolute inset-x-0 -bottom-0.5 h-px origin-left scale-x-0 bg-accent transition-transform duration-300 group-hover:scale-x-100"
              />
            </Link>
          </div>
        </div>
      )}
      <TransitionProvider>{children}</TransitionProvider>
    </div>
  );
}
