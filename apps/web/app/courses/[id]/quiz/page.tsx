import Link from "next/link";
import { PracticeCard } from "@/components/quiz/practice-card";
import { Course, QuizQuestion, getApi } from "@/lib/api";

type PageProps = {
  params: Promise<{ id: string }>;
};

async function loadQuiz(id: string) {
  try {
    const [courseResponse, questionResponse] = await Promise.all([
      getApi<{ course: Course }>(`/courses/${id}`),
      getApi<{ questions: QuizQuestion[] }>(`/courses/${id}/questions`),
    ]);
    return {
      course: courseResponse.data.course,
      questions: questionResponse.data.questions,
      error: "",
    };
  } catch (error) {
    return {
      course: null as Course | null,
      questions: [] as QuizQuestion[],
      error: error instanceof Error ? error.message : "API unavailable",
    };
  }
}

export default async function CourseQuizPage({ params }: PageProps) {
  const { id } = await params;
  const { course, questions, error } = await loadQuiz(id);

  return (
    <main className="min-h-screen px-5 py-6 sm:px-8">
      <section className="mx-auto max-w-4xl">
        <nav className="flex items-center justify-between text-sm">
          <Link className="font-semibold text-sage" href={`/courses/${id}`}>
            返回课程
          </Link>
          <Link href="/courses">课程库</Link>
        </nav>

        <header className="mt-8">
          <p className="text-sm font-medium text-sage">在线刷题</p>
          <h1 className="mt-2 text-3xl font-semibold">{course ? course.name : "课程刷题"}</h1>
          <p className="mt-3 text-sm leading-6 text-slate-600">
            未登录用户可以试做；登录后答错会保存到错题本。题目答案不会在列表或详情接口中返回。
          </p>
        </header>

        {error ? <p className="mt-6 rounded-md border border-line bg-white p-4 text-sm text-slate-600">{error}</p> : null}

        <div className="mt-6 grid gap-4">
          {questions.map((question) => (
            <PracticeCard key={question.id} question={question} />
          ))}
        </div>

        {!error && questions.length === 0 ? (
          <p className="mt-6 rounded-md border border-line bg-white p-4 text-sm text-slate-600">暂无已发布题目。</p>
        ) : null}
      </section>
    </main>
  );
}
