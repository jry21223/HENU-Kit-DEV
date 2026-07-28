export default function LeaderboardPage() {
  return (
    <main className="mx-auto max-w-[1440px] px-5 py-12 md:px-8 md:py-16">
      <div className="max-w-2xl">
        <p className="font-mono text-xs tracking-[0.3em] text-ink/60">
          <span className="text-accent">RANK</span>
          <span className="mx-2">/</span>
          CUTOVER PENDING
        </p>
        <h1 className="mt-3 font-display text-5xl font-bold tracking-tight md:text-6xl">排行榜</h1>
        <section data-practice-leaderboard-state="unavailable" role="status" className="mt-10 border border-line bg-paper p-6 md:p-8">
          <h2 className="font-display text-2xl font-bold">排行榜正在迁移，暂不可用</h2>
          <p className="mt-3 leading-7 text-ink/70">公开排行榜将在 QuizCraft V2 全量切流时启用。当前不会展示示例排名、历史缓存或会话内数据。</p>
        </section>
      </div>
    </main>
  );
}
