import Link from "next/link";

export default function LoginPage() {
  return (
    <main className="min-h-screen px-5 py-6">
      <section className="mx-auto max-w-md rounded-lg border border-line bg-white p-6 shadow-sm">
        <Link className="text-sm text-sage" href="/">
          返回首页
        </Link>
        <h1 className="mt-6 text-2xl font-semibold">学生邮箱登录</h1>
        <p className="mt-3 text-sm leading-6 text-slate-600">
          Go API 已支持邮箱验证码、JWT Cookie 和角色权限。本页面下一步会接入真实登录表单；当前仍是前端入口占位。
        </p>
        <form className="mt-6 space-y-4">
          <label className="block text-sm font-medium">
            学生邮箱
            <input
              className="mt-2 w-full rounded-md border border-line px-3 py-2"
              placeholder="name@stu.henu.edu.cn"
              type="email"
              disabled
            />
          </label>
          <button className="w-full rounded-md bg-sage px-4 py-2 text-sm font-medium text-white opacity-60" disabled>
            登录表单待接入
          </button>
        </form>
      </section>
    </main>
  );
}
