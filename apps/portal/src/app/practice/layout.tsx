import type { Metadata } from "next";
import PracticeNav from "@/components/practice/practice-nav";
import SiteFooter from "@/components/site-footer";
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
    <div className="flex min-h-svh flex-col bg-paper text-ink">
      <PracticeNav />
      <div className="flex-1">
        <TransitionProvider>{children}</TransitionProvider>
      </div>
      <SiteFooter />
    </div>
  );
}
