import Link from "next/link";
import { notFound } from "next/navigation";
import { AccessBadge } from "@/components/material/access-badge";
import { PageShell } from "@/components/layout/page-shell";
import { materialTypeLabels } from "@/constants/enums";
import { getCurrentUser } from "@/lib/auth";
import { getCourseById } from "@/services/course-service";
import { getMaterialById } from "@/services/material-service";
import { userCanAccessMaterial } from "@/services/package-service";

type MaterialDetailPageProps = {
  params: Promise<{
    id: string;
  }>;
};

export default async function MaterialDetailPage({ params }: MaterialDetailPageProps) {
  const { id } = await params;
  const material = await getMaterialById(id);

  if (!material) {
    notFound();
  }

  const course = await getCourseById(material.courseId);
  const isPaid = material.accessLevel === "paid";
  const user = await getCurrentUser();
  const isUnlocked = isPaid && user ? await userCanAccessMaterial(user.id, material.id) : false;

  return (
    <PageShell
      eyebrow={materialTypeLabels[material.type]}
      title={material.title}
      description={material.description}
    >
      <section className="grid gap-5 lg:grid-cols-[1fr_0.8fr]">
        <article className="rounded-lg border border-line bg-white p-6 shadow-soft">
          <div className="mb-5 flex flex-wrap items-center gap-2">
            <AccessBadge accessLevel={material.accessLevel} />
            <span className="rounded-md bg-panel px-2.5 py-1 text-xs font-semibold text-muted">
              {isUnlocked ? "已解锁" : "资料预览"}
            </span>
          </div>
          <h2 className="text-lg font-semibold text-ink">预览内容</h2>
          <p className="mt-3 text-sm leading-7 text-muted">{material.previewContent}</p>
          <div className="mt-6 rounded-lg border border-line bg-panel p-4 text-sm leading-6 text-muted">
            下载权限由服务端校验。paid 资料需要课程复习包或单资料授权。
          </div>
        </article>
        <aside className="rounded-lg border border-line bg-white p-6 shadow-soft">
          <h2 className="text-lg font-semibold text-ink">资料信息</h2>
          <dl className="mt-4 grid gap-3 text-sm">
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">所属课程</dt>
              <dd className="font-semibold text-ink">{course?.name ?? "未知课程"}</dd>
            </div>
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">文件名</dt>
              <dd className="text-right font-semibold text-ink">{material.fileName}</dd>
            </div>
            <div className="flex justify-between gap-3 border-b border-line pb-3">
              <dt className="text-muted">大小</dt>
              <dd className="font-semibold text-ink">{material.fileSize}</dd>
            </div>
            <div className="flex justify-between gap-3">
              <dt className="text-muted">更新</dt>
              <dd className="font-semibold text-ink">{material.updatedAt}</dd>
            </div>
          </dl>
          {isPaid && !isUnlocked ? (
            <Link
              href="/packages"
              className="mt-5 inline-flex h-10 w-full items-center justify-center rounded-md bg-line px-4 text-sm font-semibold text-muted hover:bg-panel focus-ring"
            >
              需要解锁
            </Link>
          ) : (
            <a
              href={`/api/materials/${material.id}/download`}
              className="mt-5 inline-flex h-10 w-full items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
            >
              下载资料
            </a>
          )}
          {course ? (
            <Link
              href={`/courses/${course.id}`}
              className="mt-3 inline-flex h-10 w-full items-center justify-center rounded-md border border-line px-4 text-sm font-semibold text-ink hover:bg-panel focus-ring"
            >
              返回课程详情
            </Link>
          ) : null}
        </aside>
      </section>
    </PageShell>
  );
}
