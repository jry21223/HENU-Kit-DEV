"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Clock, Coins, CreditCard } from "lucide-react";
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

type RedeemPayload = {
  membership: MembershipRow["membership"];
  plan: MembershipPlan;
  pointsBalance: number;
  alreadyRedeemed: boolean;
};

const copy = {
  back: "返回个人中心",
  login: "去登录",
  eyebrow: "会员权益",
  title: "我的会员与积分兑换",
  intro: "当前会员状态以后端记录为准。积分兑换会由服务端扣减积分、写入流水并发放会员，前端不会自行修改权益。",
  loading: "正在加载会员信息...",
  current: "当前会员",
  noCurrent: "当前没有有效会员。",
  activeMemberships: "有效会员记录",
  plans: "可兑换套餐",
  balance: "当前积分",
  empty: "暂无有效会员。",
  noPlans: "暂无已发布会员套餐。",
  fallbackError: "会员信息暂时不可用",
  redeem: "积分兑换",
  redeeming: "兑换中...",
  redeemDone: "会员兑换成功。",
  redeemUnavailable: "暂不可积分兑换",
  insufficient: "积分不足",
  pointsCost: "积分",
  duration: "有效期",
  days: "天",
  source: "来源",
  expires: "到期",
};

export default function MyMembershipPage() {
  const [memberships, setMemberships] = useState<MembershipRow[]>([]);
  const [current, setCurrent] = useState<MembershipRow | null>(null);
  const [plans, setPlans] = useState<MembershipPlan[]>([]);
  const [balance, setBalance] = useState<number>(0);
  const [loading, setLoading] = useState(true);
  const [redeemingCode, setRedeemingCode] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const planByCode = useMemo(() => Object.fromEntries(plans.map((plan) => [plan.code, plan])), [plans]);

  async function loadMembership() {
    setLoading(true);
    setError("");
    try {
      const [membershipPayload, planPayload, pointsPayload] = await Promise.all([
        request<MembershipPayload>("/me/membership"),
        request<{ plans: MembershipPlan[] }>("/membership/plans"),
        request<{ balance: number }>("/me/points"),
      ]);
      setMemberships(membershipPayload.data?.memberships ?? []);
      setCurrent(membershipPayload.data?.current ?? null);
      setPlans(planPayload.data?.plans ?? []);
      setBalance(pointsPayload.data?.balance ?? 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.fallbackError);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadMembership();
  }, []);

  async function redeem(plan: MembershipPlan) {
    setRedeemingCode(plan.code);
    setMessage("");
    setError("");
    try {
      const requestId = typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `${Date.now()}-${plan.code}`;
      const payload = await request<RedeemPayload>("/membership/redeem", {
        method: "POST",
        body: JSON.stringify({ planCode: plan.code, requestId }),
      });
      setBalance(payload.data?.pointsBalance ?? balance);
      setMessage(payload.data?.alreadyRedeemed ? copy.redeemDone : copy.redeemDone);
      await loadMembership();
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.fallbackError);
    } finally {
      setRedeemingCode("");
    }
  }

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
      {message ? <p className="rounded-2xl border border-border bg-card p-4 text-sm text-foreground">{message}</p> : null}

      {!loading && !error ? (
        <>
          <section className="grid gap-4 md:grid-cols-[1fr_0.7fr]">
            <div className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
              <div className="flex items-start gap-3">
                <span className="grid size-11 flex-none place-items-center rounded-2xl bg-primary/10 text-primary">
                  <CreditCard className="size-5" aria-hidden="true" />
                </span>
                <div className="min-w-0">
                  <p className="text-sm text-muted-foreground">{copy.current}</p>
                  {current ? (
                    <>
                      <h2 className="mt-1 text-2xl font-semibold tracking-tight">{current.plan?.name ?? planByCode[current.membership.planCode]?.name ?? current.membership.planCode}</h2>
                      <p className="mt-2 text-sm text-muted-foreground">
                        {copy.expires}：{formatExpiry(current.membership.expiresAt)}
                      </p>
                    </>
                  ) : (
                    <p className="mt-2 text-sm text-muted-foreground">{copy.noCurrent}</p>
                  )}
                </div>
              </div>
            </div>

            <div className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
              <div className="flex items-center gap-3">
                <span className="grid size-11 place-items-center rounded-2xl bg-amber-50 text-amber-700">
                  <Coins className="size-5" aria-hidden="true" />
                </span>
                <div>
                  <p className="text-sm text-muted-foreground">{copy.balance}</p>
                  <p className="text-3xl font-semibold tracking-tight">{balance}</p>
                </div>
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
                        <p className="mt-1 text-xs text-muted-foreground">
                          {copy.source}：{row.membership.source || "system"}
                        </p>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {copy.expires}：{formatExpiry(row.membership.expiresAt)}
                        </p>
                      </div>
                    </div>
                  </article>
                ))}
                {memberships.length === 0 ? <p className="rounded-2xl bg-muted/60 p-4 text-sm text-muted-foreground">{copy.empty}</p> : null}
              </div>
            </div>

            <div className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
              <h2 className="text-lg font-semibold tracking-tight">{copy.plans}</h2>
              <div className="mt-4 grid gap-3">
                {plans.map((plan) => {
                  const redeemable = plan.pointsCost > 0 && plan.durationDays > 0;
                  const canRedeem = redeemable && balance >= plan.pointsCost;
                  return (
                    <article key={plan.id} className="rounded-2xl border border-border bg-background p-4">
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div>
                          <h3 className="text-sm font-medium">{plan.name}</h3>
                          <p className="mt-1 text-xs text-muted-foreground">{plan.code}</p>
                        </div>
                        <span className="rounded-full bg-muted px-3 py-1 text-xs text-muted-foreground">{formatPrice(plan.priceFen)}</span>
                      </div>
                      <div className="mt-3 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                        <span className="inline-flex items-center gap-1">
                          <Coins className="size-3.5" aria-hidden="true" />
                          {redeemable ? `${plan.pointsCost} ${copy.pointsCost}` : copy.redeemUnavailable}
                        </span>
                        {plan.durationDays > 0 ? (
                          <span className="inline-flex items-center gap-1">
                            <Clock className="size-3.5" aria-hidden="true" />
                            {copy.duration} {plan.durationDays} {copy.days}
                          </span>
                        ) : null}
                      </div>
                      <button
                        className="mt-4 w-full rounded-xl bg-primary px-3 py-2 text-sm font-medium text-primary-foreground disabled:cursor-not-allowed disabled:opacity-50"
                        disabled={!canRedeem || redeemingCode === plan.code}
                        onClick={() => void redeem(plan)}
                        type="button"
                      >
                        {redeemingCode === plan.code ? copy.redeeming : !redeemable ? copy.redeemUnavailable : canRedeem ? copy.redeem : copy.insufficient}
                      </button>
                    </article>
                  );
                })}
                {plans.length === 0 ? <p className="rounded-2xl bg-muted/60 p-4 text-sm text-muted-foreground">{copy.noPlans}</p> : null}
              </div>
            </div>
          </section>
        </>
      ) : null}
    </SiteShell>
  );
}

async function request<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
  const headers = new Headers(init.headers);
  if (init.body && !(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    headers,
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
  return `￥${(priceFen / 100).toFixed(2)}`;
}
