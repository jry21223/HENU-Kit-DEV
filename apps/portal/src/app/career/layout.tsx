import type { Metadata } from "next";
import CareerNav from "@/components/career/career-nav";

export const metadata: Metadata = {
  title: "求职雷达 — henukit",
};

export default function CareerLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-svh bg-paper text-ink">
      <CareerNav />
      {children}
    </div>
  );
}
