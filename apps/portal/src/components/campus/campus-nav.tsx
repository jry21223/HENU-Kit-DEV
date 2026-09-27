"use client";

import SubSiteNav, { type SubSiteTab } from "@/components/sub-site-nav";
import { LEVEL_LABELS } from "@/lib/navigation/parent-route";

const TABS: SubSiteTab[] = [
  { href: "/campus", index: "M-01", label: LEVEL_LABELS.campus, match: (p: string) => p === "/campus" || p.startsWith("/campus/item") },
  { href: "/campus/deals", index: "M-02", label: "我的交易", match: (p: string) => p.startsWith("/campus/deals") },
  { href: "/campus/publish", index: "M-03", label: "发布", match: (p: string) => p.startsWith("/campus/publish") },
];

export default function CampusNav() {
  return <SubSiteNav brand="CAMPUS" tabs={TABS} />;
}
