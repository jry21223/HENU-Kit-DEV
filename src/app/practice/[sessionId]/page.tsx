import { redirect } from "next/navigation";

type PracticeSessionPageProps = {
  params: Promise<{ sessionId: string }>;
};

export default async function PracticeSessionPage({ params }: PracticeSessionPageProps) {
  const { sessionId } = await params;
  redirect(`/courses/${sessionId}/practice`);
}
