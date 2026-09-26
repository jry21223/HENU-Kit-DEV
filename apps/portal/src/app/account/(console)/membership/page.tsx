"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { useAccountConsoleUnauthorizedHandler } from "@/components/account/account-console-session";
import { MembershipPurchase } from "@/components/account/membership-purchase";
import { useReveal } from "@/components/account/use-reveal";
import { fetchAccountMembership, formatPortalError } from "@/lib/api/client";
import { LIFETIME_BENEFITS } from "@/lib/membership";
import type { AccountMembershipResponse } from "@/lib/api/types";

type MembershipState =
  | { kind: "loading" }
  | { kind: "success"; membership: AccountMembershipResponse }
  | { kind: "error"; message: string };

export default function MembershipPage() {
  const [state, setState] = useState<MembershipState>({ kind: "loading" });
  const requestVersion = useRef(0);
  const handleUnauthorized = useAccountConsoleUnauthorizedHandler();
  useReveal();

  const loadMembership = useCallback(() => {
    const version = ++requestVersion.current;
    void fetchAccountMembership().then(
      (membership) => {
        if (version === requestVersion.current) setState({ kind: "success", membership });
      },
      (error: unknown) => {
        if (version === requestVersion.current && !handleUnauthorized(error)) {
          setState({ kind: "error", message: formatPortalError(error) });
        }
      }
    );
  }, [handleUnauthorized]);

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
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent-text">A-04</span>
          <span className="mx-2">/</span>
          MEMBERSHIP
        </p>
        <h1 className="mt-3 font-display text-4xl font-bold tracking-tight">会员权益</h1>
        <p className="mt-3 max-w-2xl text-sm leading-6 text-ink/60">
          查看你当前的会员状态与权益。
        </p>
      </section>

      {state.kind === "loading" ? (
        <section
          data-account-membership-state="loading"
          aria-live="polite"
          className="mt-6 border border-line px-5 py-8 font-mono text-xs tracking-[0.2em] text-ink/60"
        >
          MEMBERSHIP LOADING<span aria-hidden className="animate-pulse text-accent-text">…</span>
        </section>
      ) : null}

      {state.kind === "error" ? (
        <section data-account-membership-state="error" role="alert" className="mt-6 border border-accent px-5 py-6">
          <p className="font-mono text-xs tracking-[0.14em] text-accent-text">MEMBERSHIP UNAVAILABLE</p>
          <p className="mt-3 text-sm leading-6 text-ink/65">{state.message}</p>
          <p className="mt-3 text-sm leading-6 text-ink/60">页面加载失败不会改变你的会员状态，请稍后重新加载。</p>
          <button
            type="button"
            onClick={() => {
              setState({ kind: "loading" });
              loadMembership();
            }}
            className="mt-5 inline-flex min-h-11 items-center justify-center border border-ink px-4 py-2 font-mono text-xs transition-colors hover:bg-ink hover:text-paper"
          >
            重新加载
          </button>
        </section>
      ) : null}

      {membership ? (
        <section data-account-membership-state="success" className="mt-6 border border-ink p-6 sm:p-8">
          <p className="font-mono text-xs tracking-[0.2em] text-ink/60">CURRENT ENTITLEMENT</p>
          <h2 className="mt-3 font-display text-4xl font-bold tracking-tight">
            {isLifetime ? "终身会员" : "免费会员"}
          </h2>
          <p className="mt-4 max-w-2xl text-base leading-7 text-ink/75">
            {isLifetime
              ? "终身会员已生效，换设备登录同样可用。"
              : "当前为免费会员，可在下方开通终身会员。"}
          </p>
          {isLifetime ? (
            <p className="mt-6 border-t border-line pt-5 text-sm leading-6 text-ink/70">
              {LIFETIME_BENEFITS}
            </p>
          ) : null}
          <p className="mt-6 border-t border-line pt-5 text-sm leading-6 text-ink/60">
            运营人员为你开通或撤销会员时，会通过系统通知告诉你。
          </p>
        </section>
      ) : null}

      {membership && !isLifetime ? <MembershipPurchase onPaid={loadMembership} /> : null}
    </div>
  );
}
