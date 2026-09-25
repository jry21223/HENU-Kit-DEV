"use client";

import SubSiteNav, { type SubSiteTab } from "@/components/sub-site-nav";
import { LEVEL_LABELS } from "@/lib/navigation/parent-route";

const TABS: SubSiteTab[] = [
  { href: "/food", index: "F-01", label: LEVEL_LABELS.food, match: (p: string) => p === "/food" || p.startsWith("/food/post") },
  { href: "/food/publish", index: "F-02", label: "提交推荐", match: (p: string) => p.startsWith("/food/publish") },
];

export default function FoodNav() {
  return <SubSiteNav brand="FOOD" tabs={TABS} />;
}
