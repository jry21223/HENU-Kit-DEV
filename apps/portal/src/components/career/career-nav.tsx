"use client";

import SubSiteNav, { type SubSiteTab } from "@/components/sub-site-nav";
import { LEVEL_LABELS } from "@/lib/navigation/parent-route";

const TABS: SubSiteTab[] = [
  { href: "/career", index: "R-01", label: LEVEL_LABELS.career, match: (p: string) => p === "/career" },
  { href: "/career/history", index: "R-02", label: "历史", match: (p: string) => p.startsWith("/career/history") },
];

export default function CareerNav() {
  return <SubSiteNav brand="WORK RADAR" tabs={TABS} />;
}
