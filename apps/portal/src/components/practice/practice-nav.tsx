"use client";

import SubSiteNav, { type SubSiteTab } from "@/components/sub-site-nav";
import TransitionLink from "@/components/practice/transition/transition-link";
import { LEVEL_LABELS } from "@/lib/navigation/parent-route";
import { quizCraftCatalogEnabled, quizCraftV2ReadsEnabled } from "@/lib/api/env";

const TABS: SubSiteTab[] = [
  {
    href: "/practice",
    index: "P-01",
    label: LEVEL_LABELS.practiceBank,
    match: (p: string) => p === "/practice",
  },
  {
    href: "/practice/quiz",
    index: "P-02",
    label: "刷题",
    // 题库目录开关关闭时不能点，标签旁标「未开放」。隐藏还是保留标注待 #540 决定。
    disabled: !quizCraftCatalogEnabled(),
    match: (p: string) => p.startsWith("/practice/quiz"),
  },
  { href: "/practice/favorites", index: "P-03", label: LEVEL_LABELS.practiceFavorites, match: (p: string) => p.startsWith("/practice/favorites") },
  { href: "/practice/stats", index: "P-04", label: "数据", match: (p: string) => p.startsWith("/practice/stats") },
  ...(quizCraftV2ReadsEnabled()
    ? [{ href: "/practice/leaderboard", index: "P-05", label: "排行榜", match: (p: string) => p.startsWith("/practice/leaderboard") }]
    : []),
];

export default function PracticeNav() {
  return <SubSiteNav brand="PRACTICE" tabs={TABS} linkAs={TransitionLink} />;
}
