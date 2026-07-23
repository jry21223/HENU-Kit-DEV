import type { Metadata } from "next";
import CampusNav from "@/components/campus/campus-nav";

export const metadata: Metadata = {
  title: "互助平台 — henukit",
};

export default function CampusLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-svh bg-paper text-ink">
      <CampusNav />
      {children}
    </div>
  );
}
