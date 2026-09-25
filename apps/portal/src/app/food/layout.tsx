import type { Metadata } from "next";
import FoodNav from "@/components/food/food-nav";
import SiteFooter from "@/components/site-footer";

export const metadata: Metadata = {
  title: "美食榜 — henukit",
};

export default function FoodLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col bg-paper text-ink">
      <FoodNav />
      <div className="flex-1">{children}</div>
      <SiteFooter />
    </div>
  );
}
