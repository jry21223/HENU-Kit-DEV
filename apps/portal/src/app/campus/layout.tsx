import type { Metadata } from "next";
import CampusNav from "@/components/campus/campus-nav";
import SiteShell from "@/components/site-shell";
import { moduleLayoutTitle } from "@/lib/seo";

export const metadata: Metadata = {
  title: moduleLayoutTitle("campus"),
};

export default function CampusLayout({ children }: { children: React.ReactNode }) {
  return (
    <SiteShell header={<CampusNav />}>{children}</SiteShell>
  );
}
