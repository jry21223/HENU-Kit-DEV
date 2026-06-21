import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";
import { getCurrentUser } from "@/lib/auth";
import { listWeakPointsForUser } from "@/services/question-service";

export default async function MyStatsPage() {
  const user = await getCurrentUser();

  if (!user) {
    return (
      <PageShell
        eyebrow="我的复习"
        title="薄弱知识点"
        description="登录后查看错题对应的知识点分布。"
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

  const weakPoints = await listWeakPointsForUser(user.id);
  const maxWrongCount = Math.max(...weakPoints.map((item) => item.wrongCount), 1);

  return (
    <PageShell
      eyebrow="我的复习"
      title="薄弱知识点"
      description="根据当前错题记录统计课程和知识点分布。"
    >
      <div className="mb-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="rounded-lg border border-line bg-white p-4 text-sm text-muted shadow-soft">
          共 {weakPoints.length} 个薄弱知识点。
        </div>
        <Link
          href="/me/wrong-questions"
          className="inline-flex h-10 items-center justify-center rounded-md border border-line bg-white px-4 text-sm font-semibold text-ink hover:bg-panel focus-ring"
        >
          返回错题本
        </Link>
      </div>

      {weakPoints.length > 0 ? (
        <div className="grid gap-4">
          {weakPoints.map((item) => {
            const width = `${Math.max(12, Math.round((item.wrongCount / maxWrongCount) * 100))}%`;

            return (
              <article
                key={`${item.course.id}-${item.knowledgePointId ?? "unknown"}`}
                className="rounded-lg border border-line bg-white p-5 shadow-soft"
              >
                <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <p className="text-sm font-semibold text-ink">{item.knowledgePointTitle}</p>
                    <p className="mt-1 text-sm text-muted">{item.course.name}</p>
                  </div>
                  <span className="rounded-md border border-line bg-panel px-2.5 py-1 text-xs font-semibold text-muted">
                    错题 {item.wrongCount} 道
                  </span>
                </div>
                <div className="mt-4 h-3 overflow-hidden rounded-full bg-panel">
                  <div className="h-full rounded-full bg-brand" style={{ width }} />
                </div>
                <Link
                  href={`/me/wrong-questions?courseId=${item.course.id}${
                    item.knowledgePointId ? `&knowledgePointId=${item.knowledgePointId}` : ""
                  }`}
                  className="mt-4 inline-flex h-10 items-center justify-center rounded-md border border-line bg-white px-4 text-sm font-semibold text-ink hover:bg-panel focus-ring"
                >
                  查看相关错题
                </Link>
              </article>
            );
          })}
        </div>
      ) : (
        <div className="rounded-lg border border-line bg-white p-6 text-sm text-muted shadow-soft">
          当前还没有错题统计。
        </div>
      )}
    </PageShell>
  );
}
