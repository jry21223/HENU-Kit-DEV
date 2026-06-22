import Link from "next/link";
import { notFound } from "next/navigation";
import { MaterialCard } from "@/components/material/material-card";
import { PageShell } from "@/components/layout/page-shell";
import {
  getCollegeName,
  getCourseMajors,
  getSchoolName,
} from "@/constants/mock-data";
import { getCourseById } from "@/services/course-service";
import { listMaterialsByCourse } from "@/services/material-service";

type CourseDetailPageProps = {
  params: Promise<{
    id: string;
  }>;
};

export default async function CourseDetailPage({ params }: CourseDetailPageProps) {
  const { id } = await params;
  const course = await getCourseById(id);

  if (!course) {
    notFound();
  }

  const materials = await listMaterialsByCourse(course.id);
  const majors = getCourseMajors(course).map((major) => major.name);

  return (
    <PageShell
      eyebrow="课程详情"
      title={course.name}
      description={course.description}
    >
      <section className="grid gap-5 lg:grid-cols-[0.9fr_1.1fr]">
        <div className="rounded-lg border border-line bg-white p-6 shadow-soft">
          <h2 className="text-lg font-semibold text-ink">适用范围</h2>
          <dl className="mt-4 grid gap-3 text-sm">
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">学校</dt>
              <dd className="font-semibold text-ink">{getSchoolName(course.schoolId)}</dd>
            </div>
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">学院</dt>
              <dd className="font-semibold text-ink">{getCollegeName(course.collegeId)}</dd>
            </div>
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">专业</dt>
              <dd className="text-right font-semibold text-ink">{majors.join(" / ")}</dd>
            </div>
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">年级</dt>
              <dd className="font-semibold text-ink">{course.grades.join(" / ")}</dd>
            </div>
            <div className="flex justify-between gap-3">
              <dt className="text-muted">任课/范围</dt>
              <dd className="text-right font-semibold text-ink">
                {course.teacher ?? "按考试范围整理"}
              </dd>
            </div>
          </dl>
        </div>
        <div className="rounded-lg border border-line bg-white p-6 shadow-soft">
          <h2 className="text-lg font-semibold text-ink">考试范围</h2>
          <p className="mt-3 text-sm leading-6 text-muted">{course.examScope}</p>
          <div className="mt-5 rounded-lg border border-line bg-panel p-4">
            <p className="text-sm font-semibold text-ink">在线刷题</p>
            <p className="mt-2 text-sm leading-6 text-muted">
              支持单选题和判断题练习，提交后可查看对错与解析。
            </p>
            {course.supportsPractice ? (
              <Link
                href={`/courses/${course.id}/practice`}
                className="mt-4 inline-flex h-10 items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
              >
                开始刷题
              </Link>
            ) : (
              <button
                type="button"
                disabled
                className="mt-4 h-10 rounded-md bg-line px-4 text-sm font-semibold text-muted"
              >
                暂无题目
              </button>
            )}
          </div>
        </div>
      </section>

      <section className="mt-8">
        <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h2 className="text-xl font-semibold text-ink">资料列表</h2>
            <p className="mt-1 text-sm text-muted">
              仅展示状态为 published 的资料。
            </p>
          </div>
          <Link
            href="/courses"
            className="inline-flex h-10 items-center rounded-md px-2 text-sm font-semibold text-brand hover:bg-panel focus-ring"
          >
            返回课程库
          </Link>
        </div>
        <div className="grid gap-4">
          {materials.map((material) => (
            <MaterialCard key={material.id} material={material} />
          ))}
        </div>
      </section>
    </PageShell>
  );
}
