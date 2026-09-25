import type { Metadata } from "next";
import LibraryNav from "@/components/library/library-nav";
import SiteShell from "@/components/site-shell";

export const metadata: Metadata = {
  title: "资料库 — henukit",
};

export default function LibraryLayout({ children }: { children: React.ReactNode }) {
  return (
    <SiteShell header={<LibraryNav />}>{children}</SiteShell>
  );
}
