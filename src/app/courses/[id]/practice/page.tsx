import Link from "next/link";
import { notFound } from "next/navigation";
import { PracticeQuestionList } from "@/components/practice/practice-question-list";
import { PageShell } from "@/components/layout/page-shell";
import { getCourseById } from "@/services/course-service";
import { listQuestionsByCourse } from "@/services/question-service";

type CoursePracticePageProps = {
  params: Promise<{ id: string }>;
};

export default async function CoursePracticePage({ params }: CoursePracticePageProps) {
  const { id } = await params;
  const course = await getCourseById(id);

  if (!course) {
    notFound();
  }

  const questions = await listQuestionsByCourse(course.id);

  return (
    <PageShell
      eyebrow="在线刷题"
      title={`${course.name} 刷题练习`}
      description="当前基础版支持单选题和判断题。提交后显示对错与解析，登录用户答错会写入错题本。"
    >
      <div className="mb-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="rounded-lg border border-line bg-white p-4 text-sm text-muted shadow-soft">
          共 {questions.length} 道已发布题目。
        </div>
        <Link
          href={`/courses/${course.id}`}
          className="inline-flex h-10 items-center justify-center rounded-md border border-line bg-white px-4 text-sm font-semibold text-ink hover:bg-panel focus-ring"
        >
          返回课程详情
        </Link>
      </div>

      <PracticeQuestionList
        courseId={course.id}
        courseName={course.name}
        questions={questions}
      />
    </PageShell>
  );
}
