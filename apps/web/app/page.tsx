import Link from "next/link";

const features = [
  { title: "找资料", body: "按学校、学院、专业、年级和课程定位讲义、试卷与解析。" },
  { title: "刷题", body: "先支持单选、判断、多选和填空的确定性判题，错题自动归档。" },
  { title: "看 Wiki", body: "课程知识点共创、审核和版本历史会在后续阶段接入。" },
  { title: "问 AI", body: "AI 先以 mock worker 闭环推进，正式内容必须审核后发布。" },
];

export default function HomePage() {
  return (
    <main className="min-h-screen px-5 py-6 sm:px-8">
      <section className="mx-auto flex max-w-5xl flex-col gap-8">
        <nav className="flex items-center justify-between text-sm">
          <span className="font-semibold text-sage">Final Review V2</span>
          <div className="flex gap-4">
            <Link href="/courses">课程库</Link>
            <Link href="/me/downloads">我的下载</Link>
            <Link href="/health">API 状态</Link>
            <Link href="/login">登录</Link>
          </div>
        </nav>

        <div className="grid gap-8 rounded-lg border border-line bg-white p-6 shadow-sm md:grid-cols-[1.1fr_0.9fr]">
          <div className="space-y-5">
            <p className="text-sm font-medium text-sage">河南大学软件学院优先内测</p>
            <h1 className="text-4xl font-semibold leading-tight sm:text-5xl">一站式学习平台 V2</h1>
            <p className="max-w-2xl text-base leading-7 text-slate-600">
              课程资料、在线刷题、错题分析、Wiki 共创、AI 学习助手和管理后台都由 Go API 统一提供能力。
            </p>
            <div className="flex flex-wrap gap-3">
              <Link className="rounded-md bg-sage px-4 py-2 text-sm font-medium text-white" href="/courses">
                进入课程库
              </Link>
              <Link className="rounded-md border border-line px-4 py-2 text-sm font-medium" href="/login">
                学生邮箱登录
              </Link>
            </div>
          </div>
          <div className="grid gap-3">
            {features.map((feature) => (
              <div key={feature.title} className="rounded-md border border-line p-4">
                <h2 className="text-base font-semibold">{feature.title}</h2>
                <p className="mt-1 text-sm leading-6 text-slate-600">{feature.body}</p>
              </div>
            ))}
          </div>
        </div>
      </section>
    </main>
  );
}
