import type { Metadata } from "next";
import FoodNav from "@/components/food/food-nav";

export const metadata: Metadata = {
  title: "美食榜 — henukit",
};

export default function FoodLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-svh bg-paper text-ink">
      <FoodNav />
      {children}
    </div>
  );
}
