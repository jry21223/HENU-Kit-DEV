import type { Metadata } from "next";
import PracticeNav from "@/components/practice/practice-nav";
import TransitionProvider from "@/components/practice/transition/transition-provider";

export const metadata: Metadata = {
  title: "刷题 — henukit",
};

export default function PracticeLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="min-h-svh bg-paper text-ink">
      <PracticeNav />
      <TransitionProvider>{children}</TransitionProvider>
    </div>
  );
}
