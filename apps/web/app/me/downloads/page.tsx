"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { MaterialDownload, apiBaseUrl } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

const copy = {
  backToCourses: "\u8fd4\u56de\u8bfe\u7a0b\u5e93",
  loginStatus: "\u767b\u5f55\u72b6\u6001",
  me: "\u4e2a\u4eba\u4e2d\u5fc3",
  title: "\u6211\u7684\u4e0b\u8f7d\u8bb0\u5f55",
  intro:
    "\u8fd9\u91cc\u53ea\u5c55\u793a\u5f53\u524d\u767b\u5f55\u8d26\u53f7\u7684\u6210\u529f\u4e0b\u8f7d\u8bb0\u5f55\u3002\u4e0b\u8f7d\u8bb0\u5f55\u4ec5\u81ea\u5df1\u53ef\u89c1\uff0c\u4e0d\u5305\u542b\u654f\u611f\u4fe1\u606f\u3002",
  loading: "\u6b63\u5728\u52a0\u8f7d\u4e0b\u8f7d\u8bb0\u5f55...",
  login: "\u53bb\u767b\u5f55",
  archived: "\u8d44\u6599\u5df2\u5f52\u6863\u6216\u4e0d\u53ef\u89c1",
  noDescription: "\u6682\u65e0\u8d44\u6599\u8bf4\u660e",
  downloadedAt: "\u4e0b\u8f7d\u65f6\u95f4",
  empty: "\u6682\u65e0\u4e0b\u8f7d\u8bb0\u5f55\u3002",
  fallbackError: "\u4e0b\u8f7d\u8bb0\u5f55\u6682\u65f6\u4e0d\u53ef\u7528",
};

export default function MyDownloadsPage() {
  const [downloads, setDownloads] = useState<MaterialDownload[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    async function loadDownloads() {
      setLoading(true);
      setError("");
      try {
        const response = await fetch(`${apiBaseUrl()}/me/downloads`, {
          credentials: "include",
        });
        const payload = (await response.json().catch(() => ({}))) as Envelope<{ downloads: MaterialDownload[] }>;
        if (!response.ok || payload.code !== 0) {
          throw new Error(payload.message || "网络不太顺畅，请检查网络后重试");
        }
        setDownloads(payload.data?.downloads ?? []);
      } catch (err) {
        setError(err instanceof Error ? err.message : copy.fallbackError);
      } finally {
        setLoading(false);
      }
    }

    void loadDownloads();
  }, []);

  return (
    <main className="min-h-screen px-5 py-6 sm:px-8">
      <section className="mx-auto max-w-4xl">
        <nav className="flex items-center justify-between text-sm">
          <Link className="font-semibold text-sage" href="/courses">
            {copy.backToCourses}
          </Link>
          <Link href="/login">{copy.loginStatus}</Link>
        </nav>

        <header className="mt-8">
          <p className="text-sm font-medium text-sage">{copy.me}</p>
          <h1 className="mt-2 text-3xl font-semibold">{copy.title}</h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-600">{copy.intro}</p>
        </header>

        {loading ? <p className="mt-6 rounded-md border border-line bg-white p-4 text-sm text-slate-600">{copy.loading}</p> : null}
        {error ? (
          <div className="mt-6 rounded-md border border-line bg-white p-4 text-sm text-slate-600">
            <p>{error}</p>
            <Link className="mt-3 inline-flex rounded-md bg-sage px-3 py-2 text-white" href="/login">
              {copy.login}
            </Link>
          </div>
        ) : null}

        {!loading && !error ? (
          <div className="mt-6 grid gap-3">
            {downloads.map((download) => (
              <Link
                key={download.id}
                className="rounded-lg border border-line bg-white p-4 shadow-sm transition hover:border-sage"
                href={`/materials/${download.materialId}`}
              >
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <h2 className="font-semibold">{download.material?.title ?? copy.archived}</h2>
                    <p className="mt-1 text-sm leading-6 text-slate-600">
                      {download.material?.description || download.material?.previewContent || copy.noDescription}
                    </p>
                  </div>
                  <span className="rounded-md bg-paper px-2 py-1 text-xs text-slate-600">{accessLevelLabel(download.accessLevel)}</span>
                </div>
                <p className="mt-3 text-xs text-slate-500">
                  {copy.downloadedAt}: {formatDate(download.downloadedAt)}
                </p>
              </Link>
            ))}
            {downloads.length === 0 ? <p className="rounded-md border border-line bg-white p-4 text-sm text-slate-600">{copy.empty}</p> : null}
          </div>
        ) : null}
      </section>
    </main>
  );
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function accessLevelLabel(level: string) {
  const labels: Record<string, string> = {
    free: "公开",
    login_required: "登录后下载",
    paid: "付费",
  };
  return labels[level] ?? level;
}
