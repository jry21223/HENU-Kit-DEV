import Link from "next/link";
import type { ReactNode } from "react";
import { ArrowRight, BookOpenCheck, Layers3, PackageOpen, ShieldCheck } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button-link";
import { CoursePackage, Major, School, getApi } from "@/lib/api";

type College = {
  id: string;
  schoolId: string;
  name: string;
  status: string;
};

type Lookup = Map<string, string>;

const copy = {
  eyebrow: "课程复习包",
  title: "按课程包集中查看讲义、模拟卷和解析",
  intro:
    "课程包用于把同一门课程的知识点讲义、模拟卷、答案解析和考前速背资料合在一起交付。这里仅展示后端已发布的课程包，付费解锁仍以服务端订单和 entitlement 为准。",
  viewCourses: "查看课程资料",
  publishedOnly: "仅展示 published",
  price: "价格",
  school: "学校",
  college: "学院",
  major: "专业",
  grade: "年级",
  detail: "查看复习包",
  empty: "暂无已发布课程包。",
  unavailable: "课程包列表暂时不可用",
  fallbackSchool: "学校信息待同步",
  fallbackCollege: "学院信息待同步",
  fallbackMajor: "专业信息待同步",
};

async function loadPackages() {
  try {
    const packageResponse = await getApi<{ packages: CoursePackage[] }>("/packages");
    const [schoolResult, collegeResult, majorResult] = await Promise.allSettled([
      getApi<{ schools: School[] }>("/schools"),
      getApi<{ colleges: College[] }>("/colleges"),
      getApi<{ majors: Major[] }>("/majors"),
    ]);

    return {
      packages: packageResponse.data.packages,
      schools: mapNames(schoolResult.status === "fulfilled" ? schoolResult.value.data.schools : []),
      colleges: mapNames(collegeResult.status === "fulfilled" ? collegeResult.value.data.colleges : []),
      majors: mapNames(majorResult.status === "fulfilled" ? majorResult.value.data.majors : []),
      error: "",
    };
  } catch (error) {
    return {
      packages: [] as CoursePackage[],
      schools: new Map<string, string>(),
      colleges: new Map<string, string>(),
      majors: new Map<string, string>(),
      error: error instanceof Error ? error.message : copy.unavailable,
    };
  }
}

export default async function PackagesPage() {
  const { packages, schools, colleges, majors, error } = await loadPackages();

  return (
    <SiteShell>
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <Badge tone="success">{copy.eyebrow}</Badge>
            <h1 className="mt-4 max-w-3xl text-3xl font-semibold tracking-tight sm:text-4xl">{copy.title}</h1>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
          </div>
          <ButtonLink className="w-full justify-center sm:w-auto" href="/courses" variant="secondary">
            <BookOpenCheck className="mr-2 size-4" aria-hidden="true" />
            {copy.viewCourses}
          </ButtonLink>
        </div>
      </section>

      {error ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p> : null}

      <section className="grid gap-4 lg:grid-cols-2">
        {packages.map((coursePackage) => (
          <Link
            className="group rounded-3xl border border-border bg-card p-5 shadow-sm transition hover:border-primary/60 hover:shadow-md"
            href={`/packages/${coursePackage.id}`}
            key={coursePackage.id}
          >
            <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap gap-2">
                  <Badge tone="success">{copy.publishedOnly}</Badge>
                  <Badge tone="muted">{coursePackage.grade || copy.grade}</Badge>
                </div>
                <h2 className="mt-4 break-words text-xl font-semibold tracking-tight">{coursePackage.title}</h2>
              </div>
              <span className="inline-flex shrink-0 items-center rounded-full border border-border bg-background px-3 py-1 text-sm font-semibold">
                {formatPrice(coursePackage.priceFen, coursePackage.currency)}
              </span>
            </div>

            <p className="mt-3 line-clamp-3 break-words text-sm leading-6 text-muted-foreground">
              {coursePackage.description || "这个课程包还没有补充说明，详情页会展示已发布的包内资料。"}
            </p>

            <dl className="mt-5 grid gap-3 text-sm sm:grid-cols-2">
              <InfoItem icon={<ShieldCheck className="size-4" aria-hidden="true" />} label={copy.school}>
                {labelFor(schools, coursePackage.schoolId, copy.fallbackSchool)}
              </InfoItem>
              <InfoItem icon={<Layers3 className="size-4" aria-hidden="true" />} label={copy.college}>
                {labelFor(colleges, coursePackage.collegeId, copy.fallbackCollege)}
              </InfoItem>
              <InfoItem icon={<BookOpenCheck className="size-4" aria-hidden="true" />} label={copy.major}>
                {labelFor(majors, coursePackage.majorId, copy.fallbackMajor)}
              </InfoItem>
              <InfoItem icon={<PackageOpen className="size-4" aria-hidden="true" />} label={copy.grade}>
                {coursePackage.grade || "未设置"}
              </InfoItem>
            </dl>

            <span className="mt-5 inline-flex items-center text-sm font-medium text-primary">
              {copy.detail}
              <ArrowRight className="ml-1.5 size-4 transition group-hover:translate-x-0.5" aria-hidden="true" />
            </span>
          </Link>
        ))}
      </section>

      {!error && packages.length === 0 ? (
        <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.empty}</p>
      ) : null}
    </SiteShell>
  );
}

function InfoItem({ children, icon, label }: { children: string; icon: ReactNode; label: string }) {
  return (
    <div className="rounded-2xl border border-border bg-background p-3">
      <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {icon}
        {label}
      </dt>
      <dd className="mt-1 min-w-0 break-words font-medium">{children}</dd>
    </div>
  );
}

function mapNames(items: Array<{ id: string; name: string }>): Lookup {
  return new Map(items.map((item) => [item.id, item.name]));
}

function labelFor(lookup: Lookup, id: string, fallback: string) {
  return lookup.get(id) ?? fallback;
}

function formatPrice(priceFen: number, currency: string) {
  const normalizedCurrency = currency?.toUpperCase() || "CNY";
  const amount = (priceFen / 100).toFixed(priceFen % 100 === 0 ? 0 : 2);
  return normalizedCurrency === "CNY" ? `CNY ${amount}` : `${amount} ${normalizedCurrency}`;
}
