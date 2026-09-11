import type { Metadata } from "next";
import FavoritesOverview from "@/components/practice/favorites-overview";
import { LEVEL_LABELS } from "@/lib/navigation/parent-route";

export const metadata: Metadata = {
  title: `${LEVEL_LABELS.practiceFavorites} — henukit`,
};

export default function FavoritesPage() {
  return <FavoritesOverview />;
}
