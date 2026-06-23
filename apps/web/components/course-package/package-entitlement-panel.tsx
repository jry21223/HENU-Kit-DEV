"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Clock3, LockKeyhole } from "lucide-react";
import { apiBaseUrl, type CoursePackage, type Entitlements, type Material, type User } from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

const copy = {
  loading: "正在检查解锁状态...",
  unlocked: "已解锁",
  unlockedBody: "当前账号已拥有这个课程包权限。包内 paid 资料仍会通过服务端下载接口校验和记录。",
  loginRequired: "登录后查看是否已解锁",
  loginBody: "课程包权益绑定到学生邮箱账号。登录后可以查看管理员授权或后续支付交付的课程包权限。",
  login: "去登录",
  locked: "尚未解锁",
  lockedBody: "微信 Native 支付仍处于联调准备中，当前内测阶段可由管理员在后台手动授权课程包。",
  paymentPending: "支付联调中",
  paymentBody: "前端只展示状态，不会伪造支付成功；正式解锁必须来自服务端 entitlement。",
  ownedMaterials: "已解锁资料",
  viewMaterial: "查看资料",
  packageGrants: "课程包授权",
  failed: "暂时无法读取账号权益",
};

export function PackageEntitlementPanel({ coursePackage, materials }: { coursePackage: CoursePackage; materials: Material[] }) {
  const [user, setUser] = useState<User | null>(null);
  const [entitlements, setEntitlements] = useState<Entitlements | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let alive = true;
    async function load() {
      setLoading(true);
      setError("");
      try {
        const me = await request<User>("/auth/me");
        if (!alive) return;
        setUser(me.data ?? null);
        const entitlementResponse = await request<Entitlements>("/me/entitlements");
        if (!alive) return;
        setEntitlements(entitlementResponse.data ?? null);
      } catch (err) {
        if (!alive) return;
        setUser(null);
        setEntitlements(null);
        setError(err instanceof Error ? err.message : copy.failed);
      } finally {
        if (alive) setLoading(false);
      }
    }
    void load();
    return () => {
      alive = false;
    };
  }, []);

  const ownedPackage = useMemo(() => {
    return entitlements?.packageGrants.find((row) => row.package?.id === coursePackage.id || row.grant.packageId === coursePackage.id);
  }, [coursePackage.id, entitlements]);

  if (loading) {
    return (
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Clock3 className="size-4" aria-hidden="true" />
          {copy.loading}
        </div>
      </section>
    );
  }

  if (ownedPackage) {
    const unlockedMaterials = ownedPackage.materials.length > 0 ? ownedPackage.materials : materials;
    return (
      <section className="rounded-3xl border border-emerald-200 bg-emerald-50 p-5 shadow-sm">
        <div className="flex items-center gap-2">
          <CheckCircle2 className="size-5 text-emerald-700" aria-hidden="true" />
          <h2 className="text-lg font-semibold tracking-tight text-emerald-900">{copy.unlocked}</h2>
        </div>
        <p className="mt-2 text-sm leading-6 text-emerald-800">{copy.unlockedBody}</p>
        <p className="mt-4 text-sm font-medium text-emerald-900">{copy.ownedMaterials}</p>
        <div className="mt-2 grid gap-2">
          {unlockedMaterials.map((material) => (
            <Link
              className="rounded-2xl border border-emerald-200 bg-white/70 px-3 py-2 text-sm text-emerald-950 transition hover:bg-white"
              href={`/materials/${material.id}`}
              key={material.id}
            >
              <span className="font-medium">{material.title}</span>
              <span className="ml-2 text-xs text-emerald-700">{copy.viewMaterial}</span>
            </Link>
          ))}
        </div>
      </section>
    );
  }

  if (!user) {
    return (
      <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
        <div className="flex items-center gap-2">
          <LockKeyhole className="size-5 text-primary" aria-hidden="true" />
          <h2 className="text-lg font-semibold tracking-tight">{copy.loginRequired}</h2>
        </div>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">{copy.loginBody}</p>
        {error ? <p className="mt-2 text-xs text-muted-foreground">{error}</p> : null}
        <Link className="mt-4 inline-flex rounded-xl bg-primary px-4 py-2 text-sm font-medium text-primary-foreground" href="/login">
          {copy.login}
        </Link>
      </section>
    );
  }

  return (
    <section className="rounded-3xl border border-border bg-card p-5 shadow-sm">
      <div className="flex items-center gap-2">
        <LockKeyhole className="size-5 text-primary" aria-hidden="true" />
        <h2 className="text-lg font-semibold tracking-tight">{copy.locked}</h2>
      </div>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">{copy.lockedBody}</p>
      <div className="mt-4 rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">
        <p className="font-medium text-foreground">{copy.paymentPending}</p>
        <p className="mt-1 leading-6">{copy.paymentBody}</p>
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        {copy.packageGrants}: {entitlements?.summary.packageGrants ?? 0}
      </p>
    </section>
  );
}

async function request<T>(path: string): Promise<Envelope<T>> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `API request failed with ${response.status}`);
  }
  return payload;
}
