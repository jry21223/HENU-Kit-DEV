import { redirect } from "next/navigation";

export default async function SlidesRoute({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  redirect(`/library/item/${encodeURIComponent(id)}`);
}
