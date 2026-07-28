"use client";

import { redirectToLogin } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { usePersonalPracticeStats } from "@/lib/practice/personal-stats";
import { EmptyBlock, ErrorBanner, LoadingBlock } from "@/components/data-state";
import { usePageEnter } from "@/components/practice/transition/use-page-enter";

function StatCards({
  totalAnswers,
  correctAnswers,
  accuracy,
  streakDays,
}: {
  totalAnswers: number;
  correctAnswers: number;
  accuracy: number;
  streakDays: number;
}) {
  const cards = [
    { label: "已确认作答", value: totalAnswers, unit: "次" },
    { label: "正确作答", value: correctAnswers, unit: "次" },
    { label: "正确率", value: accuracy, unit: "%" },
    { label: "连续学习", value: streakDays, unit: "天" },
  ];
  return (
    <div data-block data-enter className="mt-10 grid grid-cols-2 gap-4 md:grid-cols-4">
      {cards.map((card, index) => (
        <div key={card.label} className="border border-ink/25 p-5">
          <p className="font-mono text-[10px] tracking-[0.25em] text-ink/40">
            {String(index + 1).padStart(2, "0")} / {card.label}
          </p>
          <p className="mt-3 font-display text-4xl font-bold tabular-nums">
            {card.value}
            <span className="ml-1 font-mono text-xs font-normal text-ink/50">
              {card.unit}
            </span>
          </p>
        </div>
      ))}
    </div>
  );
}

export default function StatsPage() {
  usePageEnter(null);
  const { state, retry } = usePersonalPracticeStats();

  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <div data-block data-enter>
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">STATS</span>
          <span className="mx-2">/</span>
          MY DATA
        </p>
        <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">
          数据面板
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-ink/65">
          只展示由 QuizCraft 服务端已确认作答事实聚合出的数据；没有真实数据时不使用示例图表或排行榜数字。
        </p>
      </div>

      {state.status === "disabled" && (
        <section data-testid="practice-stats-disabled" className="mt-10">
          <EmptyBlock label="QuizCraft V2 真实数据将在确认切换后启用" />
        </section>
      )}

      {state.status === "loading" && (
        <section data-testid="practice-stats-loading" className="mt-10">
          <LoadingBlock label="正在同步真实学习数据" />
        </section>
      )}

      {state.status === "unauthenticated" && (
        <section data-testid="practice-stats-unauthenticated" className="mt-10 border border-ink/25 p-6">
          <p className="font-mono text-xs tracking-[0.2em] text-ink/55">
            SIGN IN REQUIRED / 登录后查看跨设备同步的学习状态
          </p>
          <button
            type="button"
            onClick={() => redirectToLogin("/practice/stats")}
            className="mt-5 border border-ink px-4 py-2 font-mono text-xs tracking-widest transition-colors hover:bg-ink hover:text-paper"
          >
            登录查看
          </button>
        </section>
      )}

      {state.status === "error" && (
        <section data-testid="practice-stats-error" className="mt-10">
          <ErrorBanner
            message={`${state.message}；未使用示例数据填充。`}
            onRetry={retry}
          />
        </section>
      )}

      {(state.status === "empty" || state.status === "ready") && (
        <>
          <StatCards
            totalAnswers={state.data.total_answers}
            correctAnswers={state.data.correct_answers}
            accuracy={state.data.accuracy}
            streakDays={state.data.streak_days}
          />

          {state.status === "empty" ? (
            <section data-testid="practice-stats-empty" className="mt-12">
              <EmptyBlock label="尚无已确认作答事实，从第一题开始建立你的学习图谱" />
            </section>
          ) : (
            <section data-testid="practice-stats-success" data-block data-enter className="mt-12 border border-ink/25 p-5 md:p-7">
              <div className="flex flex-wrap items-end justify-between gap-3">
                <div>
                  <p className="font-mono text-xs tracking-[0.25em] text-ink/60">
                    MASTERY / 题库掌握度
                  </p>
                  <p className="mt-2 text-sm text-ink/55">
                    分值为当前题库稳定题目中，至少答对一次的题目占比。
                  </p>
                </div>
                <p className="font-mono text-[10px] tracking-[0.2em] text-ink/45">
                  {state.data.mastery.length} 个有作答事实的题库
                </p>
              </div>

              <div className="mt-7 space-y-5">
                {state.data.mastery.map((subject) => {
                  const weak = subject.value < 60;
                  return (
                    <div key={subject.bank_id}>
                      <div className="mb-2 flex items-end justify-between gap-3 font-mono text-xs">
                        <span className="min-w-0 truncate">{subject.label}</span>
                        <span
                          className={cn(
                            "shrink-0 tabular-nums",
                            weak ? "text-accent" : "text-ink/60"
                          )}
                        >
                          {subject.value}% · {subject.correct_questions}/
                          {subject.total_questions}
                        </span>
                      </div>
                      <div className="h-1.5 w-full bg-ink/10">
                        <div
                          className={cn("h-full", weak ? "bg-accent" : "bg-ink/70")}
                          style={{ width: `${subject.value}%` }}
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
            </section>
          )}
        </>
      )}
    </main>
  );
}
