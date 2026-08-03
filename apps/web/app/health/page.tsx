import Link from "next/link";
import { getApi } from "@/lib/api";

type VersionData = {
  version: string;
  service: string;
  environment: string;
};

export default async function HealthPage() {
  let result: string;

  try {
    const response = await getApi<VersionData>("/version");
    result = `${response.data.service} 服务正常（版本 ${response.data.version}）`;
  } catch (error) {
    result = error instanceof Error ? error.message : "服务暂时不可用";
  }

  return (
    <main className="min-h-screen px-5 py-6">
      <section className="mx-auto max-w-xl rounded-lg border border-line bg-white p-6 shadow-sm">
        <Link className="text-sm text-sage" href="/">
          返回首页
        </Link>
        <h1 className="mt-6 text-2xl font-semibold">服务状态</h1>
        <p className="mt-3 rounded-md bg-paper p-4 text-sm text-slate-700">{result}</p>
      </section>
    </main>
  );
}
