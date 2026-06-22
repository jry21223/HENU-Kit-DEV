import Link from "next/link";
import { LoginForm } from "@/components/auth/login-form";

export default function LoginPage() {
  return (
    <main className="min-h-screen px-5 py-6">
      <section className="mx-auto max-w-md rounded-lg border border-line bg-white p-6 shadow-sm">
        <Link className="text-sm text-sage" href="/">
          返回首页
        </Link>
        <h1 className="mt-6 text-2xl font-semibold">学生邮箱登录</h1>
        <p className="mt-3 text-sm leading-6 text-slate-600">
          使用学校邮箱获取验证码。开发环境会显示固定验证码；生产环境必须接入真实邮件发送并关闭固定验证码。
        </p>
        <LoginForm />
      </section>
    </main>
  );
}
