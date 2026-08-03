import { MessageCircle, UsersRound } from "lucide-react";
import { MomentFeed } from "@/components/moment/moment-feed";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { getApi, type Moment } from "@/lib/api";

const copy = {
  eyebrow: "\u5b66\u4e60\u52a8\u6001",
  title: "\u8bb0\u5f55\u590d\u4e60\u8fdb\u5ea6\u548c\u8bfe\u7a0b\u7ecf\u9a8c",
  intro:
    "\u52a8\u6001\u6d41\u652f\u6301\u516c\u5f00\u548c\u4e92\u5173\u53ef\u89c1\u4e24\u79cd\u8303\u56f4\u3002\u670d\u52a1\u7aef\u4f1a\u5904\u7406\u4e92\u5173\u3001\u5c4f\u853d\u548c\u70b9\u8d5e\u5e42\u7b49\uff0c\u524d\u7aef\u53ea\u8d1f\u8d23\u5c55\u793a\u548c\u53d1\u8d77\u64cd\u4f5c\u3002",
  visible: "\u53ef\u89c1\u52a8\u6001",
  scope: "\u516c\u5f00 / \u4e92\u5173",
  unavailable: "\u52a8\u6001\u6d41\u6682\u65f6\u4e0d\u53ef\u7528",
};

async function loadMoments() {
  try {
    const response = await getApi<{ moments: Moment[] }>("/moments");
    return { error: "", moments: response.data.moments };
  } catch (error) {
    return { error: error instanceof Error ? error.message : copy.unavailable, moments: [] as Moment[] };
  }
}

export default async function MomentsPage() {
  const { error, moments } = await loadMoments();

  return (
    <SiteShell>
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0">
            <Badge tone="success">{copy.eyebrow}</Badge>
            <h1 className="mt-4 break-words text-3xl font-semibold tracking-tight sm:text-4xl">{copy.title}</h1>
            <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
          </div>
          <div className="grid shrink-0 gap-2 text-sm text-muted-foreground sm:grid-cols-2 lg:min-w-72">
            <div className="rounded-2xl border border-border bg-background px-4 py-3">
              <MessageCircle className="mb-2 size-4 text-primary" aria-hidden="true" />
              <strong className="block text-lg text-foreground">{moments.length}</strong>
              {copy.visible}
            </div>
            <div className="rounded-2xl border border-border bg-background px-4 py-3">
              <UsersRound className="mb-2 size-4 text-primary" aria-hidden="true" />
              {copy.scope}
            </div>
          </div>
        </div>
      </section>

      {error ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{error}</p> : null}
      <MomentFeed initialMoments={moments} />
    </SiteShell>
  );
}
