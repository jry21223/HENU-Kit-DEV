import type { Metadata } from "next";
import CareerNav from "@/components/career/career-nav";

export const metadata: Metadata = {
  title: "求职雷达 — henukit",
};

export default function CareerLayout({ children }: { children: React.ReactNode }) {
  return (
    <div data-career-layout className="min-h-svh bg-blueprint bg-paper text-ink">
      <CareerNav />
      {children}
    </div>
  );
}
