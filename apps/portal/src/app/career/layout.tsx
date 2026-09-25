import type { Metadata } from "next";
import CareerNav from "@/components/career/career-nav";
import SiteFooter from "@/components/site-footer";

export const metadata: Metadata = {
  title: "求职雷达 — henukit",
};

export default function CareerLayout({ children }: { children: React.ReactNode }) {
  return (
    <div data-career-layout className="flex min-h-svh flex-col bg-blueprint bg-paper text-ink">
      <CareerNav />
      <div className="flex-1">{children}</div>
      <SiteFooter />
    </div>
  );
}
