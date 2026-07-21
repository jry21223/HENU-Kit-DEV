import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { getAllLists, getListById } from "@/lib/practice/mock";
import ListDetail from "@/components/practice/list-detail";

export function generateStaticParams() {
  return getAllLists().map((l) => ({ id: l.id }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await params;
  const list = getListById(id);
  return { title: list ? `${list.name} — henukit` : "题单 — henukit" };
}

export default async function ListPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const list = getListById(id);
  if (!list) notFound();
  return <ListDetail list={list} />;
}
