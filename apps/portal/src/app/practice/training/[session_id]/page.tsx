import type { Metadata } from "next";
import TrainingSessionView from "@/components/practice/training-session";

export const metadata: Metadata = { title: "英语训练", robots: { index: false, follow: false } };

export default async function TrainingSessionPage({ params }: { params: Promise<{ session_id: string }> }) {
  const { session_id } = await params;
  return <TrainingSessionView key={session_id} sessionID={session_id} />;
}
