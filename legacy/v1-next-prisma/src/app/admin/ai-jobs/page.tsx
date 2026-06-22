import Link from "next/link";
import { AiJobForm } from "@/components/admin/ai-job-form";
import { PageShell } from "@/components/layout/page-shell";
import { getAdminUser } from "@/lib/admin";
import { parseAiJobStatus } from "@/lib/ai-jobs";
import { prisma } from "@/lib/db";
import { listAiJobs } from "@/services/ai-job-service";

type AdminAiJobsPageProps = {
  searchParams?: Promise<{ status?: string }>;
};

const statusOptions = [
  { href: "/admin/ai-jobs", label: "全部" },
  { href: "/admin/ai-jobs?status=succeeded", label: "成功" },
  { href: "/admin/ai-jobs?status=failed", label: "失败" },
  { href: "/admin/ai-jobs?status=queued", label: "排队" },
];

const statusLabel = {
  queued: "排队中",
  running: "运行中",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
} as const;

function formatDate(value: string) {
  return new Date(value).toLocaleString();
}

function getMaterialId(result: unknown) {
  if (result && typeof result === "object" && "materialId" in result) {
    const materialId = (result as { materialId?: unknown }).materialId;
    return typeof materialId === "string" ? materialId : "";
  }
  return "";
}

export default async function AdminAiJobsPage({ searchParams }: AdminAiJobsPageProps) {
  const admin = await getAdminUser();
  if (!admin) {
    return (
      <PageShell
        eyebrow="403"
        title="无权访问 AI 任务"
        description="AI 任务当前仅允许 admin 创建和查看。"
      >
        <Link href="/login" className="text-sm font-semibold text-brand">
          去登录
        </Link>
      </PageShell>
    );
  }

  const params = searchParams ? await searchParams : {};
  const selectedStatus = parseAiJobStatus(params.status);
  const [courses, jobs, materials] = await Promise.all([
    prisma.course.findMany({
      where: { status: "PUBLISHED" },
      select: { id: true, name: true },
      orderBy: { name: "asc" },
    }),
    listAiJobs(selectedStatus),
    prisma.material.findMany({
      where: { status: "PUBLISHED" },
      select: { id: true, title: true, course: { select: { name: true } } },
      orderBy: { updatedAt: "desc" },
      take: 12,
    }),
  ]);

  return (
    <PageShell
      eyebrow="Admin"
      title="AI 任务"
      description="当前为本地生成草稿流程，不接真实 AI；生成内容默认进入 draft，不会自动发布。"
    >
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_360px]">
        <section>
          <div className="mb-4 flex flex-wrap gap-2">
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
            {jobs.length > 0 ? (
              jobs.map((job) => {
                const materialId = getMaterialId(job.result);
                return (
                  <article key={job.id} className="rounded-lg border border-line bg-white p-5 shadow-soft">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <h2 className="text-lg font-semibold text-ink">{job.courseName}</h2>
                        <p className="mt-1 text-sm text-muted">
                          {job.outputType} / {formatDate(job.createdAt)}
                        </p>
                      </div>
                      <span className="rounded-md border border-line bg-panel px-2.5 py-1 text-xs font-semibold text-muted">
                        {statusLabel[job.status]}
                      </span>
                    </div>
                    <dl className="mt-3 grid gap-1 text-xs leading-5 text-muted">
                      <div>任务 ID：{job.id}</div>
                      <div>来源资料：{job.inputMaterialIds.length ? job.inputMaterialIds.join(", ") : "未指定"}</div>
                      {materialId ? <div>草稿资料 ID：{materialId}</div> : null}
                      {job.error ? <div>错误信息：{job.error}</div> : null}
                    </dl>
                  </article>
                );
              })
            ) : (
              <div className="rounded-lg border border-line bg-white p-6 text-sm text-muted shadow-soft">
                暂无 AI 任务。
              </div>
            )}
          </div>
        </section>
        <aside className="grid gap-4 content-start">
          <AiJobForm courses={courses} />
          <div className="rounded-lg border border-line bg-white p-4 text-sm leading-6 text-muted shadow-soft">
            <h2 className="font-semibold text-ink">可用来源资料</h2>
            <div className="mt-3 grid gap-2">
              {materials.map((material) => (
                <div key={material.id} className="rounded-md bg-panel p-3">
                  <p className="font-semibold text-ink">{material.title}</p>
                  <p className="mt-1 text-xs text-muted">
                    {material.course.name} / {material.id}
                  </p>
                </div>
              ))}
            </div>
          </div>
        </aside>
      </div>
    </PageShell>
  );
}
