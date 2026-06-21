import type { Major, School } from "@/types";

type CourseFilterFormProps = {
  selectedSchoolId?: string;
  selectedMajorId?: string;
  selectedGrade?: string;
  schools: School[];
  majors: Major[];
  grades: string[];
};

export function CourseFilterForm({
  selectedSchoolId,
  selectedMajorId,
  selectedGrade,
  schools,
  majors,
  grades,
}: CourseFilterFormProps) {
  return (
    <form
      action="/courses"
      className="mb-6 grid gap-3 rounded-lg border border-line bg-white p-4 shadow-soft md:grid-cols-[1fr_1fr_1fr_auto]"
    >
      <label className="grid gap-1 text-sm font-semibold text-ink">
        学校
        <select
          name="schoolId"
          defaultValue={selectedSchoolId ?? "henu"}
          className="h-11 rounded-md border border-line bg-white px-3 text-sm font-normal text-ink focus-ring"
        >
          {schools.map((school) => (
            <option key={school.id} value={school.id}>
              {school.name}
            </option>
          ))}
        </select>
      </label>
      <label className="grid gap-1 text-sm font-semibold text-ink">
        专业
        <select
          name="majorId"
          defaultValue={selectedMajorId ?? ""}
          className="h-11 rounded-md border border-line bg-white px-3 text-sm font-normal text-ink focus-ring"
        >
          <option value="">全部专业</option>
          {majors.map((major) => (
            <option key={major.id} value={major.id}>
              {major.name}
            </option>
          ))}
        </select>
      </label>
      <label className="grid gap-1 text-sm font-semibold text-ink">
        年级
        <select
          name="grade"
          defaultValue={selectedGrade ?? ""}
          className="h-11 rounded-md border border-line bg-white px-3 text-sm font-normal text-ink focus-ring"
        >
          <option value="">全部年级</option>
          {grades.map((grade) => (
            <option key={grade} value={grade}>
              {grade}
            </option>
          ))}
        </select>
      </label>
      <button
        type="submit"
        className="h-11 self-end rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
      >
        筛选课程
      </button>
    </form>
  );
}
