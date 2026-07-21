import { r2 } from "@/lib/practice/mock";

const W = 600;
const H = 220;
const PAD = 28;

/** 能力分折线图（手写 SVG，坐标固定两位小数） */
export default function LineChart({ series }: { series: number[] }) {
  const n = series.length;
  const stepX = (W - PAD * 2) / (n - 1);
  const y = (v: number) => r2(H - PAD - (v / 100) * (H - PAD * 2));
  const points = series.map((v, i) => `${r2(PAD + i * stepX)},${y(v)}`).join(" ");
  const last = series[n - 1];
  const lastX = r2(PAD + (n - 1) * stepX);

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="w-full" role="img" aria-label="能力分折线图">
      {/* 网格线 */}
      {[0, 25, 50, 75, 100].map((v) => (
        <g key={v}>
          <line x1={PAD} y1={y(v)} x2={W - PAD} y2={y(v)} className="stroke-line" strokeWidth="1" />
          <text x={PAD - 6} y={y(v) + 3} textAnchor="end" className="fill-ink/40" fontSize="8" fontFamily="monospace">
            {v}
          </text>
        </g>
      ))}
      <polyline
        data-line-path
        points={points}
        fill="none"
        className="stroke-ink"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
      {/* 端点 */}
      <circle cx={lastX} cy={y(last)} r="3.5" className="fill-accent" />
      <text
        x={Math.min(lastX + 8, W - PAD)}
        y={y(last) - 8}
        className="fill-accent"
        fontSize="11"
        fontFamily="monospace"
        fontWeight="bold"
      >
        {last.toFixed(1)}
      </text>
    </svg>
  );
}
