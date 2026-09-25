import type { Metadata } from "next";
import CareerNav from "@/components/career/career-nav";
import SiteShell from "@/components/site-shell";
import { moduleLayoutTitle } from "@/lib/seo";

export const metadata: Metadata = {
  title: moduleLayoutTitle("career"),
};

export default function CareerLayout({ children }: { children: React.ReactNode }) {
  return (
    <SiteShell data-career-layout className="bg-blueprint" header={<CareerNav />}>
      {children}
    </SiteShell>
  );
}
