import { HEATMAP } from "@/lib/practice/mock";

const CELL = 12;
const GAP = 3;
const WEEKS = 26;

const LEVELS = ["#e4e1d9", "#f4c9b2", "#efa17b", "#e97039", "#ff4d00"];

function levelOf(count: number) {
  if (count <= 0) return 0;
  if (count <= 3) return 1;
  if (count <= 7) return 2;
  if (count <= 12) return 3;
  return 4;
}

const WEEKDAYS = ["一", "三", "五", "日"];

/** GitHub 风格日历热力图（近 26 周，手写 SVG） */
export default function Heatmap() {
  return (
    <div className="overflow-x-auto">
      <svg
        viewBox={`0 0 ${WEEKS * (CELL + GAP) + 24} ${7 * (CELL + GAP) + 8}`}
        className="min-w-[480px]"
        role="img"
        aria-label="刷题日历热力图"
      >
        {HEATMAP.map((cell, i) => {
          const week = Math.floor(i / 7);
          const day = i % 7;
          return (
            <rect
              key={i}
              className="heat-cell"
              x={24 + week * (CELL + GAP)}
              y={4 + day * (CELL + GAP)}
              width={CELL}
              height={CELL}
              fill={LEVELS[levelOf(cell.count)]}
            >
              <title>{`${cell.date} · ${cell.count} 题`}</title>
            </rect>
          );
        })}
        {WEEKDAYS.map((label, i) => (
          <text
            key={label}
            x={0}
            y={4 + [0, 2, 4, 6][i] * (CELL + GAP) + CELL - 3}
            className="fill-ink/40"
            fontSize="8"
          >
            {label}
          </text>
        ))}
      </svg>
      <div className="mt-2 flex items-center justify-end gap-1.5 font-mono text-[10px] text-ink/40">
        少
        {LEVELS.map((c) => (
          <span key={c} className="inline-block h-2.5 w-2.5" style={{ background: c }} />
        ))}
        多
      </div>
    </div>
  );
}
