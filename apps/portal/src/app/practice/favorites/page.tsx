import type { Metadata } from "next";
import FavoritesOverview from "@/components/practice/favorites-overview";
import { LEVEL_LABELS } from "@/lib/navigation/parent-route";
import { pageTitle } from "@/lib/seo";

export const metadata: Metadata = {
  title: pageTitle(LEVEL_LABELS.practiceFavorites, "practice"),
};

export default function FavoritesPage() {
  return <FavoritesOverview />;
}
