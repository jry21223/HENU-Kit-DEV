import type { Metadata } from "next";
import CampusNav from "@/components/campus/campus-nav";
import SiteFooter from "@/components/site-footer";

export const metadata: Metadata = {
  title: "互助平台 — henukit",
};

export default function CampusLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col bg-paper text-ink">
      <CampusNav />
      <div className="flex-1">{children}</div>
      <SiteFooter />
    </div>
  );
}
