import type { Metadata } from "next";
import PracticeNav from "@/components/practice/practice-nav";
import SiteShell from "@/components/site-shell";
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
    <SiteShell header={<PracticeNav />}>
      <TransitionProvider>{children}</TransitionProvider>
    </SiteShell>
  );
}
