import Link from "next/link";
import { WrongQuestionList } from "@/components/me/wrong-question-list";
import { PageShell } from "@/components/layout/page-shell";
import { getCurrentUser } from "@/lib/auth";
import { listWrongQuestionsForUser } from "@/services/question-service";

type WrongQuestionsPageProps = {
  searchParams?: Promise<{
    courseId?: string;
    knowledgePointId?: string;
  }>;
};

function uniqueBy<T>(items: T[], getKey: (item: T) => string) {
  const seen = new Set<string>();
  return items.filter((item) => {
    const key = getKey(item);
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

export default async function WrongQuestionsPage({ searchParams }: WrongQuestionsPageProps) {
  const user = await getCurrentUser();
  const resolvedSearchParams = await searchParams;

  if (!user) {
    return (
      <PageShell
        eyebrow="我的复习"
        title="错题本"
        description="登录后查看课程错题、知识点分布和重新练习入口。"
      >
        <div className="rounded-lg border border-line bg-white p-6 shadow-soft">
          <p className="text-sm text-muted">请先使用学生邮箱登录。</p>
          <Link
            href="/login"
            className="mt-4 inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
          >
            去登录
          </Link>
        </div>
      </PageShell>
    );
  }

  const [wrongQuestions, allWrongQuestions] = await Promise.all([
    listWrongQuestionsForUser(user.id, {
      courseId: resolvedSearchParams?.courseId,
      knowledgePointId: resolvedSearchParams?.knowledgePointId,
    }),
    listWrongQuestionsForUser(user.id),
  ]);

  const courses = uniqueBy(
    allWrongQuestions.map((item) => item.course),
    (course) => course.id,
  );
  const knowledgePoints = uniqueBy(
    allWrongQuestions
      .map((item) => ({
        id: item.question.knowledgePointId,
        title: item.knowledgePointTitle,
      }))
      .filter((item): item is { id: string; title: string } => Boolean(item.id && item.title)),
    (item) => item.id,
  );

  return (
    <PageShell
      eyebrow="我的复习"
      title="错题本"
      description="按课程和知识点查看错题，移除已掌握题目，或回到课程继续练习。"
    >
      <div className="mb-5 grid gap-4 rounded-lg border border-line bg-white p-5 shadow-soft">
        <form className="grid gap-3 md:grid-cols-[1fr_1fr_auto_auto]" action="/me/wrong-questions">
          <label className="grid gap-1 text-sm font-semibold text-ink">
            课程
            <select
              name="courseId"
              defaultValue={resolvedSearchParams?.courseId ?? ""}
              className="h-10 rounded-md border border-line bg-white px-3 text-sm text-ink focus-ring"
            >
              <option value="">全部课程</option>
              {courses.map((course) => (
                <option key={course.id} value={course.id}>
                  {course.name}
                </option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-sm font-semibold text-ink">
            知识点
            <select
              name="knowledgePointId"
              defaultValue={resolvedSearchParams?.knowledgePointId ?? ""}
              className="h-10 rounded-md border border-line bg-white px-3 text-sm text-ink focus-ring"
            >
              <option value="">全部知识点</option>
              {knowledgePoints.map((knowledgePoint) => (
                <option key={knowledgePoint.id} value={knowledgePoint.id}>
                  {knowledgePoint.title}
                </option>
              ))}
            </select>
          </label>
          <button
            type="submit"
            className="h-10 self-end rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
          >
            筛选
          </button>
          <Link
            href="/me/stats"
            className="inline-flex h-10 items-center justify-center self-end rounded-md border border-line bg-panel px-4 text-sm font-semibold text-ink hover:bg-white focus-ring"
          >
            薄弱统计
          </Link>
        </form>
      </div>

      <WrongQuestionList initialItems={wrongQuestions} />
    </PageShell>
  );
}
