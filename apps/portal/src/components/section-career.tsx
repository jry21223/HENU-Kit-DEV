"use client";

import SectionHeading from "@/components/ui/section-heading";
import MagneticButton from "@/components/ui/magnetic-button";
import WorkRadar from "@/components/career/work-radar";

// 首页文案只承诺已授权来源能做到的事：来源由服务端 allowlist 控制
// （首发只有美团官方校招接口），邮件只发到已验证的账户邮箱。
const FEATURES = [
  "扫描受控的官方招聘来源",
  "按求职画像筛选匹配岗位",
  "完成后向已验证的账户邮箱发送结果简报",
];

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
            RADAR SCHEMATIC · LIFETIME VIP BENEFIT
          </p>
        </div>

        <div className="md:pl-4">
          {/* 装饰用示意表盘：不接任何真实计数，读数区不渲染。 */}
          <WorkRadar compact schematic />
        </div>
      </div>
    </section>
  );
}
