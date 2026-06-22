import Link from "next/link";
import { Material, apiBaseUrl, getApi } from "@/lib/api";

type PageProps = {
  params: Promise<{ id: string }>;
};

async function loadMaterial(id: string) {
  try {
    const response = await getApi<{ material: Material }>(`/materials/${id}`);
    return { material: response.data.material, error: "" };
  } catch (error) {
    return { material: null as Material | null, error: error instanceof Error ? error.message : "API unavailable" };
  }
}

export default async function MaterialDetailPage({ params }: PageProps) {
  const { id } = await params;
  const { material, error } = await loadMaterial(id);

  return (
    <main className="min-h-screen px-5 py-6 sm:px-8">
      <section className="mx-auto max-w-3xl">
        <nav className="flex items-center justify-between text-sm">
          <Link className="font-semibold text-sage" href={material ? `/courses/${material.courseId}` : "/courses"}>
            返回课程
          </Link>
          <Link href="/courses">课程库</Link>
        </nav>

        {error ? <p className="mt-6 rounded-md border border-line bg-white p-4 text-sm text-slate-600">{error}</p> : null}

        {material ? (
          <article className="mt-8 rounded-lg border border-line bg-white p-6 shadow-sm">
            <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500">
              <span className="rounded-md bg-paper px-2 py-1">{material.type}</span>
              <span className="rounded-md bg-paper px-2 py-1">{material.accessLevel}</span>
              <span className="rounded-md bg-paper px-2 py-1">{material.status}</span>
            </div>
            <h1 className="mt-4 text-3xl font-semibold">{material.title}</h1>
            <p className="mt-4 text-sm leading-6 text-slate-600">{material.description || "暂无资料说明"}</p>

            <div className="mt-6 rounded-md border border-line bg-paper p-4">
              <h2 className="text-sm font-semibold">预览</h2>
              <p className="mt-2 whitespace-pre-wrap text-sm leading-6 text-slate-700">{material.previewContent || "暂无预览内容"}</p>
            </div>

            <dl className="mt-6 grid gap-3 text-sm sm:grid-cols-2">
              <div className="rounded-md border border-line p-3">
                <dt className="text-slate-500">文件名</dt>
                <dd className="mt-1 font-medium">{material.fileName || "未设置"}</dd>
              </div>
              <div className="rounded-md border border-line p-3">
                <dt className="text-slate-500">大小</dt>
                <dd className="mt-1 font-medium">{material.fileSize ? `${Math.ceil(material.fileSize / 1024)} KB` : "未知"}</dd>
              </div>
            </dl>

            <a
              className="mt-6 inline-flex rounded-md bg-sage px-4 py-2 text-sm font-medium text-white"
              href={`${apiBaseUrl()}/materials/${material.id}/download`}
            >
              通过服务端下载
            </a>
            <p className="mt-3 text-xs leading-5 text-slate-500">下载权限、paid 解锁和文件路径安全都由 Go API 校验。</p>
          </article>
        ) : null}
      </section>
    </main>
  );
}
