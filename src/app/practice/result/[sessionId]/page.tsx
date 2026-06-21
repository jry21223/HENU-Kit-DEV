import { notFound } from "next/navigation";
import { PageShell } from "@/components/layout/page-shell";
import { PracticeResultClient } from "@/components/practice/practice-result-client";
import { getCourseById } from "@/services/course-service";

type PracticeResultPageProps = {
  params: Promise<{ sessionId: string }>;
};

export default async function PracticeResultPage({ params }: PracticeResultPageProps) {
  const { sessionId } = await params;
  const course = await getCourseById(sessionId);

  if (!course) {
    notFound();
  }

  return (
    <PageShell
      eyebrow="练习结果"
      title={`${course.name} 练习结果`}
      description="Phase 7 基础版在浏览器本地记录本次练习结果，错题记录仍以服务端为准。"
    >
      <PracticeResultClient courseId={course.id} courseName={course.name} />
    </PageShell>
  );
}
