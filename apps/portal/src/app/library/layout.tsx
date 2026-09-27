import type { Metadata } from "next";
import LibraryNav from "@/components/library/library-nav";
import SiteShell from "@/components/site-shell";
import { moduleLayoutTitle } from "@/lib/seo";

export const metadata: Metadata = {
  title: moduleLayoutTitle("library"),
};

export default function LibraryLayout({ children }: { children: React.ReactNode }) {
  return (
    <SiteShell header={<LibraryNav />}>{children}</SiteShell>
  );
}
