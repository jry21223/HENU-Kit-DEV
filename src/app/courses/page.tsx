import { CourseCard } from "@/components/course/course-card";
import { CourseFilterForm } from "@/components/course/course-filter-form";
import { PageShell } from "@/components/layout/page-shell";
import { listGrades, listMajors, listSchools } from "@/services/catalog-service";
import { listCourses } from "@/services/course-service";

type CoursesPageProps = {
  searchParams?: Promise<{
    schoolId?: string;
    majorId?: string;
    grade?: string;
  }>;
};

export default async function CoursesPage({ searchParams }: CoursesPageProps) {
  const resolvedSearchParams = await searchParams;
  const selectedSchoolId = resolvedSearchParams?.schoolId;
  const selectedMajorId = resolvedSearchParams?.majorId;
  const selectedGrade = resolvedSearchParams?.grade;

  const [filteredCourses, schools, majors, grades] = await Promise.all([
    listCourses({
      schoolId: selectedSchoolId,
      majorId: selectedMajorId,
      grade: selectedGrade,
    }),
    listSchools(),
    listMajors(),
    listGrades(),
  ]);

  return (
    <PageShell
      eyebrow="课程库"
      title="按专业和年级筛选课程"
      description="按学校、专业和年级筛选课程，查看资料与在线刷题入口。"
    >
      <CourseFilterForm
        selectedSchoolId={selectedSchoolId}
        selectedMajorId={selectedMajorId}
        selectedGrade={selectedGrade}
        schools={schools}
        majors={majors}
        grades={grades}
      />

      {filteredCourses.length > 0 ? (
        <div className="grid gap-4 md:grid-cols-2">
          {filteredCourses.map((course) => (
            <CourseCard key={course.id} course={course} />
          ))}
        </div>
      ) : (
        <div className="rounded-lg border border-line bg-white p-6 text-sm text-muted shadow-soft">
          暂无匹配课程。可以调整专业或年级筛选条件。
        </div>
      )}
    </PageShell>
  );
}
