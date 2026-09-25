import type { Metadata } from "next";
import FoodNav from "@/components/food/food-nav";
import SiteShell from "@/components/site-shell";

export const metadata: Metadata = {
  title: "美食榜 — henukit",
};

export default function FoodLayout({ children }: { children: React.ReactNode }) {
  return (
    <SiteShell header={<FoodNav />}>{children}</SiteShell>
  );
}
