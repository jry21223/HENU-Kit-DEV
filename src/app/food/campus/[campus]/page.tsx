import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { CAMPUSES, CAMPUS_KEYS, CampusKey } from "@/lib/food/mock";
import CampusList from "@/components/food/campus-list";

export function generateStaticParams() {
  return CAMPUS_KEYS.map((campus) => ({ campus }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ campus: string }>;
}): Promise<Metadata> {
  const { campus } = await params;
  const c = CAMPUSES[campus as CampusKey];
  return { title: c ? `${c.name} · 美食榜 — henukit` : "美食榜 — henukit" };
}

export default async function CampusPage({
  params,
}: {
  params: Promise<{ campus: string }>;
}) {
  const { campus } = await params;
  if (!CAMPUS_KEYS.includes(campus as CampusKey)) notFound();
  return <CampusList campus={campus as CampusKey} />;
}
