"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { MembershipCheckoutQR } from "@/components/account/membership-checkout-qr";
import {
  createAccountMembershipOrder,
  fetchAccountMembershipOrders,
  formatPortalError,
} from "@/lib/api/client";
import type { AccountMembershipOrder } from "@/lib/api/types";

const POLL_INTERVAL_MS = 4000;

type PurchaseState =
  | { kind: "idle" }
  | { kind: "starting" }
  | { kind: "awaiting"; order: AccountMembershipOrder; checkoutURL?: string }
  | { kind: "paid" }
  | { kind: "unavailable"; message: string }
  | { kind: "error"; message: string };

/**
 * Drives the signed-in user's own lifetime membership purchase.
 *
 * Payment is only ever confirmed by the server: this component polls its own
 * orders and reports success solely when the durable order says `paid`. It
 * never infers success from the browser having shown a QR code.
 */
export function MembershipPurchase({ onPaid }: { onPaid: () => void }) {
  const [state, setState] = useState<PurchaseState>({ kind: "idle" });
  // The key is retained so a retry resumes the same order instead of starting
  // a second purchase.
  const idempotencyKey = useRef<string | null>(null);
  const active = useRef(true);

  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);

  const start = useCallback(() => {
    setState({ kind: "starting" });
    if (!idempotencyKey.current) {
      idempotencyKey.current = `membership-${crypto.randomUUID()}`;
    }
    void createAccountMembershipOrder(idempotencyKey.current).then(
      (response) => {
        if (!active.current) return;
        const { order, checkout_url: checkoutURL } = response.data;
        if (order.status === "paid") {
          setState({ kind: "paid" });
          onPaid();
          return;
        }
        setState({ kind: "awaiting", order, checkoutURL });
      },
      (error: unknown) => {
        if (!active.current) return;
        // A disabled payment provider is an honest unavailable state, not a
        // failure the user should retry into.
        const message = formatPortalError(error);
        const unavailable =
          typeof error === "object" &&
          error !== null &&
          "status" in error &&
          (error as { status?: number }).status === 503 &&
          error instanceof Error &&
          error.message === "membership_payment_unavailable";
        setState(unavailable ? { kind: "unavailable", message } : { kind: "error", message });
      }
    );
  }, [onPaid]);

  // While an order awaits payment, the server is the only authority on whether
  // it completed.
  useEffect(() => {
    if (state.kind !== "awaiting") return;
    const orderID = state.order.id;
    const timer = setInterval(() => {
      void fetchAccountMembershipOrders().then(
        (response) => {
          if (!active.current) return;
          const current = response.data.orders.find((order) => order.id === orderID);
          if (!current) return;
          if (current.status === "paid") {
            setState({ kind: "paid" });
            onPaid();
            return;
          }
          if (current.status === "closed" || current.status === "failed" || current.status === "refunded") {
            setState({ kind: "error", message: "订单已结束，未完成支付。请重新发起。" });
          }
        },
        () => {
          // A transient read failure must not be reported as a payment result.
        }
      );
    }, POLL_INTERVAL_MS);
    return () => clearInterval(timer);
  }, [state, onPaid]);

  if (state.kind === "paid") {
    return (
      <section data-membership-purchase="paid" className="mt-6 border border-ink p-6">
        <p className="font-mono text-xs tracking-[0.2em] text-accent">PAYMENT CONFIRMED</p>
        <p className="mt-3 text-sm leading-6 text-ink/70">
          支付已由服务端确认，终身会员权益已写入你的账户。
        </p>
      </section>
    );
  }

  if (state.kind === "unavailable") {
    return (
      <section data-membership-purchase="unavailable" className="mt-6 border border-line p-6">
        <p className="font-mono text-xs tracking-[0.2em] text-ink/45">PURCHASE UNAVAILABLE</p>
        <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
        <p className="mt-3 text-sm leading-6 text-ink/60">
          支付通道尚未开放，本页不会创建订单，也不会发放权益。
        </p>
      </section>
    );
  }

  return (
    <section data-membership-purchase={state.kind} className="mt-6 border border-ink p-6 sm:p-8">
      <p className="font-mono text-xs tracking-[0.2em] text-ink/45">LIFETIME MEMBERSHIP</p>
      <h2 className="mt-3 font-display text-3xl font-bold tracking-tight">¥9.9 永久解锁</h2>

      {state.kind === "idle" || state.kind === "error" ? (
        <>
          <p className="mt-4 max-w-2xl text-sm leading-6 text-ink/70">
            一次付费永久解锁：会员包含期末押题卷等核心复习资料，也为服务器持续运行提供支持。权益由服务端持久化，可跨设备读取。
          </p>
          {state.kind === "error" ? (
            <p role="alert" className="mt-4 border border-accent px-4 py-3 text-sm leading-6 text-ink/70">
              {state.message}
            </p>
          ) : null}
          <button
            type="button"
            onClick={start}
            className="mt-6 inline-flex min-h-11 items-center justify-center border border-ink px-5 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            {state.kind === "error" ? "重新发起支付" : "购买终身会员"}
          </button>
        </>
      ) : null}

      {state.kind === "starting" ? (
        <p
          aria-live="polite"
          className="mt-6 font-mono text-xs tracking-[0.2em] text-ink/50"
        >
          CREATING ORDER<span className="animate-pulse text-accent">…</span>
        </p>
      ) : null}

      {state.kind === "awaiting" ? (
        <div className="mt-6 flex flex-col gap-6 sm:flex-row sm:items-start">
          {state.checkoutURL ? (
            <MembershipCheckoutQR checkoutURL={state.checkoutURL} />
          ) : (
            <div className="flex aspect-square w-full max-w-[280px] items-center justify-center border border-line p-6 text-center">
              <p className="text-sm leading-6 text-ink/65">
                支付二维码已过期。请重新发起支付。
              </p>
            </div>
          )}
          <div className="flex-1">
            <p className="font-mono text-xs tracking-[0.2em] text-ink/45">AWAITING PAYMENT</p>
            <p className="mt-3 text-sm leading-6 text-ink/70">
              请使用微信扫码完成支付。支付结果由服务端确认，确认后本页会自动更新。
            </p>
            <p className="mt-3 text-sm leading-6 text-ink/55">
              离开本页后回来仍是同一个订单与同一个二维码，不会重复下单。
            </p>
            <button
              type="button"
              onClick={start}
              className="mt-5 inline-flex min-h-11 items-center justify-center border border-line px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:border-ink"
            >
              刷新二维码
            </button>
          </div>
        </div>
      ) : null}
    </section>
  );
}
