import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";

export default function NotFound() {
  return (
    <PageShell title="页面不存在" eyebrow="404">
      <div className="max-w-2xl rounded-lg border border-line bg-white p-6 shadow-soft">
        <p className="text-sm leading-6 text-muted">
          当前页面不存在，或该内容尚未发布。
        </p>
        <Link
          href="/courses"
          className="mt-5 inline-flex h-10 items-center rounded-md bg-brand px-4 text-sm font-semibold text-white"
        >
          返回课程库
        </Link>
      </div>
    </PageShell>
  );
}
