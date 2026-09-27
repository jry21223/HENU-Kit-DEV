"use client";

import SubSiteNav, { type SubSiteTab } from "@/components/sub-site-nav";
import { LEVEL_LABELS } from "@/lib/navigation/parent-route";

// 书库只有一个标签，SubSiteNav 因此不渲染标签行；保留这一项，是为了日后加标签时
// 不用再改页头结构。
const TABS: SubSiteTab[] = [
  { href: "/library", index: "L-01", label: LEVEL_LABELS.library, match: (p: string) => p === "/library" || p.startsWith("/library/item") || p.startsWith("/library/read") },
];

export default function LibraryNav() {
  return <SubSiteNav brand="LIBRARY" tabs={TABS} />;
}
