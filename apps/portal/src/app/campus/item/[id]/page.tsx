import type { Metadata } from "next";
import { STATIC_ITEMS } from "@/lib/campus/mock";
import ItemDetail from "@/components/campus/item-detail";

export function generateStaticParams() {
  return STATIC_ITEMS.map((i) => ({ id: i.id }));
}

export const metadata: Metadata = { title: "单子详情 — henukit 互助平台" };

export default async function ItemPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <ItemDetail id={id} />;
}
