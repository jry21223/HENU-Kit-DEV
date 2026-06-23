import { practiceFeatures } from "./home-data";
import styles from "./home-visuals.module.css";

export function PracticeVisionSection() {
  return (
    <section
      id="practice"
      aria-labelledby="practice-title"
      className="mx-auto grid w-[min(1120px,calc(100%-32px))] gap-8 py-20 lg:grid-cols-[0.8fr_1.2fr]"
    >
      <div>
        <p className="font-mono text-xs font-semibold uppercase tracking-[0.18em] text-[#b75c32]">Practice</p>
        <h2 id="practice-title" className="mt-3 text-4xl font-black tracking-tight text-[#2b2117] sm:text-5xl">
          资料旁边就是练习
        </h2>
        <p className="mt-5 text-sm leading-7 text-[#6f604f]">
          刷题、错题本、薄弱点统计和 AI 针对性强化会回到具体课程，用于把资料阅读继续推进到练习和复盘。
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        {practiceFeatures.map((feature) => {
          const Icon = feature.icon;

          return (
            <article key={feature.title} className={`${styles.paperCard} p-5`}>
              <Icon className="size-6 text-[#2f6b58]" aria-hidden={true} />
              <h3 className="mt-5 text-xl font-black tracking-tight text-[#2b2117]">{feature.title}</h3>
              <p className="mt-3 text-sm leading-7 text-[#6f604f]">{feature.body}</p>
            </article>
          );
        })}
      </div>
    </section>
  );
}
