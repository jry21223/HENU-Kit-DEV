"use client";

import SubSiteNav, { type SubSiteTab } from "@/components/sub-site-nav";
import { LEVEL_LABELS } from "@/lib/navigation/parent-route";

// 「我的交易」「发布」尚未开放：入口不进导航，直接访问由各自页面说明「即将开放」（#568）。
// 只剩一个标签时 SubSiteNav 不渲染标签行。
const TABS: SubSiteTab[] = [
  { href: "/campus", index: "M-01", label: LEVEL_LABELS.campus, match: (p: string) => p === "/campus" || p.startsWith("/campus/item") },
];

export default function CampusNav() {
  return <SubSiteNav brand="CAMPUS" tabs={TABS} />;
}
