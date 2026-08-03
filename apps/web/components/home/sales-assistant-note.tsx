"use client";

import { useRef } from "react";
import { salesFeatures } from "./home-data";
import { homeAnimAttr, homeAnimSelector } from "./home-animation-selectors";
import { useHomeAnimeInView } from "./use-home-anime-in-view";
import { usePrefersReducedMotion } from "./use-prefers-reduced-motion";
import styles from "./home-visuals.module.css";

export function SalesAssistantNote() {
  const sectionRef = useRef<HTMLElement>(null);
  const reduceMotion = usePrefersReducedMotion();

  useHomeAnimeInView({
    reduceMotion,
    rootRef: sectionRef,
    selector: homeAnimSelector("salesNote"),
  });

  return (
    <section
      ref={sectionRef}
      aria-labelledby="sales-assistant-title"
      className="mx-auto w-[min(900px,calc(100%-32px))] py-12"
    >
      <div
        className={`${styles.salesNote} rounded-[2rem] border border-[#2b2117]/12 bg-[#d8f1ff] p-6 shadow-[0_22px_64px_rgba(71,49,27,0.12)] md:p-8`}
        {...homeAnimAttr("salesNote")}
      >
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#2a6d88]">智能助手</p>
        <h2 id="sales-assistant-title" className="mt-3 text-3xl font-black tracking-tight text-[#2b2117]">
          群里的咨询，也能接住
        </h2>
        <p className="mt-3 max-w-2xl text-sm leading-7 text-[#5b4f44]">
          群里的咨询也可以由助手及时接住。
        </p>

        <div className="mt-6 grid gap-4 md:grid-cols-2">
          {salesFeatures.map((feature) => {
            const Icon = feature.icon;

            return (
              <article key={feature.title} className={`${styles.salesFeature} rounded-2xl border border-transparent bg-white/72 p-4`}>
                <Icon className="size-5 text-[#2a6d88]" aria-hidden={true} />
                <h3 className="mt-3 text-lg font-black text-[#2b2117]">{feature.title}</h3>
                <p className="mt-2 text-sm leading-6 text-[#5b4f44]">{feature.body}</p>
              </article>
            );
          })}
        </div>
      </div>
    </section>
  );
}
