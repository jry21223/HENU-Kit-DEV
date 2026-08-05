import type { Metadata } from "next";
import SlidesPage from "@/components/library/slides-page";

export const metadata: Metadata = { title: "课件幻灯片 — henukit 资料库" };

export default async function SlidesRoute({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <SlidesPage id={id} />;
}
