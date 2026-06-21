import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";
import { colleges } from "@/constants/mock-data";
import { listGrades, listMajors, listSchools } from "@/services/catalog-service";

export default async function SchoolsPage() {
  const [schools, majors, grades] = await Promise.all([
    listSchools(),
    listMajors(),
    listGrades(),
  ]);
  const school = schools[0];
  const college = colleges[0];

  return (
    <PageShell
      eyebrow="学校 / 年级 / 专业"
      title="选择复习范围"
      description="先聚焦河南大学软件学院，围绕 2023级 / 2024级网络工程与软件工程课程做深。"
    >
      <section className="rounded-lg border border-line bg-white p-6 shadow-soft">
        <div className="grid gap-5 md:grid-cols-3">
          <div>
            <p className="text-sm font-semibold text-accent">学校</p>
            <h2 className="mt-2 text-xl font-semibold text-ink">
              {school?.name ?? "暂无学校"}
            </h2>
            <p className="mt-2 text-sm leading-6 text-muted">
              邮箱域名：{school?.emailDomains.join(" / ") ?? "待配置"}
            </p>
          </div>
          <div>
            <p className="text-sm font-semibold text-accent">学院</p>
            <h2 className="mt-2 text-xl font-semibold text-ink">{college.name}</h2>
            <p className="mt-2 text-sm leading-6 text-muted">
              先做单学院核心课程，不扩全国高校。
            </p>
          </div>
          <div>
            <p className="text-sm font-semibold text-accent">年级</p>
            <h2 className="mt-2 text-xl font-semibold text-ink">{grades.join(" / ")}</h2>
            <p className="mt-2 text-sm leading-6 text-muted">
              对应当前期末复习场景。
            </p>
          </div>
        </div>
        <div className="mt-6 grid gap-3 border-t border-line pt-5 sm:grid-cols-2">
          {majors.map((major) => (
            <Link
              key={major.id}
              href={`/courses?schoolId=${school?.id ?? "henu"}&majorId=${major.id}`}
              className="rounded-lg border border-line p-4 hover:bg-panel focus-ring"
            >
              <span className="text-xs font-semibold text-accent">专业</span>
              <h3 className="mt-1 text-lg font-semibold text-ink">{major.name}</h3>
              <p className="mt-2 text-sm text-muted">查看适配课程与资料</p>
            </Link>
          ))}
        </div>
      </section>
    </PageShell>
  );
}
