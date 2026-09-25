import type { Metadata } from "next";
import FavoritesFolder from "@/components/practice/favorites-folder";
import { pageTitle } from "@/lib/seo";

export const metadata: Metadata = {
  title: pageTitle("题库收藏夹", "practice"),
};

export default async function FavoritesFolderPage({
  params,
}: {
  params: Promise<{ bank_id: string }>;
}) {
  const { bank_id } = await params;
  return <FavoritesFolder bankID={bank_id} />;
}
