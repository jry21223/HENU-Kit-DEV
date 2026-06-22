import Link from "next/link";
import { SubmissionForm } from "@/components/submission/submission-form";
import { PageShell } from "@/components/layout/page-shell";
import { getCurrentUser } from "@/lib/auth";
import { listCourses } from "@/services/course-service";
import { listUserSubmissions } from "@/services/submission-service";

const statusLabel = {
  pending: "待审核",
  approved: "已通过",
  rejected: "已驳回",
  archived: "已归档",
} as const;

function formatDate(value: string | null) {
  return value ? new Date(value).toLocaleString() : "暂无";
}

export default async function MySubmissionsPage() {
  const user = await getCurrentUser();
  if (!user) {
    return (
      <PageShell
        eyebrow="资料共建"
        title="我的投稿"
        description="登录后可以提交课程资料，并查看审核进度。"
      >
        <Link
          href="/login"
          className="inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
        >
          去登录
        </Link>
      </PageShell>
    );
  }

  const [courses, submissions] = await Promise.all([
    listCourses(),
    listUserSubmissions(user.id),
  ]);

  return (
    <PageShell
      eyebrow="资料共建"
      title="我的投稿"
      description="提交课程资料后会进入人工审核；审核通过前不会出现在前台资料列表。"
    >
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(320px,420px)]">
        <section>
          <h2 className="mb-3 text-lg font-semibold text-ink">提交资料</h2>
          <SubmissionForm courses={courses.map((course) => ({ id: course.id, name: course.name }))} />
        </section>
        <section>
          <h2 className="mb-3 text-lg font-semibold text-ink">审核进度</h2>
          <div className="grid gap-3">
            {submissions.length > 0 ? (
              submissions.map((submission) => (
                <article
                  key={submission.id}
                  className="rounded-lg border border-line bg-white p-4 shadow-soft"
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <h3 className="font-semibold text-ink">{submission.title}</h3>
                      <p className="mt-1 text-sm text-muted">{submission.course.name}</p>
                    </div>
                    <span className="rounded-md border border-line bg-panel px-2.5 py-1 text-xs font-semibold text-muted">
                      {statusLabel[submission.status]}
                    </span>
                  </div>
                  <p className="mt-3 text-sm leading-6 text-muted">{submission.description}</p>
                  <dl className="mt-3 grid gap-1 text-xs leading-5 text-muted">
                    <div>提交时间：{formatDate(submission.createdAt)}</div>
                    <div>审核时间：{formatDate(submission.reviewedAt)}</div>
                    {submission.reviewComment ? <div>审核说明：{submission.reviewComment}</div> : null}
                  </dl>
                </article>
              ))
            ) : (
              <div className="rounded-lg border border-line bg-white p-5 text-sm text-muted shadow-soft">
                暂无投稿记录。
              </div>
            )}
          </div>
        </section>
      </div>
    </PageShell>
  );
}
