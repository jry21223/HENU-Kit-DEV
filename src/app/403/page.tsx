import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell";

export default function ForbiddenPage() {
  return (
    <PageShell title="无权访问" eyebrow="403">
      <div className="max-w-2xl rounded-lg border border-line bg-white p-6 shadow-soft">
        <p className="text-sm leading-6 text-muted">
          当前账号没有访问该页面的权限。管理员后台必须通过服务端角色校验。
        </p>
        <Link
          href="/"
          className="mt-5 inline-flex h-10 items-center rounded-md bg-brand px-4 text-sm font-semibold text-white"
        >
          返回首页
        </Link>
      </div>
    </PageShell>
  );
}

