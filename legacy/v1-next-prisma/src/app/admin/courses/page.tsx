import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";
import { getAdminUser } from "@/lib/admin";
import { listCourses } from "@/services/course-service";

export default async function AdminCoursesPage() {
  const admin = await getAdminUser();
  if (!admin) {
    return (
      <PageShell title="无权访问课程管理" eyebrow="403">
        <Link href="/login" className="text-sm font-semibold text-brand">
          去登录
        </Link>
      </PageShell>
    );
  }

  const courses = await listCourses();

  return (
    <PageShell title="课程管理" eyebrow="Admin" description="管理 published 课程基础信息。">
      <Link
        href="/admin/courses/new"
        className="mb-4 inline-flex h-10 items-center rounded-md bg-brand px-4 text-sm font-semibold text-white"
      >
        新增课程
      </Link>
      <div className="grid gap-3">
        {courses.map((course) => (
          <article key={course.id} className="rounded-lg border border-line bg-white p-4 shadow-soft">
            <h2 className="font-semibold text-ink">{course.name}</h2>
            <p className="mt-1 text-sm text-muted">{course.description}</p>
          </article>
        ))}
      </div>
    </PageShell>
  );
}

