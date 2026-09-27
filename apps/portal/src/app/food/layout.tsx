import type { Metadata } from "next";
import FoodNav from "@/components/food/food-nav";
import SiteShell from "@/components/site-shell";
import { moduleLayoutTitle } from "@/lib/seo";

export const metadata: Metadata = {
  title: moduleLayoutTitle("food"),
};

export default function FoodLayout({ children }: { children: React.ReactNode }) {
  return (
    <SiteShell header={<FoodNav />}>{children}</SiteShell>
  );
}
