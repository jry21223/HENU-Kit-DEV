import type { Metadata } from "next";
import CampusNav from "@/components/campus/campus-nav";
import SiteShell from "@/components/site-shell";

export const metadata: Metadata = {
  title: "互助平台 — henukit",
};

export default function CampusLayout({ children }: { children: React.ReactNode }) {
  return (
    <SiteShell header={<CampusNav />}>{children}</SiteShell>
  );
}
