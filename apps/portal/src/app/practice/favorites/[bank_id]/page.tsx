import type { Metadata } from "next";
import FavoritesFolder from "@/components/practice/favorites-folder";

export const metadata: Metadata = {
  title: "题库收藏夹 — henukit",
};

export default async function FavoritesFolderPage({
  params,
}: {
  params: Promise<{ bank_id: string }>;
}) {
  const { bank_id } = await params;
  return <FavoritesFolder bankID={bank_id} />;
}
