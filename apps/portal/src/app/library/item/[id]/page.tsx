import type { Metadata } from "next";
import ItemDetail from "@/components/library/item-detail";
import { pageTitle } from "@/lib/seo";

export const metadata: Metadata = { title: pageTitle("资料详情", "library") };

export default async function ItemPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <ItemDetail id={id} />;
}
