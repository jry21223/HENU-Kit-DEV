import { PageShell } from "@/components/layout/page-shell";
import { getAdminUser } from "@/lib/admin";

export default async function NewCoursePage() {
  const admin = await getAdminUser();
  if (!admin) {
    return <PageShell title="无权访问新增课程" eyebrow="403">请使用管理员账号登录。</PageShell>;
  }

  return (
    <PageShell
      title="新增课程"
      eyebrow="Admin"
      description="基础写入能力通过 POST /api/admin/courses 提供。"
    >
      <div className="rounded-lg border border-line bg-white p-6 text-sm leading-6 text-muted shadow-soft">
        表单 UI 将继续细化；当前阶段先完成服务端 API 和权限校验。
      </div>
    </PageShell>
  );
}

