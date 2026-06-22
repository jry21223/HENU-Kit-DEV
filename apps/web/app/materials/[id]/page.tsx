import Link from "next/link";
import { ArrowLeft, Download, FileText, LockKeyhole, ShieldCheck, Sparkles } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button-link";
import { Material, apiBaseUrl, getApi } from "@/lib/api";
import { formatFileSize, labelAccessLevel, labelMaterialType } from "@/lib/presentation";

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
  const isPdf = material?.fileName?.toLowerCase().endsWith(".pdf") ?? false;

  return (
    <SiteShell>
      <nav className="flex items-center justify-between gap-3 text-sm">
        <ButtonLink href={material ? `/courses/${material.courseId}` : "/courses"} variant="secondary">
          <ArrowLeft className="mr-2 size-4" aria-hidden="true" />
          返回课程
        </ButtonLink>
        <Link className="rounded-lg px-3 py-2 text-muted-foreground hover:bg-muted hover:text-foreground" href="/courses">
          课程库
        </Link>
      </nav>

      {error ? (
        <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p>
      ) : null}

      {material ? (
        <article className="grid gap-5 lg:grid-cols-[1fr_320px]">
          <section className="min-w-0 rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone="success">资料保障</Badge>
              <Badge tone="muted">{labelMaterialType(material.type)}</Badge>
              <Badge tone="muted">{labelAccessLevel(material.accessLevel)}</Badge>
            </div>
            <h1 className="mt-4 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{material.title}</h1>
            <p className="mt-4 break-words text-sm leading-6 text-muted-foreground">{material.description || "暂无资料说明"}</p>

            <dl className="mt-6 grid gap-3 text-sm sm:grid-cols-2">
              <div className="rounded-2xl border border-border bg-background p-4">
                <dt className="text-xs text-muted-foreground">文件名</dt>
                <dd className="mt-1 break-words font-medium">{material.fileName || "资料文件"}</dd>
              </div>
              <div className="rounded-2xl border border-border bg-background p-4">
                <dt className="text-xs text-muted-foreground">文件大小</dt>
                <dd className="mt-1 font-medium">{formatFileSize(material.fileSize)}</dd>
              </div>
            </dl>

            <div className="mt-6 rounded-2xl border border-border bg-background p-4">
              <h2 className="text-base font-semibold tracking-tight">资料预览</h2>
              <p className="mt-3 whitespace-pre-wrap break-words text-sm leading-6 text-muted-foreground">
                {material.previewContent || "暂无预览内容，下载前可参考标题、类型和权限信息确认资料。"}
              </p>
            </div>
          </section>

          <aside className="grid gap-5">
            <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
              <div className="flex items-center gap-2">
                <FileText className="size-5 text-primary" aria-hidden="true" />
                <h2 className="text-lg font-semibold tracking-tight">下载前说明</h2>
              </div>
              <div className="mt-4 grid gap-3 text-sm">
                <div className="flex gap-3 rounded-2xl bg-muted p-3">
                  <ShieldCheck className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" />
                  <div className="min-w-0">
                    <p className="font-medium">{isPdf ? "PDF 稳定供应" : "资料稳定下载"}</p>
                    <p className="mt-1 leading-5 text-muted-foreground">
                      {isPdf ? "PDF 入口按课程维护，优先保证可下载和可阅读。" : "资料入口按课程维护，优先保证文件可下载。"}
                    </p>
                  </div>
                </div>
                <div className="flex gap-3 rounded-2xl bg-muted p-3">
                  <Sparkles className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" />
                  <div className="min-w-0">
                    <p className="font-medium">{isPdf ? "轻水印不影响阅读" : "文件按原格式提供"}</p>
                    <p className="mt-1 leading-5 text-muted-foreground">
                      {isPdf ? "水印用于资料来源标识，不遮挡正文阅读。" : "非 PDF 资料保持原始文件格式，下载后按对应工具打开。"}
                    </p>
                  </div>
                </div>
                <div className="flex gap-3 rounded-2xl bg-muted p-3">
                  <LockKeyhole className="mt-0.5 size-4 shrink-0 text-primary" aria-hidden="true" />
                  <div className="min-w-0">
                    <p className="font-medium">下载权限由服务端校验</p>
                    <p className="mt-1 leading-5 text-muted-foreground">点击下载后会按当前账号权限完成校验。</p>
                  </div>
                </div>
              </div>
              <a
                className="mt-5 inline-flex w-full items-center justify-center rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
                href={`${apiBaseUrl()}/materials/${material.id}/download`}
              >
                {isPdf ? "下载 PDF" : "下载资料"}
                <Download className="ml-2 size-4" aria-hidden="true" />
              </a>
            </section>
          </aside>
        </article>
      ) : null}
    </SiteShell>
  );
}
