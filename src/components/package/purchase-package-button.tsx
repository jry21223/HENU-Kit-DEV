"use client";

import { useEffect, useState } from "react";

type PurchasePackageButtonProps = {
  packageId: string;
};

type OrderResponse = {
  status?: "already_owned";
  packageId?: string;
  order?: {
    id: string;
    amount: string;
    amountTotal: number;
    currency: string;
    status: string;
  };
  error?: string;
};

type NativePaymentResponse = {
  orderId?: string;
  codeUrl?: string;
  expiresAt?: string;
  status?: string;
  error?: string;
  details?: string[];
};

type OrderStatusResponse = {
  orderId?: string;
  status?: string;
  paidAt?: string | null;
  entitlementGranted?: boolean;
  error?: string;
};

export function PurchasePackageButton({ packageId }: PurchasePackageButtonProps) {
  const [pending, setPending] = useState(false);
  const [order, setOrder] = useState<OrderResponse["order"] | null>(null);
  const [payment, setPayment] = useState<NativePaymentResponse | null>(null);
  const [orderStatus, setOrderStatus] = useState<OrderStatusResponse | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!payment?.orderId || ["paid", "closed", "expired", "failed"].includes(orderStatus?.status ?? "")) {
      return;
    }

    const timer = window.setInterval(async () => {
      const response = await fetch(`/api/orders/${payment.orderId}/status`);
      const payload = (await response.json()) as OrderStatusResponse;
      if (response.ok) {
        setOrderStatus(payload);
      }
    }, 3000);

    return () => window.clearInterval(timer);
  }, [payment?.orderId, orderStatus?.status]);

  async function createOrderAndPayment() {
    setPending(true);
    setError("");
    setMessage("");
    setOrder(null);
    setPayment(null);
    setOrderStatus(null);

    try {
      const orderResponse = await fetch("/api/orders", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ packageId }),
      });
      const orderPayload = (await orderResponse.json()) as OrderResponse;

      if (!orderResponse.ok) {
        setError(orderPayload.error ?? "创建订单失败。");
        return;
      }

      if (orderPayload.status === "already_owned") {
        setMessage("当前账号已解锁该课程包。");
        return;
      }

      if (!orderPayload.order?.id) {
        setError("订单响应缺少 orderId。");
        return;
      }

      setOrder(orderPayload.order);

      const paymentResponse = await fetch("/api/payments/wechat/native", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ orderId: orderPayload.order.id }),
      });
      const paymentPayload = (await paymentResponse.json()) as NativePaymentResponse;

      if (!paymentResponse.ok) {
        setError(
          paymentPayload.details?.length
            ? paymentPayload.details.join("；")
            : paymentPayload.error ?? "发起微信 Native 支付失败。",
        );
        return;
      }

      setPayment(paymentPayload);
      setOrderStatus({
        orderId: paymentPayload.orderId,
        status: paymentPayload.status,
        entitlementGranted: false,
      });
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
        onClick={createOrderAndPayment}
        disabled={pending}
        className="inline-flex h-10 w-full items-center justify-center rounded-md bg-brand px-4 text-sm font-semibold text-white hover:bg-[#12574d] disabled:cursor-not-allowed disabled:bg-line disabled:text-muted focus-ring"
      >
        {pending ? "正在创建订单" : "微信扫码支付"}
      </button>

      {error ? (
        <div className="rounded-lg border border-[#e7c5b5] bg-[#fff7f3] p-3 text-sm font-semibold text-[#8b3f24]">
          {error}
        </div>
      ) : null}

      {message ? (
        <div className="rounded-lg border border-[#b8dccf] bg-[#f1faf6] p-3 text-sm font-semibold text-[#185c48]">
          {message}
        </div>
      ) : null}

      {order ? (
        <div className="rounded-lg border border-line bg-panel p-3 text-sm leading-6 text-muted">
          <p className="font-semibold text-ink">
            订单已创建：{order.id}，金额 ￥{order.amount}
          </p>
          <p>支付状态：{orderStatus?.status ?? order.status}</p>
          {orderStatus?.entitlementGranted ? <p>课程包权限已发放。</p> : null}
          {payment?.codeUrl ? (
            <div className="mt-3 rounded-md border border-line bg-white p-3">
              <p className="font-semibold text-ink">微信 Native mock code_url</p>
              <code className="mt-2 block break-all text-xs text-muted">{payment.codeUrl}</code>
              <p className="mt-2 text-xs">真实二维码渲染和 live 支付联调将在后续接入。</p>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
