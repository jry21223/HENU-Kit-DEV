import Link from "next/link";
import {
  getCourseMajors,
  getMaterialCount,
  getPublishedMaterialsByCourse,
} from "@/constants/mock-data";
import type { Course } from "@/types";

type CourseCardProps = {
  course: Course;
};

export function CourseCard({ course }: CourseCardProps) {
  const courseMaterials = getPublishedMaterialsByCourse(course.id);
  const materialCount = course.materialCount ?? getMaterialCount(course.id);
  const hasMockPaper =
    course.hasMockPaper ?? courseMaterials.some((item) => item.type === "mock_paper");
  const hasAnswer =
    course.hasAnswer ?? courseMaterials.some((item) => item.type === "answer");
  const majors = getCourseMajors(course).map((major) => major.name);

  return (
    <article className="rounded-lg border border-line bg-white p-5 shadow-soft">
      <div className="flex min-h-[168px] flex-col justify-between gap-5">
        <div>
          <div className="mb-3 flex flex-wrap gap-2 text-xs">
            {course.grades.map((grade) => (
              <span
                key={grade}
                className="rounded-md border border-line bg-panel px-2.5 py-1 font-semibold text-muted"
              >
                {grade}
              </span>
            ))}
          </div>
          <h3 className="text-lg font-semibold text-ink">{course.name}</h3>
          <p className="mt-2 line-clamp-2 text-sm leading-6 text-muted">
            {course.description}
          </p>
        </div>
        <div>
          <dl className="grid gap-2 text-xs text-muted">
            <div className="flex gap-2">
              <dt className="shrink-0 font-semibold text-ink">专业</dt>
              <dd>{majors.join(" / ")}</dd>
            </div>
            <div className="flex flex-wrap gap-x-4 gap-y-1">
              <span>资料 {materialCount} 份</span>
              <span>{hasMockPaper ? "含模拟卷" : "暂无模拟卷"}</span>
              <span>{hasAnswer ? "含答案解析" : "暂无答案解析"}</span>
              <span>{course.supportsPractice ? "支持刷题" : "刷题即将上线"}</span>
            </div>
          </dl>
          <Link
            href={`/courses/${course.id}`}
            className="mt-4 inline-flex h-10 w-full items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring sm:w-auto"
          >
            进入课程
          </Link>
        </div>
      </div>
    </article>
  );
}
