import type { Metadata } from "next";
import { STATIC_MATERIALS } from "@/lib/library/mock";
import Reader from "@/components/library/reader";

export function generateStaticParams() {
  return STATIC_MATERIALS.map((m) => ({ id: m.id }));
}

export const metadata: Metadata = { title: "在线阅读 — henukit 资料库" };

export default async function ReadPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <Reader id={id} />;
}
