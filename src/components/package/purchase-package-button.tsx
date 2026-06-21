"use client";

import { useState } from "react";

type PurchasePackageButtonProps = {
  packageId: string;
};

type OrderResponse = {
  order?: {
    id: string;
    amount: string;
    status: string;
  };
  payment?: {
    configured: boolean;
    paymentUrl: string | null;
  };
  error?: string;
};

export function PurchasePackageButton({ packageId }: PurchasePackageButtonProps) {
  const [pending, setPending] = useState(false);
  const [result, setResult] = useState<OrderResponse | null>(null);
  const [error, setError] = useState("");

  async function createOrder() {
    setPending(true);
    setError("");
    setResult(null);

    try {
      const response = await fetch("/api/orders", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          product_type: "package",
          product_id: packageId,
        }),
      });
      const payload = (await response.json()) as OrderResponse;

      if (!response.ok) {
        setError(payload.error ?? "创建订单失败。");
        return;
      }

      setResult(payload);
    } catch {
      setError("网络异常，请稍后重试。");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="mt-5 grid gap-3">
      <button
        type="button"
        onClick={createOrder}
        disabled={pending}
        className="inline-flex h-10 w-full items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] disabled:cursor-not-allowed disabled:bg-line disabled:text-muted focus-ring"
      >
        {pending ? "创建中" : "购买课程包"}
      </button>
      {error ? (
        <div className="rounded-lg border border-[#e7c5b5] bg-[#fff7f3] p-3 text-sm font-semibold text-[#8b3f24]">
          {error}
        </div>
      ) : null}
      {result?.order ? (
        <div className="rounded-lg border border-line bg-panel p-3 text-sm leading-6 text-muted">
          <p className="font-semibold text-ink">
            订单已创建：{result.order.id}，金额 ￥{result.order.amount}
          </p>
          {result.payment?.configured && result.payment.paymentUrl ? (
            <a
              href={result.payment.paymentUrl}
              className="mt-3 inline-flex h-10 w-full items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] focus-ring"
            >
              前往支付
            </a>
          ) : (
            <p className="mt-2">当前未配置真实支付网关，请在开发环境使用回调验收。</p>
          )}
        </div>
      ) : null}
    </div>
  );
}
