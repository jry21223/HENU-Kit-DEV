"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { useReveal } from "@/components/account/use-reveal";
import { fetchAccountMembership, formatPortalError } from "@/lib/api/client";
import type { AccountMembershipResponse } from "@/lib/api/types";

type MembershipState =
  | { kind: "loading" }
  | { kind: "success"; membership: AccountMembershipResponse }
  | { kind: "error"; message: string };

export default function MembershipPage() {
  const [state, setState] = useState<MembershipState>({ kind: "loading" });
  const requestVersion = useRef(0);
  useReveal();

  const loadMembership = useCallback(() => {
    const version = ++requestVersion.current;
    void fetchAccountMembership().then(
      (membership) => {
        if (version === requestVersion.current) setState({ kind: "success", membership });
      },
      (error: unknown) => {
        if (version === requestVersion.current) setState({ kind: "error", message: formatPortalError(error) });
      }
    );
  }, []);

  useEffect(() => {
    loadMembership();
    return () => {
      requestVersion.current += 1;
    };
  }, [loadMembership]);

  const membership = state.kind === "success" ? state.membership.data : undefined;
  const isLifetime = membership?.plan === "lifetime" && membership.lifetime;

  return (
    <div>
      <section data-enter className="border-b border-ink pb-5">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/55">
          <span className="text-accent">A-04</span>
          <span className="mx-2">/</span>
          MEMBERSHIP
        </p>
        <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">会员权益</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-ink/60">
          仅展示 Account Portfolio 返回的真实会员权益；本页不提供会话内开通或模拟支付。
        </p>
      </section>

      {state.kind === "loading" ? (
        <section
          data-account-membership-state="loading"
          aria-live="polite"
          className="mt-6 border border-line px-5 py-8 font-mono text-xs tracking-[0.2em] text-ink/50"
        >
          MEMBERSHIP LOADING<span className="animate-pulse text-accent">…</span>
        </section>
      ) : null}

      {state.kind === "error" ? (
        <section data-account-membership-state="error" role="alert" className="mt-6 border border-accent px-5 py-6">
          <p className="font-mono text-xs tracking-[0.14em] text-accent">MEMBERSHIP UNAVAILABLE</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
          <p className="mt-3 text-sm leading-6 text-ink/60">账户服务不可用时不会以本地或会话状态替代真实权益。</p>
          <button
            type="button"
            onClick={() => {
              setState({ kind: "loading" });
              loadMembership();
            }}
            className="mt-5 border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {membership ? (
        <section data-account-membership-state="success" className="mt-6 border border-ink p-6 sm:p-8">
          <p className="font-mono text-xs tracking-[0.2em] text-ink/45">CURRENT ENTITLEMENT</p>
          <h2 className="mt-3 font-display text-4xl font-bold tracking-tight">
            {isLifetime ? "终身会员" : "免费会员"}
          </h2>
          <p className="mt-4 max-w-2xl text-base leading-7 text-ink/75">
            {isLifetime
              ? "权益已由 Account Portfolio 持久化确认，可跨设备读取。"
              : "当前为免费会员；新用户会从持久化的免费会员状态开始。"}
          </p>
          <dl className="mt-6 grid gap-4 border-t border-line pt-5 sm:grid-cols-2">
            <div>
              <dt className="font-mono text-[10px] tracking-[0.18em] text-ink/45">PLAN</dt>
              <dd className="mt-2 text-sm leading-6">{membership.plan === "lifetime" ? "lifetime" : "free"}</dd>
            </div>
            <div>
              <dt className="font-mono text-[10px] tracking-[0.18em] text-ink/45">SYNCHRONIZATION</dt>
              <dd className="mt-2 text-sm leading-6">重新登录或切换设备后从服务端重新读取</dd>
            </div>
          </dl>
          <p className="mt-6 border-t border-line pt-5 text-sm leading-6 text-ink/60">
            终身会员产品金额为 ¥9.9；支付 Provider 尚未启用，运营授权与撤销会通过真实通知告知用户。
          </p>
        </section>
      ) : null}
    </div>
  );
}
