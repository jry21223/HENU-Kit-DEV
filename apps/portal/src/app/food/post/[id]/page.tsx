import type { Metadata } from "next";
import PostDetail from "@/components/food/post-detail";

export const metadata: Metadata = { title: "锐评 — henukit 美食榜" };

export default async function PostPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <PostDetail id={id} />;
}
