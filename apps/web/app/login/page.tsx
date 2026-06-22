import { LoginForm } from "@/components/auth/login-form";
import { SiteShell } from "@/components/layout/site-shell";

export default function LoginPage() {
  return (
    <SiteShell>
      <section className="grid min-h-[calc(100vh-12rem)] place-items-center py-6 sm:py-10">
        <div className="w-full max-w-md rounded-3xl border border-border bg-card p-6 shadow-sm sm:p-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold text-foreground">学生登录</h1>
            <p className="mt-3 text-sm leading-6 text-muted-foreground">
              登录后可下载需要权限的 PDF 资料，下载时会按当前账号权限完成校验。
            </p>
          </div>
          <LoginForm />
        </div>
      </section>
    </SiteShell>
  );
}
