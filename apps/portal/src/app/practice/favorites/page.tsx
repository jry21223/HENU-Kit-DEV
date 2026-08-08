import type { Metadata } from "next";
import FavoritesOverview from "@/components/practice/favorites-overview";

export const metadata: Metadata = {
  title: "收藏夹 — henukit",
};

export default function FavoritesPage() {
  return <FavoritesOverview />;
}
