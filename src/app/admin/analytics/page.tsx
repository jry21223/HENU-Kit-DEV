import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";
import { getAdminUser } from "@/lib/admin";
import { getAdminAnalytics } from "@/services/analytics-service";

const metricLabels = {
  totalUsers: "用户数",
  verifiedUsers: "邮箱认证数",
  totalCourses: "课程数",
  publishedCourses: "已发布课程",
  totalMaterials: "资料数",
  publishedMaterials: "已发布资料",
  totalDownloads: "下载量",
  totalQuestions: "题目数",
  totalWrongQuestions: "错题记录",
  paidOrders: "已支付订单",
} as const;

export default async function AdminAnalyticsPage() {
  const admin = await getAdminUser();
  if (!admin) {
    return (
      <PageShell
        eyebrow="403"
        title="无权访问数据统计"
        description="运营统计仅允许 admin 查看，避免泄露用户隐私。"
      >
        <Link href="/login" className="text-sm font-semibold text-brand">
          去登录
        </Link>
      </PageShell>
    );
  }

  const analytics = await getAdminAnalytics();
  const metricEntries = Object.entries(analytics.metrics) as Array<
    [keyof typeof analytics.metrics, number]
  >;

  return (
    <PageShell
      eyebrow="Admin"
      title="数据统计"
      description="查看核心运营指标。当前不展示个人邮箱明细，课程热度以下载量近似。"
    >
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
        {metricEntries.map(([key, value]) => (
          <section key={key} className="rounded-lg border border-line bg-white p-4 shadow-soft">
            <p className="text-xs font-semibold text-muted">{metricLabels[key]}</p>
            <p className="mt-2 text-2xl font-semibold text-ink">{value}</p>
          </section>
        ))}
      </div>

      <div className="mt-6 grid gap-5 lg:grid-cols-3">
        <section className="rounded-lg border border-line bg-white p-5 shadow-soft">
          <h2 className="text-base font-semibold text-ink">热门课程</h2>
          <div className="mt-4 grid gap-3">
            {analytics.topCourses.length > 0 ? (
              analytics.topCourses.map((course) => (
                <div key={course.courseId} className="rounded-md bg-panel p-3">
                  <p className="font-semibold text-ink">{course.courseName}</p>
                  <p className="mt-1 text-sm text-muted">{course.count} 次相关下载</p>
                </div>
              ))
            ) : (
              <p className="text-sm text-muted">暂无课程下载数据。</p>
            )}
          </div>
        </section>

        <section className="rounded-lg border border-line bg-white p-5 shadow-soft">
          <h2 className="text-base font-semibold text-ink">高下载资料</h2>
          <div className="mt-4 grid gap-3">
            {analytics.topMaterials.length > 0 ? (
              analytics.topMaterials.map((material) => (
                <div key={material.materialId} className="rounded-md bg-panel p-3">
                  <p className="font-semibold text-ink">{material.materialTitle}</p>
                  <p className="mt-1 text-sm text-muted">
                    {material.courseName} / {material.count} 次下载
                  </p>
                </div>
              ))
            ) : (
              <p className="text-sm text-muted">暂无资料下载数据。</p>
            )}
          </div>
        </section>

        <section className="rounded-lg border border-line bg-white p-5 shadow-soft">
          <h2 className="text-base font-semibold text-ink">高错题知识点</h2>
          <div className="mt-4 grid gap-3">
            {analytics.weakPoints.length > 0 ? (
              analytics.weakPoints.map((point) => (
                <div key={point.knowledgePointId} className="rounded-md bg-panel p-3">
                  <p className="font-semibold text-ink">{point.knowledgePointTitle}</p>
                  <p className="mt-1 text-sm text-muted">
                    {point.courseName} / {point.count} 条错题
                  </p>
                </div>
              ))
            ) : (
              <p className="text-sm text-muted">暂无错题知识点数据。</p>
            )}
          </div>
        </section>
      </div>

      <section className="mt-6 rounded-lg border border-line bg-panel p-4 text-sm leading-6 text-muted">
        {analytics.notes.map((note) => (
          <p key={note}>{note}</p>
        ))}
      </section>
    </PageShell>
  );
}
