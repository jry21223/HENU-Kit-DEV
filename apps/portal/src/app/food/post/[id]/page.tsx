import type { Metadata } from "next";
import PostDetail from "@/components/food/post-detail";
import { pageTitle } from "@/lib/seo";

export const metadata: Metadata = { title: pageTitle("锐评", "food") };

export default async function PostPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <PostDetail id={id} />;
}
