import Link from "next/link";
import { CourseCard } from "@/components/course/course-card";
import { PageShell } from "@/components/layout/page-shell";
import { listCourses } from "@/services/course-service";

const capabilities = [
  "按学校、学院、年级、专业定位课程",
  "集中查看讲义、模拟卷、答案解析和速背版",
  "学生邮箱登录、下载权限和课程包解锁",
  "题库、错题本、资料共建和 AI 草稿任务",
];

export default async function HomePage() {
  const hotCourses = (await listCourses()).slice(0, 4);

  return (
    <PageShell
      eyebrow="河南大学软件学院 MVP"
      title="一站式期末复习平台"
      description="按学校、年级、专业和课程获取知识点讲义、模拟卷与在线刷题。当前先服务河南大学软件学院 2023级 / 2024级课程复习场景。"
    >
      <section className="grid gap-6 lg:grid-cols-[1.1fr_0.9fr]">
        <div className="rounded-lg border border-line bg-white p-6 shadow-soft">
          <div className="grid gap-4 sm:grid-cols-2">
            <Link
              href="/login"
              className="rounded-lg border border-line p-5 hover:bg-panel focus-ring"
            >
              <span className="text-sm font-semibold text-accent">入口一</span>
              <h2 className="mt-2 text-xl font-semibold text-ink">学生邮箱登录</h2>
              <p className="mt-2 text-sm leading-6 text-muted">
                使用河南大学学生邮箱验证码登录，解锁课程资料和个人复习记录。
              </p>
            </Link>
            <Link
              href="/courses"
              className="rounded-lg border border-line p-5 hover:bg-panel focus-ring"
            >
              <span className="text-sm font-semibold text-accent">入口二</span>
              <h2 className="mt-2 text-xl font-semibold text-ink">课程库</h2>
              <p className="mt-2 text-sm leading-6 text-muted">
                先从软件学院核心课程和资料结构开始。
              </p>
            </Link>
          </div>
          <div className="mt-6 grid gap-3 border-t border-line pt-5 sm:grid-cols-2">
            {capabilities.map((capability) => (
              <div key={capability} className="flex gap-3 text-sm text-muted">
                <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-brand" />
                <span>{capability}</span>
              </div>
            ))}
          </div>
        </div>
        <aside className="rounded-lg border border-line bg-[#eef4f1] p-6">
          <p className="text-sm font-semibold text-accent">当前服务范围</p>
          <dl className="mt-4 grid gap-3 text-sm">
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">学校</dt>
              <dd className="font-semibold text-ink">河南大学</dd>
            </div>
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">学院</dt>
              <dd className="font-semibold text-ink">软件学院</dd>
            </div>
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">年级</dt>
              <dd className="font-semibold text-ink">2023级 / 2024级</dd>
            </div>
            <div className="flex justify-between gap-3">
              <dt className="text-muted">专业</dt>
              <dd className="font-semibold text-ink">网络工程 / 软件工程</dd>
            </div>
          </dl>
        </aside>
      </section>

      <section className="mt-8">
        <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h2 className="text-xl font-semibold text-ink">热门课程</h2>
            <p className="mt-1 text-sm text-muted">围绕软件学院核心期末课程组织。</p>
          </div>
          <Link
            href="/courses"
            className="inline-flex h-10 items-center rounded-md px-2 text-sm font-semibold text-brand hover:bg-panel hover:text-[#12574d] focus-ring"
          >
            查看全部课程
          </Link>
        </div>
        <div className="grid gap-4 md:grid-cols-2">
          {hotCourses.map((course) => (
            <CourseCard key={course.id} course={course} />
          ))}
        </div>
      </section>
    </PageShell>
  );
}
