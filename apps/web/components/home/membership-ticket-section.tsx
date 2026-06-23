"use client";

import { useRef } from "react";
import { membershipFeatures } from "./home-data";
import { homeAnimAttr, homeAnimSelector } from "./home-animation-selectors";
import { useHomeAnimeInView } from "./use-home-anime-in-view";
import { usePrefersReducedMotion } from "./use-prefers-reduced-motion";
import styles from "./home-visuals.module.css";

export function MembershipTicketSection() {
  const sectionRef = useRef<HTMLElement>(null);
  const reduceMotion = usePrefersReducedMotion();

  useHomeAnimeInView({
    reduceMotion,
    rootRef: sectionRef,
    selector: homeAnimSelector("membershipTicket"),
  });

  return (
    <section
      ref={sectionRef}
      aria-labelledby="membership-title"
      className="mx-auto w-[min(1120px,calc(100%-32px))] py-20"
    >
      <div className={`${styles.ticket} p-6 lg:p-10`} {...homeAnimAttr("membershipTicket")}>
        <span className={styles.membershipStamp} aria-hidden="true" {...homeAnimAttr("membershipStamp")} />
        <div className="grid gap-8 lg:grid-cols-[0.8fr_1.2fr]">
          <div>
            <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">
              Points & Membership
            </p>
            <h2 id="membership-title" className="mt-3 text-4xl font-black tracking-tight text-[#2b2117] sm:text-5xl">
              贡献、权益和成本控制
            </h2>
            <p className="mt-5 text-sm leading-7 text-[#6f604f]">
              积分会员体系用于连接创作者激励、会员权益、课程包和 AI 使用额度，成本控制会保留清晰边界。
            </p>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            {membershipFeatures.map((feature) => {
              const Icon = feature.icon;

              return (
                <article key={feature.title} className="rounded-3xl border border-[#2b2117]/10 bg-white/70 p-5">
                  <Icon className="size-6 text-[#b75c32]" aria-hidden={true} />
                  <h3 className="mt-4 text-lg font-black tracking-tight text-[#2b2117]">{feature.title}</h3>
                  <p className="mt-3 text-sm leading-7 text-[#6f604f]">{feature.body}</p>
                </article>
              );
            })}
          </div>
        </div>
      </div>
    </section>
  );
}
