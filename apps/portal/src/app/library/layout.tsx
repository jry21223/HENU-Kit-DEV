import type { Metadata } from "next";
import LibraryNav from "@/components/library/library-nav";
import SiteFooter from "@/components/site-footer";

export const metadata: Metadata = {
  title: "资料库 — henukit",
};

export default function LibraryLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col bg-paper text-ink">
      <LibraryNav />
      <div className="flex-1">{children}</div>
      <SiteFooter />
    </div>
  );
}
