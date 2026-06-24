import Link from "next/link";
import { Bell, CreditCard, Download, MessageSquareText, PenLine, Sparkles, TriangleAlert, UsersRound } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { ProfileForm } from "@/components/profile/profile-form";

const copy = {
  eyebrow: "\u4e2a\u4eba\u4e2d\u5fc3",
  title: "\u5b66\u751f\u8d44\u6599\u4e0e\u8bfe\u7a0b\u7ed1\u5b9a",
  intro:
    "\u7ef4\u62a4\u5b66\u6821\u3001\u4e13\u4e1a\u548c\u5e74\u7ea7\u540e\uff0c\u540e\u7eed\u8bfe\u7a0b\u5305\u3001\u5237\u9898\u548c\u4e0b\u8f7d\u6743\u9650\u90fd\u53ef\u4ee5\u56f4\u7ed5\u5f53\u524d\u8d26\u53f7\u8fdb\u884c\u7cbe\u51c6\u5339\u914d\u3002",
  shortcuts: "\u5b66\u4e60\u8bb0\u5f55",
};

const shortcuts = [
  { href: "/me/wrong-questions", label: "\u9519\u9898\u672c", description: "\u67e5\u770b\u9519\u9898\u548c\u8584\u5f31\u8bfe\u7a0b", icon: TriangleAlert },
  { href: "/me/points", label: "\u79ef\u5206", description: "\u67e5\u770b\u4f59\u989d\u548c\u79ef\u5206\u6d41\u6c34", icon: Sparkles },
  { href: "/me/membership", label: "\u4f1a\u5458", description: "\u67e5\u770b\u4f1a\u5458\u6743\u76ca\u548c\u5230\u671f\u65f6\u95f4", icon: CreditCard },
  { href: "/me/downloads", label: "\u4e0b\u8f7d\u8bb0\u5f55", description: "\u67e5\u770b\u5df2\u6210\u529f\u4e0b\u8f7d\u7684\u8d44\u6599", icon: Download },
  { href: "/me/notifications", label: "\u901a\u77e5", description: "\u5ba1\u6838\u7ed3\u679c\u548c\u7cfb\u7edf\u6d88\u606f", icon: Bell },
  { href: "/me/forum", label: "\u6211\u7684\u8ba8\u8bba", description: "\u8ffd\u8e2a\u5e16\u5b50\u548c\u56de\u590d\u5ba1\u6838", icon: MessageSquareText },
  { href: "/me/wiki", label: "\u6211\u7684 Wiki", description: "\u8ffd\u8e2a\u8bcd\u6761\u6295\u7a3f\u548c\u4fee\u8ba2\u63d0\u6848", icon: PenLine },
  { href: "/me/relations", label: "\u5173\u7cfb\u7ba1\u7406", description: "\u67e5\u770b\u5173\u6ce8\u3001\u7c89\u4e1d\u548c\u4e92\u5173\u597d\u53cb", icon: UsersRound },
  { href: "/wiki", label: "Wiki \u521b\u4f5c\u8005\u7533\u8bf7", description: "\u63d0\u4ea4\u8bd5\u7a3f\u5e76\u67e5\u770b\u5ba1\u6838\u72b6\u6001", icon: PenLine },
];

export default function MePage() {
  return (
    <SiteShell>
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <p className="text-sm font-medium text-primary">{copy.eyebrow}</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">{copy.title}</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
      </section>
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <h2 className="text-lg font-semibold tracking-tight">{copy.shortcuts}</h2>
        <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {shortcuts.map((item) => {
            const Icon = item.icon;
            return (
              <Link
                className="rounded-2xl border border-border bg-background p-4 transition hover:border-primary/60 hover:bg-card"
                href={item.href}
                key={item.href}
              >
                <Icon className="size-5 text-primary" aria-hidden="true" />
                <h3 className="mt-3 font-semibold">{item.label}</h3>
                <p className="mt-1 text-sm leading-6 text-muted-foreground">{item.description}</p>
              </Link>
            );
          })}
        </div>
      </section>
      <ProfileForm />
    </SiteShell>
  );
}
