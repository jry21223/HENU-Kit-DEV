import { PageShell } from "@/components/layout/page-shell";
import Link from "next/link";
import { getAdminUser } from "@/lib/admin";

const adminModules = [
  { title: "课程管理", href: "/admin/courses" },
  { title: "资料管理", href: "/admin/materials" },
  { title: "课程包管理", href: "/admin/packages" },
  { title: "投稿审核", href: "/admin/submissions" },
  { title: "AI 任务", href: "/admin/ai-jobs" },
  { title: "题库管理", href: "/admin/questions" },
  { title: "运营统计", href: "/admin/analytics" },
];

export default async function AdminPage() {
  const admin = await getAdminUser();

  if (!admin) {
    return (
      <PageShell
        eyebrow="403"
        title="无权访问管理后台"
        description="管理后台需要 admin 角色。权限必须在服务端校验，不能只靠前端隐藏按钮。"
      >
        <Link
          href="/login"
          className="inline-flex h-10 items-center rounded-md bg-brand px-4 text-sm font-semibold text-white"
        >
          去登录
        </Link>
      </PageShell>
    );
  }

  return (
    <PageShell
      eyebrow="Admin"
      title="管理后台"
      description={`当前管理员：${admin.email}`}
    >
      <section className="rounded-lg border border-line bg-white p-6 shadow-soft">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {adminModules.map((module) => (
            <Link key={module.href} href={module.href} className="rounded-lg border border-line bg-panel p-4 hover:bg-white focus-ring">
              <h2 className="text-base font-semibold text-ink">{module.title}</h2>
              <p className="mt-2 text-sm leading-6 text-muted">
                进入对应模块查看和维护数据。
              </p>
            </Link>
          ))}
        </div>
        <div className="mt-6 rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm leading-6 text-amber-900">
          后台 API 已要求服务端 admin 角色。课程和资料维护能力按 Phase 6 基础版提供。
        </div>
      </section>
    </PageShell>
  );
}
