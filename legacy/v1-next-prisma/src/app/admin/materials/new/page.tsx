import { PageShell } from "@/components/layout/page-shell";
import { getAdminUser } from "@/lib/admin";

export default async function NewMaterialPage() {
  const admin = await getAdminUser();
  if (!admin) {
    return <PageShell title="无权访问新增资料" eyebrow="403">请使用管理员账号登录。</PageShell>;
  }

  return (
    <PageShell
      title="新增资料"
      eyebrow="Admin"
      description="基础写入能力通过 POST /api/admin/materials 和 POST /api/admin/materials/upload 提供。"
    >
      <div className="rounded-lg border border-line bg-white p-6 text-sm leading-6 text-muted shadow-soft">
        表单 UI 将继续细化；当前阶段先完成服务端 API、上传限制和权限校验。
      </div>
    </PageShell>
  );
}

