import type { Metadata } from "next";
import FoodPostEditor from "@/components/food/post-editor";
import { pageTitle } from "@/lib/seo";

export const metadata: Metadata = { title: pageTitle("编辑投稿", "food") };

export default async function EditPostPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <FoodPostEditor id={id} />;
}
