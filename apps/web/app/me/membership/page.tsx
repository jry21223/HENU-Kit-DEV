"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Clock, CreditCard } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { apiBaseUrl, type MembershipPlan, type MembershipRow } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

type MembershipPayload = {
  memberships: MembershipRow[];
  current?: MembershipRow | null;
};

const copy = {
  back: "返回个人中心",
  login: "去登录",
  eyebrow: "会员权益",
  title: "我的会员与可用套餐",
  intro: "当前会员状态以后端记录为准。内测阶段管理员可以手动赠送或撤销会员，正式购买链路会在支付闭环完成后接入。",
  loading: "正在加载会员信息...",
  current: "当前会员",
  noCurrent: "当前没有有效会员。",
  activeMemberships: "有效会员记录",
  plans: "可用套餐",
  manualDelivery: "内测阶段请通过管理员手动授权或课程包订单交付。",
  empty: "暂无有效会员。",
  fallbackError: "会员信息暂时不可用",
};

export default function MyMembershipPage() {
  const [memberships, setMemberships] = useState<MembershipRow[]>([]);
  const [current, setCurrent] = useState<MembershipRow | null>(null);
  const [plans, setPlans] = useState<MembershipPlan[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const planByCode = useMemo(() => Object.fromEntries(plans.map((plan) => [plan.code, plan])), [plans]);

  useEffect(() => {
    async function loadMembership() {
      setLoading(true);
      setError("");
      try {
        const [membershipPayload, planPayload] = await Promise.all([
          request<MembershipPayload>("/me/membership"),
          request<{ plans: MembershipPlan[] }>("/membership/plans"),
        ]);
        setMemberships(membershipPayload.data?.memberships ?? []);
        setCurrent(membershipPayload.data?.current ?? null);
        setPlans(planPayload.data?.plans ?? []);
      } catch (err) {
        setError(err instanceof Error ? err.message : copy.fallbackError);
      } finally {
        setLoading(false);
      }
    }

    void loadMembership();
  }, []);

  return (
    <SiteShell>
      <nav className="flex flex-wrap items-center justify-between gap-3 text-sm">
        <Link className="font-semibold text-primary" href="/me">
          {copy.back}
        </Link>
        <Link href="/login">{copy.login}</Link>
      </nav>

      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
        <Badge tone="success">{copy.eyebrow}</Badge>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight">{copy.title}</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">{copy.intro}</p>
      </section>

      {loading ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">{copy.loading}</p> : null}
      {error ? (
        <section className="rounded-2xl border border-border bg-card p-4 text-sm text-muted-foreground">
          <p>{error}</p>
          <Link className="mt-3 inline-flex rounded-lg bg-primary px-3 py-2 text-primary-foreground" href="/login">
            {copy.login}
          </Link>
        </section>
      ) : null}

      {!loading && !error ? (
        <>
          <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
            <div className="flex items-start gap-3">
              <span className="grid size-11 flex-none place-items-center rounded-2xl bg-primary/10 text-primary">
                <CreditCard className="size-5" aria-hidden="true" />
              </span>
              <div className="min-w-0">
                <p className="text-sm text-muted-foreground">{copy.current}</p>
                {current ? (
                  <>
                    <h2 className="mt-1 text-2xl font-semibold tracking-tight">{current.plan?.name ?? planByCode[current.membership.planCode]?.name ?? current.membership.planCode}</h2>
                    <p className="mt-2 text-sm text-muted-foreground">到期时间：{formatExpiry(current.membership.expiresAt)}</p>
                  </>
                ) : (
                  <p className="mt-2 text-sm text-muted-foreground">{copy.noCurrent}</p>
                )}
              </div>
            </div>
          </section>

          <section className="grid gap-4 lg:grid-cols-[0.95fr_1.05fr]">
            <div className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
              <h2 className="text-lg font-semibold tracking-tight">{copy.activeMemberships}</h2>
              <div className="mt-4 space-y-3">
                {memberships.map((row) => (
                  <article key={row.membership.id} className="rounded-2xl border border-border bg-background p-4">
                    <div className="flex items-start gap-3">
                      <CheckCircle2 className="mt-0.5 size-5 flex-none text-emerald-700" aria-hidden="true" />
                      <div className="min-w-0">
                        <h3 className="break-words text-sm font-medium">{row.plan?.name ?? planByCode[row.membership.planCode]?.name ?? row.membership.planCode}</h3>
                        <p className="mt-1 text-xs text-muted-foreground">来源：{row.membership.source || "system"}</p>
                        <p className="mt-1 text-xs text-muted-foreground">到期：{formatExpiry(row.membership.expiresAt)}</p>
                      </div>
                    </div>
                  </article>
                ))}
                {memberships.length === 0 ? <p className="rounded-2xl bg-muted/60 p-4 text-sm text-muted-foreground">{copy.empty}</p> : null}
              </div>
            </div>

            <div className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
              <h2 className="text-lg font-semibold tracking-tight">{copy.plans}</h2>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{copy.manualDelivery}</p>
              <div className="mt-4 grid gap-3">
                {plans.map((plan) => (
                  <article key={plan.id} className="rounded-2xl border border-border bg-background p-4">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <h3 className="text-sm font-medium">{plan.name}</h3>
                        <p className="mt-1 text-xs text-muted-foreground">{plan.code}</p>
                      </div>
                      <span className="rounded-full bg-muted px-3 py-1 text-xs text-muted-foreground">{formatPrice(plan.priceFen)}</span>
                    </div>
                    <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground">
                      <Clock className="size-3.5" aria-hidden="true" />
                      权益以后台配置为准
                    </div>
                  </article>
                ))}
                {plans.length === 0 ? <p className="rounded-2xl bg-muted/60 p-4 text-sm text-muted-foreground">暂无已发布会员套餐。</p> : null}
              </div>
            </div>
          </section>
        </>
      ) : null}
    </SiteShell>
  );
}

async function request<T>(path: string): Promise<Envelope<T>> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    credentials: "include",
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `API request failed with ${response.status}`);
  }
  return payload;
}

function formatExpiry(value?: string | null) {
  if (!value) return "长期有效";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatPrice(priceFen: number) {
  if (!Number.isFinite(priceFen) || priceFen <= 0) return "免费 / 手动授权";
  return `¥${(priceFen / 100).toFixed(2)}`;
}
