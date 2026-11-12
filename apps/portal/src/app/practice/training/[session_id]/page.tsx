import type { Metadata } from "next";
import ListeningSession from "@/components/practice/listening-session";

export const metadata: Metadata = { title: "听力练习", robots: { index: false, follow: false } };

export default async function TrainingSessionPage({ params }: { params: Promise<{ session_id: string }> }) {
  const { session_id } = await params;
  return <ListeningSession key={session_id} sessionID={session_id} />;
}
