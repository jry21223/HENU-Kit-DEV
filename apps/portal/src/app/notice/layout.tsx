import type { Metadata } from "next";
import NoticeNav from "@/components/notice/notice-nav";

export const metadata: Metadata = {
  title: "通知 — henukit",
};

export default function NoticeLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-svh bg-paper text-ink">
      <NoticeNav />
      {children}
    </div>
  );
}
