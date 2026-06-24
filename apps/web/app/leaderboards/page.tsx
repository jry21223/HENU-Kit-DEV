import Link from "next/link";
import { ArrowRight, BookOpenCheck, Medal, Sparkles, Trophy } from "lucide-react";
import { SiteShell } from "@/components/layout/site-shell";
import { Badge } from "@/components/ui/badge";
import { getApi, type LeaderboardEntry, type LeaderboardResponse } from "@/lib/api";

type BoardView = {
  key: LeaderboardResponse["type"];
  title: string;
  description: string;
  scoreLabel: string;
  entries: LeaderboardEntry[];
  error: string;
};

async function loadBoard(type: LeaderboardResponse["type"]) {
  try {
    const response = await getApi<LeaderboardResponse>(`/leaderboards/${type}?limit=10`);
    return { data: response.data, error: "" };
  } catch (error) {
    return {
      data: null,
      error: error instanceof Error ? error.message : "榜单暂时不可用",
    };
  }
}

export default async function LeaderboardsPage() {
  const [wiki, quiz, overall] = await Promise.all([loadBoard("wiki"), loadBoard("quiz"), loadBoard("overall")]);
  const boards: BoardView[] = [
    {
      key: "overall",
      title: "综合学习榜",
      description: "按积分、Wiki 贡献和刷题正确数综合计算，适合观察持续学习投入。",
      scoreLabel: "综合分",
      entries: overall.data?.entries ?? [],
      error: overall.error,
    },
    {
      key: "quiz",
      title: "刷题榜",
      description: "按公开练习提交得分排序，当前只展示聚合指标，不公开具体答案。",
      scoreLabel: "刷题分",
      entries: quiz.data?.entries ?? [],
      error: quiz.error,
    },
    {
      key: "wiki",
      title: "Wiki 贡献榜",
      description: "按已发布公开 Wiki 词条和互动量排序，待审核或私密内容不会入榜。",
      scoreLabel: "贡献分",
      entries: wiki.data?.entries ?? [],
      error: wiki.error,
    },
  ];

  return (
    <SiteShell>
      <section className="rounded-2xl border border-border bg-card p-4 shadow-sm sm:p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <Badge tone="success">学习榜单</Badge>
            <h1 className="mt-3 text-2xl font-semibold tracking-tight sm:text-3xl">看见持续复习的进度</h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
              榜单只使用公开、已发布或用户自己的学习聚合数据，不展示邮箱、答案、审核字段和内部记录。
            </p>
          </div>
          <Link className="inline-flex items-center text-sm font-medium text-primary" href="/courses">
            去刷题
            <ArrowRight className="ml-1.5 size-4" aria-hidden="true" />
          </Link>
        </div>
      </section>

      <section className="grid gap-4 lg:grid-cols-3">
        {boards.map((board) => (
          <LeaderboardCard key={board.key} board={board} />
        ))}
      </section>
    </SiteShell>
  );
}

function LeaderboardCard({ board }: { board: BoardView }) {
  const Icon = boardIcon(board.key);
  return (
    <section className="rounded-2xl border border-border bg-card p-4 shadow-sm">
      <div className="flex items-start gap-3">
        <span className="grid size-10 flex-none place-items-center rounded-xl bg-muted text-primary">
          <Icon className="size-5" aria-hidden="true" />
        </span>
        <div className="min-w-0">
          <h2 className="text-base font-semibold tracking-tight">{board.title}</h2>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{board.description}</p>
        </div>
      </div>

      {board.error ? (
        <p className="mt-4 rounded-xl bg-muted/70 p-3 text-sm text-muted-foreground">{board.error}</p>
      ) : null}

      {!board.error && board.entries.length === 0 ? (
        <p className="mt-4 rounded-xl bg-muted/70 p-3 text-sm text-muted-foreground">暂无上榜记录。</p>
      ) : null}

      <div className="mt-4 space-y-2">
        {board.entries.map((entry) => (
          <LeaderboardRow key={`${board.key}-${entry.userId}`} boardType={board.key} entry={entry} scoreLabel={board.scoreLabel} />
        ))}
      </div>
    </section>
  );
}

function LeaderboardRow({
  boardType,
  entry,
  scoreLabel,
}: {
  boardType: LeaderboardResponse["type"];
  entry: LeaderboardEntry;
  scoreLabel: string;
}) {
  return (
    <Link className="block rounded-xl border border-border bg-background p-3 transition hover:border-primary/60 hover:shadow-sm" href={`/users/${entry.userId}`}>
      <div className="flex min-w-0 items-center gap-3">
        <span className="grid size-8 flex-none place-items-center rounded-lg bg-muted text-sm font-semibold text-foreground">
          {entry.rank}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center justify-between gap-3">
            <p className="min-w-0 truncate text-sm font-medium text-foreground">{entry.name}</p>
            <span className="flex-none rounded-full bg-primary/10 px-2 py-1 text-xs font-medium text-primary">
              {entry.score} {scoreLabel}
            </span>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{metricLine(boardType, entry.metrics)}</p>
        </div>
      </div>
    </Link>
  );
}

function boardIcon(type: LeaderboardResponse["type"]) {
  switch (type) {
    case "overall":
      return Trophy;
    case "quiz":
      return BookOpenCheck;
    case "wiki":
      return Sparkles;
    default:
      return Medal;
  }
}

function metricLine(type: LeaderboardResponse["type"], metrics: LeaderboardEntry["metrics"]) {
  switch (type) {
    case "overall":
      return `积分 ${numberMetric(metrics.points)} / Wiki ${numberMetric(metrics.wikiCount)} / 答对 ${numberMetric(metrics.correctCount)}`;
    case "quiz":
      return `提交 ${numberMetric(metrics.answerCount)} / 答对 ${numberMetric(metrics.correctCount)}`;
    case "wiki":
      return `词条 ${numberMetric(metrics.wikiCount)} / 互动 ${numberMetric(metrics.engagement)}`;
    default:
      return "";
  }
}

function numberMetric(value: unknown) {
  return typeof value === "number" ? value : 0;
}
