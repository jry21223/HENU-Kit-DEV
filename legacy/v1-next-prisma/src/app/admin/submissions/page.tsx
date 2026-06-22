import Link from "next/link";
import { SubmissionReviewActions } from "@/components/admin/submission-review-actions";
import { PageShell } from "@/components/layout/page-shell";
import { getReviewerUser } from "@/lib/admin";
import { parseSubmissionStatus } from "@/lib/submissions";
import { listReviewSubmissions } from "@/services/submission-service";

type AdminSubmissionsPageProps = {
  searchParams?: Promise<{ status?: string }>;
};

const statusOptions = [
  { href: "/admin/submissions", label: "全部" },
  { href: "/admin/submissions?status=pending", label: "待审核" },
  { href: "/admin/submissions?status=approved", label: "已通过" },
  { href: "/admin/submissions?status=rejected", label: "已驳回" },
];

const statusLabel = {
  pending: "待审核",
  approved: "已通过",
  rejected: "已驳回",
  archived: "已归档",
} as const;

function formatDate(value: string | null) {
  return value ? new Date(value).toLocaleString() : "暂无";
}

export default async function AdminSubmissionsPage({ searchParams }: AdminSubmissionsPageProps) {
  const reviewer = await getReviewerUser();
  if (!reviewer) {
    return (
      <PageShell
        eyebrow="403"
        title="无权访问投稿审核"
        description="投稿审核需要 reviewer 或 admin 角色，并且必须在服务端校验。"
      >
        <Link href="/login" className="text-sm font-semibold text-brand">
          去登录
        </Link>
      </PageShell>
    );
  }

  const params = searchParams ? await searchParams : {};
  const selectedStatus = parseSubmissionStatus(params.status);
  const submissions = await listReviewSubmissions(selectedStatus);

  return (
    <PageShell
      eyebrow="Admin"
      title="投稿审核"
      description={`当前审核账号：${reviewer.email}`}
    >
      <div className="mb-5 flex flex-wrap gap-2">
        {statusOptions.map((option) => (
          <Link
            key={option.href}
            href={option.href}
            className="inline-flex h-9 items-center rounded-md border border-line bg-white px-3 text-sm font-semibold text-ink hover:bg-panel focus-ring"
          >
            {option.label}
          </Link>
        ))}
      </div>
      <div className="grid gap-4">
        {submissions.length > 0 ? (
          submissions.map((submission) => (
            <article key={submission.id} className="rounded-lg border border-line bg-white p-5 shadow-soft">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <h2 className="text-lg font-semibold text-ink">{submission.title}</h2>
                  <p className="mt-1 text-sm text-muted">
                    {submission.course.name} / {submission.user.email}
                  </p>
                </div>
                <span className="rounded-md border border-line bg-panel px-2.5 py-1 text-xs font-semibold text-muted">
                  {statusLabel[submission.status]}
                </span>
              </div>
              <p className="mt-3 text-sm leading-6 text-muted">{submission.description}</p>
              <dl className="mt-3 grid gap-1 text-xs leading-5 text-muted">
                <div>提交时间：{formatDate(submission.createdAt)}</div>
                <div>审核时间：{formatDate(submission.reviewedAt)}</div>
                <div>
                  文件地址：
                  <a href={submission.fileUrl} className="font-semibold text-brand" target="_blank">
                    {submission.fileUrl}
                  </a>
                </div>
                {submission.reviewComment ? <div>审核说明：{submission.reviewComment}</div> : null}
              </dl>
              {submission.status === "pending" ? (
                <SubmissionReviewActions submissionId={submission.id} />
              ) : null}
            </article>
          ))
        ) : (
          <div className="rounded-lg border border-line bg-white p-6 text-sm text-muted shadow-soft">
            当前没有符合条件的投稿。
          </div>
        )}
      </div>
    </PageShell>
  );
}
