import type { Metadata } from "next";
import { STATIC_POSTS } from "@/lib/food/mock";
import PostDetail from "@/components/food/post-detail";

export function generateStaticParams() {
  return STATIC_POSTS.map((p) => ({ id: p.id }));
}

export const metadata: Metadata = { title: "锐评 — henukit 美食榜" };

export default async function PostPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <PostDetail id={id} />;
}
