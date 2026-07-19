import type { Metadata } from "next";
import { STATIC_MATERIALS } from "@/lib/library/mock";
import ItemDetail from "@/components/library/item-detail";

export function generateStaticParams() {
  return STATIC_MATERIALS.map((m) => ({ id: m.id }));
}

export const metadata: Metadata = { title: "资料详情 — henukit 资料库" };

export default async function ItemPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <ItemDetail id={id} />;
}
