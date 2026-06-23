import { SiteShell } from "@/components/layout/site-shell";
import { ProfileForm } from "@/components/profile/profile-form";

const copy = {
  eyebrow: "\u4e2a\u4eba\u4e2d\u5fc3",
  title: "\u5b66\u751f\u8d44\u6599\u4e0e\u8bfe\u7a0b\u7ed1\u5b9a",
  intro:
    "\u7ef4\u62a4\u5b66\u6821\u3001\u4e13\u4e1a\u548c\u5e74\u7ea7\u540e\uff0c\u540e\u7eed\u8bfe\u7a0b\u5305\u3001\u5237\u9898\u548c\u4e0b\u8f7d\u6743\u9650\u90fd\u53ef\u4ee5\u56f4\u7ed5\u5f53\u524d\u8d26\u53f7\u8fdb\u884c\u7cbe\u51c6\u5339\u914d\u3002",
};

export default function MePage() {
  return (
    <SiteShell>
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">{copy.title}</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
      </section>
      <ProfileForm />
    </SiteShell>
  );
}
