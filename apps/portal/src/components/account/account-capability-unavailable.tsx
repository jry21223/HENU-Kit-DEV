import Link from "next/link";

type AccountCapabilityUnavailableProps = {
  index: string;
  title: string;
  englishTitle: string;
};

// These routes used to mutate an in-memory accountStore. Until their owner
// endpoints and write flows are delivered, they must fail closed instead of
// presenting a successful-looking browser-only account.
export function AccountCapabilityUnavailable({
  index,
  title,
  englishTitle,
}: AccountCapabilityUnavailableProps) {
  return (
    <section data-account-capability-state="unavailable" className="max-w-2xl border border-ink p-6 md:p-8">
      <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
        <span className="text-accent">{index}</span>
        <span className="mx-2">/</span>
        {englishTitle}
      </p>
      <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">{title}</h1>
      <p className="mt-6 font-mono text-xs tracking-[0.16em] text-accent">ACCOUNT PORTFOLIO CAPABILITY PENDING</p>
      <p className="mt-4 text-sm leading-7 text-ink/65">
        该功能的持久化读写尚未完成。当前页面不会展示或修改任何会话内数据，也不会把本地状态作为真实账户结果。
      </p>
      <Link
        href="/account"
        className="mt-7 inline-flex border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
      >
        返回账户概览
      </Link>
    </section>
  );
}
