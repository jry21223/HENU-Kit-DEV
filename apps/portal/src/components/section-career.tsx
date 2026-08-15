"use client";

import SectionHeading from "@/components/ui/section-heading";
import MagneticButton from "@/components/ui/magnetic-button";
import WorkRadar from "@/components/career/work-radar";

const FEATURES = ["扫描大厂招聘官网", "按求职画像筛选匹配岗位", "完成后邮件提醒"];

export default function SectionCareer() {
  return (
    <section className="snap-screen border-t border-line bg-paper">
      <div className="mx-auto grid min-h-svh max-w-7xl items-center gap-12 px-5 py-24 md:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] md:px-10">
        <div>
          <SectionHeading index="05" en="WORK RADAR" title="求职雷达" />
          <p className="mt-6 max-w-md text-sm leading-7 text-ink/70">
            别再一个招聘网站一个招聘网站地翻。让雷达替你扫描岗位，
            按方向、技术栈和城市筛出更值得看的机会。
          </p>
          <ul className="mt-6 space-y-2 font-mono text-xs tracking-wider text-ink/60">
            {FEATURES.map((feature) => (
              <li key={feature}>
                <span className="mr-2 text-accent">+</span>
                {feature}
              </li>
            ))}
          </ul>
          <MagneticButton href="/career" className="mt-8">
            进入求职雷达
          </MagneticButton>
          <p className="mt-5 font-mono text-[10px] tracking-[0.18em] text-ink/40">
            UI PROTOTYPE · LIFETIME VIP BENEFIT
          </p>
        </div>

        <div className="md:pl-4">
          <WorkRadar
            compact
            status="running"
            sourcesCompleted={7}
            sourcesTotal={16}
            jobsFound={34}
            matched={6}
          />
        </div>
      </div>
    </section>
  );
}
