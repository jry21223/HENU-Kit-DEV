import { PageShell } from "@/components/layout/page-shell";
import { LoginForm } from "@/components/auth/login-form";
import { listGrades, listMajors, listSchools } from "@/services/catalog-service";

export default async function LoginPage() {
  const [schools, majors, grades] = await Promise.all([
    listSchools(),
    listMajors(),
    listGrades(),
  ]);

  return (
    <PageShell
      eyebrow="学生认证"
      title="学生邮箱登录"
      description="使用河南大学学生邮箱验证码登录，并绑定学校、专业和年级。"
    >
      <LoginForm schools={schools} majors={majors} grades={grades} />
    </PageShell>
  );
}
