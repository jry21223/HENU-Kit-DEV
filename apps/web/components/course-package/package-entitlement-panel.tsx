"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Clock3, LockKeyhole } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import {
  apiBaseUrl,
  type CoursePackage,
  type Entitlements,
  type Material,
  type Order,
  type OrderCreateResult,
  type OrderStatus,
  type User,
  type WeChatNativePayment,
} from "@/lib/api";

type Envelope<T> = {
  code: number;
  message: string;
  data?: T;
};

const copy = {
  loading: "\u6b63\u5728\u68c0\u67e5\u89e3\u9501\u72b6\u6001...",
  unlocked: "\u5df2\u89e3\u9501",
  unlockedBody:
    "\u5f53\u524d\u8d26\u53f7\u5df2\u62e5\u6709\u8fd9\u4e2a\u8bfe\u7a0b\u5305\u6743\u9650\u3002\u5305\u5185 paid \u8d44\u6599\u4ecd\u4f1a\u901a\u8fc7\u670d\u52a1\u7aef\u4e0b\u8f7d\u63a5\u53e3\u6821\u9a8c\u548c\u8bb0\u5f55\u3002",
  loginRequired: "\u767b\u5f55\u540e\u67e5\u770b\u662f\u5426\u5df2\u89e3\u9501",
  loginBody:
    "\u8bfe\u7a0b\u5305\u6743\u76ca\u7ed1\u5b9a\u5230\u5b66\u751f\u90ae\u7bb1\u8d26\u53f7\u3002\u767b\u5f55\u540e\u53ef\u4ee5\u67e5\u770b\u7ba1\u7406\u5458\u6388\u6743\u6216\u540e\u7eed\u652f\u4ed8\u4ea4\u4ed8\u7684\u8bfe\u7a0b\u5305\u6743\u9650\u3002",
  login: "\u53bb\u767b\u5f55",
  locked: "\u5c1a\u672a\u89e3\u9501",
  lockedBody:
    "\u5f53\u524d\u8bfe\u7a0b\u5305\u9700\u8981\u670d\u52a1\u7aef entitlement \u624d\u80fd\u89e3\u9501 paid \u8d44\u6599\u3002\u652f\u4ed8\u4e8c\u7ef4\u7801\u53ea\u7528\u4e8e\u53d1\u8d77\u8ba2\u5355\u652f\u4ed8\uff0c\u4e0d\u4f1a\u5728\u524d\u7aef\u53d1\u653e\u6743\u76ca\u3002",
  paymentPending: "\u652f\u4ed8\u8054\u8c03\u4e2d",
  paymentBody:
    "\u524d\u7aef\u53ea\u5c55\u793a\u4e8c\u7ef4\u7801\u548c\u8f6e\u8be2\u8ba2\u5355\u72b6\u6001\uff0c\u4e0d\u4f1a\u4f2a\u9020\u652f\u4ed8\u6210\u529f\u3002\u6b63\u5f0f\u89e3\u9501\u5fc5\u987b\u6765\u81ea\u670d\u52a1\u7aef entitlement\u3002",
  createOrder: "\u521b\u5efa\u5f85\u652f\u4ed8\u8ba2\u5355",
  creatingOrder: "\u521b\u5efa\u4e2d...",
  createNative: "\u751f\u6210\u5fae\u4fe1\u652f\u4ed8\u4e8c\u7ef4\u7801",
  creatingNative: "\u751f\u6210\u4e2d...",
  orderReady: "\u5f85\u652f\u4ed8\u8ba2\u5355\u5df2\u5c31\u7eea",
  orderPendingBody:
    "\u8ba2\u5355\u5df2\u8fdb\u5165\u5f85\u652f\u4ed8/\u652f\u4ed8\u4e2d\u72b6\u6001\u3002\u524d\u7aef\u4ec5\u8bfb\u53d6\u72b6\u6001\uff0c\u4e0d\u4f1a\u53d1\u653e\u6743\u76ca\u3002",
  orderNo: "\u8ba2\u5355\u53f7",
  orderStatus: "\u8ba2\u5355\u72b6\u6001",
  refreshStatus: "\u5237\u65b0\u72b6\u6001",
  qrTitle: "\u5fae\u4fe1 Native \u626b\u7801\u652f\u4ed8",
  qrHint:
    "\u8bf7\u4f7f\u7528\u5fae\u4fe1\u626b\u7801\u3002\u5f53\u524d\u5f00\u53d1/\u6d4b\u8bd5\u73af\u5883\u4f7f\u7528 mock codeUrl\uff1b\u771f\u5b9e\u652f\u4ed8\u7ed3\u679c\u5fc5\u987b\u4ee5\u670d\u52a1\u7aef\u56de\u8c03\u548c entitlement \u4e3a\u51c6\u3002",
  expiresAt: "\u8fc7\u671f\u65f6\u95f4",
  mockMode: "\u6a21\u62df\u652f\u4ed8\u7801",
  ownedMaterials: "\u5df2\u89e3\u9501\u8d44\u6599",
  viewMaterial: "\u67e5\u770b\u8d44\u6599",
  packageGrants: "\u8bfe\u7a0b\u5305\u6388\u6743",
  failed: "\u6682\u65f6\u65e0\u6cd5\u8bfb\u53d6\u8d26\u53f7\u6743\u76ca",
  nativeFailed: "\u5fae\u4fe1 Native \u4e0b\u5355\u6682\u65f6\u4e0d\u53ef\u7528",
};

