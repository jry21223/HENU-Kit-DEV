"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ArrowDownCircle, ArrowUpCircle, Sparkles } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { apiBaseUrl, type PointsLog } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

const copy = {
  back: "返回个人中心",
  eyebrow: "我的积分",
  title: "积分余额与流水",
  intro: "这里展示当前登录账号的积分余额和最近积分变动。积分以服务端记录为准。",
  loading: "正在加载积分信息...",
  login: "去登录",
  balance: "当前积分",
  logs: "最近流水",
  empty: "暂无积分流水。",
  fallbackError: "积分信息暂时不可用",
};

export default function MyPointsPage() {
  const [balance, setBalance] = useState<number | null>(null);
  const [logs, setLogs] = useState<PointsLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    async function loadPoints() {
      setLoading(true);
      setError("");
      try {
        const [balancePayload, logsPayload] = await Promise.all([
          request<{ balance: number }>("/me/points"),
          request<{ logs: PointsLog[] }>("/me/points/logs"),
        ]);
        setBalance(balancePayload.data?.balance ?? 0);
        setLogs(logsPayload.data?.logs ?? []);
      } catch (err) {
        setError(err instanceof Error ? err.message : copy.fallbackError);
      } finally {
        setLoading(false);
      }
    }

    void loadPoints();
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
            <div className="flex items-center gap-3">
              <span className="grid size-11 place-items-center rounded-2xl bg-primary/10 text-primary">
                <Sparkles className="size-5" aria-hidden="true" />
              </span>
              <div>
                <p className="text-sm text-muted-foreground">{copy.balance}</p>
                <p className="text-3xl font-semibold tracking-tight">{balance ?? 0}</p>
              </div>
            </div>
          </section>

          <section className="rounded-3xl border border-border bg-card p-5 shadow-sm sm:p-6">
            <h2 className="text-lg font-semibold tracking-tight">{copy.logs}</h2>
            <div className="mt-4 space-y-3">
              {logs.map((log) => (
                <article key={log.id} className="rounded-2xl border border-border bg-background p-4">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="flex min-w-0 items-start gap-3">
                      <span className={`grid size-9 flex-none place-items-center rounded-xl ${log.delta >= 0 ? "bg-emerald-50 text-emerald-700" : "bg-amber-50 text-amber-700"}`}>
                        {log.delta >= 0 ? <ArrowUpCircle className="size-4" aria-hidden="true" /> : <ArrowDownCircle className="size-4" aria-hidden="true" />}
                      </span>
                      <div className="min-w-0">
                        <h3 className="break-words text-sm font-medium">{log.reason}</h3>
                        <p className="mt-1 text-xs text-muted-foreground">来源：{referenceTypeLabel(log.referenceType)}</p>
                      </div>
                    </div>
                    <div className="text-right">
                      <p className={`text-sm font-semibold ${log.delta >= 0 ? "text-emerald-700" : "text-amber-700"}`}>
                        {log.delta >= 0 ? "+" : ""}
                        {log.delta}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">余额 {log.balanceAfter}</p>
                    </div>
                  </div>
                  <p className="mt-3 text-xs text-muted-foreground">{formatDate(log.createdAt)}</p>
                </article>
              ))}
              {logs.length === 0 ? <p className="rounded-2xl bg-muted/60 p-4 text-sm text-muted-foreground">{copy.empty}</p> : null}
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
    throw new Error(payload.message || "网络不太顺畅，请检查网络后重试");
  }
  return payload;
}

function referenceTypeLabel(type?: string) {
  if (!type || type === "system") return "系统";
  return "其他";
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
