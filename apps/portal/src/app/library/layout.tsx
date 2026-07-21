import type { Metadata } from "next";
import LibraryNav from "@/components/library/library-nav";

export const metadata: Metadata = {
  title: "资料库 — henukit",
};

export default function LibraryLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-svh bg-paper text-ink">
      <LibraryNav />
      {children}
    </div>
  );
}