export function PackageEntitlementPanel({ coursePackage, materials }: { coursePackage: CoursePackage; materials: Material[] }) {
  const [user, setUser] = useState<User | null>(null);
  const [entitlements, setEntitlements] = useState<Entitlements | null>(null);
  const [pendingOrder, setPendingOrder] = useState<Order | null>(null);
  const [pendingOrderStatus, setPendingOrderStatus] = useState<OrderStatus | null>(null);
  const [nativePayment, setNativePayment] = useState<WeChatNativePayment | null>(null);
  const [loading, setLoading] = useState(true);
  const [ordering, setOrdering] = useState(false);
  const [creatingNative, setCreatingNative] = useState(false);
  const [error, setError] = useState("");
  const [orderMessage, setOrderMessage] = useState("");

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

  useEffect(() => {
    if (!pendingOrder || !nativePayment) return;
    if (pendingOrderStatus?.status === "paid" || pendingOrderStatus?.status === "closed" || pendingOrderStatus?.status === "expired") return;
    const timer = window.setInterval(() => {
      void refreshOrderStatus(pendingOrder.id);
    }, 3000);
    return () => window.clearInterval(timer);
  }, [nativePayment, pendingOrder, pendingOrderStatus?.status]);

  const ownedPackage = useMemo(() => {
    return entitlements?.packageGrants.find((row) => row.package?.id === coursePackage.id || row.grant.packageId === coursePackage.id);
  }, [coursePackage.id, entitlements]);

  async function createOrder() {
    setOrdering(true);
    setError("");
    setOrderMessage("");
    try {
      const response = await request<OrderCreateResult>("/orders", {
        method: "POST",
        body: JSON.stringify({ packageId: coursePackage.id }),
      });
      if (response.data?.alreadyOwned || response.data?.entitlementGranted) {
        setOrderMessage(copy.unlocked);
        const entitlementResponse = await request<Entitlements>("/me/entitlements");
        setEntitlements(entitlementResponse.data ?? null);
        return;
      }
      const nextOrder = response.data?.order ?? null;
      setPendingOrder(nextOrder);
      setOrderMessage(copy.orderReady);
      if (nextOrder) {
        await refreshOrderStatus(nextOrder.id);
        await createNativePayment(nextOrder.id);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : copy.failed);
    } finally {
      setOrdering(false);
    }
  }

  async function refreshOrderStatus(orderId = pendingOrder?.id) {
    if (!orderId) return;
    const response = await request<OrderStatus>(`/orders/${orderId}/status`);
    const nextStatus = response.data ?? null;
    setPendingOrderStatus(nextStatus);
    if (nextStatus?.status === "paid" && nextStatus.entitlementGranted) {
      const entitlementResponse = await request<Entitlements>("/me/entitlements");
      setEntitlements(entitlementResponse.data ?? null);
    }
  }

  async function createNativePayment(orderId = pendingOrder?.id) {
    if (!orderId) return;
    setCreatingNative(true);
    setError("");
    try {
      const response = await request<WeChatNativePayment>("/payments/wechat/native", {
        method: "POST",
        body: JSON.stringify({ orderId }),
      });
      setNativePayment(response.data ?? null);
      await refreshOrderStatus(orderId);
    } catch (err) {
      setError(err instanceof Error ? formatPaymentError(err.message) : copy.nativeFailed);
    } finally {
      setCreatingNative(false);
    }
  }

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
      <section className="rounded-3xl border border-emerald-200 bg-emerald-50 p-5 shadow-sm" data-testid="package-unlocked-panel">
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
    <section className="rounded-3xl border border-border bg-card p-5 shadow-sm" data-testid="package-payment-panel">
      <div className="flex items-center gap-2">
        <LockKeyhole className="size-5 text-primary" aria-hidden="true" />
        <h2 className="text-lg font-semibold tracking-tight">{copy.locked}</h2>
      </div>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">{copy.lockedBody}</p>
      <div className="mt-4 rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">
        <p className="font-medium text-foreground">{copy.paymentPending}</p>
        <p className="mt-1 leading-6">{copy.paymentBody}</p>
      </div>

      {pendingOrder ? (
        <div className="mt-3 rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground">
          <p className="font-medium text-foreground">{copy.orderReady}</p>
          <p className="mt-1 leading-6">{copy.orderPendingBody}</p>
          <p className="mt-2 break-all text-xs" data-testid="package-order-out-trade-no">
            {copy.orderNo}: {pendingOrder.outTradeNo}
          </p>
          <p className="mt-1 text-xs" data-testid="package-order-status">
            {copy.orderStatus}: {pendingOrderStatus?.status ?? pendingOrder.status}
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <button
              className="rounded-lg border border-border px-3 py-1.5 text-xs text-foreground hover:bg-muted"
              data-testid="package-refresh-order"
              onClick={() => void refreshOrderStatus()}
              type="button"
            >
              {copy.refreshStatus}
            </button>
            {!nativePayment ? (
              <button
                className="rounded-lg border border-border px-3 py-1.5 text-xs text-foreground hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
                disabled={creatingNative}
                onClick={() => void createNativePayment()}
                type="button"
              >
                {creatingNative ? copy.creatingNative : copy.createNative}
              </button>
            ) : null}
          </div>
        </div>
      ) : null}

      {nativePayment ? (
        <div className="mt-3 rounded-2xl border border-border bg-background p-4 text-sm text-muted-foreground" data-testid="package-wechat-qr">
          <div className="flex flex-col items-center gap-3 text-center">
            <p className="font-medium text-foreground">{copy.qrTitle}</p>
            <div className="rounded-2xl border border-border bg-white p-3">
              <QRCodeSVG value={nativePayment.codeUrl} size={168} />
            </div>
            <p className="max-w-xs text-xs leading-5">{copy.qrHint}</p>
            <div className="grid w-full gap-1 text-left text-xs">
              <p data-testid="package-native-status">
                {copy.orderStatus}: {nativePayment.status}
              </p>
              <p>
                {copy.expiresAt}: {formatDate(nativePayment.expiresAt)}
              </p>
              {nativePayment.mock ? <p>{copy.mockMode}</p> : null}
            </div>
          </div>
        </div>
      ) : null}

      <button
        className="mt-4 w-full rounded-xl bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition hover:bg-[#254d42] disabled:cursor-not-allowed disabled:opacity-60"
        data-testid="package-create-order"
        disabled={ordering || creatingNative}
        onClick={createOrder}
        type="button"
      >
        {ordering ? copy.creatingOrder : copy.createOrder}
      </button>
      {orderMessage && !pendingOrder ? <p className="mt-3 text-sm text-muted-foreground">{orderMessage}</p> : null}
      {error ? <p className="mt-3 text-sm text-red-700">{error}</p> : null}
      <p className="mt-3 text-xs text-muted-foreground">
        {copy.packageGrants}: {entitlements?.summary.packageGrants ?? 0}
      </p>
    </section>
  );
}

async function request<T>(path: string, init: RequestInit = {}): Promise<Envelope<T>> {
  const response = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...init.headers,
    },
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `API request failed with ${response.status}`);
  }
  return payload;
}

function formatPaymentError(message: string) {
  const labels: Record<string, string> = {
    wechat_live_not_implemented:
      "\u771f\u5b9e\u5fae\u4fe1 Native \u4e0b\u5355\u5c1a\u672a\u5b9e\u73b0\uff0c\u5f53\u524d\u53ea\u652f\u6301\u5f00\u53d1/\u6d4b\u8bd5 mock\u3002",
    wechat_mock_forbidden_in_production: "\u751f\u4ea7\u73af\u5883\u7981\u6b62\u4f7f\u7528 mock \u652f\u4ed8\u3002",
    wechat_live_config_missing: "\u5fae\u4fe1\u652f\u4ed8\u5546\u6237\u914d\u7f6e\u4e0d\u5b8c\u6574\u3002",
    order_not_payable: "\u8be5\u8ba2\u5355\u5f53\u524d\u72b6\u6001\u4e0d\u53ef\u53d1\u8d77\u652f\u4ed8\u3002",
  };
  return labels[message] ?? message ?? copy.nativeFailed;
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}
